package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/ui-api/inbox"
	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/auth"
	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/httpapi"
	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/obs"
)

const tok = "tok-ops-secreto"

type chain struct {
	h       http.Handler
	store   *inbox.Store
	dataDir string
	events  string
	logs    *bytes.Buffer
}

// newChain arma la MISMA cadena que run() (newHandler), con un limitador diminuto.
func newChain(t *testing.T, burst int) *chain {
	t.Helper()
	root := t.TempDir()
	c := &chain{dataDir: filepath.Join(root, "d"), events: filepath.Join(root, "e.jsonl"), logs: &bytes.Buffer{}}
	var err error
	if c.store, err = inbox.OpenStore(c.dataDir, nil); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"n-1", "n-2"} {
		e := inbox.NotifyCreated{EventID: "e-" + id, Type: "notify.created", Version: 1, OccurredAt: time.Now(), TraceID: "t"}
		e.Data.NotificationID, e.Data.GithubEvent, e.Data.Repo = id, "commit", "acme/shop"
		e.Data.SHA = strings.Repeat("a", 40)
		e.Data.Artifact = &inbox.Artifact{Kind: "build-from-repo", Ref: "acme/shop@" + e.Data.SHA}
		c.store.Apply(e)
	}
	v, err := auth.NewFakeTokenVerifier(tok + "=u1:user")
	if err != nil {
		t.Fatal(err)
	}
	c.h, err = newHandler(obs.NewLogger(c.logs, serviceName, slog.LevelDebug),
		httpapi.Config{Store: c.store, Verifier: v, RateRPS: 0.001, RateBurst: burst}, c.dataDir, c.events)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (c *chain) do(method, path, body string, hdr ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, r)
	return rec
}

func (c *chain) auth(method, path, body string) *httptest.ResponseRecorder {
	return c.do(method, path, body, "Authorization", "Bearer "+tok, "Content-Type", "application/json")
}

// Las sondas no cuentan para el limitador por IP (ni consumen su cupo) y no necesitan token,
// pero las rutas de negocio con el mismo limitador sí devuelven 429 (el limitador está activo).
func TestOpsEndpointsRateLimitExemptRealChain(t *testing.T) {
	c := newChain(t, 3)
	for i := 0; i < 60; i++ {
		for _, p := range []string{"/healthz", "/readyz", "/metrics"} {
			if rec := c.do("GET", p, ""); rec.Code != 200 {
				t.Fatalf("%s #%d: %d", p, i, rec.Code)
			}
		}
	}
	for i := 1; i <= 3; i++ { // el cupo (3) sigue intacto tras 180 sondas
		if rec := c.auth("GET", "/notifications", ""); rec.Code != 200 {
			t.Fatalf("petición de negocio %d: %d", i, rec.Code)
		}
	}
	if rec := c.auth("GET", "/notifications", ""); rec.Code != 429 {
		t.Fatalf("la 4ª petición de negocio debía ser 429: %d", rec.Code)
	}
	if rec := c.do("GET", "/healthz", ""); rec.Code != 200 {
		t.Fatalf("con la IP limitada, /healthz sigue 200: %d", rec.Code)
	}
}

func TestOpsEndpointsMetricsNoBusinessData(t *testing.T) {
	c := newChain(t, 50)
	if rec := c.auth("POST", "/notifications/n-1/confirm", `{"flows":["f1"]}`); rec.Code != 201 {
		t.Fatalf("confirmar: %d %s", rec.Code, rec.Body)
	}
	c.auth("GET", "/notifications", "")
	c.auth("POST", "/notifications/n-no-existe/confirm", `{"flows":["f1"]}`)
	out := c.do("GET", "/metrics", "").Body.String()
	for _, s := range []string{"notification_id", "repo=", "acme/shop", "n-1", "n-2", "n-no-existe", tok, "u1"} {
		if strings.Contains(out, s) {
			t.Errorf("/metrics no debe contener %q", s)
		}
	}
	for _, w := range []string{
		`aqs_http_requests_total{code="201",method="POST",route="/notifications/{id}/confirm",service="ui-api"} 1`,
		`aqs_http_requests_total{code="404",method="POST",route="/notifications/{id}/confirm",service="ui-api"} 1`,
		`aqs_http_requests_total{code="200",method="GET",route="/notifications",service="ui-api"} 1`,
		"aqs_inbox_confirmations_total 1",
		`aqs_inbox_notifications{state="pending"} 1`, `aqs_inbox_notifications{state="confirmed"} 1`, `aqs_inbox_notifications{state="rejected"} 0`,
	} {
		if !strings.Contains(out, w+"\n") {
			t.Errorf("falta la serie %q", w)
		}
	}
	// El 409 (segunda confirmación) no cuenta como confirmación nueva.
	if rec := c.auth("POST", "/notifications/n-1/confirm", `{"flows":["f1"]}`); rec.Code != 409 {
		t.Fatalf("segunda confirmación: %d", rec.Code)
	}
	if out = c.do("GET", "/metrics", "").Body.String(); !strings.Contains(out, "aqs_inbox_confirmations_total 1\n") {
		t.Error("el 409 no debe incrementar aqs_inbox_confirmations_total")
	}
}

