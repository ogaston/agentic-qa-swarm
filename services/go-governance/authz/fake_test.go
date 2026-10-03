package authz

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func okInput() GateInput {
	return GateInput{RunID: "r1", From: StateConfirmed, To: StateWarmReady, Confirmed: True,
		ResetVerified: True, EnsayoPassed: True, TargetNamespace: "aqs-test", WorkflowAllowed: True}
}

func TestFakeEvaluatorDefaultDeny(t *testing.T) {
	d, err := NewFakeEvaluator().AuthorizeTransition(context.Background(), okInput())
	if err != nil || d.Allow || d.Reason == "" {
		t.Fatalf("esperaba deny con razón, got %+v err=%v", d, err)
	}
}

func TestFakeEvaluatorAllowDenyAndSpecificOverWildcard(t *testing.T) {
	f := NewFakeEvaluator().Allow("", StateWarmReady).Deny(StateConfirmed, StateWarmReady, "específica")
	d, _ := f.AuthorizeTransition(context.Background(), okInput())
	if d.Allow || d.Reason != "específica" {
		t.Fatalf("la regla específica debe ganar: %+v", d)
	}
	in := okInput()
	in.From = StateFailed
	d, _ = f.AuthorizeTransition(context.Background(), in)
	if !d.Allow {
		t.Fatalf("el comodín debe permitir: %+v", d)
	}
}

func TestFakeEvaluatorErrorIsNotAllow(t *testing.T) {
	boom := errors.New("boom")
	f := NewFakeEvaluator().Error(StateConfirmed, StateWarmReady, boom)
	d, err := f.AuthorizeTransition(context.Background(), okInput())
	if !errors.Is(err, boom) || d.Allow || d.Reason == "" {
		t.Fatalf("got %+v err=%v", d, err)
	}
}

// UnknownFactIsFalse cubre lo que existe hoy: Fact trata Unknown como no
// verdadero (IsTrue), el valor cero de GateInput es todo Unknown y el JSON
// "unknown" se lee como Unknown. El fake NO evalúa hechos; aplicar esta regla
// a las decisiones llega con el evaluador real (U4-T04).
func TestFakeEvaluatorUnknownFactIsFalse(t *testing.T) {
	if Unknown.IsTrue() || False.IsTrue() || !True.IsTrue() {
		t.Fatal("solo True es verdadero")
	}
	var in GateInput
	for _, f := range []Fact{in.Confirmed, in.ResetVerified, in.EnsayoPassed, in.WorkflowAllowed} {
		if f != Unknown || f.IsTrue() {
			t.Fatal("el valor cero debe ser Unknown y no verdadero")
		}
	}
	var f Fact = True
	if err := f.UnmarshalJSON([]byte(`"unknown"`)); err != nil || f.IsTrue() {
		t.Fatalf("unknown JSON: %v %v", f, err)
	}
	if err := f.UnmarshalJSON([]byte(`"quizas"`)); err == nil {
		t.Fatal("valor inválido debe fallar")
	}
}

// El fake solo resuelve por (From, To); no lee hechos. Se fija explícitamente.
func TestFakeEvaluatorDoesNotEvaluateFacts(t *testing.T) {
	in := GateInput{RunID: "r", From: StateDone, To: StateFailed}
	d, _ := NewFakeEvaluator().Allow("", "").AuthorizeTransition(context.Background(), in)
	if !d.Allow {
		t.Fatal("el fake permite según la regla, sin mirar hechos")
	}
}

func TestFakeEvaluatorInvalidInputDenied(t *testing.T) {
	f := NewFakeEvaluator().Allow("", "")
	for _, mut := range []func(*GateInput){
		func(i *GateInput) { i.RunID = "" },
		func(i *GateInput) { i.From = "zzz" },
		func(i *GateInput) { i.To = "" },
	} {
		in := okInput()
		mut(&in)
		d, _ := f.AuthorizeTransition(context.Background(), in)
		if d.Allow || d.Reason == "" {
			t.Fatalf("entrada inválida debe denegar: %+v", d)
		}
	}
}

func TestFakeEvaluatorRecordsCalls(t *testing.T) {
	f := NewFakeEvaluator()
	a, b := okInput(), okInput()
	a.RunID, b.RunID = "a", "b"
	f.AuthorizeTransition(context.Background(), a)
	f.AuthorizeTransition(context.Background(), b)
	c := f.Calls()
	if len(c) != 2 || c[0].RunID != "a" || c[1].RunID != "b" {
		t.Fatalf("calls=%+v", c)
	}
}

func TestFakeEvaluatorConcurrent(t *testing.T) {
	f := NewFakeEvaluator().Allow(StateConfirmed, StateWarmReady)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f.AuthorizeTransition(context.Background(), okInput())
			f.Deny(StateDone, StateFailed, "x")
			_ = f.Calls()
		}()
	}
	wg.Wait()
	if n := len(f.Calls()); n != 50 {
		t.Fatalf("calls=%d", n)
	}
}
