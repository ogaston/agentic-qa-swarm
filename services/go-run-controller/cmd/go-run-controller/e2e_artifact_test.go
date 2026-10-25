package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"k8s.io/client-go/kubernetes/fake"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/adapters"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

// F-03: cadena de punta a punta. El payload real que genera ui-api (NewRunConfirmed, fixture
// adapters/testdata/ui-api/run-confirmed.json) pasa por ParseRunConfirmed, queda en la corrida y
// RealPhases entrega kind/ref al deploy de go-warm-manager (fake HTTP con la forma del contrato).
func TestCadenaUIAPIAControladorADeploy(t *testing.T) {
	line, err := os.ReadFile("../../adapters/testdata/ui-api/run-confirmed.json")
	if err != nil {
		t.Fatal(err)
	}
	ev, err := adapters.ParseRunConfirmed(line)
	if err != nil {
		t.Fatalf("el payload de ui-api no parsea en el controlador: %v", err)
	}
	if ev.Artifact == nil {
		t.Fatal("el payload de ui-api no trae artefacto")
	}

	var mu sync.Mutex
	var posted map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/warm/ensure":
			_, _ = w.Write([]byte(`{"warm_id":"w-1","state":"ready","reset_verified":true,"baseline_version":"b1"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/deploys/"+ev.RunID:
			if posted == nil {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"code":"not_found"}`))
				return
			}
			_, _ = w.Write([]byte(`{"run_id":"` + ev.RunID + `","state":"done","attempts":1}`))
		case r.Method == http.MethodPost && r.URL.Path == "/deploys":
			_ = json.NewDecoder(r.Body).Decode(&posted)
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"run_id":"` + ev.RunID + `","state":"done","attempts":1}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	env := realEnv()
	env["WARM_URL"] = srv.URL
	env["RUN_ARTIFACT_REF"] = "ghcr.io/x/estatica:1"
	c, err := loadConfig(envOf(env))
	if err != nil {
		t.Fatal(err)
	}
	store := runctl.NewMemStore()
	if err := store.Save(runctl.Run{ID: ev.RunID, State: runctl.Confirmed, Artifact: ev.Artifact}); err != nil {
		t.Fatal(err)
	}
	cfg, err := buildConfig(c, fake.NewSimpleClientset(), runctl.AllowAll(), store, &runctl.FakePublisher{}, &runctl.FakeAlerter{}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	run, _ := store.Get(ev.RunID)
	if _, err := cfg.Phases.Launch(context.Background(), runctl.PhaseDeploy, run); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	art, _ := posted["artifact"].(map[string]any)
	if art == nil || art["kind"] != ev.Artifact.Kind || art["ref"] != ev.Artifact.Ref {
		t.Fatalf("el deploy debe llevar el artefacto del evento %+v, envió %v", ev.Artifact, posted)
	}
}
