package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/adapters"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/rehearsal"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

func realEnv() map[string]string {
	m := good()
	m["RUN_PHASES"] = "real"
	delete(m, "RUN_ALLOW_FAKE_PHASES")
	m["WARM_URL"], m["RESET_URL"] = "http://warm:8080", "https://reset:8080"
	m["WARM_SERVICE_TOKEN"], m["RESET_SERVICE_TOKEN"] = "w", "r"
	m["RUN_ARTIFACT_REF"] = "ghcr.io/x/app:1.0"
	m["REHEARSAL_IMAGE"] = "ghcr.io/ogaston/aqs-rehearsal:0.1.0"
	m["REHEARSAL_TARGET_URL"] = "http://warm-app.aqs-test.svc:8080"
	m["RUNNER_IMAGE"] = "ghcr.io/ogaston/aqs-runner:0.1.0"
	m["EVIDENCE_ENDPOINT"], m["EVIDENCE_BUCKET"] = "http://minio.aqs-system.svc:9000", "evidence"
	ak, sk := keyFiles()
	m["EVIDENCE_ACCESS_KEY_FILE"], m["EVIDENCE_SECRET_KEY_FILE"] = ak, sk
	return m
}

func TestLoadConfigRealOK(t *testing.T) {
	c, err := loadConfig(envOf(realEnv()))
	if err != nil || !c.real || c.rehearsalSA != "aqs-runner" {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestLoadConfigRealFailsClosed(t *testing.T) {
	for name, mut := range map[string]func(map[string]string){
		"sin WARM_URL":      func(m map[string]string) { delete(m, "WARM_URL") },
		"sin RESET_URL":     func(m map[string]string) { delete(m, "RESET_URL") },
		"WARM_URL ftp":      func(m map[string]string) { m["WARM_URL"] = "ftp://x" },
		"RESET_URL file":    func(m map[string]string) { m["RESET_URL"] = "file:///x" },
		"sin token warm":    func(m map[string]string) { delete(m, "WARM_SERVICE_TOKEN") },
		"sin token reset":   func(m map[string]string) { delete(m, "RESET_SERVICE_TOKEN") },
		"imagen latest":     func(m map[string]string) { m["REHEARSAL_IMAGE"] = "ghcr.io/x/y:latest" },
		"imagen sin tag":    func(m map[string]string) { m["REHEARSAL_IMAGE"] = "ghcr.io/x/y" },
		"sin imagen":        func(m map[string]string) { delete(m, "REHEARSAL_IMAGE") },
		"sin artefacto":     func(m map[string]string) { delete(m, "RUN_ARTIFACT_REF") },
		"destino no http":   func(m map[string]string) { m["REHEARSAL_TARGET_URL"] = "tcp://x" },
		"namespace staging": func(m map[string]string) { m["RUN_TEST_NAMESPACE"] = "staging" },
	} {
		m := realEnv()
		mut(m)
		if _, err := loadConfig(envOf(m)); err == nil {
			t.Errorf("%s: aceptado", name)
		}
	}
}

func TestLoadConfigFakeStillGuarded(t *testing.T) {
	m := good()
	m["RUN_PHASES"], m["RUN_ENV"], m["RUN_ALLOW_FAKE_PHASES"] = "fake", "prod", "true"
	if _, err := loadConfig(envOf(m)); err == nil {
		t.Fatal("fake con RUN_ENV=prod aceptado")
	}
	m["RUN_ENV"] = ""
	delete(m, "RUN_ALLOW_FAKE_PHASES")
	if _, err := loadConfig(envOf(m)); err == nil {
		t.Fatal("fake sin RUN_ALLOW_FAKE_PHASES aceptado")
	}
}

// Ninguna variable de entorno salta el ensayo: la configuración ni la lee.
func TestNoBypassConfigIgnoresSkipVariables(t *testing.T) {
	a, _ := loadConfig(envOf(realEnv()))
	m := realEnv()
	for _, k := range []string{"RUN_SKIP_REHEARSAL", "SKIP_REHEARSAL", "RUN_ENSAYO_PASSED"} {
		m[k] = "true"
	}
	b, err := loadConfig(envOf(m))
	if err != nil || a != b {
		t.Fatalf("la configuración cambió con variables de salto: %v", err)
	}
}

func TestRenderRehearsalJob(t *testing.T) {
	var out bytes.Buffer
	if err := renderRehearsalJob(&out, []string{"--run", "r-1", "--flow", "checkout"}, envOf(nil)); err != nil {
		t.Fatal(err)
	}
	y := out.String()
	for _, w := range []string{"kind: Job", "name: rehearsal-r-1-1", "namespace: aqs-test", "automountServiceAccountToken: false", "readOnlyRootFilesystem: true"} {
		if !strings.Contains(y, w) {
			t.Errorf("falta %q", w)
		}
	}
	for _, bad := range []string{"envFrom", "secretKeyRef", "LLM", "API_KEY"} {
		if strings.Contains(y, bad) {
			t.Errorf("contiene %q", bad)
		}
	}
	for _, args := range [][]string{nil, {"--run", "r-1"}, {"--run", "R_1", "--flow", "f"}, {"--run", "r", "--flow"}, {"--x", "1", "--flow", "f"}} {
		if err := renderRehearsalJob(&out, args, envOf(map[string]string{})); err == nil {
			t.Errorf("args %v aceptados", args)
		}
	}
	if err := renderRehearsalJob(&out, []string{"--run", "r-1", "--flow", "f"}, envOf(map[string]string{"REHEARSAL_IMAGE": "x:latest"})); err == nil {
		t.Error("imagen latest aceptada")
	}
}

func buildFor(t *testing.T, env map[string]string, cs kubernetes.Interface) (runctl.Config, error) {
	t.Helper()
	c, err := loadConfig(envOf(env))
	if err != nil {
		t.Fatal(err)
	}
	return buildConfig(c, cs, runctl.AllowAll(), runctl.NewMemStore(), &runctl.FakePublisher{}, &runctl.FakeAlerter{}, nil, nil, nil)
}

// Cableado de real: todos los puertos de warm y de fase son reales y ningún Fake* queda en la config.
func TestBuildConfigRealWiresEveryRealPort(t *testing.T) {
	cfg, err := buildFor(t, realEnv(), fake.NewSimpleClientset())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Warm.(*adapters.WarmClient); !ok {
		t.Fatalf("Warm = %T", cfg.Warm)
	}
	rp, ok := cfg.Phases.(*adapters.RealPhases)
	if !ok || rp.Rehearse == nil || rp.Warm == nil || rp.Reset == nil || rp.Artifact == nil || cfg.Results == nil {
		t.Fatalf("Phases = %T %+v Results=%v", cfg.Phases, rp, cfg.Results)
	}
	if _, ok := cfg.Results.(*rehearsal.Launcher); !ok || cfg.Results != rp.Rehearse.(runctl.RehearsalResults) {
		t.Fatalf("Results y Rehearse deben ser el mismo lanzador: %T", cfg.Results)
	}
	for _, p := range []any{cfg.Warm, cfg.Phases, cfg.Results, cfg.Gate, cfg.Store, cfg.Publisher, cfg.Alerter, rp.Rehearse} {
		if n := fmt.Sprintf("%T", p); strings.Contains(n, "Fake") && n != "*runctl.FakeAlerter" && n != "*runctl.FakePublisher" && n != "*runctl.FakeGate" {
			t.Errorf("queda un fake de fase/warm en real: %s", n)
		}
	}
}

func TestBuildConfigRealWithoutClientsetFailsAndFakeStaysFake(t *testing.T) {
	if _, err := buildFor(t, realEnv(), nil); err == nil {
		t.Fatal("real sin clientset aceptado")
	}
	cfg, err := buildFor(t, good(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Phases.(*runctl.FakePhases); !ok || cfg.Results != nil {
		t.Fatalf("fake: %T %v", cfg.Phases, cfg.Results)
	}
}

func TestLoadConfigRunEnvVariantsRejectFake(t *testing.T) {
	for _, v := range []string{"prod", "PROD", " Production ", "production", "Prod"} {
		m := good()
		m["RUN_ENV"] = v
		if _, err := loadConfig(envOf(m)); err == nil {
			t.Errorf("RUN_ENV=%q aceptado con fake", v)
		}
	}
}

const testAK, testSK = "AKIATESTACCESS0001", "sk-test-secret-key-0002"

var (
	keyOnce  sync.Once
	akf, skf string
)

// keyFiles escribe credenciales de prueba generadas aquí (nunca en el repo).
func keyFiles() (string, string) {
	keyOnce.Do(func() {
		d, _ := os.MkdirTemp("", "evkeys")
		akf, skf = filepath.Join(d, "ak"), filepath.Join(d, "sk")
		_ = os.WriteFile(akf, []byte(testAK+"\n"), 0o600)
		_ = os.WriteFile(skf, []byte(testSK+"\n"), 0o600)
	})
	return akf, skf
}

func TestLoadConfigFlowSourceU3(t *testing.T) {
	m := realEnv()
	m["RUN_FLOW_SOURCE"], m["U3_URL"] = "u3", "http://planner.u3:8080"
	c, err := loadConfig(envOf(m))
	if err != nil || c.flowSource != "u3" || c.u3URL != "http://planner.u3:8080" {
		t.Fatalf("%+v %v", c, err)
	}
	for name, mut := range map[string]func(map[string]string){
		"sin U3_URL":         func(m map[string]string) { delete(m, "U3_URL") },
		"U3_URL ftp":         func(m map[string]string) { m["U3_URL"] = "ftp://x" },
		"U3_URL con claves":  func(m map[string]string) { m["U3_URL"] = "http://u:p@x" },
		"origen desconocido": func(m map[string]string) { m["RUN_FLOW_SOURCE"] = "otro" },
		"u3 con fases fake":  func(m map[string]string) { m["RUN_PHASES"], m["RUN_ALLOW_FAKE_PHASES"] = "fake", "true" },
	} {
		m := realEnv()
		m["RUN_FLOW_SOURCE"], m["U3_URL"] = "u3", "http://planner.u3:8080"
		mut(m)
		if _, err := loadConfig(envOf(m)); err == nil {
			t.Errorf("%s: aceptado", name)
		}
	}
}

func TestLoadConfigFlowSourceFakeStillGuarded(t *testing.T) {
	m := realEnv()
	m["RUN_FLOW_SOURCE"] = "fake"
	if _, err := loadConfig(envOf(m)); err == nil {
		t.Fatal("RUN_FLOW_SOURCE=fake sin RUN_ALLOW_FAKE_PHASES aceptado")
	}
	m["RUN_ALLOW_FAKE_PHASES"] = "true"
	if _, err := loadConfig(envOf(m)); err != nil {
		t.Fatal(err)
	}
	m["RUN_ENV"] = "prod"
	if _, err := loadConfig(envOf(m)); err == nil {
		t.Fatal("RUN_FLOW_SOURCE=fake con RUN_ENV=prod aceptado")
	}
}

func TestBuildConfigU3WiresFlowSourceAndFakeStaysFailClosedByDefault(t *testing.T) {
	m := realEnv()
	m["RUN_FLOW_SOURCE"], m["U3_URL"] = "u3", "http://planner.u3:8080"
	c, err := loadConfig(envOf(m))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildConfig(c, fake.NewSimpleClientset(), runctl.AllowAll(), runctl.NewMemStore(), &runctl.FakePublisher{}, &runctl.FakeAlerter{}, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := newFlowSource(c, nil, runctl.NewMemStore()); ok {
		t.Fatal("sin cliente de warm no puede haber fuente u3")
	}
	if fs, _ := newFlowSource(realCfg(t), nil, runctl.NewMemStore()); fs == nil {
		t.Fatal("la fuente por defecto no puede ser nil")
	} else if _, err := fs.Flows("r1"); err == nil {
		t.Fatal("la fuente por defecto debe fallar cerrado")
	}
}

func realCfg(t *testing.T) config {
	t.Helper()
	c, err := loadConfig(envOf(realEnv()))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestFlowSourceU3WorkflowFallbackAndLimit(t *testing.T) {
	m := realEnv()
	m["RUN_FLOW_SOURCE"], m["U3_URL"], m["U3_DEFAULT_WORKFLOW"] = "u3", "http://planner.u3:8080", "checkout"
	c, err := loadConfig(envOf(m))
	if err != nil || c.u3Workflow != "checkout" {
		t.Fatalf("%+v %v", c, err)
	}
	m["U3_DEFAULT_WORKFLOW"] = strings.Repeat("x", 129)
	if _, err := loadConfig(envOf(m)); err == nil {
		t.Fatal("workflow de 129 caracteres aceptado")
	}
}
