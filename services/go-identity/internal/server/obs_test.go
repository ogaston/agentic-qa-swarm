package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/guard"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/obs"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/passhash"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/session"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/users"
)

func scrape(h http.Handler) string {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	return rec.Body.String()
}

func hasLine(m, line string) bool {
	for _, l := range strings.Split(m, "\n") {
		if l == line {
			return true
		}
	}
	return false
}

func TestAuthMetricsStartAtZero(t *testing.T) {
	e := setup(t, false)
	m := scrape(e.h)
	for _, l := range []string{
		`aqs_auth_lockouts_total 0`, `aqs_auth_active_sessions 0`,
		`aqs_auth_login_total{result="success"} 0`, `aqs_auth_login_total{result="invalid_credentials"} 0`,
		`aqs_auth_login_total{result="mfa_required"} 0`, `aqs_auth_login_total{result="locked"} 0`,
		`aqs_auth_login_total{result="bad_request"} 0`,
		`aqs_authz_denied_total{reason="unauthorized"} 0`, `aqs_authz_denied_total{reason="forbidden"} 0`,
		`aqs_privilege_escalation_attempts_total{endpoint="/auth/users"} 0`,
		`aqs_privilege_escalation_attempts_total{endpoint="/auth/sessions/{id}"} 0`,
	} {
		if !hasLine(m, l) {
			t.Errorf("falta la serie inicial %q", l)
		}
	}
}

func TestEscalationEndpointsMatchRegisteredRoutes(t *testing.T) {
	e := setup(t, false)
	pats := map[string]bool{}
	for _, r := range e.srv.Routes() {
		_, p, _ := strings.Cut(r.Pattern, " ")
		pats[p] = true
	}
	for _, ep := range EscalationEndpoints {
		if !pats[ep] {
			t.Errorf("%q no es un patrón de ruta registrado", ep)
		}
	}
}

func TestLoginResultMetrics(t *testing.T) {
	e := setup(t, false)
	e.post("/auth/login", "application/json", []byte(`{no es json`), nil, "")
	e.post("/auth/login", "text/plain", []byte(`x`), nil, "")
	e.login(map[string]string{"username": "julian", "password": e.apw}) // admin sin otp
	e.login(map[string]string{"username": "marta", "password": e.pw})
	for i := 0; i < 5; i++ {
		e.login(map[string]string{"username": "nadie", "password": "incorrecta-123"})
	}
	e.login(map[string]string{"username": "nadie", "password": "incorrecta-123"}) // ya bloqueado
	m := scrape(e.h)
	for _, l := range []string{
		`aqs_auth_login_total{result="bad_request"} 2`, `aqs_auth_login_total{result="mfa_required"} 1`,
		`aqs_auth_login_total{result="success"} 1`, `aqs_auth_login_total{result="invalid_credentials"} 5`,
		`aqs_auth_login_total{result="locked"} 1`, `aqs_auth_lockouts_total 1`,
	} {
		if !hasLine(m, l) {
			t.Errorf("falta %q\n%s", l, grepLines(m, "aqs_auth_"))
		}
	}
}

