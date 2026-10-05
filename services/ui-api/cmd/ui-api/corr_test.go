package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/ui-api/inbox"
	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/auth"
	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/httpapi"
	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/obs"
)

func logLines(t *testing.T, b *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, ln := range strings.Split(strings.TrimSpace(b.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			t.Fatalf("línea que no es JSON: %q", ln)
		}
		out = append(out, m)
	}
	return out
}

func find(lines []map[string]any, msgPrefix string) []map[string]any {
	var out []map[string]any
	for _, m := range lines {
		if s, _ := m["message"].(string); strings.HasPrefix(s, msgPrefix) {
			out = append(out, m)
		}
	}
	return out
}

func traceOf(rec *httptest.ResponseRecorder) string {
	p := strings.Split(rec.Header().Get("traceparent"), "-")
	if len(p) != 4 {
		return ""
	}
	return p[1]
}

// El error interno de una confirmación (almacén roto) se registra DENTRO de la petición: debe
// llevar el request_id y el trace_id de esa petición, no vacíos (F-01 de la ronda 1).
func TestOpsEndpointsErrorLogsCarryRequestIDs(t *testing.T) {
	c := newChain(t, 100)
	if err := os.RemoveAll(c.dataDir); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/notifications/n-1/confirm", strings.NewReader(`{"flows":["f"]}`))
	r.Header.Set("Authorization", "Bearer "+tok)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Request-Id", "req-corr-12345")
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, r)
	if rec.Code != 500 {
		t.Fatalf("esperado 500 con el almacén roto: %d %s", rec.Code, rec.Body)
	}
	// sin cabecera: llevan los generados (los mismos que se devuelven al cliente)
	r2 := httptest.NewRequest("POST", "/notifications/n-2/confirm", strings.NewReader(`{"flows":["f"]}`))
	r2.Header.Set("Authorization", "Bearer "+tok)
	r2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	c.h.ServeHTTP(rec2, r2)

	lines := logLines(t, c.logs)
	errs := find(lines, "confirmando la notificación")
	if len(errs) != 2 {
		t.Fatalf("líneas de error: %d en %s", len(errs), c.logs)
	}
	for i, want := range []struct{ rid, tid string }{
		{"req-corr-12345", traceOf(rec)}, {rec2.Header().Get("X-Request-Id"), traceOf(rec2)}} {
		if want.rid == "" || want.tid == "" {
			t.Fatalf("la respuesta debe traer ids: %v", rec.Header())
		}
		if errs[i]["request_id"] != want.rid || errs[i]["trace_id"] != want.tid {
			t.Errorf("error %d: request_id=%v trace_id=%v, esperado %s / %s", i, errs[i]["request_id"], errs[i]["trace_id"], want.rid, want.tid)
		}
		if errs[i]["level"] != "warn" || errs[i]["notification_id"] == nil {
			t.Errorf("error %d mal formado: %v", i, errs[i])
		}
	}
	if strings.Contains(c.logs.String(), tok) {
		t.Error("el log no debe contener el token")
	}
	// el acceso de un 5xx sube a warn; las sondas sanas siguen en info
	acc := find(lines, "request")
	if len(acc) != 2 || acc[0]["status"] != float64(500) || acc[0]["level"] != "warn" {
		t.Errorf("el acceso de un 5xx debe salir en warn: %v", acc)
	}
	c.logs.Reset()
	c.do("GET", "/healthz", "")
	if l := logLines(t, c.logs); len(l) != 1 || l[0]["level"] != "info" {
		t.Errorf("una sonda sana sale en info: %v", l)
	}
}

type panicVerifier struct{}

func (panicVerifier) Verify(context.Context, string) (auth.Principal, error) {
	panic("valor-que-no-debe-salir " + tok)
}

// El mensaje de panic tampoco sale sin correlación, y no filtra el valor del panic.
func TestOpsEndpointsPanicLogCarriesRequestIDs(t *testing.T) {
	root := t.TempDir()
	logs := &bytes.Buffer{}
	st, err := inbox.OpenStore(root+"/d", nil)
	if err != nil {
		t.Fatal(err)
	}
	h, err := newHandler(obs.NewLogger(logs, serviceName, slog.LevelDebug),
		httpapi.Config{Store: st, Verifier: panicVerifier{}}, root+"/d", root+"/e.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/notifications", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	r.Header.Set("X-Request-Id", "req-panic-1234")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("panic -> 500: %d", rec.Code)
	}
	p := find(logLines(t, logs), "panic atendiendo")
	if len(p) != 1 || p[0]["request_id"] != "req-panic-1234" || p[0]["trace_id"] != traceOf(rec) || traceOf(rec) == "" {
		t.Fatalf("línea de panic sin correlación: %v", p)
	}
	if strings.Contains(logs.String(), "valor-que-no-debe-salir") || strings.Contains(logs.String(), tok) {
		t.Error("el valor del panic no debe llegar al log")
	}
}
