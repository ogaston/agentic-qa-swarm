package server

import (
	"encoding/base32"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/guard"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/passhash"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/session"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/users"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/principal"
)

type azEnv struct {
	t        *testing.T
	h        http.Handler
	srv      *Server
	clk      *clock
	sessions *session.Store
	tm, tl   string // marta y luis (user)
	ta       string // julian (admin)
}

func setupAuthZ(t *testing.T, origins ...string) *azEnv {
	t.Helper()
	e := &azEnv{t: t, clk: &clock{t: time.Unix(1_700_000_000, 0)}}
	hash, err := passhash.Hash(randHex(t, 12), nil)
	if err != nil {
		t.Fatal(err)
	}
	us, err := users.Parse(mustJSON(t, []map[string]string{
		{"username": "marta", "password_hash": hash, "role": "user"},
		{"username": "luis", "password_hash": hash, "role": "user"},
		{"username": "julian", "password_hash": hash, "role": "admin", "mfa_secret": base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(make([]byte, 20))},
	}))
	if err != nil {
		t.Fatal(err)
	}
	e.sessions = session.New(session.Config{Clock: e.clk.now})
	e.srv = New(Config{Users: us, Guard: guard.New(5, e.clk.now), Sessions: e.sessions, Clock: e.clk.now, Decoy: hash,
		AllowedOrigins: origins, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))})
	e.h = e.srv.Handler()
	mk := func(id string, r principal.Role) string {
		tok, _, err := e.sessions.Create(principal.Principal{ID: id, Role: r})
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	e.tm, e.tl, e.ta = mk("marta", principal.RoleUser), mk("luis", principal.RoleUser), mk("julian", principal.RoleAdmin)
	return e
}

func (e *azEnv) do(method, path string, hdr map[string]string, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

func bearer(tok string) map[string]string { return map[string]string{"Authorization": "Bearer " + tok} }

func TestAuthZ_AllRoutesDeclarePolicy(t *testing.T) {
	e := setupAuthZ(t)
	routes := e.srv.Routes()
	if len(routes) != 5 {
		t.Fatalf("se esperaban 5 rutas, hay %d", len(routes))
	}
	for _, r := range routes {
		if r.Policy.kind == kindInvalid {
			t.Errorf("%s sin política", r.Pattern)
		}
		if r.Policy.kind == kindPublic && r.Pattern != "POST /auth/login" {
			t.Errorf("%s es pública y solo login puede serlo", r.Pattern)
		}
	}
	// Toda ruta no pública rechaza la petición sin token.
	for _, r := range routes {
		if r.Policy.kind == kindPublic {
			continue
		}
		parts := strings.SplitN(r.Pattern, " ", 2)
		path := strings.ReplaceAll(parts[1], "{id}", "x")
		if w := e.do(parts[0], path, nil, ""); w.Code != 401 {
			t.Errorf("%s sin token: %d", r.Pattern, w.Code)
		}
	}
}

func TestAuthZ_PanicsWithoutPolicy(t *testing.T) {
	rt := newRouter(New(Config{}))
	defer func() {
		if recover() == nil {
			t.Fatal("registrar sin política debía panicar")
		}
	}()
	rt.Handle("GET /x", Policy{}, func(http.ResponseWriter, *http.Request) {})
}

func TestAuthZ_RolesPanicsOnBadRoles(t *testing.T) {
	for name, rs := range map[string][]principal.Role{"vacio": nil, "desconocido": {"root"}} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("debía panicar")
				}
			}()
			Roles(rs...)
		})
	}
}

func TestAuthZ_UnregisteredRoute404(t *testing.T) {
	e := setupAuthZ(t)
	for _, p := range []string{"/admin", "/", "/auth", "/auth/users/x", "/healthz"} {
		w := e.do("GET", p, bearer(e.ta), "")
		if w.Code != 404 || code(t, w) != "not_found" {
			t.Errorf("%s: %d %s", p, w.Code, w.Body)
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s: content-type %q", p, ct)
		}
	}
}

func TestAuthZ_WrongMethodIsNever2xx(t *testing.T) {
	e := setupAuthZ(t)
	for _, m := range []string{"DELETE", "PUT", "POST", "PATCH"} {
		w := e.do(m, "/auth/users", bearer(e.tm), `{"role":"admin"}`)
		if w.Code != 405 || code(t, w) != "method_not_allowed" || w.Header().Get("Allow") == "" {
			t.Errorf("%s: %d %s", m, w.Code, w.Body)
		}
	}
}

