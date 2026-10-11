package adapters

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

const reportBugJSON = `{"run_id":"r1","verdict":"bug","summary":"stock negativo","report_uri":"s3://evidence/reports/r1.json",
"findings":[{"finding_id":"f1","root_cause":"causa","invariant":"inv","method":"POST","path":"/orders",
"evidence_uris":["s3://evidence/r1/f1/result.json"]}]}`

var evidence = []string{"s3://evidence/r1/f1/logs.txt", "s3://evidence/r1/f1/result.json"}

func newReporter(t *testing.T, url string) *ReporterClient {
	t.Helper()
	c, err := NewReporterClient(url)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// Reporte válido: la corrida recibe el veredicto (CA-1: reporte válido -> corrida con verdict).
func TestReporterValidoDaVeredicto(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/report" || r.Method != http.MethodPost {
			js(w, 404, `{"error":"not_found"}`)
			return
		}
		js(w, 200, reportBugJSON)
	}))
	defer s.Close()
	p := &RealPhases{Report: newReporter(t, s.URL)}
	res, err := p.LaunchReport(context.Background(), runctl.Run{ID: "r1", Evidence: evidence})
	if err != nil || res.Verdict != "bug" || res.ReportURI != "s3://evidence/reports/r1.json" {
		t.Fatalf("resultado: %+v %v", res, err)
	}
}

// Fail-closed: caído, 500, inválido o de otra corrida es fallo de fase, sin veredicto (CA-1).
func TestReporterFallaCerrado(t *testing.T) {
	cases := map[string]func(http.ResponseWriter, *http.Request){
		"500":               func(w http.ResponseWriter, _ *http.Request) { js(w, 500, `{"error":"internal"}`) },
		"422 sin evidencia": func(w http.ResponseWriter, _ *http.Request) { js(w, 422, `{"error":"no_evidence"}`) },
		"cuerpo inválido":   func(w http.ResponseWriter, _ *http.Request) { js(w, 200, `{"verdict":"nope"}`) },
		"veredicto bug sin hallazgos": func(w http.ResponseWriter, _ *http.Request) {
			js(w, 200, `{"run_id":"r1","verdict":"bug","summary":"x","findings":[]}`)
		},
		"otra corrida": func(w http.ResponseWriter, _ *http.Request) {
			js(w, 200, `{"run_id":"otra","verdict":"sin-hallazgos","summary":"x","findings":[]}`)
		},
		"sin report_uri": func(w http.ResponseWriter, _ *http.Request) {
			js(w, 200, `{"run_id":"r1","verdict":"sin-hallazgos","summary":"x","findings":[]}`)
		},
		"report_uri vacío": func(w http.ResponseWriter, _ *http.Request) {
			js(w, 200, `{"run_id":"r1","verdict":"sin-hallazgos","summary":"x","findings":[],"report_uri":""}`)
		},
		"campo extra": func(w http.ResponseWriter, _ *http.Request) {
			js(w, 200, `{"run_id":"r1","verdict":"sin-hallazgos","summary":"x","findings":[],"report_uri":"s3://x","campo_inventado":1}`)
		},
		"redirección": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://evil.example/", http.StatusFound)
		},
	}
	for name, h := range cases {
		s := httptest.NewServer(http.HandlerFunc(h))
		res, err := (&RealPhases{Report: newReporter(t, s.URL)}).LaunchReport(context.Background(), runctl.Run{ID: "r1", Evidence: evidence})
		s.Close()
		if err == nil || res.Verdict != "" {
			t.Errorf("%s: aceptado (verdict=%q, err=%v)", name, res.Verdict, err)
		}
	}
}

func TestReporterCaidoFallaCerrado(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { js(w, 200, reportBugJSON) }))
	c := newReporter(t, s.URL)
	s.Close()
	if _, err := c.Report(context.Background(), "r1", evidence); err == nil {
		t.Fatal("reporter caído aceptado")
	}
}

// Sin evidencia o sin cliente: no se llama al reporter y la fase falla.
func TestReportSinEvidenciaOSinClienteFalla(t *testing.T) {
	if _, err := (&RealPhases{}).LaunchReport(context.Background(), runctl.Run{ID: "r1", Evidence: evidence}); err == nil {
		t.Fatal("sin cliente del reporter aceptado")
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { js(w, 200, reportBugJSON) }))
	defer s.Close()
	if _, err := newReporter(t, s.URL).Report(context.Background(), "r1", nil); err == nil {
		t.Fatal("sin evidencia aceptado")
	}
}

// Solo http(s) en la URL del reporter.
func TestReporterURLSoloHTTP(t *testing.T) {
	if _, err := NewReporterClient("file:///etc/passwd"); err == nil {
		t.Fatal("URL no http(s) aceptada")
	}
}

// Circuito: 5 fallos seguidos abren; con circuito abierto no se llama al reporter.
func TestReporterCircuitoAbre(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++; js(w, 500, `{}`) }))
	defer s.Close()
	c := newReporter(t, s.URL)
	c.Timeout = time.Second
	now := time.Unix(1000, 0)
	c.Now = func() time.Time { return now }
	for i := 0; i < ReporterCircuitThresh; i++ {
		_, _ = c.Report(context.Background(), "r1", evidence)
	}
	before := calls
	if _, err := c.Report(context.Background(), "r1", evidence); err == nil || calls != before {
		t.Fatalf("circuito no abrió: err=%v llamadas %d->%d", err, before, calls)
	}
	now = now.Add(ReporterCircuitOpenFor + time.Second)
	_, _ = c.Report(context.Background(), "r1", evidence)
	if calls == before {
		t.Fatal("la sonda tras el enfriamiento no llamó al reporter")
	}
}

// Plazo: el reporter que no responde en el plazo es fallo de fase.
func TestReporterPlazo(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
		}
	}))
	defer s.Close()
	c := newReporter(t, s.URL)
	c.Timeout = 100 * time.Millisecond
	if _, err := c.Report(context.Background(), "r1", evidence); err == nil {
		t.Fatal("plazo agotado aceptado")
	}
}

// E-1 (U8-T06): la respuesta trae el URI del reporte, que la corrida guarda; sin él no hay reporte válido.
func TestReporterDevuelveReportURI(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { js(w, 200, reportBugJSON) }))
	defer s.Close()
	rep, err := newReporter(t, s.URL).Report(context.Background(), "r1", evidence)
	if err != nil || rep.ReportURI != "s3://evidence/reports/r1.json" {
		t.Fatalf("report_uri: %q %v", rep.ReportURI, err)
	}
}
