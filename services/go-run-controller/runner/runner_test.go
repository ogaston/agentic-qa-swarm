package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"pgregory.net/rapid"
	"sigs.k8s.io/yaml"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/adapters"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/plan"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

const ns = "aqs-test"

var t0 = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

type planSrc struct {
	flows []string
	err   error
}

func (p planSrc) Flows(run string) (plan.FlowPlan, error) {
	if p.err != nil {
		return plan.FlowPlan{}, p.err
	}
	fp := plan.FlowPlan{RunID: run, Workflow: "checkout"}
	for _, f := range p.flows {
		fp.Flows = append(fp.Flows, plan.Flow{FlowID: f, Name: f, Invariant: "inv", Steps: []plan.Step{{Method: "GET", Path: "/x", ExpectStatus: 200}}})
	}
	return fp, nil
}

type policyFn func() (Limits, error)

func (f policyFn) Limits(context.Context, string) (Limits, error) { return f() }

type rig struct {
	l   *Launcher
	cs  *fake.Clientset
	ev  *MemEvidence
	clk *clock
	gt  *runctl.FakeGate
}

func newRig(flows ...string) *rig {
	cs := fake.NewSimpleClientset()
	clk := &clock{t: t0}
	ev := &MemEvidence{}
	gt := runctl.AllowAll()
	l := &Launcher{Kube: ClientGo{CS: cs, NS: ns}, Plans: planSrc{flows: flows}, Exec: FakeExecutor{}, Evidence: ev, Gate: gt,
		Policy: StaticPolicy{L: Limits{FlowTimeout: time.Minute, RunTimeout: 10 * time.Minute}}, MaxParallel: 3, Now: clk.now,
		Cfg: Config{Namespace: ns, Image: "ghcr.io/ogaston/aqs-runner:0.1.0", ServiceAccount: "aqs-runner"}}
	return &rig{l: l, cs: cs, ev: ev, clk: clk, gt: gt}
}

func passedRun(id string) runctl.Run {
	return runctl.Run{ID: id, State: runctl.Running, EnsayoPassed: runctl.True, ConfirmedBy: "u", TraceID: "t-" + id}
}

func (r *rig) jobs(t testing.TB) []batchv1.Job {
	t.Helper()
	l, err := r.cs.BatchV1().Jobs(ns).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return l.Items
}

func (r *rig) active(t testing.TB) (n int) {
	for _, j := range r.jobs(t) {
		if d, _ := jobState(j); !d {
			n++
		}
	}
	return
}

// aliveWithinDeadline cuenta los Jobs sin terminar que aún no pasaron su activeDeadlineSeconds: Kubernetes
// mata el pod de un Job vencido aunque el controlador conserve el objeto hasta resolver su evidencia.
func (r *rig) aliveWithinDeadline(t testing.TB) (n int) {
	for _, j := range r.jobs(t) {
		created, _ := time.Parse(time.RFC3339, j.Annotations[AnnotCreated])
		if d, _ := jobState(j); !d && r.clk.t.Sub(created) <= time.Duration(*j.Spec.ActiveDeadlineSeconds)*time.Second {
			n++
		}
	}
	return
}

