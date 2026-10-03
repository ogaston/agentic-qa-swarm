// Package server expone /auth/login, /auth/logout, /auth/session, /auth/sessions/{id} y /auth/users.
package server

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"mime"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/guard"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/obs"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/passhash"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/session"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/totp"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/users"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/principal"
)

const (
	maxBodyBytes     = 8 << 10
	maxPasswordBytes = 1024
	maxUsernameBytes = 256
)

var otpRe = regexp.MustCompile(`^[0-9]{6}$`)

// Config reúne las dependencias del servidor.
type Config struct {
	Users      map[string]*users.User
	Guard      *guard.Guard
	Sessions   *session.Store
	Clock      func() time.Time // nil usa time.Now
	Decoy      string           // hash señuelo para usuarios inexistentes (ver NewDecoy)
	TrustProxy bool             // honrar X-Forwarded-For (IDENTITY_TRUST_PROXY)
	// AllowedOrigins es la lista blanca CORS (ver ParseOrigins); vacía = ningún origen.
	AllowedOrigins []string
	Logger         *slog.Logger // nil descarta los registros
	// Registry recibe las métricas y se sirve en /metrics; nil crea uno propio.
	Registry *prometheus.Registry
	// Ready son los chequeos de /readyz (ver obs.Check).
	Ready []obs.Check
}

// ServiceName es el valor de la etiqueta service de las métricas HTTP.
const ServiceName = "go-identity"

// EscalationEndpoints son los patrones de ruta que pueden contar como intento de escalada
// (rutas de admin y consulta de sesiones por id).
var EscalationEndpoints = []string{"/auth/users", "/auth/sessions/{id}"}

// Server implementa el flujo de autenticación.
type Server struct {
	cfg     Config
	mu      sync.Mutex
	lastOTP map[string]uint64 // por usuario: último contador TOTP aceptado
	origins map[string]bool
	reg     *prometheus.Registry
	httpm   *obs.HTTPMetrics
	m       *obs.Auth
}

// New crea el servidor.
func New(cfg Config) *Server {
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	origins := map[string]bool{}
	for _, o := range cfg.AllowedOrigins {
		origins[o] = true
	}
	if cfg.Registry == nil {
		cfg.Registry = obs.NewRegistry()
	}
	sessions := cfg.Sessions
	m := obs.NewAuth(cfg.Registry, func() float64 {
		if sessions == nil {
			return 0
		}
		return float64(sessions.Active())
	}, EscalationEndpoints)
	return &Server{cfg: cfg, lastOTP: map[string]uint64{}, origins: origins,
		reg: cfg.Registry, httpm: obs.NewHTTPMetrics(cfg.Registry, ServiceName), m: m}
}

// routePattern devuelve el patrón de la ruta atendida sin el método ("unmatched" si no hubo).
func routePattern(r *http.Request) string {
	if r.Pattern == "" {
		return "unmatched"
	}
	if _, p, ok := strings.Cut(r.Pattern, " "); ok {
		return p
	}
	return r.Pattern
}

// forbidden responde 403 y cuenta la denegación y el intento de escalada (el caller ya está autenticado).
func (s *Server) forbidden(w http.ResponseWriter, r *http.Request) {
	s.m.AuthzDenied("forbidden")
	s.m.Escalation(routePattern(r))
	s.cfg.Logger.WarnContext(r.Context(), "acceso denegado", "outcome", "forbidden", "route", routePattern(r))
	writeError(w, http.StatusForbidden, "forbidden", "acceso denegado")
}

// NewDecoy genera el hash señuelo con los parámetros más altos de los usuarios cargados.
func NewDecoy(us map[string]*users.User) (string, error) {
	hs := make([]string, 0, len(us))
	for _, u := range us {
		hs = append(hs, u.PasswordHash)
	}
	return passhash.Decoy(hs)
}

func (s *Server) newRouter() *router {
	rt := newRouter(s)
	rt.Handle("POST /auth/login", Public(), s.login)
	rt.Handle("POST /auth/logout", Authenticated(), s.logout)
	rt.Handle("GET /auth/session", Authenticated(), s.currentSession)
	rt.Handle("GET /auth/sessions/{id}", Authenticated(), s.sessionByID)
	rt.Handle("GET /auth/users", Roles(principal.RoleAdmin), s.listUsers)
	return rt
}

// Routes enumera las rutas registradas con su política.
func (s *Server) Routes() []Route { return append([]Route(nil), s.newRouter().routes...) }

// Handler devuelve el http.Handler completo (cabeceras de seguridad, CORS y rutas con política).
// Sirve además /healthz, /readyz y /metrics (sin token) y aplica el contrato de observabilidad.
func (s *Server) Handler() http.Handler {
	return obs.Wrap(obs.Config{Service: ServiceName, Log: s.cfg.Logger, Registry: s.reg, Metrics: s.httpm,
		Ready: s.cfg.Ready, Route: routePattern}, s.secure(s.newRouter()))
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Code: code, Message: msg})
}