func TestOpsEndpointsInitialSeriesAreZero(t *testing.T) {
	out := newChain(t, 5).do("GET", "/metrics", "").Body.String()
	for _, w := range []string{"aqs_inbox_confirmations_total 0", `aqs_inbox_notifications{state="pending"} 2`,
		`aqs_inbox_notifications{state="confirmed"} 0`, `aqs_inbox_notifications{state="rejected"} 0`} {
		if !strings.Contains(out, w+"\n") {
			t.Errorf("falta %q", w)
		}
	}
}

func TestOpsEndpointsRouteLabelIsPattern(t *testing.T) {
	c := newChain(t, 100)
	for _, p := range []string{"/a", "/b", "/notifications/x/y/confirm", "/notifications//confirm", "/notifications/n-9/otra"} {
		c.do("GET", p, "")
	}
	c.auth("POST", "/notifications/n-777/confirm", `{"flows":["f"]}`)
	out := c.do("GET", "/metrics", "").Body.String()
	if strings.Contains(out, `route="/a"`) || strings.Contains(out, "n-777") || strings.Contains(out, "/x/y") {
		t.Error("la ruta cruda no debe ser etiqueta")
	}
	if !strings.Contains(out, `route="unmatched"`) || !strings.Contains(out, `route="/notifications/{id}/confirm"`) {
		t.Error("faltan las etiquetas de patrón")
	}
}

// El readiness real de ui-api sale de 503 con cada dependencia rota, y no llega a la aplicación.
func TestOpsEndpointsReadyzRealFailures(t *testing.T) {
	c := newChain(t, 100)
	state := func() (int, map[string]string) {
		rec := c.do("GET", "/readyz", "")
		var r struct{ Checks map[string]string }
		_ = json.Unmarshal(rec.Body.Bytes(), &r)
		return rec.Code, r.Checks
	}
	if code, _ := state(); code != 200 {
		t.Fatalf("sano: %d", code)
	}
	if err := os.Mkdir(c.events, 0o750); err != nil { // el archivo de eventos pasa a ser un directorio
		t.Fatal(err)
	}
	if code, ch := state(); code != 503 || ch["events_file"] != "fail" || ch["data_dir"] != "ok" || ch["token_verifier"] != "ok" {
		t.Fatalf("events_file roto: %d %v", code, ch)
	}
	_ = os.Remove(c.events)
	if err := os.RemoveAll(c.dataDir); err != nil {
		t.Fatal(err)
	}
	if code, ch := state(); code != 503 || ch["data_dir"] != "fail" || ch["events_file"] != "ok" {
		t.Fatalf("data_dir roto: %d %v", code, ch)
	}
}

// La instrumentación no cambia las decisiones: sin token 401, y las cabeceras de seguridad de
// la API siguen en las rutas de negocio (las sondas quedan fuera de secure()).
func TestOpsEndpointsKeepBusinessBehaviour(t *testing.T) {
	c := newChain(t, 100)
	rec := c.do("GET", "/notifications", "", "X-Request-Id", "req-12345678")
	if rec.Code != 401 || rec.Header().Get("X-Request-Id") != "req-12345678" || rec.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("401 con id y cabeceras de seguridad: %d %v", rec.Code, rec.Header())
	}
	if rec := c.do("GET", "/healthz", ""); rec.Header().Get("Content-Security-Policy") != "" {
		t.Error("excepción documentada: las sondas no llevan las cabeceras de la API")
	}
	if rec := c.do("GET", "/no-existe", ""); rec.Code != 404 {
		t.Fatalf("ruta desconocida: %d", rec.Code)
	}
	if !strings.Contains(c.logs.String(), `"request_id":"req-12345678"`) || strings.Contains(c.logs.String(), tok) {
		t.Error("el log lleva el request_id y nunca el token")
	}
}
