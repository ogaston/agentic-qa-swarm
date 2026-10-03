package authz_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/authz"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/gen"
)

// Invariantes de los gates (PBT-03) sobre el RuleEvaluator real de U4-T04.
// El comportamiento aceptado manda: la matriz de U4-T01 sobre la tabla;
// warm_ready exige también workflow_allowed; el workflow se contrasta con la
// política salvo en resetting, reporting, done y failed.

var fixedNow = func() time.Time { return time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC) }

func eval(t *rapid.T, src *gen.Source, in authz.GateInput) authz.Decision {
	ev := &authz.RuleEvaluator{Workflows: src, Now: fixedNow}
	d, err := ev.AuthorizeTransition(context.Background(), in)
	if err != nil && d.Allow {
		t.Fatalf("error con Allow=true: %v", err)
	}
	if !d.Allow && d.Reason == "" {
		t.Fatalf("Deny sin razón para %+v", in)
	}
	return d
}

func evalPlain(src *gen.Source, in authz.GateInput) (authz.Decision, error) {
	ev := &authz.RuleEvaluator{Workflows: src, Now: fixedNow}
	return ev.AuthorizeTransition(context.Background(), in)
}

func TestPBT_Invariant_NoConfirmNeverAllow(t *testing.T) {
	gen.AtLeastChecks(t, 1000)
	dests := []authz.State{authz.StateWarmReady, authz.StateDeploying, authz.StateInferring, authz.StateRehearsing, authz.StateRunning}
	rapid.Check(t, func(t *rapid.T) {
		in := gen.GateInput(dests...).Draw(t, "in")
		in.Confirmed = gen.FactNotTrue().Draw(t, "confirmed")
		if d := eval(t, gen.WorkflowSource().Draw(t, "src"), in); d.Allow {
			t.Fatalf("Allow sin confirmed=true: %+v", in)
		}
	})
}

func TestPBT_Invariant_NoResetVerifiedNeverAllow(t *testing.T) {
	gen.AtLeastChecks(t, 1000)
	rapid.Check(t, func(t *rapid.T) {
		in := gen.GateInput(authz.StateWarmReady, authz.StateDeploying).Draw(t, "in")
		in.ResetVerified = gen.FactNotTrue().Draw(t, "reset_verified")
		if d := eval(t, gen.WorkflowSource().Draw(t, "src"), in); d.Allow {
			t.Fatalf("Allow sin reset_verified=true: %+v", in)
		}
	})
}

func TestPBT_Invariant_NoEnsayoNeverRunning(t *testing.T) {
	gen.AtLeastChecks(t, 1000)
	rapid.Check(t, func(t *rapid.T) {
		in := gen.GateInput(authz.StateRunning).Draw(t, "in")
		in.EnsayoPassed = gen.FactNotTrue().Draw(t, "ensayo_passed")
		if d := eval(t, gen.WorkflowSource().Draw(t, "src"), in); d.Allow {
			t.Fatalf("Allow a running sin ensayo_passed=true: %+v", in)
		}
	})
}

func TestPBT_Invariant_NamespaceNotTestNeverAllow(t *testing.T) {
	gen.AtLeastChecks(t, 1000)
	rapid.Check(t, func(t *rapid.T) {
		in := gen.GateInput().Draw(t, "in")
		in.TargetNamespace = gen.NamespaceNear().Draw(t, "ns")
		if in.TargetNamespace == gen.TestNS {
			t.Fatalf("el generador NamespaceNear produjo %q", gen.TestNS)
		}
		d := eval(t, gen.WorkflowSource().Draw(t, "src"), in)
		if in.To == authz.StateDone || in.To == authz.StateFailed {
			// la tabla de U4-T04 no exige namespace para done/failed: solo cuenta la legalidad.
			if want := in.Validate() == nil && gen.IsLegal(in.From, in.To); d.Allow != want {
				t.Fatalf("a %s el namespace no cuenta: Allow=%v, esperado %v (%+v)", in.To, d.Allow, want, in)
			}
			return
		}
		if d.Allow {
			t.Fatalf("Allow con namespace %q: %+v", in.TargetNamespace, in)
		}
	})
}

func withFacts(in authz.GateInput, f func(authz.Fact) authz.Fact) authz.GateInput {
	in.Confirmed, in.ResetVerified, in.EnsayoPassed = f(in.Confirmed), f(in.ResetVerified), f(in.EnsayoPassed)
	in.WorkflowAllowed, in.ApprovalRecorded = f(in.WorkflowAllowed), f(in.ApprovalRecorded)
	return in
}

func TestPBT_Invariant_UnknownEqualsFalse(t *testing.T) {
	gen.AtLeastChecks(t, 1000)
	rapid.Check(t, func(t *rapid.T) {
		in := gen.GateInput().Draw(t, "in")
		src := gen.WorkflowSource().Draw(t, "src")
		in2 := withFacts(in, func(f authz.Fact) authz.Fact {
			if f == authz.Unknown {
				return authz.False
			}
			return f
		})
		if a, b := eval(t, src, in).Allow, eval(t, src, in2).Allow; a != b {
			t.Fatalf("unknown->false cambia la decisión (%v vs %v): %+v", a, b, in)
		}
	})
}

