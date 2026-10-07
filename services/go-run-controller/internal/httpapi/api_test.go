package httpapi

import (
	"context"
	"encoding/json"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

type verifierFunc func() error

func (f verifierFunc) Verify(context.Context, string) error { return f() }

func TestRunsEndpoint(t *testing.T) {
	st := runctl.NewMemStore()
	_ = st.Save(runctl.Run{ID: "r-1", State: runctl.Deploying, TraceID: "t"})
	cases := []struct {
		name, path, auth string
		verr             error
		want             int
	}{
		{"ok", "/runs/r-1", "Bearer x", nil, 200},
		{"404", "/runs/zz", "Bearer x", nil, 404},
		{"sin token", "/runs/r-1", "", nil, 401},
		{"inválido", "/runs/r-1", "Bearer x", ErrUnauthenticated, 401},
		{"identidad caída", "/runs/r-1", "Bearer x", ErrUnavailable, 503},
	}
	for _, c := range cases {
		h := New(st, verifierFunc(func() error { return c.verr }))
		req := httptest.NewRequest("GET", c.path, nil)
		if c.auth != "" {
			req.Header.Set("Authorization", c.auth)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != c.want {
			t.Errorf("%s: %d", c.name, w.Code)
		}
		if c.want == 503 && w.Header().Get("Retry-After") == "" {
			t.Errorf("%s: sin Retry-After", c.name)
		}
	}
}

func TestIdentityVerifierMapsStatuses(t *testing.T) {
	for code, want := range map[int]error{401: ErrUnauthenticated, 500: ErrUnavailable} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }))
		if err := NewIdentityVerifier(s.URL).Verify(t.Context(), "x"); err != want {
			t.Errorf("%d: %v", code, err)
		}
		s.Close()
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"principal_id":"p","role":"root","session_id":"s","expires_at":"2026-01-01T00:00:00Z"}`))
	}))
	defer s.Close()
	if err := NewIdentityVerifier(s.URL).Verify(t.Context(), "x"); err != ErrUnavailable {
		t.Error("rol desconocido debía ser indisponible")
	}
}

func TestRunViewValidatesAgainstOpenAPISchema(t *testing.T) {
	b, err := os.ReadFile("../../../../contracts/openapi/control-plane.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	schema := doc["components"].(map[string]any)["schemas"].(map[string]any)["Run"]
	c := jsonschema.NewCompiler()
	raw, _ := json.Marshal(schema)
	var js any
	_ = json.Unmarshal(raw, &js)
	if err := c.AddResource("run.json", js); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile("run.json")
	if err != nil {
		t.Fatal(err)
	}
	st := runctl.NewMemStore()
	_ = st.Save(runctl.Run{ID: "r-1", State: runctl.Done, TraceID: "t"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/runs/r-1", nil)
	req.Header.Set("Authorization", "Bearer x")
	New(st, verifierFunc(func() error { return nil })).ServeHTTP(w, req)
	var body any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if err := sch.Validate(body); err != nil {
		t.Fatal(err)
	}
}
