package rehearsal

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/plan"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

const ns = "aqs-test"

func cfg() Config {
	return Config{Namespace: ns, Image: "ghcr.io/ogaston/aqs-rehearsal:0.1.0", ServiceAccount: "aqs-runner", TargetURL: "http://warm-app.aqs-test.svc:8080"}
}

func goodFlow() plan.Flow {
	return plan.Flow{FlowID: "f1", Name: "n", Invariant: "inv", Steps: []plan.Step{{Method: "GET", Path: "/x", ExpectStatus: 200}}}
}

type planSrc struct {
	p   plan.FlowPlan
	err error
}

func (s planSrc) Flows(run string) (plan.FlowPlan, error) {
	p := s.p
	if p.RunID == "" && s.err == nil {
		p.RunID = run
	}
	return p, s.err
}

func goodPlan() planSrc {
	return planSrc{p: plan.FlowPlan{Workflow: "checkout", Flows: []plan.Flow{goodFlow()}}}
}

func newLauncher(src FlowSource) (*Launcher, *fake.Clientset) {
	cs := fake.NewSimpleClientset()
	return &Launcher{Kube: ClientGo{CS: cs, NS: ns}, Cfg: cfg(), Plans: src}, cs
}

func jobs(t tb, cs *fake.Clientset) []batchv1.Job {
	t.Helper()
	l, err := cs.BatchV1().Jobs(ns).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return l.Items
}

func count(js []batchv1.Job, prefix string) (n int) {
	for _, j := range js {
		if strings.HasPrefix(j.Name, prefix) {
			n++
		}
	}
	return
}

