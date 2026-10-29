package httpapi

import (
	"net/http"
	"testing"
)

func TestEvidenceOwnershipAndPlan(t *testing.T) {
	var runID string
	ctl := newWarmFake(t, dyn(&runID, func(id string) http.HandlerFunc {
		return respond(http.StatusOK, `{"run_id":"`+id+`","flows":[{"flow_id":"flow-1"}]}`)
	}))
	rep := newWarmFake(t, respond(http.StatusOK, `{"status":"passed"}`))
	e, id := repEnv(t, ctl, rep)
	runID = id
	for _, tc := range []struct {
		token, flow, file string
		want              int
	}{
		{repTokU1, "flow-1", "result.json", 200},
		{repTokAdm, "flow-1", "logs.txt", 200},
		{repTokU2, "flow-1", "result.json", 403},
		{"", "flow-1", "result.json", 401},
		{repTokU1, "other", "result.json", 404},
		{repTokU1, "flow-1", "secret", 400},
	} {
		w := e.do(http.MethodGet, "/run-evidence/"+id+"/"+tc.flow+"/"+tc.file, tc.token, "")
		if w.Code != tc.want {
			t.Errorf("%s %s: got %d, want %d", tc.flow, tc.token, w.Code, tc.want)
		}
	}
	if rep.count() != 2 {
		t.Fatalf("reporter calls = %d, want 2", rep.count())
	}
}

func TestReportEvidenceRequiresOwnershipAndReportReference(t *testing.T) {
	var runID string
	ctl := newWarmFake(t, dyn(&runID, func(id string) http.HandlerFunc { return ctlRun(id, reportURI(id)) }))
	rep := newWarmFake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/reports" {
			respond(http.StatusOK, `{"run_id":"`+runID+`","findings":[{"evidence_uris":["s3://evidence/runs/`+runID+`/flow/trace.har"]}]}`)(w, r)
		} else if r.URL.Path == "/v1/report-evidence" && r.URL.Query().Get("uri") == "s3://evidence/runs/"+runID+"/flow/trace.har" {
			respond(http.StatusOK, "artifact")(w, r)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	})
	e, id := repEnv(t, ctl, rep)
	runID = id
	for _, tc := range []struct {
		token, path string
		status      int
	}{
		{repTokU1, "/0/0", 200}, {repTokAdm, "/0/0", 200},
		{repTokU2, "/0/0", 403}, {"", "/0/0", 401},
		{repTokU1, "/0/1", 404}, {repTokU1, "/-1/0", 400},
	} {
		w := e.do(http.MethodGet, "/report-evidence/"+id+tc.path, tc.token, "")
		if w.Code != tc.status {
			t.Errorf("%s: got %d want %d", tc.path, w.Code, tc.status)
		}
		if w.Code == 200 && w.Body.String() != "artifact" {
			t.Errorf("wrong artifact: %s", w.Body.String())
		}
	}
}
