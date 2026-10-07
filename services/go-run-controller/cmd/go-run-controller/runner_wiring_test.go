package main

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/adapters"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/internal/obs"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/plan"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runner"
)

// Cableado de real: la fase run usa el lanzador de runners, el mismo es el RunnerResults, con el
// gate, el motor HTTP, la evidencia S3 y la configuración de las variables. Quitar cualquiera pone rojo.
func TestBuildConfigRealWiresRunnersAndEvidence(t *testing.T) {
	m := realEnv()
	m["RUN_MAX_PARALLEL_RUNNERS"] = "2"
	m["RUNNER_FLOW_TIMEOUT_SECONDS"], m["RUNNER_RUN_TIMEOUT_SECONDS"] = "45", "90"
	cfg, err := buildFor(t, m, fake.NewSimpleClientset())
	if err != nil {
		t.Fatal(err)
	}
	rp := cfg.Phases.(*adapters.RealPhases)
	rl, ok := cfg.Runners.(*runner.Launcher)
	if !ok || rp.Run == nil || rp.Run.(*runner.Launcher) != rl {
		t.Fatalf("Runners=%T Run=%v", cfg.Runners, rp.Run)
	}
	if _, ok := rl.Evidence.(*runner.S3); !ok {
		t.Fatalf("Evidence = %T", rl.Evidence)
	}
	if _, ok := rl.Exec.(runner.HTTPStepsExecutor); !ok || rl.Gate != cfg.Gate || rl.Plans == nil || rl.Kube == nil {
		t.Fatalf("puertos del lanzador: %+v", rl)
	}
	lim, err := rl.Policy.Limits(context.Background(), "w")
	if err != nil || lim.FlowTimeout != 45*time.Second || lim.RunTimeout != 90*time.Second || rl.MaxParallel != 2 {
		t.Fatalf("%+v %v max=%d", lim, err, rl.MaxParallel)
	}
	if rl.Cfg.Image != m["RUNNER_IMAGE"] || rl.Cfg.Namespace != "aqs-test" || rl.Cfg.ServiceAccount != "aqs-runner" {
		t.Fatalf("%+v", rl.Cfg)
	}
	// sin el plan de U3 el lanzador falla cerrado: ningún runner
	if _, err := rl.Launch(context.Background(), runctl.PhaseRun, runctl.Run{ID: "r-1", EnsayoPassed: runctl.True}); err == nil {
		t.Fatal("lanzó sin FlowPlan")
	}
}

func TestBuildConfigFakeHasNoRunnersAndRealFailsWithBadKeyFile(t *testing.T) {
	cfg, err := buildFor(t, good(), nil)
	if err != nil || cfg.Runners != nil {
		t.Fatalf("fake con Runners: %v %v", cfg.Runners, err)
	}
	m := realEnv()
	m["EVIDENCE_ACCESS_KEY_FILE"] = "/no/existe"
	if _, err := buildFor(t, m, fake.NewSimpleClientset()); err == nil || strings.Contains(err.Error(), testAK) {
		t.Fatalf("archivo de credenciales inexistente aceptado: %v", err)
	}
}

func TestLoadConfigEvidenceAndRunnerFailClosed(t *testing.T) {
	for name, mut := range map[string]func(map[string]string){
		"endpoint ftp (fake)": func(m map[string]string) {
			m["RUN_PHASES"], m["RUN_EVENTS_FILE"] = "fake", "e"
			m["EVIDENCE_ENDPOINT"] = "ftp://x"
		},
		"endpoint sin host":    func(m map[string]string) { m["EVIDENCE_ENDPOINT"] = "http://" },
		"sin endpoint":         func(m map[string]string) { delete(m, "EVIDENCE_ENDPOINT") },
		"sin bucket":           func(m map[string]string) { delete(m, "EVIDENCE_BUCKET") },
		"sin clave de acceso":  func(m map[string]string) { delete(m, "EVIDENCE_ACCESS_KEY_FILE") },
		"sin clave secreta":    func(m map[string]string) { delete(m, "EVIDENCE_SECRET_KEY_FILE") },
		"sin imagen runner":    func(m map[string]string) { delete(m, "RUNNER_IMAGE") },
		"imagen runner latest": func(m map[string]string) { m["RUNNER_IMAGE"] = "ghcr.io/x/y:latest" },
		"paralelo 0":           func(m map[string]string) { m["RUN_MAX_PARALLEL_RUNNERS"] = "0" },
		"paralelo texto":       func(m map[string]string) { m["RUN_MAX_PARALLEL_RUNNERS"] = "x" },
		"paralelo enorme":      func(m map[string]string) { m["RUN_MAX_PARALLEL_RUNNERS"] = "999" },
		"timeout flujo 0":      func(m map[string]string) { m["RUNNER_FLOW_TIMEOUT_SECONDS"] = "0" },
		"timeout corrida mal":  func(m map[string]string) { m["RUNNER_RUN_TIMEOUT_SECONDS"] = "-3" },
	} {
		m := realEnv()
		mut(m)
		if _, err := loadConfig(envOf(m)); err == nil {
			t.Errorf("%s: aceptado", name)
		}
	}
	c, err := loadConfig(envOf(realEnv()))
	if err != nil || c.maxParallel != 3 || c.flowTimeout != 300*time.Second || c.runTimeout != 900*time.Second {
		t.Fatalf("valores por defecto %+v %v", c, err)
	}
	// las vallas de fake siguen intactas con las variables nuevas
	m := good()
	m["RUN_ENV"] = " Production "
	if _, err := loadConfig(envOf(m)); err == nil {
		t.Error("fake aceptado en producción")
	}
	m = good()
	delete(m, "RUN_ALLOW_FAKE_PHASES")
	if _, err := loadConfig(envOf(m)); err == nil {
		t.Error("fake sin RUN_ALLOW_FAKE_PHASES")
	}
}

