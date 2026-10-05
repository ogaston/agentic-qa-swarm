package httpapi

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/ui-api/inbox"
	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/auth"
)

const secretTok = "tok-SECRETO-123"

var sha = strings.Repeat("a", 40)

type env struct {
	h     http.Handler
	store *inbox.Store
	logs  *bytes.Buffer
	clock *time.Time
	dir   string
}

func newEnv(t *testing.T, v auth.TokenVerifier, mod func(*Config)) *env {
	t.Helper()
	dir := t.TempDir()
	st, err := inbox.OpenStore(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	e := inbox.NotifyCreated{EventID: "e1", Type: "notify.created", Version: 1, OccurredAt: time.Now(), TraceID: "t"}
	e.Data.NotificationID, e.Data.GithubEvent, e.Data.Repo, e.Data.SHA = "n-1", "commit", "acme/shop", sha
	e.Data.Artifact = &inbox.Artifact{Kind: "build-from-repo", Ref: "acme/shop@" + sha}
	st.Apply(e)
	if v == nil {
		v, _ = auth.NewFakeTokenVerifier(secretTok + "=u1:user,tok-admin=a1:admin")
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	env := &env{store: st, logs: &bytes.Buffer{}, clock: &now, dir: dir}
	cfg := Config{Store: st, Verifier: v, AllowedOrigins: []string{"https://app.example"}, RateRPS: 1000, RateBurst: 1000,
		Now: func() time.Time { return *env.clock }, Logger: log.New(env.logs, "", 0)}
	if mod != nil {
		mod(&cfg)
	}
	h, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	env.h = h
	return env
}

func (e *env) do(method, path, token, body string, hdr ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = "192.0.2.1:1234"
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if method == http.MethodPost {
		r.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

func TestListAndFilter(t *testing.T) {
	e := newEnv(t, nil, nil)
	for _, tc := range []struct {
		path string
		code int
		body string
	}{
		{"/notifications", 200, `"id":"n-1"`},
		{"/notifications?state=pending", 200, `"state":"pending"`},
		{"/notifications?state=confirmed", 200, `[]`},
		{"/notifications?state=bogus", 400, `invalid_request`},
		{"/notifications?state=", 400, `invalid_request`},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := e.do("GET", tc.path, secretTok, "")
			if w.Code != tc.code || !strings.Contains(w.Body.String(), tc.body) {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
		})
	}
}

func TestAuthFailClosed(t *testing.T) {
	e := newEnv(t, nil, nil)
	for name, h := range map[string][]string{
		"sin-token": nil, "token-invalido": {"Authorization", "Bearer nope"},
		"basic": {"Authorization", "Basic dG9rOnRvaw=="}, "bearer-vacio": {"Authorization", "Bearer"},
	} {
		t.Run(name, func(t *testing.T) {
			if w := e.do("GET", "/notifications", "", "", h...); w.Code != 401 {
				t.Fatalf("GET %d", w.Code)
			}
			if w := e.do("POST", "/notifications/n-1/confirm", "", `{"flows":["f"]}`, h...); w.Code != 401 {
				t.Fatalf("POST %d", w.Code)
			}
		})
	}
	if len(e.store.List(inbox.StateConfirmed)) != 0 {
		t.Fatal("una peticion no autenticada confirmo algo")
	}
}

type failVerifier struct{}

func (failVerifier) Verify(context.Context, string) (auth.Principal, error) {
	return auth.Principal{}, errors.New("backend caido " + secretTok)
}

type panicVerifier struct{}

func (panicVerifier) Verify(_ context.Context, b string) (auth.Principal, error) { panic("boom " + b) }

func TestVerifierErrorIs401NoLeak(t *testing.T) {
	e := newEnv(t, failVerifier{}, nil)
	w := e.do("GET", "/notifications", secretTok, "")
	if w.Code != 401 || strings.Contains(w.Body.String(), secretTok) || strings.Contains(e.logs.String(), secretTok) {
		t.Fatalf("%d %s | %s", w.Code, w.Body, e.logs)
	}
}

func TestVerifierPanicIs500NoLeak(t *testing.T) {
	e := newEnv(t, panicVerifier{}, nil)
	w := e.do("GET", "/notifications", secretTok, "")
	if w.Code != 500 || strings.Contains(w.Body.String(), secretTok) || strings.Contains(e.logs.String(), secretTok) {
		t.Fatalf("%d %s | %s", w.Code, w.Body, e.logs)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("el 500 por panic debe llevar security headers")
	}
}

func TestConfirmFlow(t *testing.T) {
	e := newEnv(t, nil, nil)
	w := e.do("POST", "/notifications/n-1/confirm", secretTok, `{"flows":["checkout"],"confirmed_by":"mallory"}`)
	if w.Code != 201 || !strings.Contains(w.Body.String(), `"confirmed_by":"u1"`) || strings.Contains(w.Body.String(), "mallory") {
		t.Fatalf("confirmed_by debe venir del token: %d %s", w.Code, w.Body)
	}
	if w := e.do("POST", "/notifications/n-1/confirm", secretTok, `{"flows":["checkout"]}`); w.Code != 409 {
		t.Fatalf("doble confirmacion: %d", w.Code)
	}
	if w := e.do("POST", "/notifications/nope/confirm", secretTok, `{"flows":["x"]}`); w.Code != 404 {
		t.Fatalf("inexistente: %d", w.Code)
	}
	if w := e.do("GET", "/notifications?state=confirmed", secretTok, ""); !strings.Contains(w.Body.String(), `"n-1"`) {
		t.Fatalf("lista confirmed: %s", w.Body)
	}
}

func TestConfirmBadBodies(t *testing.T) {
	e := newEnv(t, nil, nil)
	big := `{"flows":["` + strings.Repeat("x", MaxBodyBytes) + `"]}`
	for name, tc := range map[string]struct {
		body string
		code int
	}{
		"flows-ausente": {`{}`, 400}, "flows-vacio": {`{"flows":[]}`, 400}, "flows-null": {`{"flows":null}`, 400},
		"elemento-vacio": {`{"flows":["a",""]}`, 400}, "elemento-blanco": {`{"flows":["  "]}`, 400},
		"no-json": {`{`, 400}, "tipo-erroneo": {`{"flows":"a"}`, 400}, "basura-final": {`{"flows":["a"]} x`, 400},
		"demasiado-grande": {big, 413},
	} {
		t.Run(name, func(t *testing.T) {
			if w := e.do("POST", "/notifications/n-1/confirm", secretTok, tc.body); w.Code != tc.code {
				t.Fatalf("%d (esperado %d)", w.Code, tc.code)
			}
		})
	}
	if w := e.do("POST", "/notifications/n-1/confirm", secretTok, `{"flows":["a"]}`, "Content-Type", "text/plain"); w.Code != 415 {
		t.Fatalf("content-type: %d", w.Code)
	}
	if len(e.store.List(inbox.StateConfirmed)) != 0 {
		t.Fatal("un cuerpo invalido confirmo algo")
	}
}

func TestSecurityHeadersOnEveryStatus(t *testing.T) {
	e := newEnv(t, nil, nil)
	lim := newEnv(t, nil, func(c *Config) { c.RateRPS, c.RateBurst = 1, 1 })
	lim.do("GET", "/notifications", secretTok, "")
	pn := newEnv(t, panicVerifier{}, nil)
	want := []string{"Content-Security-Policy", "Strict-Transport-Security", "X-Content-Type-Options", "Referrer-Policy", "Cache-Control"}
	big := `{"flows":["` + strings.Repeat("x", MaxBodyBytes) + `"]}`
	for name, tc := range map[string]struct {
		w    *httptest.ResponseRecorder
		code int
	}{
		"200": {e.do("GET", "/notifications", secretTok, ""), 200},
		"201": {e.do("POST", "/notifications/n-1/confirm", secretTok, `{"flows":["a"]}`), 201},
		"400": {e.do("GET", "/notifications?state=x", secretTok, ""), 400},
		"401": {e.do("GET", "/notifications", "", ""), 401},
		"404": {e.do("GET", "/otra", "", ""), 404},
		"405": {e.do("DELETE", "/notifications", secretTok, ""), 405},
		"409": {e.do("POST", "/notifications/n-1/confirm", secretTok, `{"flows":["a"]}`), 409},
		"413": {e.do("POST", "/notifications/n-1/confirm", secretTok, big), 413},
		"415": {e.do("POST", "/notifications/n-1/confirm", secretTok, `{}`, "Content-Type", "text/plain"), 415},
		"429": {lim.do("GET", "/notifications", secretTok, ""), 429},
		"500": {pn.do("GET", "/notifications", secretTok, ""), 500},
		"204": {e.do("OPTIONS", "/notifications", "", "", "Origin", "https://app.example"), 204},
	} {
		t.Run(name, func(t *testing.T) {
			if tc.w.Code != tc.code {
				t.Fatalf("codigo %d", tc.w.Code)
			}
			for _, h := range want {
				if tc.w.Header().Get(h) == "" {
					t.Fatalf("falta %s", h)
				}
			}
			if tc.w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(tc.w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
				t.Fatal("valor de header incorrecto")
			}
		})
	}
}

func TestCORS(t *testing.T) {
	e := newEnv(t, nil, nil)
	if got := e.do("GET", "/notifications", secretTok, "", "Origin", "https://app.example").Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Fatalf("origen listado: %q", got)
	}
	if got := e.do("GET", "/notifications", secretTok, "", "Origin", "https://evil.example").Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("origen no listado: %q", got)
	}
	w := e.do("OPTIONS", "/notifications/n-1/confirm", "", "", "Origin", "https://evil.example", "Access-Control-Request-Method", "POST")
	if w.Header().Get("Access-Control-Allow-Origin") != "" || w.Header().Get("Access-Control-Allow-Methods") != "" {
		t.Fatal("preflight de origen no listado no debe recibir permisos")
	}
	if _, err := New(Config{Store: e.store, Verifier: failVerifier{}, AllowedOrigins: []string{"*"}}); err == nil {
		t.Fatal("'*' debe rechazarse")
	}
	none := newEnv(t, nil, func(c *Config) { c.AllowedOrigins = nil })
	if got := none.do("GET", "/notifications", secretTok, "", "Origin", "https://app.example").Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatal("lista vacia = ningun origen")
	}
}

