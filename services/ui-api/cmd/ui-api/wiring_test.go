package main

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/ui-api/inbox"
	"log/slog"
)

// El cableado de newHandler fija service y route de las series de las que depende el dashboard.
func TestOpsEndpointsWiringLabels(t *testing.T) {
	c := newChain(t, 100)
	c.auth("GET", "/notifications", "")
	c.do("GET", "/notifications", "")
	metrics := c.do("GET", "/metrics", "").Body.String()
	for _, want := range []string{
		`aqs_http_requests_total{code="200",method="GET",route="/notifications",service="ui-api"} 1`,
		`aqs_http_requests_total{code="401",method="GET",route="/notifications",service="ui-api"} 1`,
		`aqs_http_in_flight{service="ui-api"} 1`, // solo el propio scrape
	} {
		if !strings.Contains(metrics, want+"\n") {
			t.Errorf("falta %q en:\n%s", want, grepLines(metrics, "aqs_http"))
		}
	}
	if !regexp.MustCompile(`aqs_http_request_duration_seconds_count\{method="GET",route="/notifications",service="ui-api"\} 2`).MatchString(metrics) {
		t.Error("el histograma debe llevar service y route")
	}
	if strings.Contains(metrics, `service="go-intake"`) {
		t.Error("etiqueta service inesperada")
	}
	for _, l := range logLines(t, c.logs) {
		if l["service"] != "ui-api" {
			t.Errorf("service del log: %v", l)
		}
	}
}

func restoreLogging(t *testing.T) {
	t.Helper()
	old := slog.Default()
	t.Cleanup(func() { slog.SetDefault(old); log.SetOutput(os.Stderr); log.SetFlags(log.LstdFlags) })
}

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
	log.Printf("residual %d", 7)
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil {
		t.Fatalf("un log.Printf residual debe salir como JSON: %q", buf.String())
	}
	if m["service"] != "ui-api" || m["message"] != "residual 7" || m["level"] != "info" {
		t.Errorf("línea residual: %v", m)
	}
}

// Las líneas del suscriptor salen en JSON, a nivel info (visibles con el nivel por defecto) y sin contenido del evento.
func TestSubscriberBridgeLogsDiscardAtInfo(t *testing.T) {
	restoreLogging(t)
	t.Setenv("LOG_LEVEL", "")
	var buf bytes.Buffer
	lg := newLog(&buf)
	p := filepath.Join(t.TempDir(), "e.jsonl")
	if err := os.WriteFile(p, []byte(`{"event_id":"e1","PII-CLAVE-XYZ":1,"PII-CLAVE-XYZ":2}`+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	sub := newSubscriber(lg, p)
	if n, err := sub.Drain(func(inbox.NotifyCreated) {}); err != nil || n != 0 || sub.Discarded() != 1 {
		t.Fatalf("n=%d err=%v descartadas=%d", n, err, sub.Discarded())
	}
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil {
		t.Fatalf("la línea del suscriptor debe ser JSON: %q", buf.String())
	}
	if m["level"] != "info" || !strings.Contains(m["message"].(string), "evento descartado") || m["service"] != "ui-api" {
		t.Errorf("línea del suscriptor: %v", m)
	}
	if strings.Contains(strings.ToLower(buf.String()), "pii-") {
		t.Error("sin contenido del evento")
	}
}
