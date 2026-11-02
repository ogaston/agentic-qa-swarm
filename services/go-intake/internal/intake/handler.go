package intake

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/artifact"
	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/githubsig"
	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/obs"
)

// MaxBodyBytes es el limite del cuerpo del webhook (1 MiB).
const MaxBodyBytes = 1 << 20

// Deps son las dependencias del handler. Secret, Verifier, Store, Publisher y
// Resolver son obligatorios; Now es opcional (por defecto time.Now).
type Deps struct {
	Secret    []byte
	Verifier  githubsig.Verifier
	Store     NotificationStore
	Publisher EventPublisher
	Resolver  ArtifactResolver
	// AllowedRepos es la lista de repositorios owner/repo permitidos (INTAKE_ALLOWED_REPOS).
	// Vacía = fail-closed: todo evento se rechaza con 422 repo_not_allowed.
	// Si RepoLookup es nil, se envuelve automáticamente en un StaticRepoLookup.
	AllowedRepos []string
	RepoLookup   RepoLookup
	Now          func() time.Time
	// Log y Metrics son opcionales (nil = sin log / sin métricas de dominio).
	Log     *slog.Logger
	Metrics *obs.Intake
}

// Handler sirve POST /webhooks/github.
type Handler struct {
	d      Deps
	lookup RepoLookup
	mu     sync.Mutex // serializa persistir+publicar para que una entrega no se duplique
}

// ParseAllowedRepos interpreta INTAKE_ALLOWED_REPOS: lista separada por comas de owner/repo.
// Espacios alrededor y entradas vacías se descartan; la comparación posterior no distingue mayúsculas.
func ParseAllowedRepos(raw string) []string {
	var out []string
	for _, r := range strings.Split(raw, ",") {
		if r = strings.TrimSpace(r); r != "" {
			out = append(out, r)
		}
	}
	return out
}

// NewHandler valida las dependencias y devuelve el handler HTTP completo.
func NewHandler(d Deps) (http.Handler, error) {
	if len(d.Secret) == 0 {
		return nil, errors.New("intake: secreto vacio")
	}
	if d.Verifier == nil || d.Store == nil || d.Publisher == nil || d.Resolver == nil {
		return nil, errors.New("intake: dependencias incompletas")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	var lookup RepoLookup
	if d.RepoLookup != nil {
		lookup = d.RepoLookup
	} else if len(d.AllowedRepos) > 0 {
		lookup = NewStaticRepoLookup(d.AllowedRepos)
	} else {
		lookup = NewStaticRepoLookup(nil)
	}
	h := &Handler{d: d, lookup: lookup}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhooks/github", h.webhook)
	return mux, nil
}

// WebhookRoute es el patrón de la única ruta de negocio de go-intake.
const WebhookRoute = "/webhooks/github"

// RoutePattern devuelve el PATRÓN de ruta para las métricas ("unmatched" si no hay), nunca la ruta cruda.
func RoutePattern(r *http.Request) string {
	if r.URL.Path == WebhookRoute {
		return WebhookRoute
	}
	return "unmatched"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"code": code, "message": msg})
}