func TestRateLimitWithInjectedClock(t *testing.T) {
	e := newEnv(t, nil, func(c *Config) { c.RateRPS, c.RateBurst = 2, 3 })
	for i := 0; i < 3; i++ {
		if w := e.do("GET", "/notifications", secretTok, ""); w.Code != 200 {
			t.Fatalf("peticion %d: %d", i, w.Code)
		}
	}
	w := e.do("GET", "/notifications", secretTok, "")
	if w.Code != 429 || w.Header().Get("Retry-After") != "1" {
		t.Fatalf("%d retry-after=%q (esperado 1: ceil(1/2))", w.Code, w.Header().Get("Retry-After"))
	}
	*e.clock = e.clock.Add(time.Second) // 2 tokens
	if w := e.do("GET", "/notifications", secretTok, ""); w.Code != 200 {
		t.Fatalf("tras refill: %d", w.Code)
	}
}

func TestRateLimitPerIPAndProxy(t *testing.T) {
	e := newEnv(t, nil, func(c *Config) { c.RateRPS, c.RateBurst = 1, 1 })
	e.do("GET", "/notifications", secretTok, "")
	// Sin TrustProxy, X-Forwarded-For se ignora: no sirve para evadir el limite.
	if w := e.do("GET", "/notifications", secretTok, "", "X-Forwarded-For", "203.0.113.9"); w.Code != 429 {
		t.Fatalf("XFF no confiable debia ignorarse: %d", w.Code)
	}
	p := newEnv(t, nil, func(c *Config) { c.RateRPS, c.RateBurst, c.TrustProxy = 1, 1, true })
	p.do("GET", "/notifications", secretTok, "", "X-Forwarded-For", "203.0.113.9")
	if w := p.do("GET", "/notifications", secretTok, "", "X-Forwarded-For", "203.0.113.9"); w.Code != 429 {
		t.Fatalf("misma IP: %d", w.Code)
	}
	if w := p.do("GET", "/notifications", secretTok, "", "X-Forwarded-For", "203.0.113.10"); w.Code != 200 {
		t.Fatalf("otra IP: %d", w.Code)
	}
}