func TestAuthZ_BearerParsing(t *testing.T) {
	e := setupAuthZ(t)
	ok := e.tm
	cases := map[string]string{
		"vacio":              "",
		"solo esquema":       "Bearer",
		"esquema y espacio":  "Bearer ",
		"basic":              "Basic " + ok,
		"sin esquema":        ok,
		"doble espacio":      "Bearer  " + ok,
		"espacio final":      "Bearer " + ok + " ",
		"tab":                "Bearer\t" + ok,
		"42 caracteres":      "Bearer " + ok[:42],
		"44 caracteres":      "Bearer " + ok + "A",
		"fuera de base64url": "Bearer " + ok[:42] + "=",
		"con mas":            "Bearer " + ok[:42] + "+",
		"inexistente":        "Bearer " + strings.Repeat("A", 43),
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			hdr := map[string]string{}
			if h != "" {
				hdr["Authorization"] = h
			}
			w := e.do("GET", "/auth/session", hdr, "")
			if w.Code != 401 || code(t, w) != "unauthorized" || w.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatalf("%d %s %v", w.Code, w.Body, w.Header())
			}
		})
	}
	for _, scheme := range []string{"Bearer", "bearer", "BEARER", "bEaReR"} {
		if w := e.do("GET", "/auth/session", map[string]string{"Authorization": scheme + " " + ok}, ""); w.Code != 200 {
			t.Errorf("esquema %q: %d", scheme, w.Code)
		}
	}
}

func TestAuthZ_TwoAuthorizationHeaders401(t *testing.T) {
	e := setupAuthZ(t)
	r := httptest.NewRequest("GET", "/auth/session", nil)
	r.Header.Add("Authorization", "Bearer "+e.tm)
	r.Header.Add("Authorization", "Bearer "+e.tm)
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("%d", w.Code)
	}
}

func TestAuthZ_UniformUnauthorizedBody(t *testing.T) {
	e := setupAuthZ(t)
	tokExp, _, _ := e.sessions.Create(principal.Principal{ID: "marta", Role: principal.RoleUser})
	e.clk.add(31 * time.Minute)
	var bodies []string
	for _, h := range []map[string]string{nil, {"Authorization": "Bearer x"}, bearer(strings.Repeat("A", 43)), bearer(tokExp)} {
		w := e.do("GET", "/auth/session", h, "")
		if w.Code != 401 {
			t.Fatalf("%d", w.Code)
		}
		bodies = append(bodies, w.Body.String())
	}
	for _, b := range bodies[1:] {
		if b != bodies[0] {
			t.Fatalf("cuerpos distintos: %q vs %q", b, bodies[0])
		}
	}
}

func TestAuthZ_SessionOK(t *testing.T) {
	e := setupAuthZ(t)
	w := e.do("GET", "/auth/session", bearer(e.tm), "")
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var b map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	if len(b) != 4 || b["principal_id"] != "marta" || b["role"] != "user" || len(b["session_id"].(string)) != 32 {
		t.Fatalf("%v", b)
	}
	if _, err := time.Parse(time.RFC3339, b["expires_at"].(string)); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(w.Body.String(), e.tm) {
		t.Fatal("la respuesta no debe contener el token")
	}
}

func TestAuthZ_SessionCountsAsUseForIdle(t *testing.T) {
	e := setupAuthZ(t)
	for i := 0; i < 3; i++ {
		e.clk.add(10 * time.Minute) // < 15 min de inactividad, pero 30 min en total vence la absoluta
		if i < 2 {
			if w := e.do("GET", "/auth/session", bearer(e.tm), ""); w.Code != 200 {
				t.Fatalf("iteración %d: %d", i, w.Code)
			}
		}
	}
	if w := e.do("GET", "/auth/session", bearer(e.tm), ""); w.Code != 401 {
		t.Fatalf("la expiración absoluta debía aplicar: %d", w.Code)
	}
}

func TestAuthZ_IdleExpired401(t *testing.T) {
	e := setupAuthZ(t)
	e.clk.add(16 * time.Minute)
	if w := e.do("GET", "/auth/session", bearer(e.tm), ""); w.Code != 401 {
		t.Fatalf("%d", w.Code)
	}
}

