package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-identity/authz"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/session"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/principal"
)

type policyKind int

const (
	kindInvalid policyKind = iota // valor cero: registrar sin política panica
	kindPublic
	kindAuthenticated
	kindRoles
)

// Policy es la política obligatoria de una ruta.
type Policy struct {
	kind  policyKind
	roles []principal.Role
}

// Public no exige token. Solo para POST /auth/login.
func Public() Policy { return Policy{kind: kindPublic} }

// Authenticated exige un token válido de cualquier rol.
func Authenticated() Policy { return Policy{kind: kindAuthenticated} }

// Roles exige un token válido cuyo rol (el de la sesión) esté en la lista.
// Panica si la lista está vacía o trae un rol desconocido.
func Roles(rs ...principal.Role) Policy {
	if len(rs) == 0 {
		panic("server: Roles sin roles")
	}
	for _, r := range rs {
		if _, err := principal.ParseRole(string(r)); err != nil {
			panic("server: " + err.Error())
		}
	}
	return Policy{kind: kindRoles, roles: append([]principal.Role(nil), rs...)}
}

func (p Policy) String() string {
	switch p.kind {
	case kindPublic:
		return "public"
	case kindAuthenticated:
		return "authenticated"
	case kindRoles:
		s := make([]string, len(p.roles))
		for i, r := range p.roles {
			s[i] = string(r)
		}
		return "roles(" + strings.Join(s, "|") + ")"
	}
	return "invalid"
}

// Route es una ruta registrada con su política.
type Route struct {
	Pattern string
	Policy  Policy
}

// router es el registro de rutas: no hay forma de añadir una ruta sin política.
type router struct {
	mux    *http.ServeMux
	routes []Route
	srv    *Server
}

func newRouter(s *Server) *router { return &router{mux: http.NewServeMux(), srv: s} }

// Handle registra patrón (con método, p. ej. "GET /auth/session") con su política obligatoria.
// Panica si la política es la de valor cero. Lo no registrado responde 404.
func (rt *router) Handle(pattern string, pol Policy, h http.HandlerFunc) {
	if pol.kind == kindInvalid {
		panic(fmt.Sprintf("server: la ruta %q se registró sin política", pattern))
	}
	rt.routes = append(rt.routes, Route{Pattern: pattern, Policy: pol})
	var handler http.Handler = h
	if pol.kind != kindPublic {
		handler = rt.srv.authenticate(pol, h)
	}
	rt.mux.Handle(pattern, handler)
}

// ServeHTTP delega en el mux; los 404/405 propios del mux salen como Error JSON.
func (rt *router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, pat := rt.mux.Handler(r); pat == "" {
		w = &muxErrorWriter{ResponseWriter: w}
	}
	rt.mux.ServeHTTP(w, r)
}

// muxErrorWriter reescribe el cuerpo de texto del mux como Error JSON.
type muxErrorWriter struct {
	http.ResponseWriter
	wrote bool
	// passthrough: respuestas del mux que no son 404/405 (p. ej. redirecciones de ruta canónica).
	passthrough bool
}

func (m *muxErrorWriter) WriteHeader(status int) {
	if m.wrote {
		return
	}
	m.wrote = true
	if status != http.StatusNotFound && status != http.StatusMethodNotAllowed {
		m.passthrough = true
		m.ResponseWriter.WriteHeader(status)
		return
	}
	code, msg := "not_found", "recurso no encontrado"
	if status == http.StatusMethodNotAllowed {
		code, msg = "method_not_allowed", "método no permitido"
	}
	m.Header().Set("Content-Type", "application/json")
	m.Header().Del("Content-Length")
	m.ResponseWriter.WriteHeader(status)
	_ = json.NewEncoder(m.ResponseWriter).Encode(errorBody{Code: code, Message: msg})
}

func (m *muxErrorWriter) Write(b []byte) (int, error) {
	if !m.wrote {
		m.WriteHeader(http.StatusOK)
	}
	if m.passthrough {
		return m.ResponseWriter.Write(b)
	}
	return len(b), nil // el cuerpo de texto del mux se descarta
}

type ctxKey struct{}

type authCtx struct {
	info  session.Info
	token string
}

func fromCtx(r *http.Request) authCtx {
	v, _ := r.Context().Value(ctxKey{}).(authCtx)
	return v
}

const tokenLen = 43 // 32 bytes en base64url sin relleno

