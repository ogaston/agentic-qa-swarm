package runctl

import (
	"context"
	"errors"
	"testing"

	"pgregory.net/rapid"
)

// downFrom devuelve un gate que cae (error) en las transiciones cuyo origen es from, cuando *down es true.
func downFrom(from State, down *bool) *FakeGate {
	return &FakeGate{Decide: func(q GateRequest) (GateDecision, error) {
		if *down && q.From == from {
			return GateDecision{}, errors.New("down")
		}
		return GateDecision{Allow: true, Reason: "ok", AuditRef: "a"}, nil
	}}
}

func TestResetExhaustedWithGateDownNeverRelaunchesNorDuplicatesHandoff(t *testing.T) {
	down := false
	r := newRig(t, downFrom(Resetting, &down))
	r.ph.FailFirst = map[string]int{PhaseReset: 99}
	_ = r.k.Apply(t.Context(), confirmedEv())
	driveUntilStable(r)
	_ = r.k.Apply(t.Context(), Event{Type: EvRehearsalPassed, EventID: "e2", RunID: "r-1"})
	down = true
	for i := 0; i < 30; i++ {
		r.k.DriveAll(t.Context())
	}
	if n := r.ph.Launches[PhaseReset]; n != 3 {
		t.Fatalf("lanzamientos de reset = %d, esperado exactamente 3", n)
	}
	if len(r.al.Calls) != 1 || r.state("r-1") != Resetting {
		t.Fatalf("handoffs %v estado %s", r.al.Calls, r.state("r-1"))
	}
	down = false // el gate vuelve: la salida pendiente cierra en failed sin relanzar
	r.k.DriveAll(t.Context())
	if r.state("r-1") != Failed || r.ph.Launches[PhaseReset] != 3 || len(r.al.Calls) != 1 {
		t.Fatalf("%s %d %v", r.state("r-1"), r.ph.Launches[PhaseReset], r.al.Calls)
	}
}

// Barrido de clase: para cada fase lanzable agotada con el gate caído en su estado.
func TestEveryPhaseExhaustedWithGateDownLaunchesExactlyThree(t *testing.T) {
	cases := []struct {
		phase string
		state State
	}{{PhaseDeploy, Deploying}, {PhaseInfer, Inferring}, {PhaseRun, Running}, {PhaseReset, Resetting}, {PhaseReport, Reporting}}
	for _, c := range cases {
		down := false
		r := newRig(t, downFrom(c.state, &down))
		r.ph.FailFirst = map[string]int{c.phase: 99}
		_ = r.store.Save(Run{ID: "r", State: c.state, ConfirmedBy: "u", Flows: []string{"f"}})
		down = true
		for i := 0; i < 30; i++ {
			r.k.DriveAll(t.Context())
		}
		if n := r.ph.Launches[c.phase]; n != 3 {
			t.Errorf("%s: %d lanzamientos", c.phase, n)
		}
		if len(r.al.Calls) != 1 {
			t.Errorf("%s: handoffs %v", c.phase, r.al.Calls)
		}
	}
	// rehearse: agotada por rehearsal.failed y gate caído
	down := false
	r := newRig(t, downFrom(Rehearsing, &down))
	_ = r.store.Save(Run{ID: "r", State: Rehearsing, ConfirmedBy: "u", Flows: []string{"f"}})
	down = true
	for i := 0; i < 3; i++ {
		r.k.DriveAll(t.Context())
		_ = r.k.Apply(t.Context(), Event{Type: EvRehearsalFailed, EventID: "f" + string(rune('a'+i)), RunID: "r"})
	}
	for i := 0; i < 30; i++ {
		r.k.DriveAll(t.Context())
	}
	if n := r.ph.Launches[PhaseRehearse]; n != 3 || len(r.al.Calls) != 1 {
		t.Errorf("rehearse: %d lanzamientos, handoffs %v", n, r.al.Calls)
	}
}

