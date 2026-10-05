package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// dashboardExprs lee el ConfigMap del dashboard de U1 y devuelve el título y las expresiones de sus paneles.
func dashboardExprs(t *testing.T) (string, []string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "deploy", "flux", "base", "observability", "dashboard-u1.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	_, after, ok := strings.Cut(string(b), "  aqs-u1.json: |\n")
	if !ok {
		t.Fatal("no se encuentra aqs-u1.json en el ConfigMap")
	}
	var lines []string
	for _, l := range strings.Split(after, "\n") {
		lines = append(lines, strings.TrimPrefix(l, "    "))
	}
	var d struct {
		Title  string
		Panels []struct {
			Targets []struct{ Expr string }
		}
	}
	if err := json.Unmarshal([]byte(strings.Join(lines, "\n")), &d); err != nil {
		t.Fatalf("el JSON embebido no parsea: %v", err)
	}
	var exprs []string
	for _, p := range d.Panels {
		for _, tg := range p.Targets {
			exprs = append(exprs, tg.Expr)
		}
	}
	return d.Title, exprs
}

// Cada métrica que consulta el dashboard la publica go-intake (scrape real de la cadena) o ui-api
// (lista cerrada de la tarea); un nombre inventado o con typo rompe esta prueba.
func TestDashboardU1OnlyUsesPublishedMetrics(t *testing.T) {
	title, exprs := dashboardExprs(t)
	if title != "AQS U1 Ingesta" || len(exprs) < 4 {
		t.Fatalf("dashboard: %q con %d expresiones", title, len(exprs))
	}
	h, _, _, _ := newTestHandler(t, "s3cret")
	get(h, "/healthz") // para que aqs_http_* tengan al menos una serie
	_, scrape := get(h, "/metrics")
	published := map[string]bool{"aqs_inbox_confirmations_total": true, "aqs_inbox_notifications": true} // ui-api
	for _, m := range regexp.MustCompile(`(?m)^(aqs_[a-z_]+)[{ ]`).FindAllStringSubmatch(scrape, -1) {
		published[m[1]] = true
	}
	published["aqs_http_request_duration_seconds_bucket"] = true // el histograma publica _bucket/_sum/_count
	used := map[string]bool{}
	for _, e := range exprs {
		for _, n := range regexp.MustCompile(`aqs_[a-z_]+`).FindAllString(e, -1) {
			used[n] = true
			if !published[n] {
				t.Errorf("el dashboard usa %s, que ningún servicio publica", n)
			}
		}
	}
	for _, must := range []string{"aqs_http_requests_total", "aqs_http_request_duration_seconds_bucket", "aqs_intake_webhook_rejected_total"} {
		if !used[must] {
			t.Errorf("el dashboard debe usar %s", must)
		}
	}
}