func TestAuthZ_RevokedFailsNextRequest(t *testing.T) {
	e := setupAuthZ(t)
	if w := e.do("GET", "/auth/session", bearer(e.tm), ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := e.do("POST", "/auth/logout", bearer(e.tm), ""); w.Code != 204 {
		t.Fatal(w.Code)
	}
	for _, p := range []string{"/auth/session", "/auth/sessions/x", "/auth/users"} {
		if w := e.do("GET", p, bearer(e.tm), ""); w.Code != 401 {
			t.Errorf("%s tras logout: %d", p, w.Code)
		}
	}
}

func (e *azEnv) sid(tok string) string {
	e.t.Helper()
	w := e.do("GET", "/auth/session", bearer(tok), "")
	var b struct {
		SessionID string `json:"session_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	return b.SessionID
}

func TestAuthZ_IDOR(t *testing.T) {
	e := setupAuthZ(t)
	sid := e.sid(e.tm)
	zero := strings.Repeat("0", 32)
	cases := []struct {
		name string
		tok  string
		id   string
		want int
		code string
	}{
		{"propia", e.tm, sid, 200, ""},
		{"ajena", e.tl, sid, 403, "forbidden"},
		{"inexistente user", e.tl, zero, 403, "forbidden"},
		{"id raro user", e.tl, "..%2f..", 403, "forbidden"},
		{"admin ajena", e.ta, sid, 200, ""},
		{"admin inexistente", e.ta, zero, 404, "not_found"},
		{"sin token", "", sid, 401, "unauthorized"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var h map[string]string
			if c.tok != "" {
				h = bearer(c.tok)
			}
			w := e.do("GET", "/auth/sessions/"+c.id, h, "")
			if w.Code != c.want || (c.code != "" && code(t, w) != c.code) {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
		})
	}
	// 403 de sesión ajena e inexistente son indistinguibles.
	a, b := e.do("GET", "/auth/sessions/"+sid, bearer(e.tl), ""), e.do("GET", "/auth/sessions/"+zero, bearer(e.tl), "")
	if a.Body.String() != b.Body.String() {
		t.Fatal("el cuerpo del 403 revela existencia")
	}
}

func TestAuthZ_SessionIDIsNotTheToken(t *testing.T) {
	e := setupAuthZ(t)
	sid := e.sid(e.tm)
	if sid == e.tm || strings.Contains(e.tm, sid) {
		t.Fatal("el id público no puede derivarse trivialmente del token")
	}
	if w := e.do("GET", "/auth/session", bearer(sid), ""); w.Code != 401 {
		t.Fatalf("el session_id no debe valer como token: %d", w.Code)
	}
}

func TestAuthZ_UsersByRole(t *testing.T) {
	e := setupAuthZ(t)
	if w := e.do("GET", "/auth/users", bearer(e.tm), ""); w.Code != 403 || code(t, w) != "forbidden" {
		t.Fatalf("user: %d %s", w.Code, w.Body)
	}
	w := e.do("GET", "/auth/users", bearer(e.ta), "")
	if w.Code != 200 {
		t.Fatalf("admin: %d", w.Code)
	}
	var got []map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if len(got) != 3 || got[0]["username"] != "julian" || got[0]["role"] != "admin" || got[1]["username"] != "luis" || got[2]["username"] != "marta" {
		t.Fatalf("%v", got)
	}
	for _, m := range got {
		if len(m) != 2 {
			t.Fatalf("campos de más: %v", m)
		}
	}
	for _, bad := range []string{"argon2", "password", "mfa", "secret"} {
		if strings.Contains(strings.ToLower(w.Body.String()), bad) {
			t.Errorf("la respuesta contiene %q", bad)
		}
	}
}

func TestAuthZ_RoleComesFromServerOnly(t *testing.T) {
	e := setupAuthZ(t)
	h := bearer(e.tm)
	h["X-Role"], h["X-User"] = "admin", "julian"
	if w := e.do("GET", "/auth/users", h, ""); w.Code != 403 {
		t.Errorf("cabeceras: %d", w.Code)
	}
	if w := e.do("GET", "/auth/users?role=admin&user=julian", bearer(e.tm), ""); w.Code != 403 {
		t.Errorf("query: %d", w.Code)
	}
	if w := e.do("POST", "/auth/users", bearer(e.tm), `{"role":"admin"}`); w.Code/100 == 2 {
		t.Errorf("cuerpo: %d", w.Code)
	}
	w := e.do("GET", "/auth/session?role=admin", h, "")
	var b struct{ PrincipalID, Role string }
	_ = json.Unmarshal(w.Body.Bytes(), &struct {
		P *string `json:"principal_id"`
		R *string `json:"role"`
	}{&b.PrincipalID, &b.Role})
	if b.PrincipalID != "marta" || b.Role != "user" {
		t.Errorf("session con cabeceras de rol: %v", b)
	}
}

func TestAuthZ_CORS(t *testing.T) {
	e := setupAuthZ(t, "https://app.example", "http://localhost:3000")
	cases := []struct {
		origin string
		allow  bool
	}{
		{"https://app.example", true},
		{"http://localhost:3000", true},
		{"https://evil.example", false},
		{"null", false},
		{"https://app.example:8443", false},
		{"https://sub.app.example", false},
		{"http://app.example", false},
		{"https://APP.example", false},
		{"*", false},
	}
	for _, c := range cases {
		t.Run(c.origin, func(t *testing.T) {
			h := bearer(e.tm)
			h["Origin"] = c.origin
			w := e.do("GET", "/auth/session", h, "")
			got := w.Header().Get("Access-Control-Allow-Origin")
			if c.allow && (got != c.origin || w.Header().Get("Vary") != "Origin") {
				t.Fatalf("permitido: %q %v", got, w.Header())
			}
			if !c.allow {
				for k := range w.Header() {
					if strings.HasPrefix(strings.ToLower(k), "access-control-") {
						t.Fatalf("origen no permitido con %s", k)
					}
				}
			}
			if w.Header().Get("Access-Control-Allow-Credentials") != "" || got == "*" {
				t.Fatal("jamás credentials ni *")
			}
			// preflight
			p := e.do("OPTIONS", "/auth/session", map[string]string{"Origin": c.origin, "Access-Control-Request-Method": "GET", "Access-Control-Request-Headers": "authorization"}, "")
			if c.allow {
				if p.Code != 204 || p.Header().Get("Access-Control-Allow-Origin") != c.origin ||
					p.Header().Get("Access-Control-Allow-Headers") != "Authorization, Content-Type" ||
					p.Header().Get("Access-Control-Allow-Methods") != "GET, POST" || p.Header().Get("Access-Control-Allow-Credentials") != "" {
					t.Fatalf("preflight: %d %v", p.Code, p.Header())
				}
			} else if p.Header().Get("Access-Control-Allow-Origin") != "" || p.Code == 204 {
				t.Fatalf("preflight de origen no permitido: %d %v", p.Code, p.Header())
			}
		})
	}
}

func TestAuthZ_CORSEmptyListAllowsNoOrigin(t *testing.T) {
	e := setupAuthZ(t)
	h := bearer(e.tm)
	h["Origin"] = "https://app.example"
	if w := e.do("GET", "/auth/session", h, ""); w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("lista vacía = ningún origen")
	}
}

func TestAuthZ_ParseOrigins(t *testing.T) {
	if o, err := ParseOrigins(""); err != nil || len(o) != 0 {
		t.Fatalf("vacío: %v %v", o, err)
	}
	if o, err := ParseOrigins(" https://a.example , http://localhost:3000,"); err != nil || len(o) != 2 || o[0] != "https://a.example" {
		t.Fatalf("%v %v", o, err)
	}
	for _, bad := range []string{"*", "null", "https://a.example/", "https://a.example/x", "ftp://a.example", "a.example", "https://u@a.example", "https://a.example,*"} {
		if _, err := ParseOrigins(bad); err == nil {
			t.Errorf("%q debía rechazarse", bad)
		}
	}
}

func TestAuthZ_SecurityHeadersOnEveryResponse(t *testing.T) {
	e := setupAuthZ(t)
	reqs := []struct{ m, p string }{{"GET", "/auth/session"}, {"GET", "/no-existe"}, {"DELETE", "/auth/users"}, {"POST", "/auth/login"}, {"OPTIONS", "/auth/session"}}
	for _, rq := range reqs {
		w := e.do(rq.m, rq.p, nil, "")
		want := map[string]string{
			"X-Content-Type-Options":    "nosniff",
			"Cache-Control":             "no-store",
			"Content-Security-Policy":   "default-src 'none'; frame-ancestors 'none'",
			"Strict-Transport-Security": "max-age=63072000; includeSubDomains",
		}
		for k, v := range want {
			if w.Header().Get(k) != v {
				t.Errorf("%s %s: %s = %q", rq.m, rq.p, k, w.Header().Get(k))
			}
		}
	}
	// También en 2xx y en preflight permitido.
	if w := e.do("GET", "/auth/session", bearer(e.tm), ""); w.Header().Get("Cache-Control") != "no-store" {
		t.Error("200 sin no-store")
	}
}

func TestAuthZ_LoginReturnsSessionID(t *testing.T) {
	e := setup(t, false)
	w := e.login(map[string]string{"username": "marta", "password": e.pw})
	var b map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	if len(b["session_id"]) != 32 {
		t.Fatalf("%v", b)
	}
	s := e.post("/auth/logout", "", nil, map[string]string{"Authorization": "Bearer " + b["token"]}, "")
	if s.Code != 204 {
		t.Fatal(s.Code)
	}
}
