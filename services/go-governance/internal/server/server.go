// Package server expone la API HTTP de go-governance.
package server

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/authz"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/auth"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/obs"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/policy"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/service"
)

// MaxBody es el tope del cuerpo de cualquier petición.
const MaxBody = 64 << 10

// Access es la política de acceso de una ruta.
type Access int

const (
	// AccessAuthenticated: cualquier persona autenticada (user o admin).
	AccessAuthenticated Access = iota + 1
	// AccessAdmin: solo admin.
	AccessAdmin
	// AccessService: solo el token de servicio (no personas).
	AccessService
)

// PersonPermits decide, sin tocar HTTP, si un rol puede usar una ruta de personas.
// Deny-by-default: rol inválido o política desconocida deniegan.
func PersonPermits(a Access, r authz.Role) bool {
	if !r.Valid() {
		return false
	}
	switch a {
	case AccessAuthenticated:
		return true
	case AccessAdmin:
		return r == authz.RoleAdmin
	}
	return false
}

// Server es el handler HTTP.
type Server struct {
	svc      *service.Service
	verifier auth.TokenVerifier
	svcToken auth.ServiceToken
	log      *slog.Logger
	registry *prometheus.Registry
	httpm    *obs.HTTPMetrics
	ready    []obs.Check
}

// ServiceName es el valor de la etiqueta service de las métricas HTTP.
const ServiceName = "go-governance"

// Config son las dependencias del servidor.
type Config struct {
	Service  *service.Service
	Verifier auth.TokenVerifier
	Token    auth.ServiceToken
	Logger   *slog.Logger
	// Registry recibe las métricas HTTP y se sirve en /metrics; nil crea uno propio.
	Registry *prometheus.Registry
	// Ready son los chequeos de /readyz.
	Ready []obs.Check
}

// New construye el servidor.
func New(c Config) *Server {
	if c.Logger == nil {
		c.Logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	if c.Registry == nil {
		c.Registry = obs.NewRegistry()
	}
	return &Server{svc: c.Service, verifier: c.Verifier, svcToken: c.Token, log: c.Logger,
		registry: c.Registry, httpm: obs.NewHTTPMetrics(c.Registry, ServiceName), ready: c.Ready}
}

// routeOf devuelve el patrón de la ruta (nunca la ruta cruda) para la etiqueta route.
func routeOf(r *http.Request) string {
	switch {
	case r.URL.Path == "/gates/authorize":
		return "/gates/authorize"
	case r.URL.Path == "/audit":
		return "/audit"
	case strings.HasPrefix(r.URL.Path, "/policies/"):
		return "/policies/{name}"
	}
	return "unmatched"
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, errCode, msg string) {
	writeJSON(w, code, map[string]string{"code": errCode, "message": msg})
}

func methodNotAllowed(w http.ResponseWriter, allow ...string) {
	w.Header().Set("Allow", strings.Join(allow, ", "))
	writeErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "método no permitido")
}

// Handler devuelve el enrutador envuelto con el contrato de observabilidad
// (/healthz, /readyz y /metrics sin token, identificadores, log de acceso y métricas HTTP).
func (s *Server) Handler() http.Handler {
	return obs.Wrap(obs.Config{Service: ServiceName, Log: s.log, Registry: s.registry, Metrics: s.httpm,
		Ready: s.ready, Route: routeOf}, s.routes())
}

func (s *Server) routes() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/gates/authorize":
			if r.Method != http.MethodPost {
				methodNotAllowed(w, http.MethodPost)
				return
			}
			s.gate(w, r)
		case r.URL.Path == "/audit":
			if r.Method != http.MethodGet {
				methodNotAllowed(w, http.MethodGet)
				return
			}
			s.queryAudit(w, r)
		case strings.HasPrefix(r.URL.Path, "/policies/"):
			name := strings.TrimPrefix(r.URL.Path, "/policies/")
			switch r.Method {
			case http.MethodGet:
				s.getPolicy(w, r, name)
			case http.MethodPut:
				s.putPolicy(w, r, name)
			default:
				methodNotAllowed(w, http.MethodGet, http.MethodPut)
			}
		default:
			writeErr(w, http.StatusNotFound, "not_found", "ruta desconocida")
		}
	})
}

// person autentica a la persona y comprueba el acceso. ok=false: ya se respondió.
func (s *Server) person(w http.ResponseWriter, r *http.Request, a Access) (authz.Principal, bool) {
	tok, ok := auth.BearerToken(r.Header)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "falta el token")
		return authz.Principal{}, false
	}
	p, err := s.verifier.Verify(r.Context(), tok)
	switch {
	case errors.Is(err, auth.ErrInvalidToken):
		writeErr(w, http.StatusUnauthorized, "unauthorized", "token inválido")
		return authz.Principal{}, false
	case err != nil:
		s.log.ErrorContext(r.Context(), "verificación de identidad fallida", "error", err.Error())
		writeErr(w, http.StatusServiceUnavailable, "identity_unavailable", "identidad no disponible")
		return authz.Principal{}, false
	}
	if !PersonPermits(a, p.Role) {
		writeErr(w, http.StatusForbidden, "forbidden", "rol insuficiente")
		return p, false
	}
	return p, true
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, int) {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBody))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return nil, http.StatusRequestEntityTooLarge
		}
		return nil, http.StatusBadRequest
	}
	return b, 0
}

