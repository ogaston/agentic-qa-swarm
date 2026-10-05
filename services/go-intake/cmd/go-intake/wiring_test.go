package main

import (
	"bytes"
	"encoding/json"
	"log"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/githubsig"
)

// El cableado de newHandler (serviceName, Wrap.Service, Route, NewHTTPMetrics) fija las etiquetas de las
// que depende el dashboard (`service=~"go-intake|ui-api"`, route por patrón, in_flight por servicio).
func TestNewHandlerWiringLabels(t *testing.T) {
	h, _, _, logs := newTestHandler(t, "s3cret")
	body := []byte(`{}`)
	r := httptest.NewRequest("POST", "/webhooks/github", bytes.NewReader(body))
	r.Header.Set("X-Hub-Signature-256", "sha256=00")
	h.ServeHTTP(httptest.NewRecorder(), r)
	ok := httptest.NewRequest("POST", "/webhooks/github", bytes.NewReader(body))
	ok.Header.Set("X-Hub-Signature-256", githubsig.Sign([]byte("s3cret"), body))
	ok.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(httptest.NewRecorder(), ok)
	_, metrics := get(h, "/metrics")
	for _, want := range []string{
		`aqs_http_requests_total{code="401",method="POST",route="/webhooks/github",service="go-intake"} 1`,
		`aqs_http_requests_total{code="400",method="POST",route="/webhooks/github",service="go-intake"} 1`,
		`aqs_http_in_flight{service="go-intake"} 1`, // solo el propio scrape
		`aqs_intake_webhook_rejected_total{reason="invalid_signature"} 1`,
	} {
		if !strings.Contains(metrics, want+"\n") {
			t.Errorf("falta %q en:\n%s", want, grepLines(metrics, "aqs_http"))
		}
	}
	if !regexp.MustCompile(`aqs_http_request_duration_seconds_count\{method="POST",route="/webhooks/github",service="go-intake"\} 2`).MatchString(metrics) {
		t.Error("el histograma debe llevar service y route del webhook")
	}
	if strings.Contains(metrics, `route="unmatched",service="go-intake"} 2`) || strings.Contains(metrics, `service="go-identity"`) {
		t.Error("etiquetas inesperadas")
	}
	var seen int
	for _, ln := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			t.Fatal(err)
		}
		if m["service"] != "go-intake" {
			t.Errorf("service del log: %s", ln)
		}
		if m["message"] == "request" && m["route"] == "/webhooks/github" {
			seen++
		}
	}
	if seen != 2 {
		t.Errorf("las dos líneas de acceso del webhook deben llevar route=/webhooks/github: %d\n%s", seen, logs)
	}
	// solo los cinco motivos declarados: ninguna serie reason="other" ni extra
	reasons := regexp.MustCompile(`(?m)^aqs_intake_webhook_rejected_total\{reason="([^"]+)"\}`).FindAllStringSubmatch(metrics, -1)
	got := map[string]bool{}
	for _, m := range reasons {
		got[m[1]] = true
	}
	for _, r := range []string{"invalid_signature", "unsupported_event", "unresolvable_artifact", "bad_request", "too_large"} {
		delete(got, r)
	}
	if len(reasons) != 5 || len(got) != 0 {
		t.Errorf("deben existir exactamente los 5 motivos declarados: %v", reasons)
	}
}

func restoreLogging(t *testing.T) {
	t.Helper()
	old := slog.Default()
	t.Cleanup(func() { slog.SetDefault(old); log.SetOutput(os.Stderr); log.SetFlags(log.LstdFlags) })
}

// LOG_LEVEL del entorno gobierna el logger del servicio, y slog.SetDefault lleva los log.Printf
// residuales (p. ej. del almacén) al mismo JSON.
func TestNewLogUsesLogLevelFromEnv(t *testing.T) {
	restoreLogging(t)
	for lvl, want := range map[string][]string{"debug": {"d", "i", "w", "e"}, "info": {"i", "w", "e"}, "warn": {"w", "e"}, "error": {"e"}, "": {"i", "w", "e"}, "ERROR": {"e"}} {
		t.Setenv("LOG_LEVEL", lvl)
		var buf bytes.Buffer
		l := newLog(&buf)
		l.Debug("d")
		l.Info("i")
		l.Warn("w")
		l.Error("e")
		var got []string
		for _, ln := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
			var m map[string]any
			_ = json.Unmarshal([]byte(ln), &m)
			if s, _ := m["message"].(string); s != "" {
				got = append(got, s)
			}
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("LOG_LEVEL=%q: salieron %v, esperado %v", lvl, got, want)
		}
	}
}

func TestNewLogSetsDefaultSoStdLogIsJSON(t *testing.T) {
	restoreLogging(t)
	t.Setenv("LOG_LEVEL", "")
	var buf bytes.Buffer
	newLog(&buf)
	log.Printf("store: linea %d ilegible, se ignora", 7)
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil {
		t.Fatalf("un log.Printf residual debe salir como JSON: %q", buf.String())
	}
	if m["service"] != "go-intake" || !strings.Contains(m["message"].(string), "linea 7 ilegible") || m["level"] != "info" {
		t.Errorf("línea residual: %v", m)
	}
}

// Una línea ilegible del almacén al abrir sale en JSON por el logger predeterminado (camino real de run).
func TestStoreUnreadableLineGoesThroughJSONLogger(t *testing.T) {
	restoreLogging(t)
	t.Setenv("LOG_LEVEL", "")
	var buf bytes.Buffer
	lg := newLog(&buf)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "d"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "d", "notifications.jsonl"), []byte("basura\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := newHandler(lg, []byte("s"), filepath.Join(root, "d"), filepath.Join(root, "e"), ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"message":"store: linea 1 ilegible, se ignora"`) {
		t.Errorf("falta la línea JSON del almacén: %q", buf.String())
	}
}

func grepLines(s, sub string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, sub) {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