func TestPBT_Invariant_Monotonic(t *testing.T) {
	gen.AtLeastChecks(t, 1000)
	rapid.Check(t, func(t *rapid.T) {
		in := gen.GateInput().Draw(t, "in")
		src := gen.WorkflowSource().Draw(t, "src")
		in2 := withFacts(in, func(f authz.Fact) authz.Fact {
			if f == authz.True && rapid.Bool().Draw(t, "downgrade") {
				return gen.FactNotTrue().Draw(t, "to")
			}
			return f
		})
		if !eval(t, src, in).Allow && eval(t, src, in2).Allow {
			t.Fatalf("degradar hechos convirtió Deny en Allow: %+v -> %+v", in, in2)
		}
	})
}

func TestPBT_Invariant_IllegalTransitionNeverAllow(t *testing.T) {
	gen.AtLeastChecks(t, 1000)
	rapid.Check(t, func(t *rapid.T) {
		in := gen.GateInput().Draw(t, "in")
		in.From, in.To = gen.RunState().Draw(t, "from"), gen.RunState().Draw(t, "to")
		if gen.IsLegal(in.From, in.To) {
			t.Skip()
		}
		if d := eval(t, gen.WorkflowSource().Draw(t, "src"), in); d.Allow {
			t.Fatalf("Allow en transición ilegal %s->%s: %+v", in.From, in.To, in)
		}
	})
}

func TestPBT_Invariant_Deterministic(t *testing.T) {
	gen.AtLeastChecks(t, 1000)
	rapid.Check(t, func(t *rapid.T) {
		in := gen.GateInput().Draw(t, "in")
		src := gen.WorkflowSource().Draw(t, "src")
		d1, e1 := evalPlain(src, in)
		d2, e2 := evalPlain(src, in)
		if !reflect.DeepEqual(d1, d2) || !reflect.DeepEqual(e1, e2) {
			t.Fatalf("decisiones distintas para la misma entrada: %+v vs %+v", d1, d2)
		}
	})
}

// Ejemplos fijos (uno por invariante): si los generadores fallaran, la
// propiedad no se quedaría sin cobertura. Incluye el grafo legal contra el oráculo.
func TestPBT_Examples_Canonical(t *testing.T) {
	src := &gen.Source{Rules: map[string]authz.WorkflowRule{"checkout": {Name: "checkout", MaxRunsPerDay: 2}}}
	ok := authz.GateInput{RunID: "r", From: authz.StateRehearsing, To: authz.StateRunning, Confirmed: authz.True,
		ResetVerified: authz.True, EnsayoPassed: authz.True, WorkflowAllowed: authz.True, TargetNamespace: gen.TestNS, Workflow: "checkout"}
	cases := []struct {
		name  string
		mod   func(in *authz.GateInput)
		allow bool
	}{
		{"base permitida", func(*authz.GateInput) {}, true},
		{"sin confirmed", func(in *authz.GateInput) { in.Confirmed = authz.False }, false},
		{"confirmed unknown", func(in *authz.GateInput) { in.Confirmed = authz.Unknown }, false},
		{"sin ensayo", func(in *authz.GateInput) { in.EnsayoPassed = authz.False }, false},
		{"namespace prefijo", func(in *authz.GateInput) { in.TargetNamespace = "aqs-test-prod" }, false},
		{"namespace mayúsculas", func(in *authz.GateInput) { in.TargetNamespace = "AQS-TEST" }, false},
		{"transición ilegal", func(in *authz.GateInput) { in.From = authz.StateConfirmed }, false},
		{"sin reset_verified a warm_ready", func(in *authz.GateInput) {
			in.From, in.To, in.ResetVerified = authz.StateConfirmed, authz.StateWarmReady, authz.False
		}, false},
		{"done no exige namespace", func(in *authz.GateInput) {
			in.From, in.To, in.TargetNamespace = authz.StateReporting, authz.StateDone, "prod"
		}, true},
	}
	for _, c := range cases {
		in := ok
		c.mod(&in)
		a, err := evalPlain(src, in)
		b, _ := evalPlain(src, in)
		if err != nil || a.Allow != c.allow || !reflect.DeepEqual(a, b) {
			t.Errorf("%s: Allow=%v err=%v, esperado %v", c.name, a.Allow, err, c.allow)
		}
	}
	for _, from := range authz.AllStates {
		for _, to := range authz.AllStates {
			if authz.LegalTransition(from, to) != gen.IsLegal(from, to) {
				t.Errorf("grafo: %s->%s difiere del oráculo", from, to)
			}
		}
	}
	if _, err := (&authz.RuleEvaluator{}).AuthorizeTransition(canceled(), ok); err == nil {
		t.Error("contexto cancelado debe dar error")
	}
}

func canceled() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
