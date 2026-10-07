package runctl

import (
	"context"
	"errors"
	"testing"
)

func TestEnsayoGateRejectsUnrecordedPassedWithoutAskingGate(t *testing.T) {
	for _, f := range []Fact{"", Unknown, False} {
		r := newRig(t, AllowAll())
		_ = r.store.Save(Run{ID: "r", State: Rehearsing, ConfirmedBy: "u", EnsayoPassed: f})
		if err := r.k.Transition(t.Context(), "r", Running); !errors.Is(err, ErrIllegal) || r.state("r") != Rehearsing {
			t.Fatalf("ensayo=%q: %v", f, err)
		}
		if len(r.gate.Calls) != 0 {
			t.Fatalf("ensayo=%q: el gate no debe ni consultarse", f)
		}
	}
}

func TestEnsayoGateDeniedByGateEvenWithRecordedPass(t *testing.T) {
	g := &FakeGate{Decide: func(GateRequest) (GateDecision, error) {
		return GateDecision{Allow: false, Reason: "U4 dice no", AuditRef: "x"}, nil
	}}
	r := newRig(t, g)
	_ = r.store.Save(Run{ID: "r", State: Rehearsing, ConfirmedBy: "u", EnsayoPassed: True})
	if err := r.k.Transition(t.Context(), "r", Running); !errors.Is(err, ErrDenied) || r.state("r") != Rehearsing {
		t.Fatal(err)
	}
	if len(g.Calls) != 1 || g.Calls[0].EnsayoPassed != True {
		t.Fatalf("el gate debía recibir ensayo_passed=true: %v", g.Calls)
	}
}

func TestEnsayoGateEnsayoPassedTrueAdvances(t *testing.T) {
	r := newRig(t, AllowAll())
	_ = r.store.Save(Run{ID: "r", State: Rehearsing, ConfirmedBy: "u", EnsayoPassed: True})
	if err := r.k.Transition(t.Context(), "r", Running); err != nil || r.state("r") != Running {
		t.Fatal(err)
	}
}

// No existe ninguna variable ni configuración que salte el ensayo.
func TestNoBypassEnvAndEmptyPlanDoNotSkipEnsayo(t *testing.T) {
	for _, name := range []string{"RUN_SKIP_REHEARSAL", "SKIP_REHEARSAL", "RUN_ENSAYO_PASSED", "RUN_ALLOW_FAKE_PHASES"} {
		t.Setenv(name, "true")
	}
	r := newRig(t, AllowAll())
	_ = r.store.Save(Run{ID: "r", State: Rehearsing, ConfirmedBy: "u"}) // sin flujos (plan vacío)
	for i := 0; i < 10; i++ {
		r.k.DriveAll(t.Context())
		if r.state("r") == Running {
			t.Fatal("pasó a running sin ensayo_passed")
		}
	}
	if err := r.k.Transition(t.Context(), "r", Running); !errors.Is(err, ErrIllegal) {
		t.Fatal(err)
	}
}

func TestNoBypassRunningWithoutEnsayoLaunchesNoRunner(t *testing.T) {
	r := newRig(t, AllowAll())
	_ = r.store.Save(Run{ID: "r", State: Running, ConfirmedBy: "u"})
	r.k.DriveAll(t.Context())
	if r.ph.Launches[PhaseRun] != 0 || r.state("r") != Resetting {
		t.Fatalf("runner lanzado=%d estado=%s", r.ph.Launches[PhaseRun], r.state("r"))
	}
}

type resultsFn func(context.Context, Run) (RehearsalOutcome, error)

func (f resultsFn) Result(ctx context.Context, r Run) (RehearsalOutcome, error) { return f(ctx, r) }

func rigWithResults(t *testing.T, res RehearsalResults) *rig {
	r := newRig(t, AllowAll())
	k, err := New(Config{Gate: r.gate, Store: r.store, Publisher: r.pub, Warm: r.warm, Alerter: r.al, Phases: r.ph, Results: res})
	if err != nil {
		t.Fatal(err)
	}
	r.k = k
	_ = r.store.Save(Run{ID: "r", State: Rehearsing, ConfirmedBy: "u", Flows: []string{"f"}})
	return r
}

func TestResultsPortPassedAdvancesFailedRetriesErrorWaits(t *testing.T) {
	// error de lectura: no avanza, no fija ensayo_passed
	r := rigWithResults(t, resultsFn(func(context.Context, Run) (RehearsalOutcome, error) { return RehearsalOutcome{}, errors.New("api") }))
	for i := 0; i < 5; i++ {
		r.k.DriveAll(t.Context())
	}
	if cur, _ := r.store.Get("r"); cur.State != Rehearsing || cur.EnsayoPassed == True {
		t.Fatalf("%+v", cur)
	}
	// passed
	r = rigWithResults(t, resultsFn(func(context.Context, Run) (RehearsalOutcome, error) {
		return RehearsalOutcome{Done: true, Passed: true, EventID: "e"}, nil
	}))
	r.k.DriveAll(t.Context())
	if cur, _ := r.store.Get("r"); cur.State != Running || cur.EnsayoPassed != True || len(cur.Seen) != 1 {
		t.Fatalf("%+v", cur)
	}
	// failed: cuenta un fallo y reintenta
	r = rigWithResults(t, resultsFn(func(context.Context, Run) (RehearsalOutcome, error) {
		return RehearsalOutcome{Done: true, Passed: false, EventID: "e"}, nil
	}))
	r.k.DriveAll(t.Context())
	if cur, _ := r.store.Get("r"); cur.State != Rehearsing || cur.Attempts[PhaseRehearse] != 1 || cur.EnsayoPassed == True {
		t.Fatalf("%+v", cur)
	}
}
