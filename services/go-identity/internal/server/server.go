// Package server expone POST /auth/login y POST /auth/logout.
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

	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/guard"
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
	Logger     *slog.Logger     // nil descarta los registros
}

// Server implementa el flujo de autenticación.
type Server struct {
	cfg     Config
	mu      sync.Mutex
	lastOTP map[string]uint64 // por usuario: último contador TOTP aceptado
}

// New crea el servidor.
func New(cfg Config) *Server {
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	return &Server{cfg: cfg, lastOTP: map[string]uint64{}}
}

// NewDecoy genera el hash señuelo con los parámetros más altos de los usuarios cargados.
func NewDecoy(us map[string]*users.User) (string, error) {
	hs := make([]string, 0, len(us))
	for _, u := range us {
		hs = append(hs, u.PasswordHash)
	}
	return passhash.Decoy(hs)
}

// Handler devuelve el http.Handler con las dos rutas.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/login", s.login)
	mux.HandleFunc("POST /auth/logout", s.logout)
	return mux
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
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "se espera application/json")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req loginRequest
	if err := dec.Decode(&req); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "cuerpo demasiado grande")
			return
		}
		writeError(w, http.StatusBadRequest, "bad_request", "cuerpo JSON inválido")
		return
	}
	if _, err := dec.Token(); err != io.EOF {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "cuerpo demasiado grande")
			return
		}
		writeError(w, http.StatusBadRequest, "bad_request", "cuerpo JSON inválido")
		return
	}
	if req.Username == "" || req.Password == "" || len(req.Username) > maxUsernameBytes || len(req.Password) > maxPasswordBytes {
		writeError(w, http.StatusBadRequest, "bad_request", "username o password ausentes o fuera de rango")
		return
	}
	if req.OTP != nil && !otpRe.MatchString(*req.OTP) {
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
		s.cfg.Logger.Warn("login bloqueado", "outcome", "locked", "ip", ip)
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
		s.fail(w, ip)
		return
	}

	if u.Role == principal.RoleAdmin {
		if req.OTP == nil {
			s.cfg.Guard.Release(ticket)
			s.cfg.Logger.Info("login requiere mfa", "outcome", "mfa_required", "ip", ip)
			writeError(w, http.StatusUnauthorized, "mfa_required", "se requiere el código OTP")
			return
		}
		if !s.checkOTP(key, u.MFASecret, *req.OTP) {
			s.fail(w, ip)
			return
		}
	}

	token, exp, err := s.cfg.Sessions.Create(principal.Principal{ID: u.Name, Role: u.Role})
	if err != nil {
		s.cfg.Guard.Release(ticket)
		s.cfg.Logger.Error("no se pudo crear la sesión", "outcome", "error")
		writeError(w, http.StatusInternalServerError, "internal", "error interno")
		return
	}
	s.cfg.Guard.Success(ticket)
	s.cfg.Logger.Info("login correcto", "outcome", "ok", "ip", ip)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]string{"token": token, "expires_at": exp.UTC().Format(time.RFC3339)})
}

func (s *Server) fail(w http.ResponseWriter, ip string) {
	s.cfg.Logger.Info("login fallido", "outcome", "invalid_credentials", "ip", ip)
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

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	const prefix = "bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) || len(h) > 512 || !s.cfg.Sessions.Revoke(strings.TrimSpace(h[len(prefix):])) {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "unauthorized", "token inválido o ausente")
		return
	}
	s.cfg.Logger.Info("logout", "outcome", "ok", "ip", s.clientIP(r))
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
