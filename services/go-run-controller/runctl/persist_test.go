package runctl

import (
	"context"
	"errors"
	"testing"
)

// schedStore es un RunStore cuyo Save falla según una regla sobre la versión que se guarda.
type schedStore struct {
	*MemStore
	failIf func(Run) bool
	saves  int
}

func (s *schedStore) Save(r Run) error {
	s.saves++
	if s.failIf != nil && s.failIf(r) {
		return errors.New("disco")
	}
	return s.MemStore.Save(r)
}

func newSched(t testing.TB, gate GateClient, fail func(Run) bool) (*Controller, *schedStore, *FakePhases, *FakeAlerter) {
	t.Helper()
	st := &schedStore{MemStore: NewMemStore(), failIf: fail}
	ph, al := &FakePhases{}, &FakeAlerter{}
	k, err := New(Config{Gate: gate, Store: st, Publisher: &FakePublisher{}, Warm: &FakeWarm{Fact: True}, Alerter: al, Phases: ph})
	if err != nil {
		t.Fatal(err)
	}
	return k, st, ph, al
}

func seed(st *schedStore, s State) {
	r := Run{ID: "r", State: s, ConfirmedBy: "u", Flows: []string{"f"}}
	if s == Running {
		r.EnsayoPassed = True // running solo se alcanza con el ensayo registrado
	}
	_ = st.MemStore.Save(r)
}

func ticks(k *Controller, n int) {
	for i := 0; i < n; i++ {
		k.DriveAll(context.Background())
	}
}

