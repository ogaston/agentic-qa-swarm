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

	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/obs"
)

func newTestHandler(t *testing.T, secret string) (h http.Handler, dataDir, events string, logs *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	dataDir, events, logs = filepath.Join(root, "d"), filepath.Join(root, "e.jsonl"), &bytes.Buffer{}
	h, err := newHandler(obs.NewLogger(logs, serviceName, slog.LevelDebug), []byte(secret), dataDir, events, "")
	if err != nil {
		t.Fatal(err)
	}
	return h, dataDir, events, logs
}

func get(h http.Handler, path string) (int, string) {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec.Code, rec.Body.String()
}

// La cadena real (la misma que arma run) sirve las sondas sin token y /readyz sale de 503 al
// romper cada dependencia real: directorio de datos, outbox y (por inyección) el secreto.
func TestOpsEndpointsRealChain(t *testing.T) {
	h, dataDir, events, _ := newTestHandler(t, "s3cret")
	for _, p := range []string{"/healthz", "/readyz", "/metrics"} {
		if code, _ := get(h, p); code != 200 {
			t.Fatalf("%s: %d", p, code)
		}
	}
	if code, body := get(h, "/healthz"); code != 200 || !strings.Contains(body, `"ok"`) {
		t.Fatalf("healthz: %s", body)
	}

	// 1) outbox convertido en directorio
	if err := os.Mkdir(events, 0o750); err != nil {
		t.Fatal(err)
	}
	code, body := get(h, "/readyz")
	var r struct {
		Status string
		Checks map[string]string
	}
	_ = json.Unmarshal([]byte(body), &r)
	if code != 503 || r.Checks["outbox"] != "fail" || r.Checks["data_dir"] != "ok" || r.Checks["webhook_secret"] != "ok" {
		t.Fatalf("outbox roto: %d %s", code, body)
	}
	if err := os.Remove(events); err != nil {
		t.Fatal(err)
	}
	if code, _ := get(h, "/readyz"); code != 200 {
		t.Fatalf("se recupera al arreglar el outbox: %d", code)
	}
	// 2) directorio de datos desaparecido
	if err := os.RemoveAll(dataDir); err != nil {
		t.Fatal(err)
	}
	code, body = get(h, "/readyz")
	r.Checks = nil
	_ = json.Unmarshal([]byte(body), &r)
	if code != 503 || r.Checks["data_dir"] != "fail" || r.Checks["outbox"] != "ok" {
		t.Fatalf("data_dir roto: %d %s", code, body)
	}
}

// El secreto vacío es un invariante de arranque: la cadena ni se construye (el chequeo
// webhook_secret de /readyz es defensa en profundidad, probado en internal/obs).
func TestEmptySecretNeverServes(t *testing.T) {
	root := t.TempDir()
	if _, err := newHandler(slog.New(slog.DiscardHandler), nil, filepath.Join(root, "d"), filepath.Join(root, "e"), ""); err == nil {
		t.Fatal("sin secreto no debe haber handler")
	}
}