type loginRequest struct {
	Username string  `json:"username"`
	Password string  `json:"password"`
	OTP      *string `json:"otp"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ip := s.clientIP(r)
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		s.m.Login("bad_request")
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "se espera application/json")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req loginRequest
	if err := dec.Decode(&req); err != nil {
		s.m.Login("bad_request")
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "cuerpo demasiado grande")
			return
		}
		writeError(w, http.StatusBadRequest, "bad_request", "cuerpo JSON inválido")
		return
	}
	if _, err := dec.Token(); err != io.EOF {
		s.m.Login("bad_request")
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "cuerpo demasiado grande")
			return
		}
		writeError(w, http.StatusBadRequest, "bad_request", "cuerpo JSON inválido")
		return
	}
	if req.Username == "" || req.Password == "" || len(req.Username) > maxUsernameBytes || len(req.Password) > maxPasswordBytes {
		s.m.Login("bad_request")
		writeError(w, http.StatusBadRequest, "bad_request", "username o password ausentes o fuera de rango")
		return
	}
	if req.OTP != nil && !otpRe.MatchString(*req.OTP) {
		s.m.Login("bad_request")
		writeError(w, http.StatusBadRequest, "bad_request", "otp debe ser de 6 dígitos")
		return
	}

	key := users.Key(req.Username)
	ticket, retry, ok := s.cfg.Guard.Allow(key, ip)
	if !ok {
		secs := int(math.Ceil(retry.Seconds()))
		if secs < 1 {
			secs = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		s.m.Login("locked")
		s.cfg.Logger.WarnContext(r.Context(), "login bloqueado", "outcome", "locked", "ip", ip)
		writeError(w, http.StatusTooManyRequests, "too_many_attempts", "demasiados intentos; reintente más tarde")
		return
	}

	u := s.cfg.Users[key]
	hash := s.cfg.Decoy
	if u != nil {
		hash = u.PasswordHash
	}
	good, err := passhash.Verify(req.Password, hash)
	if err != nil || !good || u == nil {
		s.fail(w, r, ip, ticket)
		return
	}

	if u.Role == principal.RoleAdmin {
		if req.OTP == nil {
			s.cfg.Guard.Release(ticket)
			s.m.Login("mfa_required")
			s.cfg.Logger.InfoContext(r.Context(), "login requiere mfa", "outcome", "mfa_required", "ip", ip)
			writeError(w, http.StatusUnauthorized, "mfa_required", "se requiere el código OTP")
			return
		}
		if !s.checkOTP(key, u.MFASecret, *req.OTP) {
			s.fail(w, r, ip, ticket)
			return
		}
	}

	token, info, err := s.cfg.Sessions.CreateSession(principal.Principal{ID: u.Name, Role: u.Role})
	if err != nil {
		s.cfg.Guard.Release(ticket)
		s.cfg.Logger.ErrorContext(r.Context(), "no se pudo crear la sesión", "outcome", "error")
		writeError(w, http.StatusInternalServerError, "internal", "error interno")
		return
	}
	s.cfg.Guard.Success(ticket)
	s.m.Login("success")
	s.cfg.Logger.InfoContext(r.Context(), "login correcto", "outcome", "ok", "ip", ip)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]string{"token": token, "expires_at": info.ExpiresAt.UTC().Format(time.RFC3339), "session_id": info.ID})
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, ip string, tk guard.Ticket) {
	s.m.Login("invalid_credentials")
	if tk.Locked() {
		s.m.Lockout()
	}
	s.cfg.Logger.InfoContext(r.Context(), "login fallido", "outcome", "invalid_credentials", "ip", ip)
	writeError(w, http.StatusUnauthorized, "invalid_credentials", "credenciales inválidas")
}

// checkOTP verifica y consume el código de forma atómica (rechaza reutilización).
func (s *Server) checkOTP(userKey string, secret []byte, code string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := totp.Verify(secret, code, s.cfg.Clock(), s.lastOTP[userKey])
	if ok {
		s.lastOTP[userKey] = c
	}
	return ok
}

// logout revoca la sesión del token ya validado por el middleware (política Authenticated).
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Sessions.Revoke(fromCtx(r).token) {
		s.m.AuthzDenied("unauthorized")
		unauthorized(w)
		return
	}
	s.cfg.Logger.InfoContext(r.Context(), "logout", "outcome", "ok", "ip", s.clientIP(r))
	w.WriteHeader(http.StatusNoContent)
}

// clientIP usa RemoteAddr; X-Forwarded-For (último salto, el añadido por el proxy de confianza)
// solo con TrustProxy.
func (s *Server) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if s.cfg.TrustProxy {
		parts := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
		if ip := net.ParseIP(strings.TrimSpace(parts[len(parts)-1])); ip != nil {
			return ip.String()
		}
	}
	return host
}