func TestRunnerRenderJobSubcommand(t *testing.T) {
	var out bytes.Buffer
	if err := renderRunnerJob(&out, []string{"--run", "r-1", "--flow", "checkout"}, envOf(map[string]string{})); err != nil {
		t.Fatal(err)
	}
	y := out.String()
	for _, want := range []string{"name: runner-r-1-checkout", "automountServiceAccountToken: false", "readOnlyRootFilesystem: true",
		"runAsNonRoot: true", "serviceAccountName: aqs-runner", "activeDeadlineSeconds: 300", "namespace: aqs-test"} {
		if !strings.Contains(y, want) {
			t.Errorf("falta %q", want)
		}
	}
	for _, bad := range []string{"secretKeyRef", "envFrom", "API_KEY", "LLM", "PASSWORD", ":latest"} {
		if strings.Contains(y, bad) {
			t.Errorf("contiene %q", bad)
		}
	}
	for _, args := range [][]string{nil, {"--run", "r-1"}, {"--run", "R_1", "--flow", "f"}, {"--run", "r", "--flow"}, {"--x", "1", "--flow", "f"}} {
		if err := renderRunnerJob(&out, args, envOf(map[string]string{})); err == nil {
			t.Errorf("args %v aceptados", args)
		}
	}
	if err := renderRunnerJob(&out, []string{"--run", "r-1", "--flow", "f"}, envOf(map[string]string{"RUNNER_IMAGE": "x:latest"})); err == nil {
		t.Error("imagen latest aceptada")
	}
}

type logKube struct {
	runner.KubeAPI
	logs []byte
}

func (l logKube) JobLogs(context.Context, string) ([]byte, error) { return l.logs, nil }

// Ni los logs ni /metrics contienen la clave de acceso, la secreta ni el contenido de la evidencia.
func TestEvidenceSecretsNotLogged(t *testing.T) {
	const content = "CONTENIDO-SECRETO-DE-LA-EVIDENCIA-9921"
	var logs bytes.Buffer
	log := obs.NewLogger(&logs, "go-run-controller", -4)
	reg := obs.NewRegistry()
	rm := obs.NewRunnerMetrics(reg)
	m := realEnv()
	m["EVIDENCE_ENDPOINT"] = "http://127.0.0.1:1" // inalcanzable: recorre la ruta de error del adaptador
	cs := fake.NewSimpleClientset()
	c, err := loadConfig(envOf(m))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := buildConfig(c, cs, runctl.AllowAll(), runctl.NewMemStore(), &runctl.FakePublisher{}, &runctl.FakeAlerter{}, nil, rm, log)
	if err != nil {
		t.Fatal(err)
	}
	rl := cfg.Runners.(*runner.Launcher)
	rl.Plans = oneFlow{}
	rl.Kube = logKube{KubeAPI: rl.Kube, logs: []byte(content)}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	run := runctl.Run{ID: "r-1", EnsayoPassed: runctl.True, TraceID: "t"}
	_, e1 := rl.Launch(ctx, runctl.PhaseRun, run) // el almacén inalcanzable corta antes de crear nada
	// Mismo recorrido con evidencia en memoria: el contenido llega a settle y no debe salir en logs ni métricas.
	rl.Evidence = &runner.MemEvidence{FailPut: func(string) error { return errString("falló " + content) }}
	rl.Kube = logKube{KubeAPI: runner.ClientGo{CS: cs, NS: "aqs-test"}, logs: []byte(content)}
	_, e2 := rl.Launch(ctx, runctl.PhaseRun, run)
	j, _ := cs.BatchV1().Jobs("aqs-test").Get(ctx, "runner-r-1-f1", metav1.GetOptions{})
	j.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}
	_, _ = cs.BatchV1().Jobs("aqs-test").Update(ctx, j, metav1.UpdateOptions{})
	_, e3 := rl.Progress(ctx, run)
	h := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body, _ := io.ReadAll(rec.Body)
	all := logs.String() + string(body)
	for _, e := range []error{e1, e2, e3} {
		if e != nil {
			all += e.Error()
		}
	}
	for _, secret := range []string{testAK, testSK, content} {
		if strings.Contains(all, secret) {
			t.Errorf("se filtró %q", secret)
		}
	}
	if !strings.Contains(string(body), `aqs_evidence_objects_total{result="error"} 1`) {
		t.Errorf("la métrica de evidencia fallida no se contó:\n%s", body)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

type oneFlow struct{}

func (oneFlow) Flows(run string) (plan.FlowPlan, error) {
	return plan.FlowPlan{RunID: run, Workflow: "w", Flows: []plan.Flow{{FlowID: "f1", Name: "f1", Invariant: "i",
		Steps: []plan.Step{{Method: "GET", Path: "/x", ExpectStatus: 200}}}}}, nil
}
