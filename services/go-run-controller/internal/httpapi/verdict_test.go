package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

type okVerifier struct{}

func (okVerifier) Verify(context.Context, string) (Principal, error) { return Principal{ID: "u-1"}, nil }

// GET /runs/{id} expone el veredicto de la fase report y omite el campo mientras no hay reporte.
func TestGetRunExponeVeredicto(t *testing.T) {
	store := runctl.NewMemStore()
	_ = store.Save(runctl.Run{ID: "r1", State: runctl.Done, Verdict: "bug", ReportURI: "s3://evidence/reports/r1.json"})
	_ = store.Save(runctl.Run{ID: "r2", State: runctl.Running})
	h := New(store, noExec{}, okVerifier{})
	get := func(id string) map[string]any {
		req := httptest.NewRequest(http.MethodGet, "/runs/"+id, nil)
		req.Header.Set("Authorization", "Bearer t")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", id, rec.Code, rec.Body.String())
		}
		var out map[string]any
		_ = json.NewDecoder(strings.NewReader(rec.Body.String())).Decode(&out)
		return out
	}
	if v := get("r1")["verdict"]; v != "bug" {
		t.Fatalf("verdict = %v", v)
	}
	if u := get("r1")["report_uri"]; u != "s3://evidence/reports/r1.json" {
		t.Fatalf("report_uri = %v", u)
	}
	if _, ok := get("r2")["verdict"]; ok {
		t.Fatal("verdict presente sin reporte")
	}
}
