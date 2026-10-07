package runctl

import (
	"context"
	"errors"
	"testing"

	"pgregory.net/rapid"
)

type rig struct {
	store *MemStore
	gate  *FakeGate
	warm  *FakeWarm
	al    *FakeAlerter
	pub   *FakePublisher
	ph    *FakePhases
	k     *Controller
}

func newRig(t testing.TB, gate *FakeGate) *rig {
	t.Helper()
	r := &rig{store: NewMemStore(), gate: gate, warm: &FakeWarm{Fact: True}, al: &FakeAlerter{}, pub: &FakePublisher{}, ph: &FakePhases{}}
	k, err := New(Config{Gate: gate, Store: r.store, Publisher: r.pub, Warm: r.warm, Alerter: r.al, Phases: r.ph})
	if err != nil {
		t.Fatal(err)
	}
	r.k = k
	return r
}

func newCtl(t testing.TB, store RunStore, gate GateClient) *Controller {
	t.Helper()
	k, err := New(Config{Gate: gate, Store: store, Publisher: &FakePublisher{}, Warm: &FakeWarm{Fact: True}, Alerter: &FakeAlerter{}, Phases: &FakePhases{}})
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func confirmedEv() Event {
	return Event{Type: EvRunConfirmed, EventID: "e1", TraceID: "t1", RunID: "r-1", ConfirmedBy: "u-1", Flows: []string{"checkout"}}
}

func (r *rig) state(id string) State { x, _ := r.store.Get(id); return x.State }

func TestEveryLegalTransitionAppliesWithGate(t *testing.T) {
	for p := range legalPairs {
		r := newRig(t, AllowAll())
		_ = r.store.Save(Run{ID: "r", State: p[0], ConfirmedBy: "u", Flows: []string{"f"}, EnsayoPassed: True})
		if err := r.k.Transition(t.Context(), "r", p[1]); err != nil {
			t.Fatalf("%v: %v", p, err)
		}
		if r.state("r") != p[1] || len(r.gate.Calls) != 1 || r.gate.Calls[0].TargetNamespace != "aqs-test" {
			t.Fatalf("%v: estado %s calls %d", p, r.state("r"), len(r.gate.Calls))
		}
	}
}

func denyAll() *FakeGate {
	return &FakeGate{Decide: func(GateRequest) (GateDecision, error) {
		return GateDecision{Reason: "no", AuditRef: "a"}, nil
	}}
}

func TestGateDenyDoesNotAdvanceNorRetry(t *testing.T) {
	r := newRig(t, denyAll())
	_ = r.store.Save(Run{ID: "r", State: Confirmed, ConfirmedBy: "u"})
	if err := r.k.Transition(t.Context(), "r", WarmReady); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	if r.state("r") != Confirmed || len(r.gate.Calls) != 1 {
		t.Fatalf("estado %s llamadas %d", r.state("r"), len(r.gate.Calls))
	}
}

func TestGateErrorAndTimeoutDoNotAdvance(t *testing.T) {
	for _, e := range []error{errors.New("boom"), context.DeadlineExceeded} {
		r := newRig(t, &FakeGate{Decide: func(GateRequest) (GateDecision, error) { return GateDecision{Allow: true}, e }})
		_ = r.store.Save(Run{ID: "r", State: Confirmed, ConfirmedBy: "u"})
		if err := r.k.Transition(t.Context(), "r", WarmReady); !errors.Is(err, ErrGate) {
			t.Fatal(err)
		}
		if r.state("r") != Confirmed {
			t.Fatal("avanzó con el gate caído")
		}
	}
}

// facts devuelve un gate que permite solo si los hechos son los exigidos.
func factsGate() *FakeGate {
	return &FakeGate{Decide: func(q GateRequest) (GateDecision, error) {
		ok := q.Confirmed == True && q.TargetNamespace == "aqs-test"
		switch q.To {
		case WarmReady, Deploying:
			ok = ok && q.ResetVerified == True
		case Running:
			ok = ok && q.EnsayoPassed == True
		}
		return GateDecision{Allow: ok, Reason: "hechos", AuditRef: "a"}, nil
	}}
}

func TestGateUnknownFactNeverSentAsTrue(t *testing.T) {
	r := newRig(t, factsGate())
	r.warm.Fact = Unknown
	_ = r.store.Save(Run{ID: "r", State: WarmReady, ConfirmedBy: "u"})
	_ = r.k.Transition(t.Context(), "r", Deploying)
	q := r.gate.Calls[0]
	if q.ResetVerified != Unknown || q.EnsayoPassed != Unknown || q.WorkflowAllowed != Unknown {
		t.Fatalf("hechos: %+v", q)
	}
	if r.state("r") != WarmReady {
		t.Fatal("avanzó")
	}
}

func TestGateWithoutConfirmDenied(t *testing.T) {
	r := newRig(t, factsGate())
	_ = r.store.Save(Run{ID: "r", State: Confirmed}) // sin confirmed_by
	if err := r.k.Transition(t.Context(), "r", WarmReady); !errors.Is(err, ErrDenied) || r.state("r") != Confirmed {
		t.Fatal(err, r.state("r"))
	}
}

func TestGateWithoutResetDenied(t *testing.T) {
	for _, f := range []Fact{False, Unknown} {
		r := newRig(t, factsGate())
		r.warm.Fact = f
		_ = r.store.Save(Run{ID: "r", State: WarmReady, ConfirmedBy: "u"})
		if err := r.k.Transition(t.Context(), "r", Deploying); !errors.Is(err, ErrDenied) || r.state("r") != WarmReady {
			t.Fatal(f, err)
		}
	}
	r := newRig(t, factsGate())
	r.warm.Err = errors.New("no se sabe")
	_ = r.store.Save(Run{ID: "r", State: WarmReady, ConfirmedBy: "u"})
	if err := r.k.Transition(t.Context(), "r", Deploying); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
}

func TestGateWithoutEnsayoDenied(t *testing.T) {
	r := newRig(t, factsGate())
	_ = r.store.Save(Run{ID: "r", State: Rehearsing, ConfirmedBy: "u"})
	if err := r.k.Transition(t.Context(), "r", Running); !errors.Is(err, ErrDenied) || r.state("r") != Rehearsing {
		t.Fatal(err)
	}
}

func TestGateErrorDownStopsRun(t *testing.T) {
	r := newRig(t, &FakeGate{Decide: func(GateRequest) (GateDecision, error) { return GateDecision{}, errors.New("caído") }})
	_ = r.store.Save(Run{ID: "r", State: Confirmed, ConfirmedBy: "u"})
	_ = r.k.Drive(t.Context(), "r")
	got, _ := r.store.Get("r")
	if got.State != Confirmed || !got.Halted || len(r.al.Calls) != 1 {
		t.Fatalf("%+v %v", got, r.al.Calls)
	}
}

func TestIdempotentRunConfirmedCreatesOneRun(t *testing.T) {
	r := newRig(t, AllowAll())
	_ = r.k.Apply(t.Context(), confirmedEv())
	_ = r.k.Drive(t.Context(), "r-1")
	_ = r.k.Apply(t.Context(), confirmedEv())
	if len(r.store.List()) != 1 || r.state("r-1") == Confirmed {
		t.Fatalf("%v", r.store.List())
	}
}

func driveUntilStable(r *rig) {
	for i := 0; i < 30; i++ {
		r.k.DriveAll(context.Background())
	}
}

func TestHappyPathReachesDoneAndPublishesOnce(t *testing.T) {
	r := newRig(t, AllowAll())
	_ = r.k.Apply(t.Context(), confirmedEv())
	driveUntilStable(r)
	if r.state("r-1") != Rehearsing {
		t.Fatal(r.state("r-1"))
	}
	_ = r.k.Apply(t.Context(), Event{Type: EvRehearsalPassed, EventID: "e2", RunID: "r-1"})
	driveUntilStable(r)
	if r.state("r-1") != Done || len(r.pub.Events) != 1 {
		t.Fatal(r.state("r-1"), r.pub.Events)
	}
}

func TestPhaseRetryOnceAndTwiceThenSucceeds(t *testing.T) {
	for _, n := range []int{1, 2} {
		r := newRig(t, AllowAll())
		r.ph.FailFirst = map[string]int{PhaseDeploy: n}
		_ = r.k.Apply(t.Context(), confirmedEv())
		driveUntilStable(r)
		if r.state("r-1") != Rehearsing || r.ph.Launches[PhaseDeploy] != n+1 || len(r.al.Calls) != 0 {
			t.Fatalf("n=%d estado %s lanzamientos %d", n, r.state("r-1"), r.ph.Launches[PhaseDeploy])
		}
	}
}

func TestPhaseThirdFailureHandoffNoFourthLaunch(t *testing.T) {
	r := newRig(t, AllowAll())
	r.ph.FailFirst = map[string]int{PhaseDeploy: 99}
	_ = r.k.Apply(t.Context(), confirmedEv())
	driveUntilStable(r)
	if r.ph.Launches[PhaseDeploy] != 3 || len(r.al.Calls) != 1 {
		t.Fatalf("lanzamientos %d handoffs %v", r.ph.Launches[PhaseDeploy], r.al.Calls)
	}
	got, _ := r.store.Get("r-1")
	if got.State != Failed || got.FailReason == "" {
		t.Fatalf("%+v", got) // deploying -> resetting -> failed
	}
	if r.ph.Launches[PhaseReset] != 1 {
		t.Fatal("el reset verificado debía ejecutarse antes de cerrar")
	}
}

func TestResetFailureThreeTimesFails(t *testing.T) {
	r := newRig(t, AllowAll())
	r.ph.FailFirst = map[string]int{PhaseReset: 99}
	_ = r.k.Apply(t.Context(), confirmedEv())
	driveUntilStable(r)
	_ = r.k.Apply(t.Context(), Event{Type: EvRehearsalPassed, EventID: "e2", RunID: "r-1"})
	driveUntilStable(r)
	if r.state("r-1") != Failed || r.ph.Launches[PhaseReset] != 3 || len(r.al.Calls) != 1 {
		t.Fatal(r.state("r-1"), r.ph.Launches, r.al.Calls)
	}
}

func TestRehearsalFailedRetriesThenHandoff(t *testing.T) {
	r := newRig(t, AllowAll())
	_ = r.k.Apply(t.Context(), confirmedEv())
	driveUntilStable(r)
	for i := 0; i < 3; i++ {
		_ = r.k.Apply(t.Context(), Event{Type: EvRehearsalFailed, EventID: "f" + string(rune('a'+i)), RunID: "r-1"})
		driveUntilStable(r)
	}
	if r.ph.Launches[PhaseRehearse] != 3 || len(r.al.Calls) != 1 || r.state("r-1") != Failed {
		t.Fatal(r.ph.Launches, r.al.Calls, r.state("r-1"))
	}
}

func TestResumeFromPersistedState(t *testing.T) {
	r := newRig(t, AllowAll())
	_ = r.store.Save(Run{ID: "r-1", State: Inferring, ConfirmedBy: "u", Flows: []string{"f"}})
	// "reinicio": otro controlador sobre el mismo almacén; repetir run.confirmed no lo regresa.
	k2 := newCtl(t, r.store, r.gate)
	_ = k2.Apply(t.Context(), confirmedEv())
	if r.state("r-1") != Inferring {
		t.Fatal("volvió atrás")
	}
	_ = k2.Drive(t.Context(), "r-1")
	if r.state("r-1") != Rehearsing {
		t.Fatal(r.state("r-1"))
	}
}

func TestPropertyNeverAdvancesOnDenyNeverLeavesTerminalMaxTwoRetries(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		var decisions []int // 0 allow, 1 deny, 2 error
		idx := 0
		gate := &FakeGate{Decide: func(GateRequest) (GateDecision, error) {
			d := 0
			if idx < len(decisions) {
				d = decisions[idx]
			}
			idx++
			switch d {
			case 1:
				return GateDecision{Reason: "x"}, nil
			case 2:
				return GateDecision{}, errors.New("x")
			}
			return GateDecision{Allow: true, Reason: "ok"}, nil
		}}
		decisions = rapid.SliceOfN(rapid.IntRange(0, 2), 0, 40).Draw(rt, "gate")
		r := newRig(t, gate)
		r.ph.FailFirst = map[string]int{
			PhaseDeploy: rapid.IntRange(0, 4).Draw(rt, "fd"), PhaseInfer: rapid.IntRange(0, 4).Draw(rt, "fi"),
			PhaseReset: rapid.IntRange(0, 4).Draw(rt, "fr")}
		_ = r.k.Apply(context.Background(), confirmedEv())
		evs := rapid.SliceOfN(rapid.SampledFrom([]string{EvRehearsalPassed, EvRehearsalFailed}), 0, 6).Draw(rt, "ev")
		prev := Confirmed
		for i := 0; i < 40; i++ {
			if i < len(evs) {
				_ = r.k.Apply(context.Background(), Event{Type: evs[i], EventID: "x" + string(rune('a'+i)), RunID: "r-1"})
			}
			before, _ := r.store.Get("r-1")
			n := len(gate.Calls)
			r.k.DriveAll(context.Background())
			after, _ := r.store.Get("r-1")
			if before.State.Terminal() && after.State != before.State {
				rt.Fatalf("salió de terminal %s -> %s", before.State, after.State)
			}
			if after.State != before.State {
				if !Legal(before.State, after.State) {
					rt.Fatalf("ilegal %s -> %s", before.State, after.State)
				}
				last := gate.Calls[n:]
				if len(last) == 0 || last[len(last)-1].To != after.State {
					rt.Fatalf("avanzó sin gate")
				}
			}
			_ = prev
		}
		for p, n := range r.ph.Launches {
			if n > 3*5 { // reintentos de ensayo reinician; el tope por fase es 3 lanzamientos por ciclo
				rt.Fatalf("fase %s lanzada %d veces", p, n)
			}
		}
		got, _ := r.store.Get("r-1")
		for p, n := range got.Attempts {
			if n > MaxRetries+1 {
				rt.Fatalf("fase %s con %d fallos", p, n)
			}
		}
	})
}
