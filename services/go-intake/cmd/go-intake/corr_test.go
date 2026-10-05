package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/githubsig"
)

func post(t *testing.T, h interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, event, delivery string, body []byte, sig string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", "/webhooks/github", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-GitHub-Event", event)
	if delivery != "" {
		r.Header.Set("X-GitHub-Delivery", delivery)
	}
	if sig == "" {
		sig = githubsig.Sign([]byte("s3cret"), body)
	}
	r.Header.Set("X-Hub-Signature-256", sig)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// Barrido (F-01 de la ronda 1): con la cadena real, cada camino de la petición (aceptada, cada
// rechazo, 503 de almacén y de publicación) deja líneas que llevan request_id y trace_id no vacíos,
// y el acceso de un 5xx sale en warn.
func TestEveryRequestLogLineCarriesIDs(t *testing.T) {
	h, dataDir, events, logs := newTestHandler(t, "s3cret")
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "github", name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	pr, push := read("pull-request-opened.json"), read("push-branch.json")
	big := append(append([]byte(`{"x":"`), bytes.Repeat([]byte("a"), 1<<20)...), []byte(`"}`)...)
	post(t, h, "pull_request", "d1", pr, "")                             // 202
	post(t, h, "pull_request", "d2", pr, "sha256=00")                    // firma
	post(t, h, "ping", "d3", read("ping.json"), "")                      // evento no soportado
	post(t, h, "pull_request", "d4", read("pull-request-fork.json"), "") // artefacto irresoluble
	post(t, h, "push", "d5", []byte(`{"a":`), "")                        // JSON roto
	post(t, h, "push", "", push, "")                                     // sin delivery
	post(t, h, "push", "d6", big, "")                                    // demasiado grande
	// 503 de publicación: el outbox pasa a ser un directorio
	if err := os.Remove(events); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(events, 0o750); err != nil {
		t.Fatal(err)
	}
	if rec := post(t, h, "push", "d7", push, ""); rec.Code != 503 {
		t.Fatalf("publicación rota: %d", rec.Code)
	}
	// 503 de almacén: el directorio de datos desaparece
	if err := os.RemoveAll(dataDir); err != nil {
		t.Fatal(err)
	}
	if rec := post(t, h, "push", "d8", read("push-tag.json"), ""); rec.Code != 503 {
		t.Fatalf("almacén roto: %d", rec.Code)
	}

	n, warn5xx := 0, 0
	for _, ln := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			t.Fatalf("no es JSON: %q", ln)
		}
		n++
		if m["request_id"] == "" || m["request_id"] == nil || m["trace_id"] == "" || m["trace_id"] == nil {
			t.Errorf("línea dentro de una petición sin ids: %s", ln)
		}
		if m["message"] == "request" && m["status"] == float64(503) {
			if m["level"] != "warn" {
				t.Errorf("el acceso de un 5xx debe ser warn: %s", ln)
			}
			warn5xx++
		}
	}
	if n < 18 || warn5xx != 2 {
		t.Errorf("barrido incompleto: %d líneas, %d accesos 5xx", n, warn5xx)
	}
	if !strings.Contains(logs.String(), `"message":"no se pudo publicar notify.created"`) || !strings.Contains(logs.String(), `"message":"no se pudo persistir"`) {
		t.Error("faltan las líneas de error de publicación y de almacén")
	}
}
