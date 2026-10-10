package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/adapters"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/plan"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/rehearsal"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

// Recorrido local de la demostración de U2 (scripts/test/u2-demo-local.sh): todo con fakes de Kubernetes,
// un gate que aplica las reglas de go-governance y un stub HTTP de U3. No toca ningún clúster.

// demoPhases: la fase run ensucia el warm y la fase reset lo deja verificado (como go-reset real).
type demoPhases struct {
	*runctl.FakePhases
	warm *runctl.FakeWarm
}

func (d demoPhases) Launch(ctx context.Context, phase string, run runctl.Run) ([]string, error) {
	out, err := d.FakePhases.Launch(ctx, phase, run)
	switch {
	case err != nil:
	case phase == runctl.PhaseRun:
		d.warm.Fact = runctl.False
	case phase == runctl.PhaseReset:
		d.warm.Fact = runctl.True
	}
	return out, err
}

// governance replica las reglas del gate: confirm, namespace aqs-test, reset_verified y ensayo_passed.
func governance(calls *[]runctl.GateRequest) *runctl.FakeGate {
	return &runctl.FakeGate{Decide: func(q runctl.GateRequest) (runctl.GateDecision, error) {
		*calls = append(*calls, q)
		ok := q.Confirmed == runctl.True && q.TargetNamespace == "aqs-test"
		switch q.To {
		case runctl.WarmReady, runctl.Deploying:
			ok = ok && q.ResetVerified == runctl.True
		case runctl.Running:
			ok = ok && q.EnsayoPassed == runctl.True
		}
		return runctl.GateDecision{Allow: ok, Reason: "demo", AuditRef: "a"}, nil
	}}
}

type demoRig struct {
	store *runctl.MemStore
	warm  *runctl.FakeWarm
	ph    *runctl.FakePhases
	pub   *runctl.FakePublisher
	gate  runctl.GateClient
	ctl   *runctl.Controller
}

func newDemo(t *testing.T, gate runctl.GateClient, ns string, warm runctl.WarmStateReader) *demoRig {
	t.Helper()
	r := &demoRig{store: runctl.NewMemStore(), warm: &runctl.FakeWarm{Fact: runctl.True}, ph: &runctl.FakePhases{}, pub: &runctl.FakePublisher{}, gate: gate}
	if warm == nil {
		warm = r.warm
	}
	ctl, err := runctl.New(runctl.Config{Namespace: ns, Gate: gate, Store: r.store, Publisher: r.pub, Warm: warm,
		Alerter: &runctl.FakeAlerter{}, Phases: demoPhases{r.ph, r.warm}})
	if err != nil {
		t.Fatal(err)
	}
	r.ctl = ctl
	return r
}

func (r *demoRig) drive() {
	for range 30 {
		r.ctl.DriveAll(context.Background())
	}
}

func (r *demoRig) state(id string) runctl.State { x, _ := r.store.Get(id); return x.State }

func (r *demoRig) confirm(t *testing.T, id string) {
	t.Helper()
	if err := r.ctl.Apply(t.Context(), runctl.Event{Type: runctl.EvRunConfirmed, EventID: "c-" + id, TraceID: "t", RunID: id, ConfirmedBy: "u", Flows: []string{"checkout"}}); err != nil {
		t.Fatal(err)
	}
}

func (r *demoRig) pass(t *testing.T, id string) {
	t.Helper()
	if err := r.ctl.Apply(t.Context(), runctl.Event{Type: runctl.EvRehearsalPassed, EventID: "p-" + id, RunID: id}); err != nil {
		t.Fatal(err)
	}
}

// u3Stub sirve POST /v1/plan; mutate ajusta el plan antes de responder.
func u3Stub(t *testing.T, mutate func(*plan.FlowPlan)) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Surface  plan.SurfaceArtifact `json:"surface"`
			Workflow string               `json:"workflow"`
		}
		if r.URL.Path != "/v1/plan" || json.NewDecoder(r.Body).Decode(&in) != nil {
			http.Error(w, "{}", 400)
			return
		}
		fp := plan.FlowPlan{RunID: in.Surface.RunID, Workflow: in.Workflow, Flows: []plan.Flow{{FlowID: "f1", Name: "crear orden", Invariant: "total correcto",
			Steps: []plan.Step{{Method: "POST", Path: "/orders", ExpectStatus: 201}}}}}
		if mutate != nil {
			mutate(&fp)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(fp)
	}))
	t.Cleanup(s.Close)
	return s
}

func demoU3Launcher(t *testing.T, url string) (*rehearsal.Launcher, *fake.Clientset) {
	t.Helper()
	src, err := adapters.NewFlowSourceU3(url, func(_ context.Context, id string) (plan.SurfaceArtifact, error) {
		return plan.SurfaceArtifact{RunID: id, BaseURL: "http://warm-app.aqs-test.svc:8080", Source: "openapi",
			Endpoints: []plan.Endpoint{{Method: "POST", Path: "/orders"}}}, nil
	}, func(string) (string, bool) { return "checkout", true })
	if err != nil {
		t.Fatal(err)
	}
	cs := fake.NewSimpleClientset()
	return &rehearsal.Launcher{Kube: rehearsal.ClientGo{CS: cs, NS: "aqs-test"}, Plans: src,
		Cfg: rehearsal.Config{Namespace: "aqs-test", Image: "ghcr.io/ogaston/aqs-rehearsal:0.1.0", ServiceAccount: "aqs-runner", TargetURL: "http://warm-app.aqs-test.svc:8080"}}, cs
}

