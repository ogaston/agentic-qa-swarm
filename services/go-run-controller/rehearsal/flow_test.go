package rehearsal

import (
	"context"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/plan"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

// mix lanza el ensayo con el Launcher real (sobre el clientset falso) y simula en el clientset
// el Job runner-* de U2-T05 para poder LEER de vuelta si alguna vez existe sin ensayo_passed.
type mix struct {
	rh    *Launcher
	cs    *fake.Clientset
	other *runctl.FakePhases
	store *runctl.MemStore
	bad   []string // runners lanzados sin ensayo_passed=true en el almacén
}

func (m *mix) Launch(ctx context.Context, phase string, run runctl.Run) ([]string, error) {
	switch phase {
	case runctl.PhaseRehearse:
		return m.rh.Launch(ctx, phase, run)
	case runctl.PhaseRun:
		if cur, _ := m.store.Get(run.ID); cur.EnsayoPassed != runctl.True {
			m.bad = append(m.bad, run.ID)
		}
		_, err := m.cs.BatchV1().Jobs(ns).Create(ctx, &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "runner-" + run.ID, Namespace: ns}}, metav1.CreateOptions{})
		return nil, err
	}
	return m.other.Launch(ctx, phase, run)
}

type env struct {
	k     *runctl.Controller
	m     *mix
	gate  *runctl.FakeGate
	store *runctl.MemStore
	al    *runctl.FakeAlerter
	cs    *fake.Clientset
	seen  []runctl.State
}

func newEnv(t tb, src FlowSource, other *runctl.FakePhases) *env {
	t.Helper()
	l, cs := newLauncher(src)
	store := runctl.NewMemStore()
	m := &mix{rh: l, cs: cs, other: other, store: store}
	e := &env{m: m, gate: runctl.AllowAll(), store: store, al: &runctl.FakeAlerter{}, cs: cs}
	k, err := runctl.New(runctl.Config{Gate: e.gate, Store: store, Publisher: &runctl.FakePublisher{}, Warm: &runctl.FakeWarm{Fact: runctl.True},
		Alerter: e.al, Phases: m, Results: l})
	if err != nil {
		t.Fatal(err)
	}
	e.k = k
	_ = store.Save(runctl.Run{ID: "r-1", State: runctl.Confirmed, ConfirmedBy: "u", Flows: []string{"f1"}})
	return e
}

func (e *env) state() runctl.State { r, _ := e.store.Get("r-1"); return r.State }

// step da un paso al controlador, resuelve los Jobs vivos con ok() y recuerda el estado visto.
func (e *env) step(t tb, ok func(name string) bool) {
	t.Helper()
	e.k.DriveAll(context.Background())
	e.seen = append(e.seen, e.state())
	for _, j := range jobs(t, e.cs) {
		if len(j.Status.Conditions) == 0 && j.Name[:2] == "re" {
			finish(t, e.cs, j.Name, ok(j.Name))
		}
	}
}

func (e *env) alwaysFail(string) bool { return false }

func hasState(seen []runctl.State, s runctl.State) bool {
	for _, v := range seen {
		if v == s {
			return true
		}
	}
	return false
}

func TestRehearsalExhaustedThreeJobsHandoffNoRunner(t *testing.T) {
	e := newEnv(t, goodPlan(), &runctl.FakePhases{})
	for i := 0; i < 60 && e.state() != runctl.Failed; i++ {
		e.step(t, e.alwaysFail)
	}
	js := jobs(t, e.cs)
	if e.state() != runctl.Failed || count(js, "rehearsal-r-1-") != 3 || count(js, "runner-") != 0 {
		t.Fatalf("estado %s, ensayos %d, runners %d", e.state(), count(js, "rehearsal-r-1-"), count(js, "runner-"))
	}
	if !hasState(e.seen, runctl.Resetting) || len(e.al.Calls) != 1 || e.al.Calls[0] != "r-1/rehearse" {
		t.Fatalf("estados %v handoffs %v", e.seen, e.al.Calls)
	}
}

// Plan roto: el ensayo falla ANTES de crear ningún Job (ni de ensayo ni de runner); 3 intentos, handoff, reset y failed.
func TestRehearsalBrokenPlanNeverCreatesRunner(t *testing.T) {
	e := newEnv(t, planSrc{p: plan.FlowPlan{Workflow: "w"}}, &runctl.FakePhases{})
	for i := 0; i < 60 && e.state() != runctl.Failed; i++ {
		e.step(t, e.alwaysFail)
	}
	js := jobs(t, e.cs)
	if e.state() != runctl.Failed || len(js) != 0 || len(e.m.bad) != 0 {
		t.Fatalf("estado %s, Jobs %d, runners sin ensayo %v", e.state(), len(js), e.m.bad)
	}
	if !hasState(e.seen, runctl.Resetting) || len(e.al.Calls) != 1 {
		t.Fatalf("estados %v handoffs %v", e.seen, e.al.Calls)
	}
}

