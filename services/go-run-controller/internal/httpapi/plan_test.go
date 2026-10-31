package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/plan"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

func planFor(runID string) plan.FlowPlan {
	return plan.FlowPlan{RunID: runID, Workflow: "checkout", Flows: []plan.Flow{{
		FlowID: "f1", Name: "crear", Invariant: "la orden existe",
		Steps: []plan.Step{{Method: "POST", Path: "/orders", ExpectStatus: 201}},
	}}}
}

func getPlan(t *testing.T, st runctl.RunStore, path string) *httptest.ResponseRecorder {
	t.Helper()
	h := New(st, noExec{}, verifierFunc(func() error { return nil }))
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer x")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestPlanEndpoint200ReturnsStoredPlan(t *testing.T) {
	st := runctl.NewMemStore()
	raw, _ := json.Marshal(planFor("r-1"))
	_ = st.Save(runctl.Run{ID: "r-1", State: runctl.Running, Plan: raw})
	w := getPlan(t, st, "/runs/r-1/plan")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var got plan.FlowPlan
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || !reflect.DeepEqual(got, planFor("r-1")) {
		t.Fatalf("cuerpo inesperado: %v %s", err, w.Body.String())
	}
}

func TestPlanEndpointRunNotFound(t *testing.T) {
	w := getPlan(t, runctl.NewMemStore(), "/runs/zz/plan")
	if w.Code != http.StatusNotFound || !contains(w.Body.String(), "run_not_found") {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}

func TestPlanEndpointNotReady(t *testing.T) {
	st := runctl.NewMemStore()
	_ = st.Save(runctl.Run{ID: "r-1", State: runctl.Confirmed})
	w := getPlan(t, st, "/runs/r-1/plan")
	if w.Code != http.StatusNotFound || !contains(w.Body.String(), "plan_not_ready") {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}

func TestPlanEndpointCorruptPlanFailsClosed(t *testing.T) {
	st := runctl.NewMemStore()
	_ = st.Save(runctl.Run{ID: "r-1", State: runctl.Running, Plan: json.RawMessage(`{"run_id":"r-1"}`)})
	w := getPlan(t, st, "/runs/r-1/plan")
	if w.Code != http.StatusInternalServerError || !contains(w.Body.String(), "plan_invalid") {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}

func TestPlanEndpointRequiresAuth(t *testing.T) {
	st := runctl.NewMemStore()
	_ = st.Save(runctl.Run{ID: "r-1", State: runctl.Running})
	h := New(st, noExec{}, verifierFunc(func() error { return nil }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/runs/r-1/plan", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", w.Code)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func TestPlanEndpointValidatesAgainstOpenAPISchema(t *testing.T) {
	b, err := os.ReadFile("../../../../contracts/openapi/control-plane.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	raw, _ := json.Marshal(doc)
	var js any
	_ = json.Unmarshal(raw, &js)
	if err := c.AddResource("control-plane.json", js); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile("control-plane.json#/components/schemas/FlowPlan")
	if err != nil {
		t.Fatal(err)
	}
	st := runctl.NewMemStore()
	raw, _ = json.Marshal(planFor("r-1"))
	_ = st.Save(runctl.Run{ID: "r-1", State: runctl.Running, Plan: raw})
	w := getPlan(t, st, "/runs/r-1/plan")
	var body any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if err := sch.Validate(body); err != nil {
		t.Fatal(err)
	}
}
