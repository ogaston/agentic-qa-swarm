package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/adapters"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/internal/obs"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

// CA-2 (U11-T03): el respaldo RUN_ARTIFACT_REF queda registrado: un WARN artifact_source=fallback una vez
// por corrida y aqs_run_artifact_fallback_total sube en 1 (no sube con un artefacto del evento).
func TestArtifactFallbackWarnUnaVezPorCorridaYContador(t *testing.T) {
	env := realEnv()
	env["RUN_ARTIFACT_REF"] = "ghcr.io/x/app:1.0"
	c, err := loadConfig(envOf(env))
	if err != nil {
		t.Fatal(err)
	}
	reg := prometheus.NewRegistry()
	metrics := obs.NewRunMetrics(reg, func() float64 { return 0 })
	var logs bytes.Buffer
	store := runctl.NewMemStore()
	evento := runctl.Artifact{Kind: "published-image", Ref: "ghcr.io/ogaston/demo-target-app-qa-swarm:" + strings.Repeat("c", 40)}
	if err := store.Save(runctl.Run{ID: "run-ev", State: runctl.Confirmed, Artifact: &evento}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(runctl.Run{ID: "run-fb", State: runctl.Confirmed}); err != nil {
		t.Fatal(err)
	}
	cfg, err := buildConfig(c, fake.NewSimpleClientset(), runctl.AllowAll(), store, &runctl.FakePublisher{}, &runctl.FakeAlerter{},
		metrics, nil, slog.New(slog.NewJSONHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	rp := cfg.Phases.(*adapters.RealPhases)
	for i := 0; i < 3; i++ {
		if _, _, err := rp.Artifact("run-fb"); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := rp.Artifact("run-ev"); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(logs.String(), `"artifact_source":"fallback"`); n != 1 {
		t.Fatalf("WARN de respaldo: %d, quiero exactamente 1 por corrida:\n%s", n, logs.String())
	}
	if v := counterValue(t, reg, "aqs_run_artifact_fallback_total"); v != 1 {
		t.Fatalf("aqs_run_artifact_fallback_total=%v, quiero 1", v)
	}
}

// counterValue lee un contador del registro (sin dependencias de testutil).
func counterValue(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, mf := range mfs {
		if mf.GetName() == name && len(mf.GetMetric()) > 0 {
			return mf.GetMetric()[0].GetCounter().GetValue()
		}
	}
	t.Fatalf("métrica %s ausente", name)
	return 0
}
