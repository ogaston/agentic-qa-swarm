package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

// principalVerifier devuelve siempre el mismo principal (o error).
type principalVerifier struct {
	p   Principal
	err error
}

func (v principalVerifier) Verify(context.Context, string) (Principal, error) { return v.p, v.err }

// execRig arma el handler con un controlador real sobre un MemStore.
func execRig(t *testing.T, p Principal) (http.Handler, *runctl.MemStore) {
	t.Helper()
	st := runctl.NewMemStore()
	k, err := runctl.New(runctl.Config{Gate: runctl.AllowAll(), Store: st, Publisher: &runctl.FakePublisher{},
		Warm: &runctl.FakeWarm{Fact: runctl.True}, Alerter: &runctl.FakeAlerter{}, Phases: &runctl.FakePhases{}})
	if err != nil {
		t.Fatal(err)
	}
	return New(st, k, principalVerifier{p: p}), st
}

func doReq(t *testing.T, h http.Handler, method, path, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func awaitingView(t *testing.T, h http.Handler, path string) map[string]any {
	t.Helper()
	w := doReq(t, h, http.MethodGet, path, "tok")
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestExecuteRun401SinToken(t *testing.T) {
	h, _ := execRig(t, Principal{ID: "u-1"})
	if w := doReq(t, h, http.MethodPost, "/runs/r-1/execute", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", w.Code)
	}
}

func TestExecuteRun404(t *testing.T) {
	h, _ := execRig(t, Principal{ID: "u-1"})
	w := doReq(t, h, http.MethodPost, "/runs/zz/execute", "tok")
	if w.Code != http.StatusNotFound || !contains(w.Body.String(), "not_found") {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}

func TestExecuteRun403(t *testing.T) {
	h, st := execRig(t, Principal{ID: "u-1"})
	_ = st.Save(runctl.Run{ID: "r-2", State: runctl.Rehearsing, ConfirmedBy: "otro", EnsayoPassed: runctl.True})
	w := doReq(t, h, http.MethodPost, "/runs/r-2/execute", "tok")
	if w.Code != http.StatusForbidden || !contains(w.Body.String(), "forbidden") {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}

func TestExecuteRun409(t *testing.T) {
	h, st := execRig(t, Principal{ID: "u-1"})
	_ = st.Save(runctl.Run{ID: "r-3", State: runctl.Running, ConfirmedBy: "u-1", EnsayoPassed: runctl.True})
	w := doReq(t, h, http.MethodPost, "/runs/r-3/execute", "tok")
	if w.Code != http.StatusConflict || !contains(w.Body.String(), "not_awaiting") {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}

func TestExecuteRun202YReadback(t *testing.T) {
	h, st := execRig(t, Principal{ID: "u-1"})
	_ = st.Save(runctl.Run{ID: "r-4", State: runctl.Rehearsing, ConfirmedBy: "u-1", EnsayoPassed: runctl.True})

	if got := awaitingView(t, h, "/runs/r-4")["awaiting_execution"]; got != true {
		t.Fatalf("antes de aprobar awaiting_execution = %v, esperado true", got)
	}
	w := doReq(t, h, http.MethodPost, "/runs/r-4/execute", "tok")
	if w.Code != http.StatusAccepted {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["awaiting_execution"] != false {
		t.Fatalf("202 con awaiting_execution = %v, esperado false", body["awaiting_execution"])
	}
	if got := awaitingView(t, h, "/runs/r-4")["awaiting_execution"]; got != false {
		t.Fatalf("tras aprobar awaiting_execution = %v, esperado false", got)
	}
	if r, _ := st.Get("r-4"); r.ExecutionApprovedBy != "u-1" {
		t.Fatalf("ExecutionApprovedBy = %q", r.ExecutionApprovedBy)
	}
}