func TestRehearsalEventOutsideRehearsingIsDroppedAndNeverSetsFact(t *testing.T) {
	r := newRig(t, AllowAll())
	_ = r.k.Apply(t.Context(), confirmedEv())
	if err := r.k.Apply(t.Context(), Event{Type: EvRehearsalPassed, EventID: "early", RunID: "r-1"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.store.Get("r-1"); got.EnsayoPassed != "" {
		t.Fatalf("fijó ensayo_passed=%q fuera de rehearsing", got.EnsayoPassed)
	}
	driveUntilStable(r)
	if r.state("r-1") != Rehearsing {
		t.Fatalf("estado %s: debía esperar el ensayo real", r.state("r-1"))
	}
	for _, c := range r.gate.Calls {
		if c.EnsayoPassed == True {
			t.Fatalf("el gate recibió ensayo_passed=true sin ensayo: %+v", c)
		}
	}
	// el evento descartado no se aplica después (replay) ...
	_ = r.k.Apply(t.Context(), Event{Type: EvRehearsalPassed, EventID: "early", RunID: "r-1"})
	if got, _ := r.store.Get("r-1"); got.EnsayoPassed != "" {
		t.Fatal("el replay aplicó el evento descartado")
	}
	// ... y uno nuevo en rehearsing sí.
	_ = r.k.Apply(t.Context(), Event{Type: EvRehearsalPassed, EventID: "real", RunID: "r-1"})
	driveUntilStable(r)
	if r.state("r-1") != Done {
		t.Fatal(r.state("r-1"))
	}
}

type countObs struct{ dropped int }

func (c *countObs) Transition(State, State, string) {}
func (c *countObs) GateCall(string)                 {}
func (c *countObs) Handoff(string)                  {}
func (c *countObs) EventDropped(string)             { c.dropped++ }

func TestDroppedEventIsCounted(t *testing.T) {
	o := &countObs{}
	k, _ := New(Config{Gate: AllowAll(), Store: NewMemStore(), Publisher: &FakePublisher{}, Warm: &FakeWarm{Fact: True},
		Alerter: &FakeAlerter{}, Phases: &FakePhases{}, Observer: o})
	_ = k.Apply(t.Context(), confirmedEv())
	_ = k.Apply(t.Context(), Event{Type: EvRehearsalFailed, EventID: "x", RunID: "r-1"})
	if o.dropped != 1 {
		t.Fatal(o.dropped)
	}
}

type failingStore struct {
	*MemStore
	failSave bool
}

func (s *failingStore) Save(r Run) error {
	if s.failSave {
		return errors.New("disco lleno")
	}
	return s.MemStore.Save(r)
}

func TestPersistErrorIsNotADenial(t *testing.T) {
	st := &failingStore{MemStore: NewMemStore()}
	al := &FakeAlerter{}
	k, _ := New(Config{Gate: AllowAll(), Store: st, Publisher: &FakePublisher{}, Warm: &FakeWarm{Fact: True}, Alerter: al, Phases: &FakePhases{}})
	_ = st.MemStore.Save(Run{ID: "r", State: Confirmed, ConfirmedBy: "u", Flows: []string{"f"}})
	st.failSave = true
	if err := k.Drive(t.Context(), "r"); err == nil {
		t.Fatal("debía fallar")
	}
	got, _ := st.Get("r")
	if got.Halted || len(al.Calls) != 0 {
		t.Fatalf("un error de disco se trató como denegación: %+v %v", got, al.Calls)
	}
}

// Propiedad fuerte: toda llamada previa al avance fue allow, y ninguna fase pasa de 3 lanzamientos.
func TestPropertyAdvanceOnlyAfterAllowAndExactlyThreeLaunchCap(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		decisions := rapid.SliceOfN(rapid.IntRange(0, 2), 0, 60).Draw(rt, "gate")
		idx := 0
		var lastAllowed bool
		gate := &FakeGate{Decide: func(GateRequest) (GateDecision, error) {
			d := 0
			if idx < len(decisions) {
				d = decisions[idx]
			}
			idx++
			lastAllowed = false
			switch d {
			case 1:
				return GateDecision{Reason: "x"}, nil
			case 2:
				return GateDecision{}, errors.New("x")
			}
			lastAllowed = true
			return GateDecision{Allow: true, Reason: "ok"}, nil
		}}
		r := newRig(t, gate)
		r.ph.FailFirst = map[string]int{PhaseDeploy: rapid.IntRange(0, 9).Draw(rt, "fd"), PhaseInfer: rapid.IntRange(0, 9).Draw(rt, "fi"),
			PhaseRun: rapid.IntRange(0, 9).Draw(rt, "fu"), PhaseReset: rapid.IntRange(0, 9).Draw(rt, "fr")}
		_ = r.k.Apply(context.Background(), confirmedEv())
		for i := 0; i < 60; i++ {
			if i%7 == 3 {
				ty := rapid.SampledFrom([]string{EvRehearsalPassed, EvRehearsalFailed}).Draw(rt, "ev")
				_ = r.k.Apply(context.Background(), Event{Type: ty, EventID: "x" + string(rune('A'+i)), RunID: "r-1"})
			}
			before, _ := r.store.Get("r-1")
			n := len(gate.Calls)
			r.k.DriveAll(context.Background())
			after, _ := r.store.Get("r-1")
			if before.State.Terminal() && after.State != before.State {
				rt.Fatalf("salió de terminal")
			}
			if after.State != before.State {
				calls := gate.Calls[n:]
				if len(calls) == 0 || calls[len(calls)-1].To != after.State || !lastAllowedAt(gate, decisions, len(gate.Calls)-1) {
					rt.Fatalf("avanzó %s->%s sin un allow del gate", before.State, after.State)
				}
			}
		}
		_ = lastAllowed
		for p, n := range r.ph.Launches {
			if p == PhaseRehearse {
				continue
			}
			if n > 3 {
				rt.Fatalf("fase %s lanzada %d veces (tope 3)", p, n)
			}
		}
		if len(r.al.Calls) > 4*1+len(r.al.Calls)/(len(r.al.Calls)+1) { // un handoff por agotamiento: acotado por fases + salida
			rt.Fatalf("handoffs %v", r.al.Calls)
		}
	})
}

// lastAllowedAt: la llamada idx-ésima consumió decisions[idx]==0 (allow); más allá de la lista, allow.
func lastAllowedAt(_ *FakeGate, decisions []int, idx int) bool {
	if idx < len(decisions) {
		return decisions[idx] == 0
	}
	return true
}