func total(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

// Disco caído: nada se lanza (la intención de lanzar se persiste antes de lanzar).
func TestDiskDownNeverLaunchesAnyPhase(t *testing.T) {
	for _, s := range []State{Deploying, Inferring, Rehearsing, Running, Resetting, Reporting} {
		k, st, ph, al := newSched(t, AllowAll(), func(Run) bool { return true })
		seed(st, s)
		ticks(k, 20)
		if total(ph.Launches) != 0 || len(al.Calls) != 0 {
			t.Errorf("%s: lanzamientos %v handoffs %v con el disco caído", s, ph.Launches, al.Calls)
		}
		if got, _ := st.Get("r"); got.State != s {
			t.Errorf("%s: transitó con el disco caído", s)
		}
	}
}

// Fase lanzada con éxito pero cuyo marcado (Launched) no se guarda: tope duro de 3 lanzamientos.
func TestLaunchThenSaveFailsIsCappedAtThree(t *testing.T) {
	for _, c := range []struct {
		s State
		p string
	}{{Deploying, PhaseDeploy}, {Inferring, PhaseInfer}, {Rehearsing, PhaseRehearse}, {Running, PhaseRun}, {Resetting, PhaseReset}, {Reporting, PhaseReport}} {
		k, st, ph, al := newSched(t, AllowAll(), func(r Run) bool { return len(r.Launched) > 0 })
		seed(st, c.s)
		ticks(k, 30)
		if n := ph.Launches[c.p]; n != 3 {
			t.Errorf("%s: %d lanzamientos, esperado exactamente 3", c.p, n)
		}
		if len(al.Calls) > 1 {
			t.Errorf("%s: handoffs duplicados %v", c.p, al.Calls)
		}
	}
}

// Fase que falla y cuyo conteo de fallos no se guarda: sigue acotada a 3.
func TestFailedLaunchAndAttemptSaveFailsIsCappedAtThree(t *testing.T) {
	k, st, ph, al := newSched(t, AllowAll(), func(r Run) bool { return len(r.Attempts) > 0 })
	ph.FailFirst = map[string]int{PhaseDeploy: 99}
	seed(st, Deploying)
	ticks(k, 30)
	if ph.Launches[PhaseDeploy] != 3 || len(al.Calls) > 1 {
		t.Fatalf("lanzamientos %d handoffs %v", ph.Launches[PhaseDeploy], al.Calls)
	}
}

// Handoff: Save antes de alertar; si el Save falla no hay alerta, y nunca más de una.
func TestHandoffAtMostOncePerPhaseWhenSaveFails(t *testing.T) {
	for name, fail := range map[string]func(Run) bool{
		"falla al marcar HandedOff": func(r Run) bool { return len(r.HandedOff) > 0 },
		"falla al fijar FailReason": func(r Run) bool { return r.FailReason != "" },
		"falla tras la salida":      func(r Run) bool { return r.State == Failed || r.State == Resetting },
	} {
		k, st, ph, al := newSched(t, AllowAll(), fail)
		ph.FailFirst = map[string]int{PhaseDeploy: 99}
		seed(st, Deploying)
		ticks(k, 30)
		if len(al.Calls) > 1 || ph.Launches[PhaseDeploy] > 3 {
			t.Errorf("%s: handoffs %v lanzamientos %d", name, al.Calls, ph.Launches[PhaseDeploy])
		}
	}
	// handoff por salida denegada (fase gate): una sola vez aunque el Save de Halted falle
	k, st, _, al := newSched(t, denyAll(), func(r Run) bool { return r.Halted })
	seed(st, Confirmed)
	ticks(k, 30)
	if len(al.Calls) > 1 {
		t.Fatalf("handoffs de gate duplicados: %v", al.Calls)
	}
}

// Un error de disco en el avance normal no mata la corrida ni hace handoff: no avanza y se reintenta.
func TestPersistErrorOnAdvanceDoesNotFailRun(t *testing.T) {
	failing := true
	k, st, _, al := newSched(t, AllowAll(), func(r Run) bool { return failing && r.State == WarmReady })
	seed(st, Confirmed)
	ticks(k, 5)
	got, _ := st.Get("r")
	if got.State != Confirmed || got.FailReason != "" || got.Halted || len(al.Calls) != 0 {
		t.Fatalf("un error de disco alteró la corrida: %+v handoffs %v", got, al.Calls)
	}
	failing = false
	ticks(k, 1)
	if got, _ = st.Get("r"); got.State != WarmReady {
		t.Fatalf("no reintentó: %s", got.State)
	}
}

// Salida ya pendiente (FailReason fijada) y el Save de la transición falla: no es denegación.
func TestPersistErrorOnPendingExitIsNotADenial(t *testing.T) {
	k, st, _, al := newSched(t, AllowAll(), func(r Run) bool { return r.State == Failed })
	_ = st.MemStore.Save(Run{ID: "r", State: Confirmed, ConfirmedBy: "u", FailReason: "x"})
	ticks(k, 5)
	got, _ := st.Get("r")
	if got.Halted || len(al.Calls) != 0 || got.State != Confirmed {
		t.Fatalf("error de disco tratado como denegación: %+v %v", got, al.Calls)
	}
	st.failIf = nil
	ticks(k, 1)
	if got, _ = st.Get("r"); got.State != Failed {
		t.Fatalf("no cerró al sanar el disco: %s", got.State)
	}
}

func TestPersistHealthAndMetric(t *testing.T) {
	o := &countObs{}
	fail := true
	st := &schedStore{MemStore: NewMemStore(), failIf: func(Run) bool { return fail }}
	k, _ := New(Config{Gate: AllowAll(), Store: st, Publisher: &FakePublisher{}, Warm: &FakeWarm{Fact: True},
		Alerter: &FakeAlerter{}, Phases: &FakePhases{}, Observer: o})
	_ = k.Apply(t.Context(), confirmedEv())
	if k.PersistHealthy() == nil || o.persist == 0 {
		t.Fatal("el fallo de Save no se reportó")
	}
	fail = false
	_ = k.Apply(t.Context(), confirmedEv())
	if k.PersistHealthy() != nil {
		t.Fatal("debía volver a sano tras un Save correcto")
	}
}

// El hecho de ensayo no se acepta hasta que la fase se lanzó.
func TestRehearsalEventBeforeLaunchIsDropped(t *testing.T) {
	r := newRig(t, AllowAll())
	_ = r.store.Save(Run{ID: "r-1", State: Rehearsing, ConfirmedBy: "u", Flows: []string{"f"}}) // aún sin lanzar
	_ = r.k.Apply(t.Context(), Event{Type: EvRehearsalPassed, EventID: "p", RunID: "r-1"})
	if got, _ := r.store.Get("r-1"); got.EnsayoPassed != "" {
		t.Fatalf("fijó ensayo_passed antes de lanzar: %q", got.EnsayoPassed)
	}
}

// Una sola llamada al gate por paso con el gate caído en resetting.
func TestResettingGateDownOneGateCallPerTick(t *testing.T) {
	g := &FakeGate{Decide: func(GateRequest) (GateDecision, error) { return GateDecision{}, errors.New("down") }}
	k, st, _, _ := newSched(t, g, nil)
	seed(st, Resetting)
	ticks(k, 1) // lanza reset y pregunta al gate
	n := len(g.Calls)
	ticks(k, 10)
	if len(g.Calls)-n != 10 {
		t.Fatalf("%d llamadas en 10 pasos", len(g.Calls)-n)
	}
}