func TestTokenNeverInLogs(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.do("GET", "/notifications", secretTok, "")
	e.do("POST", "/notifications/n-1/confirm", secretTok, `{`)
	e.do("GET", "/notifications", secretTok+"x", "")
	pn := newEnv(t, panicVerifier{}, nil)
	pn.do("GET", "/notifications", secretTok, "")
	if strings.Contains(e.logs.String()+pn.logs.String(), "SECRETO") {
		t.Fatalf("token en logs: %s %s", e.logs, pn.logs)
	}
}

func TestConcurrentHTTPConfirm(t *testing.T) {
	e := newEnv(t, nil, nil)
	codes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		go func() { codes <- e.do("POST", "/notifications/n-1/confirm", secretTok, `{"flows":["a"]}`).Code }()
	}
	a, b := <-codes, <-codes
	if !(a == 201 && b == 409 || a == 409 && b == 201) {
		t.Fatalf("codigos %d %d", a, b)
	}
}

func TestRetryAfterExactAndDecreasing(t *testing.T) {
	e := newEnv(t, nil, func(c *Config) { c.RateRPS, c.RateBurst = 0.25, 1 }) // 1 token cada 4 s
	e.do("GET", "/notifications", secretTok, "")
	var got []string
	for i := 0; i < 3; i++ {
		w := e.do("GET", "/notifications", secretTok, "")
		if w.Code != 429 {
			t.Fatalf("%d", w.Code)
		}
		got = append(got, w.Header().Get("Retry-After"))
		*e.clock = e.clock.Add(1 * time.Second)
	}
	if strings.Join(got, ",") != "4,3,2" {
		t.Fatalf("Retry-After = %v, esperado 4,3,2", got)
	}
}

