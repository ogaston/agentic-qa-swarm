package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/plan"
)

const validPlanJSON = `{"run_id":"r1","workflow":"checkout","flows":[{"flow_id":"f1","name":"crear orden",
"steps":[{"method":"POST","path":"/orders","expect_status":201},{"method":"GET","path":"/orders/1","expect_status":200}],
"invariant":"el total coincide"}]}`

func surfaceOK(_ context.Context, runID string) (plan.SurfaceArtifact, error) {
	return plan.SurfaceArtifact{RunID: runID, BaseURL: "http://warm-app.aqs-test.svc:8080", Source: "openapi",
		Endpoints: []plan.Endpoint{{Method: "POST", Path: "/orders"}}}, nil
}

func workflowOK(string) (string, bool) { return "checkout", true }

func u3Server(t *testing.T, code int, body string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { js(w, code, body) }))
	t.Cleanup(s.Close)
	return s
}

func newU3(t *testing.T, url string) *FlowSourceU3 {
	t.Helper()
	f, err := NewFlowSourceU3(url, surfaceOK, workflowOK)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestFlowSourceU3ValidPlan(t *testing.T) {
	var got struct {
		Surface  plan.SurfaceArtifact `json:"surface"`
		Workflow string               `json:"workflow"`
	}
	var path, method, auth string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, method, auth = r.URL.Path, r.Method, r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		js(w, 200, validPlanJSON)
	}))
	defer s.Close()
	fp, err := newU3(t, s.URL).Flows("r1")
	if err != nil || fp.RunID != "r1" || len(fp.Flows) != 1 {
		t.Fatal(fp, err)
	}
	if method != "POST" || path != "/v1/plan" || got.Workflow != "checkout" || got.Surface.RunID != "r1" || auth != "" {
		t.Fatal(method, path, got, auth)
	}
}

func TestFlowSourceU3InvalidPlanFailsClosed(t *testing.T) {
	for name, body := range map[string]string{
		"campo desconocido": strings.Replace(validPlanJSON, `"workflow"`, `"extra":1,"workflow"`, 1),
		"método inválido":   strings.Replace(validPlanJSON, `"POST"`, `"TRACE"`, 1),
		"sin invariante":    strings.Replace(validPlanJSON, `"invariant":"el total coincide"`, `"invariant":""`, 1),
		"no es JSON":        `<html>`,
		"basura al final":   validPlanJSON + `{}`,
		"otra corrida":      strings.Replace(validPlanJSON, `"r1"`, `"r2"`, 1),
		"otro workflow":     strings.Replace(validPlanJSON, `"checkout"`, `"refund"`, 1),
	} {
		if _, err := newU3(t, u3Server(t, 200, body).URL).Flows("r1"); err == nil {
			t.Errorf("%s: aceptado", name)
		}
	}
}

func TestFlowSourceU3EmptyPlanFailsClosed(t *testing.T) {
	for _, body := range []string{`{"run_id":"r1","workflow":"checkout","flows":[]}`, `{"run_id":"r1","workflow":"checkout"}`, `{}`} {
		if _, err := newU3(t, u3Server(t, 200, body).URL).Flows("r1"); err == nil {
			t.Errorf("plan vacío aceptado: %s", body)
		}
	}
}

func TestFlowSourceU3StepOutsideRootFailsClosed(t *testing.T) {
	for _, p := range []string{"orders", "http://evil.example/x", "//evil.example/x", `/\\evil.example`} {
		body := strings.Replace(validPlanJSON, `"/orders/1"`, `"`+p+`"`, 1)
		if _, err := newU3(t, u3Server(t, 200, body).URL).Flows("r1"); err == nil {
			t.Errorf("path %q aceptado", p)
		}
	}
}

func TestFlowSourceU3TimeoutFailsClosed(t *testing.T) {
	release := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer s.Close()
	defer close(release)
	f := newU3(t, s.URL)
	if f.Timeout != 30*time.Second {
		t.Fatalf("timeout por defecto %v", f.Timeout)
	}
	f.Timeout = 50 * time.Millisecond
	start := time.Now()
	if _, err := f.Flows("r1"); err == nil || time.Since(start) > 5*time.Second {
		t.Fatal("el timeout no falló cerrado", err)
	}
}

func TestFlowSourceU3NonHTTPURLRejected(t *testing.T) {
	for _, u := range []string{"ftp://x", "file:///x", "x", "", "http://u:p@h", "gopher://h"} {
		if _, err := NewFlowSourceU3(u, surfaceOK, workflowOK); err == nil {
			t.Errorf("URL %q aceptada", u)
		}
	}
	if _, err := NewFlowSourceU3("http://h", nil, workflowOK); err == nil {
		t.Error("sin superficie aceptado")
	}
}

func TestFlowSourceU3NonOKAndRedirectFailClosed(t *testing.T) {
	for _, c := range []int{201, 204, 302, 307, 400, 422, 500, 502, 503} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "/v1/plan")
			js(w, c, validPlanJSON)
		}))
		_, err := newU3(t, s.URL).Flows("r1")
		s.Close()
		if err == nil {
			t.Errorf("estado %d aceptado", c)
		}
	}
}

func TestFlowSourceU3SurfaceOrWorkflowFailureNeverCallsU3(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); js(w, 200, validPlanJSON) }))
	defer s.Close()
	f, _ := NewFlowSourceU3(s.URL, func(context.Context, string) (plan.SurfaceArtifact, error) {
		return plan.SurfaceArtifact{}, errors.New("sin superficie")
	}, workflowOK)
	if _, err := f.Flows("r1"); err == nil {
		t.Error("sin superficie aceptado")
	}
	f, _ = NewFlowSourceU3(s.URL, surfaceOK, func(string) (string, bool) { return "", false })
	if _, err := f.Flows("r1"); err == nil {
		t.Error("sin workflow aceptado")
	}
	if _, err := newU3(t, s.URL).Flows("../x"); err == nil {
		t.Error("run_id inválido aceptado")
	}
	if calls.Load() != 0 {
		t.Fatalf("U3 llamado %d veces", calls.Load())
	}
}

func TestFlowSourceU3CachesValidatedPlanPerRun(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); js(w, 200, validPlanJSON) }))
	defer s.Close()
	f := newU3(t, s.URL)
	for range 3 {
		if _, err := f.Flows("r1"); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("llamadas a U3: %d, quería 1 (mismo plan en ensayo y runners)", calls.Load())
	}
}

func TestFlowSourceU3CircuitOpensAndRecovers(t *testing.T) {
	var calls atomic.Int32
	var healthy atomic.Bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if healthy.Load() {
			js(w, 200, validPlanJSON)
			return
		}
		js(w, 503, `{}`)
	}))
	defer s.Close()
	now := time.Unix(1000, 0)
	f := newU3(t, s.URL)
	f.Now = func() time.Time { return now }
	for range FlowCircuitThreshold {
		if _, err := f.Flows("r1"); err == nil {
			t.Fatal("503 aceptado")
		}
	}
	before := calls.Load()
	if _, err := f.Flows("r1"); err == nil || calls.Load() != before {
		t.Fatal("circuito abierto debe rechazar sin llamar", err, calls.Load(), before)
	}
	healthy.Store(true)
	now = now.Add(FlowCircuitOpen + time.Second)
	if _, err := f.Flows("r1"); err != nil {
		t.Fatal("la sonda sana debía cerrar el circuito:", err)
	}
}
