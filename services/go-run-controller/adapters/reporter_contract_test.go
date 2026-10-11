//go:build contract

package adapters

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// Contrato U8↔reporter (POST /v1/report {run_id, evidence_uris} → report.schema.json). Si REPORTER_URL
// está definida, la prueba de reporte válido corre contra el reporter real (loopback) en vez del stub.

const contractReport = `{"run_id":"run-bug-ecom-01","verdict":"bug","summary":"el stock quedo negativo tras una orden",
"findings":[{"finding_id":"flow-1","root_cause":"invariante violado en la orden","invariant":"el stock nunca es negativo tras una orden",
"method":"POST","path":"/orders","evidence_uris":["s3://aqs-evidence/runs/run-bug-ecom-01/flow-1/result.json"]}]}`

var contractEvidence = []string{
	"s3://aqs-evidence/runs/run-bug-ecom-01/flow-1/logs.txt",
	"s3://aqs-evidence/runs/run-bug-ecom-01/flow-1/result.json",
	"s3://aqs-evidence/runs/run-bug-ecom-01/flow-2/logs.txt",
	"s3://aqs-evidence/runs/run-bug-ecom-01/flow-2/result.json",
}

func TestContractReporterValido(t *testing.T) {
	url := os.Getenv("REPORTER_URL")
	if url == "" {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/v1/report" || r.Header.Get("Content-Type") != "application/json" {
				js(w, 404, `{"error":"not_found"}`)
				return
			}
			js(w, 200, contractReport)
		}))
		defer s.Close()
		url = s.URL
	}
	c, err := NewReporterClient(url)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := c.Report(context.Background(), "run-bug-ecom-01", contractEvidence)
	if err != nil || rep.Verdict == "" || rep.RunID != "run-bug-ecom-01" {
		t.Fatalf("reporte válido rechazado: %+v %v", rep, err)
	}
}

// Reporter detenido: la fase falla, nunca hay veredicto.
func TestContractReporterCaido(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { js(w, 200, contractReport) }))
	url := s.URL
	s.Close()
	c, err := NewReporterClient(url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Report(context.Background(), "run-bug-ecom-01", contractEvidence); err == nil {
		t.Fatal("reporter detenido aceptado")
	}
}