func demoJobs(t *testing.T, cs *fake.Clientset) int {
	t.Helper()
	l, err := cs.BatchV1().Jobs("aqs-test").List(t.Context(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return len(l.Items)
}

func TestDemoLocal_CaminoFeliz(t *testing.T) {
	var calls []runctl.GateRequest
	r := newDemo(t, governance(&calls), "", nil)
	r.confirm(t, "r-1")
	r.drive()
	if r.state("r-1") != runctl.Rehearsing {
		t.Fatalf("sin ensayo debe esperar en rehearsing, está en %s", r.state("r-1"))
	}
	// el ensayo toma el plan de U3 (stub) y crea su Job en aqs-test
	l, cs := demoU3Launcher(t, u3Stub(t, nil).URL)
	if _, err := l.Launch(t.Context(), runctl.PhaseRehearse, runctl.Run{ID: "r-1"}); err != nil || demoJobs(t, cs) != 1 {
		t.Fatalf("ensayo con plan de U3: %v jobs=%d", err, demoJobs(t, cs))
	}
	r.pass(t, "r-1")
	r.drive()
	if r.state("r-1") != runctl.Done || len(r.pub.Events) != 1 {
		t.Fatal(r.state("r-1"), r.pub.Events)
	}
	for _, q := range calls {
		if q.TargetNamespace != "aqs-test" {
			t.Fatalf("transición fuera de aqs-test: %+v", q)
		}
	}
}

func TestDemoLocal_ResetEntreCorridas(t *testing.T) {
	var calls []runctl.GateRequest
	r := newDemo(t, governance(&calls), "", nil)
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("r-%d", i)
		before := len(calls)
		r.confirm(t, id)
		r.drive()
		r.pass(t, id)
		r.drive()
		if r.state(id) != runctl.Done {
			t.Fatalf("corrida %d terminó en %s", i, r.state(id))
		}
		if r.warm.Fact != runctl.True {
			t.Fatalf("tras la corrida %d el warm no quedó con reset verificado", i)
		}
		started := false
		for _, q := range calls[before:] {
			if q.RunID == id && q.To == runctl.WarmReady {
				started = true
				if q.ResetVerified != runctl.True {
					t.Fatalf("corrida %d arrancó sin reset_verified=true: %+v", i, q)
				}
			}
		}
		if !started {
			t.Fatalf("corrida %d nunca pidió warm_ready", i)
		}
	}
}

func TestDemoLocal_FailClosedGobernanza(t *testing.T) {
	down := &runctl.FakeGate{Decide: func(runctl.GateRequest) (runctl.GateDecision, error) {
		return runctl.GateDecision{}, errors.New("go-governance detenido")
	}}
	r := newDemo(t, down, "", nil)
	r.confirm(t, "r-1")
	r.drive()
	if r.state("r-1") != runctl.Confirmed || len(r.ph.Launches) != 0 {
		t.Fatalf("con gobernanza caída avanzó: %s %v", r.state("r-1"), r.ph.Launches)
	}
	if err := r.ctl.Transition(t.Context(), "r-1", runctl.WarmReady); !errors.Is(err, runctl.ErrGate) {
		t.Fatal(err)
	}
}

func TestDemoLocal_CuarentenaPorDBSucia(t *testing.T) {
	// go-warm-manager real (httptest) informa cuarentena con reset_verified=false
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"warm_id":"w","state":"cuarentena","reset_verified":false,"baseline_version":"v1"}`))
	}))
	defer s.Close()
	wc, err := adapters.NewWarmClient(s.URL, "t")
	if err != nil {
		t.Fatal(err)
	}
	var calls []runctl.GateRequest
	r := newDemo(t, governance(&calls), "", wc)
	r.confirm(t, "r-1")
	r.drive()
	// la corrida no arranca: el gate niega warm_ready y ninguna fase de despliegue/ensayo/run se lanza
	if st := r.state("r-1"); (st != runctl.Confirmed && st != runctl.Failed) || r.ph.Launches[runctl.PhaseDeploy]+r.ph.Launches[runctl.PhaseRehearse]+r.ph.Launches[runctl.PhaseRun] != 0 {
		t.Fatalf("warm en cuarentena: la corrida avanzó a %s %v", st, r.ph.Launches)
	}
	for _, q := range calls {
		if q.ResetVerified == runctl.True {
			t.Fatalf("cuarentena informada como reset verificado: %+v", q)
		}
	}
}

func TestDemoLocal_BloqueoFueraDeAqsTest(t *testing.T) {
	// 1) el controlador no arranca apuntando a otro namespace
	m := good()
	m["RUN_TEST_NAMESPACE"] = "prod"
	if _, err := loadConfig(envOf(m)); err == nil {
		t.Fatal("namespace prod aceptado")
	}
	// 2) aunque se construya, el gate niega toda transición hacia otro namespace
	var calls []runctl.GateRequest
	r := newDemo(t, governance(&calls), "staging", nil)
	r.confirm(t, "r-1")
	r.drive()
	if r.state("r-1") != runctl.Confirmed || len(r.ph.Launches) != 0 {
		t.Fatalf("transición hacia staging avanzó: %s", r.state("r-1"))
	}
	// 3) un plan de U3 con un paso que sale del warm se bloquea antes de crear Jobs
	for _, p := range []string{"http://evil.example/x", "//evil.example/x"} {
		l, cs := demoU3Launcher(t, u3Stub(t, func(fp *plan.FlowPlan) { fp.Flows[0].Steps[0].Path = p }).URL)
		if _, err := l.Launch(t.Context(), runctl.PhaseRehearse, runctl.Run{ID: "r-1"}); err == nil || demoJobs(t, cs) != 0 {
			t.Fatalf("path %q: err=%v jobs=%d", p, err, demoJobs(t, cs))
		}
	}
}