func grepLines(s, sub string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, sub) {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

func TestActiveSessionsGauge(t *testing.T) {
	e := setup(t, false)
	w := e.login(map[string]string{"username": "marta", "password": e.pw})
	var b struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	if !hasLine(scrape(e.h), `aqs_auth_active_sessions 1`) {
		t.Fatal("una sesión abierta")
	}
	r := httptest.NewRequest("POST", "/auth/logout", nil)
	r.Header.Set("Authorization", "Bearer "+b.Token)
	e.h.ServeHTTP(httptest.NewRecorder(), r)
	if !hasLine(scrape(e.h), `aqs_auth_active_sessions 0`) {
		t.Fatal("tras el logout no quedan sesiones")
	}
	e.login(map[string]string{"username": "marta", "password": e.pw})
	e.clk.add(time.Hour)
	if !hasLine(scrape(e.h), `aqs_auth_active_sessions 0`) {
		t.Fatal("una sesión expirada no cuenta")
	}
}

func TestAuthzDeniedAndPrivilegeEscalationMetrics(t *testing.T) {
	e := setupAuthZ(t)
	e.do("GET", "/auth/users", nil, "")                                          // sin token
	e.do("GET", "/auth/users", bearer("x"), "")                                  // token inválido
	e.do("GET", "/auth/users", bearer(e.tm), "")                                 // user a ruta de admin
	e.do("GET", "/auth/sessions/"+strings.Repeat("c0ffee", 5), bearer(e.tm), "") // sesión ajena/inexistente
	e.do("GET", "/auth/users", bearer(e.ta), "")                                 // admin: permitido
	e.do("GET", "/auth/sessions/"+strings.Repeat("c0ffee", 5), bearer(e.ta), "")
	m := scrape(e.h)
	for _, l := range []string{
		`aqs_authz_denied_total{reason="unauthorized"} 2`, `aqs_authz_denied_total{reason="forbidden"} 2`,
		`aqs_privilege_escalation_attempts_total{endpoint="/auth/users"} 1`,
		`aqs_privilege_escalation_attempts_total{endpoint="/auth/sessions/{id}"} 1`,
	} {
		if !hasLine(m, l) {
			t.Errorf("falta %q\n%s", l, grepLines(m, "aqs_a"))
		}
	}
	if strings.Contains(m, "c0ffee") {
		t.Error("el id de sesión no debe aparecer en las métricas")
	}
}

func TestMetricsLabelsHaveNoUsernameIPOrToken(t *testing.T) {
	e := setupAuthZ(t)
	for i := 0; i < 15; i++ {
		e.do("GET", fmt.Sprintf("/ruta-%d", i), bearer(e.tm), "")
		e.do("GET", fmt.Sprintf("/auth/sessions/id-%d", i), bearer(e.tm), "")
	}
	m := scrape(e.h)
	for _, s := range []string{"marta", "luis", "julian", "192.0.2.", "127.0.0.1", e.tm, e.ta, "ruta-", "id-1"} {
		if strings.Contains(m, s) {
			t.Errorf("/metrics contiene %q", s)
		}
	}
	if !hasLine(m, `aqs_http_requests_total{code="404",method="GET",route="unmatched",service="go-identity"} 15`) {
		t.Errorf("las rutas desconocidas deben agruparse en unmatched\n%s", grepLines(m, "aqs_http_requests_total"))
	}
}

// obsEnv arma un servidor con el logger del contrato y chequeos de readiness inyectables.
func obsEnv(t *testing.T, ready []obs.Check) (h http.Handler, logs *bytes.Buffer, pw string, hash string) {
	t.Helper()
	pw = randHex(t, 12)
	hash, _ = passhash.Hash(pw, nil)
	us, err := users.Parse(mustJSON(t, []map[string]string{{"username": "marta", "password_hash": hash, "role": "user"}}))
	if err != nil {
		t.Fatal(err)
	}
	logs = &bytes.Buffer{}
	s := New(Config{Users: us, Guard: guard.New(5, nil), Sessions: session.New(session.Config{}), Decoy: hash,
		Logger: obs.NewLogger(logs, ServiceName, slog.LevelDebug), Ready: ready})
	return s.Handler(), logs, pw, hash
}

func TestOpsEndpointsPublicAndOutsideRateLimit(t *testing.T) {
	h, _, pw, _ := obsEnv(t, nil)
	for i := 0; i < 100; i++ {
		for _, p := range []string{"/healthz", "/readyz", "/metrics"} {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
			if rec.Code != 200 {
				t.Fatalf("%s: %d", p, rec.Code)
			}
		}
	}
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/auth/login", strings.NewReader(fmt.Sprintf(`{"username":"marta","password":%q}`, pw)))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, r)
	if rec.Code != 200 {
		t.Fatalf("las sondas no deben afectar al guard: %d", rec.Code)
	}
}

func TestReadyzFailsWhenUsersOrSessionsCheckFails(t *testing.T) {
	for _, failing := range []string{"users", "sessions"} {
		var cs []obs.Check
		for _, n := range []string{"users", "sessions"} {
			var err error
			if n == failing {
				err = errors.New("falla")
			}
			cs = append(cs, obs.Check{Name: n, Fn: func(context.Context) error { return err }})
		}
		h, _, _, _ := obsEnv(t, cs)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
		if rec.Code != 503 || !strings.Contains(rec.Body.String(), `"`+failing+`":"fail"`) {
			t.Errorf("%s: %d %s", failing, rec.Code, rec.Body)
		}
	}
}

func TestLogsFollowContractAndHoldNoSecrets(t *testing.T) {
	h, logs, pw, hash := obsEnv(t, nil)
	post := func(body string) {
		r := httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Request-Id", "req-9081727878")
		h.ServeHTTP(httptest.NewRecorder(), r)
	}
	post(fmt.Sprintf(`{"username":"marta","password":%q}`, pw))
	post(`{"username":"marta","password":"CLAVE-INCORRECTA-XYZ","otp":"555019"}`)
	post(`{"username":"marta","password":"CLAVE-INCORRECTA-XYZ"}`)
	out := logs.String()
	for _, s := range []string{pw, hash, "CLAVE-INCORRECTA-XYZ", "555019"} {
		if strings.Contains(out, s) {
			t.Errorf("el log contiene un secreto: %q\n%s", s, out)
		}
	}
	n := 0
	for _, ln := range strings.Split(strings.TrimSpace(out), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			t.Fatalf("línea no JSON: %s", ln)
		}
		n++
		for _, k := range []string{"timestamp", "request_id", "trace_id", "level", "message"} {
			if _, ok := m[k]; !ok {
				t.Errorf("sin %q: %s", k, ln)
			}
		}
		if lv, _ := m["level"].(string); lv != strings.ToLower(lv) {
			t.Errorf("level en mayúsculas: %s", ln)
		}
		if m["message"] == "login correcto" && m["request_id"] != "req-9081727878" {
			t.Errorf("el log de aplicación debe llevar el request_id de la petición: %s", ln)
		}
	}
	if n < 3 {
		t.Fatalf("pocas líneas: %d", n)
	}
}