func TestDuplicateAuthorizationHeaderIs401(t *testing.T) {
	e := newEnv(t, nil, nil)
	for _, order := range [][]string{{"Bearer nope", "Bearer " + secretTok}, {"Bearer " + secretTok, "Bearer nope"}, {"Bearer " + secretTok, "Bearer " + secretTok}} {
		r := httptest.NewRequest("GET", "/notifications", nil)
		r.RemoteAddr = "192.0.2.1:1"
		r.Header["Authorization"] = order
		w := httptest.NewRecorder()
		e.h.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatalf("%v -> %d", order, w.Code)
		}
	}
}

func TestRepeatedStateParamIs400(t *testing.T) {
	e := newEnv(t, nil, nil)
	for _, q := range []string{"state=pending&state=bogus", "state=bogus&state=pending", "state=pending&state=pending"} {
		if w := e.do("GET", "/notifications?"+q, secretTok, ""); w.Code != 400 {
			t.Fatalf("%s -> %d", q, w.Code)
		}
	}
}

func TestBodyLimitLiteral64KiB(t *testing.T) {
	e := newEnv(t, nil, nil)
	pad := func(total int) string { // {"flows":["xxx"]} con tamano exacto total
		const base = len(`{"flows":[""]}`)
		return `{"flows":["` + strings.Repeat("x", total-base) + `"]}`
	}
	if b := pad(65536); len(b) != 65536 {
		t.Fatalf("len %d", len(b))
	}
	if w := e.do("POST", "/notifications/nope/confirm", secretTok, pad(65536)); w.Code != 404 {
		t.Fatalf("65536 debe pasar a validacion (404 por id): %d", w.Code)
	}
	if w := e.do("POST", "/notifications/nope/confirm", secretTok, pad(65537)); w.Code != 413 {
		t.Fatalf("65537 debe ser 413: %d", w.Code)
	}
}

func TestRetryAfterRoundsUp(t *testing.T) {
	// 0.4 rps, ráfaga 1: faltan 2,5 s -> Ceil=3 (Floor daría 2; con rps=3 el
	// minimo de 1 s enmascararia la diferencia).
	e := newEnv(t, nil, func(c *Config) { c.RateRPS, c.RateBurst = 0.4, 1 })
	e.do("GET", "/notifications", secretTok, "")
	if got := e.do("GET", "/notifications", secretTok, "").Header().Get("Retry-After"); got != "3" {
		t.Fatalf("Retry-After=%q, esperado 3", got)
	}
}

func TestRoutePatternBoundsCardinality(t *testing.T) {
	for p, want := range map[string]string{
		"/notifications": RouteList, "/notifications/n-1/confirm": RouteConfirm, "/notifications/zzz/confirm": RouteConfirm,
		"/notifications//confirm": "unmatched", "/notifications/a/b/confirm": "unmatched", "/notifications/n-1": "unmatched",
		"/healthz": "unmatched", "/": "unmatched",
	} {
		if got := RoutePattern(httptest.NewRequest("GET", p, nil)); got != want {
			t.Errorf("%s -> %s, esperado %s", p, got, want)
		}
	}
}

// Sin Config.Slog (modo clásico) el error interno sigue saliendo por Logger, sin token ni cuerpo.
func TestInternalErrorFallsBackToLoggerWithoutSlog(t *testing.T) {
	e := newEnv(t, nil, nil)
	if err := os.RemoveAll(e.dir); err != nil {
		t.Fatal(err)
	}
	w := e.do("POST", "/notifications/n-1/confirm", secretTok, `{"flows":["PII-FLUJO-XYZ"]}`, "Content-Type", "application/json")
	if w.Code != 500 {
		t.Fatalf("esperado 500: %d %s", w.Code, w.Body)
	}
	got := e.logs.String()
	if !strings.Contains(got, "confirmando la notificación") || !strings.Contains(got, "n-1") {
		t.Errorf("el error interno debe registrarse por Logger: %q", got)
	}
	if strings.Contains(got, secretTok) || strings.Contains(got, "PII-FLUJO-XYZ") {
		t.Errorf("el log no debe llevar token ni cuerpo: %q", got)
	}
}