// bearerToken extrae el token de forma estricta: una sola cabecera, esquema "bearer" sin distinguir
// mayúsculas, un único espacio y exactamente 43 caracteres base64url.
func bearerToken(r *http.Request) (string, bool) {
	vs := r.Header.Values("Authorization")
	if len(vs) != 1 {
		return "", false
	}
	const prefix = "bearer "
	h := vs[0]
	if len(h) != len(prefix)+tokenLen || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	tok := h[len(prefix):]
	for i := 0; i < len(tok); i++ {
		c := tok[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return "", false
		}
	}
	return tok, true
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	writeError(w, http.StatusUnauthorized, "unauthorized", "token inválido o ausente")
}

// authenticate valida el token en cada petición (sin caché de decisiones) y aplica la política.
// El rol sale solo de la sesión; ninguna cabecera, parámetro o cuerpo participa.
func (s *Server) authenticate(pol Policy, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, ok := bearerToken(r)
		if !ok {
			s.m.AuthzDenied("unauthorized")
			unauthorized(w)
			return
		}
		info, ok := s.cfg.Sessions.Authenticate(tok)
		if !ok {
			s.m.AuthzDenied("unauthorized")
			unauthorized(w)
			return
		}
		if pol.kind == kindRoles {
			allowed := false
			for _, role := range pol.roles {
				if role == info.Principal.Role {
					allowed = true
				}
			}
			if !allowed {
				s.forbidden(w, r)
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, authCtx{info: info, token: tok})))
	})
}

type sessionBody struct {
	PrincipalID string         `json:"principal_id"`
	Role        principal.Role `json:"role"`
	SessionID   string         `json:"session_id"`
	ExpiresAt   string         `json:"expires_at"`
}

func toSessionBody(i session.Info) sessionBody {
	return sessionBody{PrincipalID: i.Principal.ID, Role: i.Principal.Role, SessionID: i.ID, ExpiresAt: i.ExpiresAt.UTC().Format(time.RFC3339)}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) currentSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, toSessionBody(fromCtx(r).info))
}

func (s *Server) sessionByID(w http.ResponseWriter, r *http.Request) {
	caller := fromCtx(r).info.Principal
	id := r.PathValue("id")
	target, found := s.cfg.Sessions.Lookup(id)
	res := authz.Resource{Type: authz.TypeSession, ID: id}
	if found {
		res.Owner = target.Principal.ID
	}
	// Un recurso inexistente no tiene propietario: solo un admin pasa Authorize y ve 404;
	// un user recibe 403 igual que con una sesión ajena (sin oráculo de existencia).
	if authz.Authorize(caller, authz.ActionSessionRead, res) != authz.Allow {
		s.forbidden(w, r)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "sesión no encontrada")
		return
	}
	writeJSON(w, toSessionBody(target))
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	caller := fromCtx(r).info.Principal
	if authz.Authorize(caller, authz.ActionUsersList, authz.Resource{Type: authz.TypeUsers}) != authz.Allow {
		s.forbidden(w, r)
		return
	}
	type item struct {
		Username string         `json:"username"`
		Role     principal.Role `json:"role"`
	}
	out := make([]item, 0, len(s.cfg.Users))
	for _, u := range s.cfg.Users {
		out = append(out, item{Username: u.Name, Role: u.Role})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	writeJSON(w, out)
}

// ParseOrigins interpreta IDENTITY_ALLOWED_ORIGINS (coma-separada). Vacío = ningún origen.
// Cada entrada debe ser un origen http(s) exacto (esquema://host[:puerto], sin ruta); nunca "*".
func ParseOrigins(v string) ([]string, error) {
	var out []string
	for _, o := range strings.Split(v, ",") {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		u, err := url.Parse(o)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.User != nil || u.Fragment != "" || strings.HasSuffix(o, "/") {
			return nil, errors.New("IDENTITY_ALLOWED_ORIGINS inválido: cada entrada debe ser un origen http(s) exacto")
		}
		out = append(out, o)
	}
	return out, nil
}

// secure añade las cabeceras de seguridad a toda respuesta y aplica CORS por lista blanca.
func (s *Server) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "no-store")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		if origin := r.Header.Get("Origin"); origin != "" && s.origins[origin] {
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				h.Set("Access-Control-Allow-Methods", "GET, POST")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				h.Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