func finish(t tb, cs *fake.Clientset, name string, ok bool) {
	t.Helper()
	j, err := cs.BatchV1().Jobs(ns).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	typ := batchv1.JobFailed
	if ok {
		typ = batchv1.JobComplete
		j.Status.Succeeded = 1
	} else {
		j.Status.Failed = 1
	}
	j.Status.Conditions = []batchv1.JobCondition{{Type: typ, Status: corev1.ConditionTrue}}
	if _, err := cs.BatchV1().Jobs(ns).Update(context.Background(), j, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
}

// --- constructor del Job ---

func TestBuildJobHasNoCredentialsAndIsHardened(t *testing.T) {
	j, err := BuildJob(cfg(), "r-1", goodFlow(), 1)
	if err != nil {
		t.Fatal(err)
	}
	p, c := j.Spec.Template.Spec, j.Spec.Template.Spec.Containers[0]
	for _, e := range c.Env {
		u := strings.ToUpper(e.Name)
		if strings.Contains(u, "LLM") || strings.Contains(u, "API_KEY") || strings.Contains(u, "TOKEN") || e.ValueFrom != nil {
			t.Errorf("env prohibida %s", e.Name)
		}
	}
	if len(c.EnvFrom) != 0 || len(p.Volumes) != 0 {
		t.Error("envFrom o volúmenes")
	}
	if p.AutomountServiceAccountToken == nil || *p.AutomountServiceAccountToken || p.ServiceAccountName != "aqs-runner" {
		t.Error("SA/automount")
	}
	if p.SecurityContext.RunAsNonRoot == nil || !*p.SecurityContext.RunAsNonRoot || !*c.SecurityContext.ReadOnlyRootFilesystem || *c.SecurityContext.AllowPrivilegeEscalation {
		t.Error("securityContext")
	}
	if c.Resources.Limits.Cpu().IsZero() || c.Resources.Limits.Memory().IsZero() || c.Resources.Requests.Cpu().IsZero() || c.Resources.Requests.Memory().IsZero() {
		t.Error("resources incompletos")
	}
	if j.Spec.ActiveDeadlineSeconds == nil || *j.Spec.ActiveDeadlineSeconds != 120 || *j.Spec.BackoffLimit != 0 {
		t.Error("plazo o reintentos del Job")
	}
	if j.Name != "rehearsal-r-1-1" || j.Labels[LabelRun] != "r-1" || j.Labels[LabelPhase] != PhaseValue || j.Namespace != ns {
		t.Errorf("identidad %v", j.ObjectMeta)
	}
}

func TestBuildJobRejectsBadConfig(t *testing.T) {
	for name, mut := range map[string]func(*Config){
		"latest":      func(c *Config) { c.Image = "ghcr.io/x/y:latest" },
		"sin tag":     func(c *Config) { c.Image = "ghcr.io/x/y" },
		"vacía":       func(c *Config) { c.Image = "" },
		"sin SA":      func(c *Config) { c.ServiceAccount = "" },
		"sin destino": func(c *Config) { c.TargetURL = "" },
	} {
		c := cfg()
		mut(&c)
		if _, err := BuildJob(c, "r-1", goodFlow(), 1); !errors.Is(err, ErrConfig) {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, run := range []string{"", "R_1", "../x", strings.Repeat("a", 60)} {
		if _, err := BuildJob(cfg(), run, goodFlow(), 1); err == nil {
			t.Errorf("run %q aceptado", run)
		}
	}
	for _, a := range []int{0, 4, -1} {
		if _, err := BuildJob(cfg(), "r-1", goodFlow(), a); err == nil {
			t.Errorf("intento %d aceptado", a)
		}
	}
	c := cfg()
	c.Image = "ghcr.io/x/y@sha256:" + strings.Repeat("a", 64)
	if _, err := BuildJob(c, "r-1", goodFlow(), 1); err != nil {
		t.Errorf("digest rechazado: %v", err)
	}
}

// --- Launch idempotente por (corrida, fase) ---

func TestLaunchAdoptsLiveJobAndCreatesNewOneAfterFailure(t *testing.T) {
	l, cs := newLauncher(goodPlan())
	ctx := context.Background()
	if _, err := l.Launch(ctx, runctl.PhaseRehearse, runctl.Run{ID: "r-1", Started: map[string]int{"rehearse": 1}}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Launch(ctx, runctl.PhaseRehearse, runctl.Run{ID: "r-1", Started: map[string]int{"rehearse": 2}}); err != nil {
		t.Fatal(err)
	}
	if js := jobs(t, cs); len(js) != 1 || js[0].Name != "rehearsal-r-1-1" {
		t.Fatalf("debe haber UN Job vivo: %v", js)
	}
	finish(t, cs, "rehearsal-r-1-1", false)
	if _, err := l.Launch(ctx, runctl.PhaseRehearse, runctl.Run{ID: "r-1", Started: map[string]int{"rehearse": 2}}); err != nil {
		t.Fatal(err)
	}
	if js := jobs(t, cs); len(js) != 2 || js[1].Name != "rehearsal-r-1-2" {
		t.Fatalf("tras fallar debe crear el siguiente: %v", js)
	}
}

type alreadyExists struct{ KubeAPI }

func (alreadyExists) ListJobs(context.Context, string) ([]batchv1.Job, error) { return nil, nil }
func (alreadyExists) CreateJob(context.Context, *batchv1.Job) error {
	return apierrors.NewAlreadyExists(schema.GroupResource{Group: "batch", Resource: "jobs"}, "x")
}

type failingKube struct{ KubeAPI }

func (failingKube) ListJobs(context.Context, string) ([]batchv1.Job, error) {
	return nil, errors.New("api caída")
}

func TestLaunchAlreadyExistsIsSuccessAndKubeErrorsFailClosed(t *testing.T) {
	l, _ := newLauncher(goodPlan())
	l.Kube = alreadyExists{}
	if _, err := l.Launch(context.Background(), runctl.PhaseRehearse, runctl.Run{ID: "r-1", Started: map[string]int{"rehearse": 1}}); err != nil {
		t.Fatalf("AlreadyExists debe ser éxito: %v", err)
	}
	l.Kube = failingKube{}
	if _, err := l.Launch(context.Background(), runctl.PhaseRehearse, runctl.Run{ID: "r-1", Started: map[string]int{"rehearse": 1}}); err == nil {
		t.Fatal("un error al listar no puede ser éxito")
	}
	if _, err := l.Result(context.Background(), runctl.Run{ID: "r-1"}); err == nil {
		t.Fatal("un error al leer el resultado no puede ser éxito")
	}
	if _, err := l.Launch(context.Background(), runctl.PhaseRun, runctl.Run{ID: "r-1"}); err == nil {
		t.Fatal("otra fase")
	}
}

func TestLaunchBrokenPlanCreatesNoJob(t *testing.T) {
	for name, src := range map[string]planSrc{
		"vacío":        {p: plan.FlowPlan{Workflow: "w"}},
		"sin pasos":    {p: plan.FlowPlan{Workflow: "w", Flows: []plan.Flow{{FlowID: "f", Name: "n", Invariant: "i"}}}},
		"método":       {p: plan.FlowPlan{Workflow: "w", Flows: []plan.Flow{{FlowID: "f", Name: "n", Invariant: "i", Steps: []plan.Step{{Method: "TRACE", Path: "/", ExpectStatus: 200}}}}}},
		"error":        {err: errors.New("sin plan")},
		"otra corrida": {p: plan.FlowPlan{RunID: "otra", Workflow: "w", Flows: []plan.Flow{goodFlow()}}},
	} {
		l, cs := newLauncher(src)
		if _, err := l.Launch(context.Background(), runctl.PhaseRehearse, runctl.Run{ID: "r-1", Started: map[string]int{"rehearse": 1}}); err == nil {
			t.Errorf("%s: aceptado", name)
		}
		if n := len(jobs(t, cs)); n != 0 {
			t.Errorf("%s: %d Jobs creados con un plan roto", name, n)
		}
	}
}

func TestLaunchNeverCreatesFourthJob(t *testing.T) {
	l, cs := newLauncher(goodPlan())
	for i := 1; i <= 3; i++ {
		if _, err := l.Launch(context.Background(), runctl.PhaseRehearse, runctl.Run{ID: "r-1", Started: map[string]int{"rehearse": i}}); err != nil {
			t.Fatal(err)
		}
		finish(t, cs, JobName("r-1", i), false)
	}
	if _, err := l.Launch(context.Background(), runctl.PhaseRehearse, runctl.Run{ID: "r-1", Started: map[string]int{"rehearse": 4}}); err == nil {
		t.Fatal("cuarto Job aceptado")
	}
	if n := len(jobs(t, cs)); n != 3 {
		t.Fatalf("%d Jobs", n)
	}
}

// --- resultado ---

func TestResultFromJobStatus(t *testing.T) {
	l, cs := newLauncher(goodPlan())
	run := runctl.Run{ID: "r-1", Started: map[string]int{"rehearse": 1}}
	ctx := context.Background()
	if out, err := l.Result(ctx, run); err != nil || out.Done {
		t.Fatalf("sin Job: %+v %v", out, err)
	}
	_, _ = l.Launch(ctx, runctl.PhaseRehearse, run)
	if out, _ := l.Result(ctx, run); out.Done || out.Passed {
		t.Fatalf("Job corriendo no es resultado: %+v", out)
	}
	finish(t, cs, "rehearsal-r-1-1", false)
	if out, _ := l.Result(ctx, run); !out.Done || out.Passed || out.EventID == "" {
		t.Fatalf("failed: %+v", out)
	}
	run.Started["rehearse"] = 2
	_, _ = l.Launch(ctx, runctl.PhaseRehearse, run)
	if out, _ := l.Result(ctx, run); out.Done {
		t.Fatalf("el Job fallado anterior no debe contar mientras el nuevo corre: %+v", out)
	}
	finish(t, cs, "rehearsal-r-1-2", true)
	out, _ := l.Result(ctx, run)
	if !out.Done || !out.Passed {
		t.Fatalf("succeeded: %+v", out)
	}
	if out.EventID == EventID("r-1", "rehearsal-r-1-1") {
		t.Fatal("event_id debe diferir por Job")
	}
}

func TestEventJSONValidatesAgainstSchema(t *testing.T) {
	b, err := os.ReadFile("../../../contracts/events/rehearsal.schema.json")
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
	for _, passed := range []bool{true, false} {
		ev := EventJSON(runctl.Run{ID: "r-1"}, "f1", "rehearsal-r-1-1", runctl.RehearsalOutcome{Done: true, Passed: passed, EventID: EventID("r-1", "rehearsal-r-1-1")}, time.Now())
		raw, _ := json.Marshal(ev)
		var v any
		_ = json.Unmarshal(raw, &v)
		if err := sc.Validate(v); err != nil {
			t.Errorf("passed=%v: %v", passed, err)
		}
	}
}