func TestRehearsalNeverSkippedRunnerOnlyAfterPassed(t *testing.T) {
	e := newEnv(t, goodPlan(), &runctl.FakePhases{})
	// Falla el primero, pasa el segundo (1 reintento).
	for i := 0; i < 60 && e.state() != runctl.Done; i++ {
		e.step(t, func(n string) bool { return n == "rehearsal-r-1-2" })
	}
	js := jobs(t, e.cs)
	if e.state() != runctl.Done || count(js, "rehearsal-r-1-") != 2 || count(js, "runner-") != 1 || len(e.m.bad) != 0 {
		t.Fatalf("estado %s ensayos %d runners %d sin ensayo %v", e.state(), count(js, "rehearsal-r-1-"), count(js, "runner-"), e.m.bad)
	}
}

func TestFlowToRehearsal(t *testing.T) {
	e := newEnv(t, goodPlan(), &runctl.FakePhases{})
	want := []runctl.State{runctl.WarmReady, runctl.Deploying, runctl.Inferring, runctl.Rehearsing}
	for _, w := range want {
		e.k.DriveAll(context.Background())
		if e.state() != w {
			t.Fatalf("esperaba %s, hay %s", w, e.state())
		}
	}
	// En rehearsing: lanza el Job y espera (sin gate); con el Job corriendo no avanza.
	e.k.DriveAll(context.Background())
	e.k.DriveAll(context.Background())
	if e.state() != runctl.Rehearsing || count(jobs(t, e.cs), "rehearsal-r-1-") != 1 {
		t.Fatalf("estado %s jobs %v", e.state(), jobs(t, e.cs))
	}
	if cur, _ := e.store.Get("r-1"); cur.EnsayoPassed == runctl.True {
		t.Fatal("ensayo_passed sin Job succeeded")
	}
	finish(t, e.cs, "rehearsal-r-1-1", true)
	e.k.DriveAll(context.Background())
	if e.state() != runctl.Running {
		t.Fatalf("estado %s", e.state())
	}
	// cada paso pasó por el gate y el último llevó ensayo_passed=true
	var tos []runctl.State
	for _, c := range e.gate.Calls {
		tos = append(tos, c.To)
	}
	if len(tos) != 5 || tos[4] != runctl.Running || e.gate.Calls[4].EnsayoPassed != runctl.True {
		t.Fatalf("gate vio %v", e.gate.Calls)
	}
}

func TestFlowResetOnFailureDeploying(t *testing.T) {
	e := newEnv(t, goodPlan(), &runctl.FakePhases{FailFirst: map[string]int{runctl.PhaseDeploy: 99}})
	for i := 0; i < 40 && e.state() != runctl.Failed; i++ {
		e.step(t, e.alwaysFail)
	}
	if e.state() != runctl.Failed || !hasState(e.seen, runctl.Resetting) || e.m.other.Launches[runctl.PhaseReset] != 1 {
		t.Fatalf("estado %s vistos %v resets %d", e.state(), e.seen, e.m.other.Launches[runctl.PhaseReset])
	}
	if count(jobs(t, e.cs), "") != 0 {
		t.Fatal("no debe haber Jobs")
	}
}

// Propiedad: para toda secuencia de resultados de Job, nunca más de 3 ensayos, nunca un runner sin
// ensayo_passed=true registrado, y la corrida solo llega a Done si algún Job terminó succeeded.
func TestPropertyRehearsalCapAndNoRunnerWithoutPass(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		e := newEnv(rt, goodPlan(), &runctl.FakePhases{})
		results := rapid.SliceOfN(rapid.Bool(), 1, 6).Draw(rt, "resultados")
		i, anyPass := 0, false
		for n := 0; n < 80 && !e.state().Terminal(); n++ {
			e.step(rt, func(string) bool {
				ok := results[i%len(results)]
				i++
				if ok {
					anyPass = true
				}
				return ok
			})
			js := jobs(rt, e.cs)
			if c := count(js, "rehearsal-r-1-"); c > 3 {
				rt.Fatalf("%d Jobs de ensayo", c)
			}
			if count(js, "runner-") > 0 && !anyPass {
				rt.Fatal("runner sin ensayo succeeded")
			}
		}
		if len(e.m.bad) != 0 {
			rt.Fatalf("runner lanzado sin ensayo_passed: %v", e.m.bad)
		}
		if e.state() == runctl.Done && !anyPass {
			rt.Fatal("done sin ensayo")
		}
	})
}

type tb interface {
	Helper()
	Fatal(args ...any)
	Fatalf(format string, args ...any)
}