func (h *Handler) webhook(w http.ResponseWriter, r *http.Request) {
	delivery := r.Header.Get("X-GitHub-Delivery")
	ctx := r.Context()
	reject := func(status int, code, msg string) {
		// Solo identificadores y el código de error: nunca cuerpo, firma ni cabeceras de autenticación.
		h.d.Log.WarnContext(ctx, "webhook rechazado", "delivery_id", clip(delivery), "github_event", clip(r.Header.Get("X-GitHub-Event")),
			"status", status, "code", code)
		if reason := rejectReason(code); reason != "" {
			h.d.Metrics.Rejected(reason)
		}
		writeError(w, status, code, msg)
	}

	// 1. Firma: sin cabecera no se lee el cuerpo ni se toca el almacen.
	sig := r.Header.Get("X-Hub-Signature-256")
	if sig == "" {
		reject(http.StatusUnauthorized, "invalid_signature", "firma ausente")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBodyBytes))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			reject(http.StatusRequestEntityTooLarge, "payload_too_large", "cuerpo mayor a 1 MiB")
		} else {
			reject(http.StatusBadRequest, "invalid_body", "no se pudo leer el cuerpo")
		}
		return
	}
	// La firma se verifica sobre el cuerpo crudo, antes de parsear nada.
	if err := h.d.Verifier.Verify(h.d.Secret, body, sig); err != nil {
		reject(http.StatusUnauthorized, "invalid_signature", "firma invalida")
		return
	}

	// 2. Forma de la peticion.
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		reject(http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type debe ser application/json")
		return
	}
	if delivery == "" {
		reject(http.StatusBadRequest, "missing_delivery", "falta X-GitHub-Delivery")
		return
	}

	// 3. Ping de GitHub (C-126): con firma válida responde 200 sin notificación ni evento.
	if r.Header.Get("X-GitHub-Event") == "ping" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "pong"})
		return
	}

	// 4. Clasificacion.
	c, err := Classify(r.Header.Get("X-GitHub-Event"), body)
	switch {
	case errors.Is(err, ErrUnsupported):
		reject(http.StatusBadRequest, "unsupported_event", "evento o accion no soportados")
		return
	case errors.Is(err, ErrInvalidPayload):
		reject(http.StatusBadRequest, "invalid_payload", "faltan repositorio o sha validos")
		return
	case err != nil:
		reject(http.StatusBadRequest, "invalid_json", "JSON invalido")
		return
	}

	// 5. Repositorio permitido: fail-closed (lista vacía = nada permitido).
	if !h.lookup.Allowed(r.Context(), c.Repo) {
		reject(http.StatusUnprocessableEntity, "repo_not_allowed", "el repositorio no está en la lista permitida")
		return
	}

	// 6. Persistir y luego publicar (idempotente por entrega).
	h.mu.Lock()
	defer h.mu.Unlock()
	rec, found := h.d.Store.GetByDelivery(delivery)
	if !found {
		art, err := h.d.Resolver.Resolve(r.Context(), c)
		if errors.Is(err, artifact.ErrUnresolvableArtifact) {
			reject(http.StatusUnprocessableEntity, "unresolvable_artifact", "no se puede fijar un artefacto desplegable para este evento")
			return
		}
		if err != nil {
			reject(http.StatusServiceUnavailable, "artifact_unavailable", "no se pudo resolver el artefacto")
			return
		}
		rec = Record{
			Notification: Notification{
				ID: "n-" + uuidV4(), GithubEvent: c.GithubEvent, Repo: c.Repo, SHA: c.SHA,
				State: StatePending, Artifact: &art,
			},
			DeliveryID:     delivery,
			PublishPending: true,
		}
		if err := h.d.Store.Put(rec); err != nil {
			h.d.Log.ErrorContext(ctx, "no se pudo persistir", "delivery_id", clip(delivery), "error", err.Error())
			reject(http.StatusServiceUnavailable, "store_unavailable", "no se pudo persistir")
			return
		}
		h.d.Metrics.Created()
	}
	if rec.PublishPending {
		if err := h.publish(r.Context(), rec, r.Header.Get("traceparent")); err != nil {
			h.d.Metrics.PublishFailed()
			h.d.Log.ErrorContext(ctx, "no se pudo publicar notify.created", "delivery_id", clip(delivery), "notification_id", rec.ID, "error", err.Error())
			reject(http.StatusServiceUnavailable, "publish_failed", "no se pudo publicar; reintente la entrega")
			return
		}
		rec.PublishPending = false
		if err := h.d.Store.Put(rec); err != nil {
			h.d.Log.ErrorContext(ctx, "no se pudo persistir", "delivery_id", clip(delivery), "error", err.Error())
			reject(http.StatusServiceUnavailable, "store_unavailable", "no se pudo persistir")
			return
		}
	}
	h.d.Log.InfoContext(ctx, "webhook aceptado", "delivery_id", clip(delivery), "notification_id", rec.ID,
		"repo", rec.Repo, "sha", rec.SHA, "status", http.StatusAccepted)
	writeJSON(w, http.StatusAccepted, rec.Notification)
}

// clip acota a 64 bytes un valor controlado por el remitente antes de loguearlo.
func clip(s string) string {
	if len(s) > 64 {
		return s[:64]
	}
	return s
}

// rejectReason traduce el código de error al motivo de aqs_intake_webhook_rejected_total
// ("" = no es un rechazo del remitente: 503 internos, que cuentan solo en las métricas HTTP).
func rejectReason(code string) string {
	switch code {
	case "invalid_signature", "unsupported_event", "unresolvable_artifact", "repo_not_allowed":
		return code
	case "payload_too_large":
		return "too_large"
	case "invalid_body", "unsupported_media_type", "missing_delivery", "invalid_payload", "invalid_json":
		return "bad_request"
	}
	return ""
}

// eventTraceID usa el trace_id de la petición (el mismo del log y del traceparent de la respuesta)
// y, sin middleware, cae al traceparent entrante.
func eventTraceID(ctx context.Context, traceparent string) string {
	if id := obs.TraceID(ctx); id != "" {
		return id
	}
	return traceID(traceparent)
}

func (h *Handler) publish(ctx context.Context, rec Record, traceparent string) error {
	ev := Event{
		EventID:    uuidV4(),
		Type:       "notify.created",
		Version:    1,
		OccurredAt: h.d.Now().UTC().Format(time.RFC3339),
		TraceID:    eventTraceID(ctx, traceparent),
		Data: EventData{
			NotificationID: rec.ID, GithubEvent: rec.GithubEvent, Repo: rec.Repo, SHA: rec.SHA,
		},
	}
	if rec.Artifact != nil {
		ev.Data.Artifact = *rec.Artifact
	}
	return h.d.Publisher.Publish(ctx, ev)
}

var traceparentRE = regexp.MustCompile(`^[0-9a-f]{2}-([0-9a-f]{32})-[0-9a-f]{16}-[0-9a-f]{2}$`)

// traceID toma el trace-id de un traceparent W3C valido; si no, genera 32 hex.
func traceID(traceparent string) string {
	if m := traceparentRE.FindStringSubmatch(traceparent); m != nil && m[1] != "00000000000000000000000000000000" {
		return m[1]
	}
	b := make([]byte, 16)
	mustRand(b)
	return hex.EncodeToString(b)
}

func mustRand(b []byte) {
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand: %v", err))
	}
}

func uuidV4() string {
	b := make([]byte, 16)
	mustRand(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
