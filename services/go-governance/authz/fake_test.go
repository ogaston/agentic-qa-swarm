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

func TestFakeEvaluatorUnknownFactIsFalse(t *testing.T) {
	if Unknown.IsTrue() || False.IsTrue() || !True.IsTrue() {
		t.Fatal("solo True es verdadero")
	}
	var in GateInput
	if in.Confirmed != Unknown {
		t.Fatal("el valor cero debe ser Unknown")
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