func (r *rig) finish(t testing.TB, name string, ok bool) {
	t.Helper()
	j, err := r.cs.BatchV1().Jobs(ns).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	typ := batchv1.JobFailed
	if ok {
		typ = batchv1.JobComplete
	}
	j.Status.Conditions = []batchv1.JobCondition{{Type: typ, Status: corev1.ConditionTrue}}
	if _, err := r.cs.BatchV1().Jobs(ns).Update(context.Background(), j, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
}

// --- constructor del Job ---

func goodCfg() Config {
	return Config{Namespace: ns, Image: "ghcr.io/ogaston/aqs-runner:0.1.0", ServiceAccount: "aqs-runner", DeadlineSeconds: 60}
}

func TestRunnerBuildJobIsHardenedAndHasNoCredentials(t *testing.T) {
	spec, _ := HTTPStepsExecutor{Target: "http://warm-app.aqs-test.svc:8080"}.Plan(plan.Flow{FlowID: "checkout", Invariant: "i", Steps: []plan.Step{{Method: "GET", Path: "/x", ExpectStatus: 200}}})
	j, err := BuildJob(goodCfg(), "r-1", "checkout", spec, t0)
	if err != nil {
		t.Fatal(err)
	}
	p, c := j.Spec.Template.Spec, j.Spec.Template.Spec.Containers[0]
	if j.Name != "runner-r-1-checkout" || j.Namespace != ns || j.Labels[LabelRun] != "r-1" || j.Labels[LabelFlow] != "checkout" || j.Labels[LabelPhase] != "run" {
		t.Errorf("identidad %v", j.ObjectMeta)
	}
	if *p.AutomountServiceAccountToken || p.ServiceAccountName != "aqs-runner" || !*p.SecurityContext.RunAsNonRoot || *p.SecurityContext.RunAsUser == 0 {
		t.Error("cuenta o usuario")
	}
	if !*c.SecurityContext.ReadOnlyRootFilesystem || *c.SecurityContext.AllowPrivilegeEscalation || len(c.Resources.Limits) != 2 || len(c.Resources.Requests) != 2 {
		t.Error("contenedor")
	}
	if *j.Spec.ActiveDeadlineSeconds != 60 || *j.Spec.BackoffLimit != 0 || strings.HasSuffix(c.Image, ":latest") {
		t.Error("plazo, reintentos o imagen")
	}
	if len(c.EnvFrom) != 0 || len(p.Volumes) != 0 {
		t.Error("envFrom o volúmenes")
	}
	y, _ := yaml.Marshal(j)
	for _, bad := range []string{"secretKeyRef", "envFrom", "LLM", "API_KEY", "PASSWORD"} {
		if strings.Contains(string(y), bad) {
			t.Errorf("contiene %q", bad)
		}
	}
}

func TestRunnerBuildJobRejectsBadConfigAndIDs(t *testing.T) {
	for name, mut := range map[string]func(*Config){
		"latest": func(c *Config) { c.Image = "ghcr.io/x/y:latest" }, "sin tag": func(c *Config) { c.Image = "ghcr.io/x/y" },
		"sin SA": func(c *Config) { c.ServiceAccount = "" }, "sin plazo": func(c *Config) { c.DeadlineSeconds = 0 },
	} {
		c := goodCfg()
		mut(&c)
		if _, err := BuildJob(c, "r-1", "f", JobSpec{}, t0); !errors.Is(err, ErrConfig) {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, id := range []string{"", "R_1", "../x", strings.Repeat("a", 45)} {
		if _, err := BuildJob(goodCfg(), id, "f", JobSpec{}, t0); err == nil {
			t.Errorf("run %q aceptado", id)
		}
		if _, err := BuildJob(goodCfg(), "r-1", id, JobSpec{}, t0); err == nil {
			t.Errorf("flow %q aceptado", id)
		}
	}
	if _, err := BuildJob(goodCfg(), strings.Repeat("a", 30), strings.Repeat("b", 30), JobSpec{}, t0); err == nil {
		t.Error("nombre de más de 63 caracteres aceptado")
	}
}

func TestRunnerNoSensitiveEnv(t *testing.T) {
	for _, name := range []string{"OPENAI_API_KEY", "API_TOKEN", "MY_SECRET", "DB_PASSWORD", "LLM_URL", "anthropic_api_key", "GithubToken", ""} {
		_, err := BuildJob(goodCfg(), "r-1", "f", JobSpec{Env: []EnvVar{{name, "x"}}}, t0)
		if !errors.Is(err, ErrSensitiveEnv) {
			t.Errorf("env %q aceptado: %v", name, err)
		}
	}
	if _, err := BuildJob(goodCfg(), "r-1", "f", JobSpec{EnvFrom: []string{"mi-secret"}}, t0); !errors.Is(err, ErrSensitiveEnv) {
		t.Errorf("envFrom aceptado: %v", err)
	}
	if _, err := BuildJob(goodCfg(), "r-1", "f", JobSpec{Env: []EnvVar{{"RUNNER_STEPS", "[]"}, {"TARGET_URL", "http://x"}}}, t0); err != nil {
		t.Errorf("env no sensible rechazado: %v", err)
	}
	// Inyección por el Executor dentro del Launcher: se rechaza y no se crea ningún Job.
	r := newRig("checkout")
	r.l.Exec = FakeExecutor{Spec: JobSpec{Args: []string{"x"}, Env: []EnvVar{{"OPENAI_API_KEY", "sk-x"}}}}
	if _, err := r.l.Launch(context.Background(), runctl.PhaseRun, passedRun("r-1")); !errors.Is(err, ErrSensitiveEnv) {
		t.Fatalf("err=%v", err)
	}
	r.l.Exec = FakeExecutor{Spec: JobSpec{Args: []string{"x"}, EnvFrom: []string{"s"}}}
	if _, err := r.l.Launch(context.Background(), runctl.PhaseRun, passedRun("r-1")); !errors.Is(err, ErrSensitiveEnv) {
		t.Fatalf("err=%v", err)
	}
	if n := len(r.jobs(t)); n != 0 {
		t.Fatalf("%d Jobs con env sensible", n)
	}
}

// --- sin ensayo, cuota, política ---

func TestRunnerNeedsEnsayo(t *testing.T) {
	for _, f := range []runctl.Fact{"", runctl.Unknown, runctl.False} {
		r := newRig("a")
		run := passedRun("r-1")
		run.EnsayoPassed = f
		if _, err := r.l.Launch(context.Background(), runctl.PhaseRun, run); err == nil {
			t.Errorf("ensayo=%q: lanzó", f)
		}
		if _, err := r.l.Progress(context.Background(), run); err == nil {
			t.Errorf("ensayo=%q: Progress avanzó", f)
		}
		if n := len(r.jobs(t)); n != 0 || len(r.gt.Calls) != 0 {
			t.Errorf("ensayo=%q: %d Jobs, %d llamadas al gate", f, n, len(r.gt.Calls))
		}
	}
	r := newRig("a")
	if _, err := r.l.Launch(context.Background(), runctl.PhaseReset, passedRun("r-1")); err == nil {
		t.Error("fase ajena aceptada")
	}
}

func TestRunnerQuotaDenied(t *testing.T) {
	for name, dec := range map[string]func(runctl.GateRequest) (runctl.GateDecision, error){
		"deniega": func(runctl.GateRequest) (runctl.GateDecision, error) {
			return runctl.GateDecision{Reason: "cuota"}, nil
		},
		"error": func(runctl.GateRequest) (runctl.GateDecision, error) {
			return runctl.GateDecision{}, errors.New("caído")
		},
	} {
		r := newRig("a", "b")
		r.l.Gate = &runctl.FakeGate{Decide: dec}
		if _, err := r.l.Launch(context.Background(), runctl.PhaseRun, passedRun("r-1")); err == nil {
			t.Errorf("%s: lanzó", name)
		}
		if n := len(r.jobs(t)); n != 0 {
			t.Errorf("%s: %d Jobs", name, n)
		}
	}
	r := newRig("a")
	if _, err := r.l.Launch(context.Background(), runctl.PhaseRun, passedRun("r-1")); err != nil {
		t.Fatal(err)
	}
	g := r.gt.Calls[0]
	if g.Workflow != "checkout" || g.To != runctl.Running || g.EnsayoPassed != runctl.True || g.TargetNamespace != ns {
		t.Fatalf("petición al gate %+v", g)
	}
}

func TestRunnerPolicyUnreadableLaunchesNothing(t *testing.T) {
	r := newRig("a")
	r.l.Policy = policyFn(func() (Limits, error) { return Limits{}, errors.New("sin política") })
	if _, err := r.l.Launch(context.Background(), runctl.PhaseRun, passedRun("r-1")); err == nil || len(r.jobs(t)) != 0 || len(r.gt.Calls) != 0 {
		t.Fatalf("lanzó con la política ilegible: %v", err)
	}
	r.l.Policy = StaticPolicy{} // plazos en cero: ilegible
	if _, err := r.l.Launch(context.Background(), runctl.PhaseRun, passedRun("r-1")); err == nil || len(r.jobs(t)) != 0 {
		t.Fatal("lanzó con plazos en cero")
	}
	r2 := newRig()
	r2.l.Plans = planSrc{err: errors.New("sin plan")}
	if _, err := r2.l.Launch(context.Background(), runctl.PhaseRun, passedRun("r-1")); err == nil || len(r2.jobs(t)) != 0 {
		t.Fatal("lanzó sin plan")
	}
	r3 := newRig("a", "a") // plan inválido (flow_id repetido es válido; plan vacío no)
	r3.l.Plans = planSrc{flows: nil}
	if _, err := r3.l.Launch(context.Background(), runctl.PhaseRun, passedRun("r-1")); err == nil || len(r3.jobs(t)) != 0 {
		t.Fatal("lanzó con plan vacío")
	}
}

// --- idempotencia y concurrencia ---

func TestRunnerLaunchIsIdempotentPerRunAndFlowAndAdopts(t *testing.T) {
	r := newRig("a", "b")
	for i := 1; i <= 3; i++ {
		run := passedRun("r-1")
		run.Started = map[string]int{"run": i}
		if _, err := r.l.Launch(context.Background(), runctl.PhaseRun, run); err != nil {
			t.Fatal(err)
		}
	}
	if js := r.jobs(t); len(js) != 2 {
		t.Fatalf("debe haber un Job por flujo: %d", len(js))
	}
	if len(r.gt.Calls) != 1 {
		t.Fatalf("la cuota se consulta una vez por corrida: %d", len(r.gt.Calls))
	}
	// AlreadyExists cuenta como éxito: un Job con el nombre pero que ListJobs no ve (otra etiqueta).
	r2 := newRig("a")
	_, _ = r2.cs.BatchV1().Jobs(ns).Create(context.Background(), &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "runner-r-1-a", Namespace: ns}}, metav1.CreateOptions{})
	if _, err := r2.l.Launch(context.Background(), runctl.PhaseRun, passedRun("r-1")); err != nil {
		t.Fatalf("AlreadyExists no es éxito: %v", err)
	}
}

func TestRunnerMaxParallelNeverExceeded(t *testing.T) {
	r := newRig("a", "b", "c", "d", "e")
	r.l.MaxParallel = 2
	run := passedRun("r-1")
	for i := 0; i < 40; i++ {
		out, err := r.l.Progress(context.Background(), run)
		if err != nil {
			t.Fatal(err)
		}
		if a := r.active(t); a > 2 {
			t.Fatalf("%d Jobs activos con máximo 2", a)
		}
		if out.Done {
			if len(out.URIs) != 10 || out.FailReason != "" {
				t.Fatalf("%+v", out)
			}
			return
		}
		for _, j := range r.jobs(t) {
			if d, _ := jobState(j); !d {
				r.finish(t, j.Name, true)
				break
			}
		}
	}
	t.Fatal("no terminó")
}

// --- timeouts ---

func TestRunnerTimeoutDeletesJobAndStoresResult(t *testing.T) {
	r := newRig("a", "b")
	run := passedRun("r-1")
	if _, err := r.l.Progress(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	r.clk.t = t0.Add(2 * time.Minute) // > 1 min por flujo
	out, err := r.l.Progress(context.Background(), run)
	if err != nil || !out.Done || out.FailReason != "" || len(out.URIs) != 4 {
		t.Fatalf("%+v %v", out, err)
	}
	if n := len(r.jobs(t)); n != 0 {
		t.Fatalf("quedan %d Jobs colgados", n)
	}
	b, _ := r.ev.Get(context.Background(), "runs/r-1/a/result.json")
	if !strings.Contains(string(b), `"timeout"`) {
		t.Fatalf("resultado %s", b)
	}
}

func TestRunnerRunTimeoutBoundsPendingFlows(t *testing.T) {
	r := newRig("a", "b", "c")
	r.l.MaxParallel = 1
	r.l.Policy = StaticPolicy{L: Limits{FlowTimeout: time.Hour, RunTimeout: time.Minute}}
	run := passedRun("r-1")
	_, _ = r.l.Progress(context.Background(), run) // lanza solo a
	r.clk.t = t0.Add(5 * time.Minute)
	out, err := r.l.Progress(context.Background(), run)
	if err != nil || !out.Done || len(out.URIs) != 6 || len(r.jobs(t)) != 0 {
		t.Fatalf("%+v %v jobs=%d", out, err, len(r.jobs(t)))
	}
	if r.active(t) != 0 {
		t.Fatal("Job activo tras el plazo de la corrida")
	}
}

// --- evidencia ---

func TestEvidenceWriteFailureMakesFlowFailedAndNeverClaimsIt(t *testing.T) {
	for name, mut := range map[string]func(*MemEvidence){
		"put falla": func(e *MemEvidence) {
			e.FailPut = func(k string) error {
				if notMarker(k) { // el marcador de inicio ya se escribió al lanzar
					return errors.New("bucket inexistente")
				}
				return nil
			}
		},
		"hash distinto": func(e *MemEvidence) {
			e.Corrupt = func(k string) []byte {
				if strings.HasSuffix(k, "logs.txt") {
					return []byte("otro")
				}
				return nil
			}
		},
		"result no sube": func(e *MemEvidence) {
			e.FailPut = func(k string) error {
				if strings.HasSuffix(k, "result.json") {
					return errors.New("x")
				}
				return nil
			}
		},
	} {
		r := newRig("a")
		mut(r.ev)
		run := passedRun("r-1")
		_, _ = r.l.Progress(context.Background(), run)
		r.finish(t, "runner-r-1-a", true)
		out, err := r.l.Progress(context.Background(), run)
		if err != nil || !out.Done || out.FailReason == "" || len(out.URIs) != 0 {
			t.Errorf("%s: %+v %v", name, out, err)
		}
		if _, err := r.ev.Get(context.Background(), "runs/r-1/a/result.json"); err == nil {
			t.Errorf("%s: result.json existe", name)
		}
	}
}

func TestEvidenceHashVerificationRejectsDifferentReadBack(t *testing.T) {
	ev := &MemEvidence{Corrupt: func(string) []byte { return []byte("mala") }}
	if _, err := PutVerified(context.Background(), ev, "k", []byte("buena")); !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("%v", err)
	}
	ev = &MemEvidence{GetErr: func(string) error { return errors.New("lectura") }}
	if _, err := PutVerified(context.Background(), ev, "k", []byte("x")); err == nil {
		t.Fatal("lectura fallida aceptada")
	}
	ev = &MemEvidence{}
	if u, err := PutVerified(context.Background(), ev, "runs/r/f/x", []byte("x")); err != nil || u != "s3://evidence/runs/r/f/x" {
		t.Fatalf("%q %v", u, err)
	}
}

func TestEvidenceUnreadableResultIsNeverAssumedMissingOrDone(t *testing.T) {
	r := newRig("a")
	r.ev.GetErr = func(string) error { return errors.New("minio caído") }
	if _, err := r.l.Progress(context.Background(), passedRun("r-1")); err == nil || len(r.jobs(t)) != 0 {
		t.Fatalf("avanzó con el almacén ilegible: %v", err)
	}
}

// --- run.done ---

func runDoneSchema(t testing.TB) *jsonschema.Schema {
	t.Helper()
	b, err := os.ReadFile("../../../contracts/events/run.done.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	_ = json.Unmarshal(b, &doc)
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()
	_ = c.AddResource("mem:///r.json", doc)
	sc, err := c.Compile("mem:///r.json")
	if err != nil {
		t.Fatal(err)
	}
	return sc
}

func eviSchema(t testing.TB) *jsonschema.Schema {
	b, err := os.ReadFile("../../../contracts/plans/evidence-uris.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	_ = json.Unmarshal(b, &doc)
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()
	_ = c.AddResource("mem:///e.json", doc)
	sc, err := c.Compile("mem:///e.json")
	if err != nil {
		t.Fatal(err)
	}
	return sc
}

type mux struct {
	ph *runctl.FakePhases
	rl *Launcher
}

func (m mux) Launch(ctx context.Context, phase string, run runctl.Run) ([]string, error) {
	if phase == runctl.PhaseRun {
		return m.rl.Launch(ctx, phase, run)
	}
	return m.ph.Launch(ctx, phase, run)
}

type ctlRig struct {
	*rig
	k     *runctl.Controller
	store *runctl.MemStore
	pub   *runctl.FakePublisher
	seen  []runctl.State
}

func newCtl(t testing.TB, r *rig, st runctl.State) *ctlRig {
	t.Helper()
	c := &ctlRig{rig: r, store: runctl.NewMemStore(), pub: &runctl.FakePublisher{}}
	k, err := runctl.New(runctl.Config{Gate: r.gt, Store: c.store, Publisher: c.pub, Warm: &runctl.FakeWarm{Fact: runctl.True}, Alerter: &runctl.FakeAlerter{},
		Phases: mux{&runctl.FakePhases{}, r.l}, Runners: r.l})
	if err != nil {
		t.Fatal(err)
	}
	c.k = k
	run := passedRun("r-1")
	run.State = st
	if st == runctl.Rehearsing {
		run.Launched = map[string]bool{runctl.PhaseRehearse: true}
	}
	run.Flows = []string{"f"}
	_ = c.store.Save(run)
	return c
}

func (c *ctlRig) step(t testing.TB) {
	c.k.DriveAll(context.Background())
	r, _ := c.store.Get("r-1")
	c.seen = append(c.seen, r.State)
}

func (c *ctlRig) state() runctl.State { r, _ := c.store.Get("r-1"); return r.State }

func has(s []runctl.State, x runctl.State) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}

func TestFlowToRunDone(t *testing.T) {
	r := newRig("checkout")
	c := newCtl(t, r, runctl.Rehearsing)
	for i := 0; i < 20 && c.state() != runctl.Done; i++ {
		c.step(t)
		for _, j := range r.jobs(t) {
			if d, _ := jobState(j); !d {
				r.finish(t, j.Name, true)
			}
		}
	}
	if !has(c.seen, runctl.Running) || !has(c.seen, runctl.Resetting) || c.state() != runctl.Done {
		t.Fatalf("estados %v", c.seen)
	}
	if len(c.pub.Events) != 1 || c.pub.Events[0].Type != "run.done" || len(c.pub.Events[0].EvidenceURIs) != 2 {
		t.Fatalf("eventos %+v", c.pub.Events)
	}
	// run.done sale ANTES de resetting: nunca después de salir de running.
	run, _ := c.store.Get("r-1")
	if !run.DonePublish {
		t.Fatal("DonePublish")
	}
	if len(r.jobs(t)) != 1 {
		t.Fatal("un Job por flujo")
	}
}

func TestFlowMultiFlow(t *testing.T) {
	r := newRig("a", "b", "c")
	r.l.MaxParallel = 2
	c := newCtl(t, r, runctl.Running)
	for i := 0; i < 40 && c.state() != runctl.Done; i++ {
		c.step(t)
		if a := r.active(t); a > 2 {
			t.Fatalf("%d Jobs activos con máximo 2", a)
		}
		for _, j := range r.jobs(t) {
			if d, _ := jobState(j); !d {
				r.finish(t, j.Name, true)
				break // uno por paso: se ve la ola
			}
		}
	}
	if c.state() != runctl.Done || len(c.pub.Events) != 1 {
		t.Fatalf("estado %s eventos %d", c.state(), len(c.pub.Events))
	}
	uris := c.pub.Events[0].EvidenceURIs
	if len(uris) != 6 {
		t.Fatalf("URIs %v", uris)
	}
	for _, f := range []string{"a", "b", "c"} {
		n := 0
		for _, u := range uris {
			if strings.Contains(u, "/r-1/"+f+"/") {
				n++
			}
		}
		if n != 2 {
			t.Errorf("flujo %s: %d URIs", f, n)
		}
	}
	// las URIs devueltas son válidas contra evidence-uris.schema.json
	raw, _ := json.Marshal(plan.EvidenceURIs{RunID: "r-1", URIs: uris})
	var v any
	_ = json.Unmarshal(raw, &v)
	if err := eviSchema(t).Validate(v); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerFailureToReset(t *testing.T) {
	// El runner falla: la evidencia del fallo se guarda y la corrida igual publica run.done y llega a resetting.
	r := newRig("a")
	c := newCtl(t, r, runctl.Running)
	for i := 0; i < 20 && c.state() != runctl.Done; i++ {
		c.step(t)
		for _, j := range r.jobs(t) {
			if d, _ := jobState(j); !d {
				r.finish(t, j.Name, false)
			}
		}
	}
	if !has(c.seen, runctl.Resetting) || len(c.pub.Events) != 1 {
		t.Fatalf("estados %v eventos %d", c.seen, len(c.pub.Events))
	}
	b, _ := r.ev.Get(context.Background(), "runs/r-1/a/result.json")
	if !strings.Contains(string(b), `"failed"`) {
		t.Fatalf("resultado %s", b)
	}
	// La evidencia no se pudo escribir: sin run.done, la corrida pasa por resetting y termina failed.
	r = newRig("a")
	r.ev.FailPut = func(string) error { return errors.New("bucket inexistente") }
	c = newCtl(t, r, runctl.Running)
	for i := 0; i < 20 && c.state() != runctl.Failed; i++ {
		c.step(t)
		for _, j := range r.jobs(t) {
			if d, _ := jobState(j); !d {
				r.finish(t, j.Name, true)
			}
		}
	}
	if c.state() != runctl.Failed || !has(c.seen, runctl.Resetting) || len(c.pub.Events) != 0 {
		t.Fatalf("estados %v eventos %d", c.seen, len(c.pub.Events))
	}
}

func TestRunnerTimeoutToReset(t *testing.T) {
	r := newRig("a")
	c := newCtl(t, r, runctl.Running)
	c.step(t) // lanza
	r.clk.t = t0.Add(time.Hour)
	for i := 0; i < 20 && c.state() != runctl.Done; i++ {
		c.step(t)
	}
	if !has(c.seen, runctl.Resetting) || c.state() != runctl.Done || len(r.jobs(t)) != 0 {
		t.Fatalf("estados %v jobs %d", c.seen, len(r.jobs(t)))
	}
}

func TestRunnerQuotaDeniedControllerLaunchesNoJobAndResets(t *testing.T) {
	r := newRig("a")
	gate := &runctl.FakeGate{Decide: func(req runctl.GateRequest) (runctl.GateDecision, error) {
		if req.Workflow != "" { // la consulta de cuota del lanzador
			return runctl.GateDecision{Reason: "cuota"}, nil
		}
		return runctl.GateDecision{Allow: true, Reason: "ok"}, nil
	}}
	r.l.Gate, r.gt = gate, gate
	c := newCtl(t, r, runctl.Running)
	for i := 0; i < 30 && c.state() != runctl.Failed; i++ {
		c.step(t)
	}
	if len(r.jobs(t)) != 0 || !has(c.seen, runctl.Resetting) || c.state() != runctl.Failed || len(c.pub.Events) != 0 {
		t.Fatalf("estados %v jobs %d eventos %d", c.seen, len(r.jobs(t)), len(c.pub.Events))
	}
}

func TestRunnerNeedsEnsayoController(t *testing.T) {
	r := newRig("a")
	c := newCtl(t, r, runctl.Running)
	run, _ := c.store.Get("r-1")
	run.EnsayoPassed = runctl.Unknown
	_ = c.store.Save(run)
	for i := 0; i < 10; i++ {
		c.step(t)
	}
	if len(r.jobs(t)) != 0 || !has(c.seen, runctl.Resetting) || len(c.pub.Events) != 0 {
		t.Fatalf("estados %v jobs %d", c.seen, len(r.jobs(t)))
	}
}

func TestRunDonePublishedAndValidAgainstSchema(t *testing.T) {
	r := newRig("a", "b")
	dir := t.TempDir()
	out := &adapters.Outbox{Path: dir + "/outbox.jsonl"}
	k, _ := runctl.New(runctl.Config{Gate: r.gt, Store: runctl.NewMemStore(), Publisher: out, Warm: &runctl.FakeWarm{Fact: runctl.True},
		Alerter: &runctl.FakeAlerter{}, Phases: mux{&runctl.FakePhases{}, r.l}, Runners: r.l})
	st := runctl.NewMemStore()
	_ = st.Save(func() runctl.Run { x := passedRun("r-1"); x.Flows = []string{"a"}; return x }())
	k, _ = runctl.New(runctl.Config{Gate: r.gt, Store: st, Publisher: out, Warm: &runctl.FakeWarm{Fact: runctl.True},
		Alerter: &runctl.FakeAlerter{}, Phases: mux{&runctl.FakePhases{}, r.l}, Runners: r.l})
	for i := 0; i < 20; i++ {
		k.DriveAll(context.Background())
		for _, j := range r.jobs(t) {
			if d, _ := jobState(j); !d {
				r.finish(t, j.Name, true)
			}
		}
	}
	b, err := os.ReadFile(out.Path)
	if err != nil || strings.Count(string(b), "\n") != 1 {
		t.Fatalf("outbox %q %v", b, err)
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	if err := runDoneSchema(t).Validate(v); err != nil {
		t.Fatalf("run.done inválido: %v", err)
	}
	if p := os.Getenv("RUNDONE_DUMP"); p != "" {
		_ = os.WriteFile(p, b, 0o600)
	}
}

type emptyRunners struct{}

func (emptyRunners) Progress(context.Context, runctl.Run) (runctl.RunOutcome, error) {
	return runctl.RunOutcome{Done: true}, nil
}

func TestRunDoneWithEmptyEvidenceIsNeverPublished(t *testing.T) {
	st, pub := runctl.NewMemStore(), &runctl.FakePublisher{}
	_ = st.Save(passedRun("r-1"))
	k, _ := runctl.New(runctl.Config{Gate: runctl.AllowAll(), Store: st, Publisher: pub, Warm: &runctl.FakeWarm{Fact: runctl.True},
		Alerter: &runctl.FakeAlerter{}, Phases: &runctl.FakePhases{}, Runners: emptyRunners{}})
	for i := 0; i < 10; i++ {
		k.DriveAll(context.Background())
	}
	if len(pub.Events) != 0 {
		t.Fatalf("run.done sin evidencia: %+v", pub.Events)
	}
	if r, _ := st.Get("r-1"); r.State == runctl.Running || r.FailReason == "" {
		t.Fatalf("%+v", r)
	}
}

func TestRunDoneProgressErrorNeitherAdvancesNorPublishes(t *testing.T) {
	r := newRig("a")
	c := newCtl(t, r, runctl.Running)
	c.step(t) // lanza
	r.finish(t, "runner-r-1-a", true)
	r.ev.GetErr = func(string) error { return errors.New("minio caído") }
	for i := 0; i < 10; i++ {
		c.step(t)
	}
	if c.state() != runctl.Running || len(c.pub.Events) != 0 {
		t.Fatalf("estado %s", c.state())
	}
}

// --- propiedad ---

func TestRunnerProperty(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		n := rapid.IntRange(1, 5).Draw(rt, "flujos")
		max := rapid.IntRange(1, 3).Draw(rt, "max")
		var flows []string
		for i := 0; i < n; i++ {
			flows = append(flows, fmt.Sprintf("f%d", i))
		}
		r := newRig(flows...)
		r.l.MaxParallel = max
		failEv := map[string]bool{}
		for _, f := range flows {
			failEv[f] = rapid.Bool().Draw(rt, "falla "+f)
		}
		r.ev.FailPut = func(k string) error {
			for f, bad := range failEv {
				if bad && strings.Contains(k, "/"+f+"/") {
					return errors.New("x")
				}
			}
			return nil
		}
		run := passedRun("r-1")
		for i := 0; i < 60; i++ {
			out, err := r.l.Progress(context.Background(), run)
			if err != nil {
				rt.Fatal(err)
			}
			if a := r.aliveWithinDeadline(t); a > max {
				rt.Fatalf("%d activos con máximo %d", a, max)
			}
			if out.Done {
				bad := false
				for _, f := range flows {
					bad = bad || failEv[f]
					_, e1 := r.ev.Get(context.Background(), "runs/r-1/"+f+"/result.json")
					if e1 != nil && out.FailReason == "" {
						rt.Fatalf("Done sin FailReason y sin evidencia de %s", f)
					}
				}
				if bad != (out.FailReason != "") || (!bad && len(out.URIs) != 2*n) || (bad && len(out.URIs) == 2*n) {
					rt.Fatalf("bad=%v %+v", bad, out)
				}
				return
			}
			for _, j := range r.jobs(t) {
				if d, _ := jobState(j); !d {
					switch rapid.IntRange(0, 2).Draw(rt, "resultado") {
					case 0:
						r.finish(t, j.Name, true)
					case 1:
						r.finish(t, j.Name, false)
					default:
						r.clk.t = r.clk.t.Add(2 * time.Minute) // vence por timeout
					}
					break
				}
			}
		}
		rt.Fatal("no terminó")
	})
}