// GateRequest es el cuerpo de POST /gates/authorize (los hechos son "true"|"false"|"unknown").
type GateRequest = authz.GateInput

func (s *Server) gate(w http.ResponseWriter, r *http.Request) {
	tok, ok := auth.BearerToken(r.Header)
	if !ok || !s.svcToken.Matches(tok) {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "token de servicio inválido")
		return
	}
	body, code := readBody(w, r)
	if code == http.StatusRequestEntityTooLarge {
		writeErr(w, code, "payload_too_large", "cuerpo demasiado grande")
		return
	} else if code != 0 {
		writeErr(w, code, "bad_request", "cuerpo ilegible")
		return
	}
	var in GateRequest
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "JSON inválido")
		return
	}
	if _, err := dec.Token(); err != io.EOF {
		writeErr(w, http.StatusBadRequest, "bad_request", "JSON inválido")
		return
	}
	d, err := s.svc.Authorize(r.Context(), "service:gate-client", in)
	if err != nil {
		// Fail-closed: error interno o auditoría imposible. Nunca allow=true.
		s.log.ErrorContext(r.Context(), "gate fail-closed", "error", err.Error(), "run_id", in.RunID)
		d.Allow = false
		if d.Reason == "" {
			d.Reason = "error interno, se deniega"
		}
		writeJSON(w, http.StatusServiceUnavailable, d)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) getPolicy(w http.ResponseWriter, r *http.Request, name string) {
	if _, ok := s.person(w, r, AccessAuthenticated); !ok {
		return
	}
	v, found, err := s.svc.GetPolicy(r.Context(), name)
	switch {
	case errors.Is(err, service.ErrUnknownPolicy), err == nil && !found:
		writeErr(w, http.StatusNotFound, "not_found", "política sin valor o desconocida")
	case err != nil:
		s.log.ErrorContext(r.Context(), "lectura de política", "error", err.Error())
		writeErr(w, http.StatusServiceUnavailable, "unavailable", "almacén no disponible")
	default:
		writeJSON(w, http.StatusOK, map[string]any{"name": v.Name, "version": v.Version, "value": v.Value})
	}
}

func clip(s string) string {
	if len(s) > 64 {
		return s[:64]
	}
	return s
}

func (s *Server) putPolicy(w http.ResponseWriter, r *http.Request, name string) {
	p, ok := s.person(w, r, AccessAdmin)
	if !ok {
		if p.ID != "" { // autenticado pero sin permiso: queda en el log
			_ = s.svc.Forbidden(p.ID, clip(name), "rol insuficiente (403)")
		}
		return
	}
	if !policy.Known(name) {
		_ = s.svc.Rejected(p.ID, clip(name), "política desconocida (404)")
		writeErr(w, http.StatusNotFound, "not_found", "política desconocida")
		return
	}
	body, code := readBody(w, r)
	if code != 0 {
		reason, status, ec := "cuerpo ilegible (400)", code, "bad_request"
		if code == http.StatusRequestEntityTooLarge {
			reason, ec = "cuerpo mayor de 64 KiB (413)", "payload_too_large"
		}
		_ = s.svc.Rejected(p.ID, name, reason)
		writeErr(w, status, ec, "cuerpo no aceptado")
		return
	}
	var req struct {
		Value json.RawMessage `json:"value"`
	}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || len(req.Value) == 0 {
		_ = s.svc.Rejected(p.ID, name, "cuerpo sin value válido (422)")
		writeErr(w, http.StatusUnprocessableEntity, "invalid_policy", "se espera {\"value\": {...}}")
		return
	}
	ver, err := s.svc.SetPolicy(r.Context(), p.ID, name, req.Value)
	switch {
	case errors.Is(err, policy.ErrInvalid):
		writeErr(w, http.StatusUnprocessableEntity, "invalid_policy", err.Error())
	case err != nil:
		s.log.ErrorContext(r.Context(), "PUT de política fallido", "policy", name, "error", err.Error())
		writeErr(w, http.StatusServiceUnavailable, "unavailable", "no se pudo guardar y auditar la política")
	default:
		writeJSON(w, http.StatusOK, map[string]any{"name": name, "version": ver})
	}
}

func (s *Server) queryAudit(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.person(w, r, AccessAuthenticated); !ok {
		return
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			writeErr(w, http.StatusBadRequest, "bad_request", "limit debe estar entre 1 y 500")
			return
		}
		limit = n
	}
	type view struct {
		At     string `json:"at"`
		Actor  string `json:"actor"`
		Action string `json:"action"`
		RunID  string `json:"run_id,omitempty"`
	}
	entries := s.svc.Query(r.URL.Query().Get("run"), limit)
	out := make([]view, 0, len(entries))
	for _, e := range entries {
		out = append(out, view{e.At.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), e.Actor, e.Action, e.RunID})
	}
	writeJSON(w, http.StatusOK, out)
}
