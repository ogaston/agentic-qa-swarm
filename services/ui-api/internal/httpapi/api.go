// Package httpapi expone GET /notifications y POST /notifications/{id}/confirm.
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/ui-api/inbox"
	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/auth"
	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/obs"
)

// MaxBodyBytes es el limite del cuerpo del POST (64 KiB).
const MaxBodyBytes = 64 << 10

// Config reune las dependencias y parametros del servicio.
type Config struct {
	Store          *inbox.Store
	Verifier       auth.TokenVerifier
	AllowedOrigins []string // nunca "*"
	RateRPS        float64
	RateBurst      int
	TrustProxy     bool
	Now            func() time.Time // reloj del limitador; nil = real
	Logger         *log.Logger      // nil = descarta
	Metrics        *obs.Inbox       // nil = sin métricas de dominio
}

// Handler es el http.Handler de ui-api.
type Handler struct {
	cfg     Config
	lim     *limiter
	origins map[string]struct{}
	log     *log.Logger
}

// New construye el handler con toda la cadena de middleware.
func New(cfg Config) (http.Handler, error) {
	if cfg.Store == nil || cfg.Verifier == nil {
		return nil, errors.New("Store y Verifier son obligatorios")
	}
	if cfg.RateRPS <= 0 {
		cfg.RateRPS = 10
	}
	if cfg.RateBurst <= 0 {
		cfg.RateBurst = 20
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	h := &Handler{cfg: cfg, lim: newLimiter(cfg.RateRPS, cfg.RateBurst, cfg.Now), origins: map[string]struct{}{}, log: cfg.Logger}
	if h.log == nil {
		h.log = log.New(io.Discard, "", 0)
	}
	for _, o := range cfg.AllowedOrigins {
		if o == "*" || o == "" {
			return nil, errors.New("origen CORS invalido: nunca * ni vacio")
		}
		h.origins[o] = struct{}{}
	}
	return h, nil
}

// Patrones de ruta de ui-api (etiqueta route de las métricas).
const (
	RouteList    = "/notifications"
	RouteConfirm = "/notifications/{id}/confirm"
)

// RoutePattern devuelve el PATRÓN de ruta de la petición ("unmatched" si no hay), nunca la ruta cruda.
// Sigue el mismo criterio que route(): un id vacío o con "/" no coincide.
func RoutePattern(r *http.Request) string {
	p := r.URL.Path
	switch {
	case p == RouteList:
		return RouteList
	case strings.HasPrefix(p, "/notifications/") && strings.HasSuffix(p, "/confirm"):
		id := strings.TrimSuffix(strings.TrimPrefix(p, "/notifications/"), "/confirm")
		if id != "" && !strings.Contains(id, "/") {
			return RouteConfirm
		}
	}
	return "unmatched"
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w)
	defer func() {
		if rec := recover(); rec != nil {
			// No se registra el valor del panic: podria contener el token.
			h.log.Printf("panic atendiendo %s (valor omitido)", r.Method)
			writeError(w, http.StatusInternalServerError, "internal", "error interno")
		}
	}()
	if ok, wait := h.lim.allow(clientIP(r, h.cfg.TrustProxy)); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(wait))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "demasiadas peticiones")
		return
	}
	if h.cors(w, r) {
		return
	}
	h.route(w, r)
}

func setSecurityHeaders(w http.ResponseWriter) {
	hd := w.Header()
	hd.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	hd.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Referrer-Policy", "no-referrer")
	hd.Set("Cache-Control", "no-store")
}

// cors añade ACAO solo para origenes listados; responde el preflight. Devuelve
// true si ya respondio.
func (h *Handler) cors(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	w.Header().Add("Vary", "Origin")
	if origin == "" {
		return false
	}
	_, allowed := h.origins[origin]
	if allowed {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
	if r.Method == http.MethodOptions {
		if allowed {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

func (h *Handler) route(w http.ResponseWriter, r *http.Request) {
	switch p := r.URL.Path; {
	case p == "/notifications":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		if pr, ok := h.authenticate(w, r); ok {
			h.list(w, r, pr)
		}
	case strings.HasPrefix(p, "/notifications/") && strings.HasSuffix(p, "/confirm"):
		id := strings.TrimSuffix(strings.TrimPrefix(p, "/notifications/"), "/confirm")
		if id == "" || strings.Contains(id, "/") {
			writeError(w, http.StatusNotFound, "not_found", "ruta inexistente")
			return
		}
		if r.Method != http.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		if pr, ok := h.authenticate(w, r); ok {
			h.confirm(w, r, pr, id)
		}
	default:
		writeError(w, http.StatusNotFound, "not_found", "ruta inexistente")
	}
}

// authenticate es fail-closed: cualquier error del verificador es 401.
func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request) (auth.Principal, bool) {
	// Mas de una cabecera Authorization es ambiguo (un proxy podria validar otra): 401.
	tok, ok := auth.BearerToken(r.Header.Get("Authorization"))
	if len(r.Header.Values("Authorization")) == 1 && ok {
		if pr, err := h.cfg.Verifier.Verify(r.Context(), tok); err == nil {
			return pr, true
		}
	}
	w.Header().Set("WWW-Authenticate", "Bearer")
	writeError(w, http.StatusUnauthorized, "unauthorized", "credenciales ausentes o invalidas")
	return auth.Principal{}, false
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request, _ auth.Principal) {
	var state inbox.State
	if q, present := r.URL.Query()["state"]; present {
		state = inbox.State(q[0])
		if len(q) != 1 || !state.Valid() {
			writeError(w, http.StatusBadRequest, "invalid_request", "state debe ser pending, confirmed o rejected")
			return
		}
	}
	writeJSON(w, http.StatusOK, h.cfg.Store.List(state))
}

type confirmBody struct {
	Flows []string `json:"flows"`
}

func (h *Handler) confirm(w http.ResponseWriter, r *http.Request, pr auth.Principal, id string) {
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type debe ser application/json")
		return
	}
	var body confirmBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBodyBytes))
	err := dec.Decode(&body)
	if err == nil {
		if _, e2 := dec.Token(); !errors.Is(e2, io.EOF) {
			err = e2
			if err == nil {
				err = errors.New("datos tras el objeto")
			}
		}
	}
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "cuerpo mayor a 64 KiB")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "cuerpo JSON invalido")
		return
	}
	if len(body.Flows) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "flows es obligatorio y no puede estar vacio")
		return
	}
	for _, f := range body.Flows {
		if strings.TrimSpace(f) == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "flows no admite elementos vacios")
			return
		}
	}
	// confirmed_by sale siempre del token, nunca del cuerpo.
	rc, err := h.cfg.Store.Confirm(id, pr.ID, body.Flows)
	switch {
	case errors.Is(err, inbox.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "notificacion inexistente")
	case errors.Is(err, inbox.ErrAlreadyConfirmed):
		writeError(w, http.StatusConflict, "already_confirmed", "la notificacion ya fue confirmada")
	case err != nil:
		h.log.Printf("confirmando %q: %v", id, err)
		writeError(w, http.StatusInternalServerError, "internal", "error interno")
	default:
		h.cfg.Metrics.Confirmed()
		writeJSON(w, http.StatusCreated, rc)
	}
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "metodo no permitido")
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"code": code, "message": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		status, b = http.StatusInternalServerError, []byte(`{"code":"internal","message":"error interno"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(append(b, '\n'))
}
