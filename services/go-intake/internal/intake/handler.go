package intake

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/artifact"
	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/githubsig"
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
	Now       func() time.Time
}

// Handler sirve POST /webhooks/github.
type Handler struct {
	d  Deps
	mu sync.Mutex // serializa persistir+publicar para que una entrega no se duplique
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
	h := &Handler{d: d}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhooks/github", h.webhook)
	return mux, nil
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
	reject := func(status int, code, msg string) {
		log.Printf("webhook delivery=%q event=%q status=%d code=%s", delivery, r.Header.Get("X-GitHub-Event"), status, code)
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

	// 3. Clasificacion.
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

	// 4. Persistir y luego publicar (idempotente por entrega).
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
			log.Printf("webhook delivery=%q store error: %v", delivery, err)
			reject(http.StatusServiceUnavailable, "store_unavailable", "no se pudo persistir")
			return
		}
	}
	if rec.PublishPending {
		if err := h.publish(r.Context(), rec, r.Header.Get("traceparent")); err != nil {
			log.Printf("webhook delivery=%q publish error: %v", delivery, err)
			reject(http.StatusServiceUnavailable, "publish_failed", "no se pudo publicar; reintente la entrega")
			return
		}
		rec.PublishPending = false
		if err := h.d.Store.Put(rec); err != nil {
			log.Printf("webhook delivery=%q store error: %v", delivery, err)
			reject(http.StatusServiceUnavailable, "store_unavailable", "no se pudo persistir")
			return
		}
	}
	log.Printf("webhook delivery=%q notification=%s status=202", delivery, rec.ID)
	writeJSON(w, http.StatusAccepted, rec.Notification)
}

func (h *Handler) publish(ctx context.Context, rec Record, traceparent string) error {
	ev := Event{
		EventID:    uuidV4(),
		Type:       "notify.created",
		Version:    1,
		OccurredAt: h.d.Now().UTC().Format(time.RFC3339),
		TraceID:    traceID(traceparent),
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
