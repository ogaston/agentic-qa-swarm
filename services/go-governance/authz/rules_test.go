package authz

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func allTrue(from, to State) GateInput {
	return GateInput{RunID: "r1", From: from, To: to, Confirmed: True, ResetVerified: True,
		EnsayoPassed: True, TargetNamespace: "aqs-test", WorkflowAllowed: True}
}

func authorize(t *testing.T, ev Evaluator, in GateInput) Decision {
	t.Helper()
	d, err := ev.AuthorizeTransition(context.Background(), in)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !d.Allow && d.Reason == "" {
		t.Fatal("deny sin razón")
	}
	return d
}

// Tabla de la tarea escrita de forma independiente al evaluador: (origen legal, destino, hechos exigidos).
var factTable = []struct {
	from, to State
	need     []string
}{
	{StateConfirmed, StateWarmReady, []string{"confirmed", "reset_verified", "ns", "workflow_allowed"}},
	{StateWarmReady, StateDeploying, []string{"confirmed", "reset_verified", "ns", "workflow_allowed"}},
	{StateDeploying, StateInferring, []string{"confirmed", "ns"}},
	{StateInferring, StateRehearsing, []string{"confirmed", "ns", "workflow_allowed"}},
	{StateRehearsing, StateRunning, []string{"confirmed", "ensayo_passed", "ns", "workflow_allowed"}},
	{StateRunning, StateResetting, []string{"ns"}},
	{StateResetting, StateReporting, []string{"ns"}},
	{StateReporting, StateDone, nil},
	{StateRunning, StateFailed, nil},
	{StateConfirmed, StateFailed, nil},
}

func setFact(in *GateInput, name string, v Fact) {
	switch name {
	case "confirmed":
		in.Confirmed = v
	case "reset_verified":
		in.ResetVerified = v
	case "ensayo_passed":
		in.EnsayoPassed = v
	case "workflow_allowed":
		in.WorkflowAllowed = v
	case "ns":
		if v != True {
			in.TargetNamespace = "staging"
		}
	}
}

func TestGateFactTable(t *testing.T) {
	ev := &RuleEvaluator{}
	for _, row := range factTable {
		t.Run(fmt.Sprintf("%s_%s_todo_true", row.from, row.to), func(t *testing.T) {
			if d := authorize(t, ev, allTrue(row.from, row.to)); !d.Allow {
				t.Fatalf("debía permitir: %s", d.Reason)
			}
		})
		for _, f := range row.need {
			for _, v := range []Fact{False, Unknown} {
				t.Run(fmt.Sprintf("%s_%s_%s_%s", row.from, row.to, f, v), func(t *testing.T) {
					in := allTrue(row.from, row.to)
					setFact(&in, f, v)
					if d := authorize(t, ev, in); d.Allow {
						t.Fatalf("debía denegar con %s=%s", f, v)
					}
				})
			}
		}
	}
}

// Los hechos NO exigidos no deben bloquear (p. ej. resetting sin confirmed).
func TestGateIgnoresUnrequiredFacts(t *testing.T) {
	ev := &RuleEvaluator{}
	in := GateInput{RunID: "r", From: StateRunning, To: StateResetting, TargetNamespace: "aqs-test"}
	if d := authorize(t, ev, in); !d.Allow {
		t.Fatalf("resetting solo exige namespace: %s", d.Reason)
	}
	in = GateInput{RunID: "r", From: StateReporting, To: StateDone}
	if d := authorize(t, ev, in); !d.Allow {
		t.Fatalf("done no exige hechos: %s", d.Reason)
	}
}

// C-54: un hecho Unknown deniega a través de AuthorizeTransition, y también el valor cero.
func TestUnknownFactDeniesThroughAuthorizeTransition(t *testing.T) {
	ev := &RuleEvaluator{}
	for _, row := range factTable {
		for _, f := range row.need {
			if f == "ns" {
				continue
			}
			in := allTrue(row.from, row.to)
			setFact(&in, f, Unknown)
			d := authorize(t, ev, in)
			if d.Allow || !strings.Contains(d.Reason, f+"=unknown") {
				t.Fatalf("%s->%s con %s unknown: allow=%v reason=%q", row.from, row.to, f, d.Allow, d.Reason)
			}
		}
	}
	// valor cero de GateInput: todo Unknown, sin namespace.
	if d := authorize(t, ev, GateInput{RunID: "r", From: StateConfirmed, To: StateWarmReady}); d.Allow {
		t.Fatal("entrada con ceros no debe permitir")
	}
}

func TestLegalTransitionsExhaustive(t *testing.T) {
	want := map[[2]State]bool{}
	for _, p := range [][2]State{
		{StateConfirmed, StateWarmReady}, {StateWarmReady, StateDeploying}, {StateDeploying, StateInferring},
		{StateInferring, StateRehearsing}, {StateRehearsing, StateRunning}, {StateRunning, StateResetting},
		{StateResetting, StateReporting}, {StateReporting, StateDone},
		{StateDeploying, StateResetting}, {StateInferring, StateResetting}, {StateRehearsing, StateResetting},
	} {
		want[p] = true
	}
	for _, s := range []State{StateConfirmed, StateWarmReady, StateDeploying, StateInferring, StateRehearsing,
		StateRunning, StateResetting, StateReporting} {
		want[[2]State{s, StateFailed}] = true
	}
	ev := &RuleEvaluator{}
	for _, from := range AllStates {
		for _, to := range AllStates {
			t.Run(string(from)+"->"+string(to), func(t *testing.T) {
				d := authorize(t, ev, allTrue(from, to))
				if d.Allow != want[[2]State{from, to}] {
					t.Fatalf("allow=%v esperado %v (%s)", d.Allow, want[[2]State{from, to}], d.Reason)
				}
			})
		}
	}
}

func TestTerminalStatesDeny(t *testing.T) {
	ev := &RuleEvaluator{}
	for _, from := range []State{StateDone, StateFailed} {
		for _, to := range AllStates {
			if d := authorize(t, ev, allTrue(from, to)); d.Allow {
				t.Fatalf("desde terminal %s a %s no debe permitir", from, to)
			}
		}
	}
}

func TestNamespaceExact(t *testing.T) {
	ev := &RuleEvaluator{}
	cases := map[string]bool{
		"aqs-test": true, "AQS-TEST": false, "aqs-test ": false, "aqs-test\n": false, " aqs-test": false,
		"aqs-test.evil": false, "": false, "staging": false, "prod": false, "default": false,
		"aqs-tеst": false, // 'е' cirílica
		"aqs‐test": false, // guion Unicode
	}
	for ns, ok := range cases {
		t.Run(fmt.Sprintf("%q", ns), func(t *testing.T) {
			in := allTrue(StateInferring, StateRehearsing)
			in.TargetNamespace = ns
			if d := authorize(t, ev, in); d.Allow != ok {
				t.Fatalf("allow=%v esperado %v", d.Allow, ok)
			}
		})
	}
	custom := &RuleEvaluator{TestNamespace: "otro-test"}
	in := allTrue(StateInferring, StateRehearsing)
	if authorize(t, custom, in).Allow {
		t.Fatal("aqs-test no debe valer si el namespace de prueba configurado es otro")
	}
	in.TargetNamespace = "otro-test"
	if !authorize(t, custom, in).Allow {
		t.Fatal("el namespace configurado debe valer")
	}
}

func TestInvalidInputsDeny(t *testing.T) {
	ev := &RuleEvaluator{}
	for name, in := range map[string]GateInput{
		"run vacío":      {From: StateConfirmed, To: StateWarmReady},
		"origen limbo":   {RunID: "r", From: "limbo", To: StateWarmReady},
		"destino limbo":  {RunID: "r", From: StateConfirmed, To: "limbo"},
		"estados vacíos": {RunID: "r"},
	} {
		if d := authorize(t, ev, in); d.Allow {
			t.Fatalf("%s: debía denegar", name)
		}
	}
}

func TestCancelledContextDenies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d, err := (&RuleEvaluator{}).AuthorizeTransition(ctx, allTrue(StateConfirmed, StateWarmReady))
	if d.Allow || err == nil || d.Reason == "" {
		t.Fatalf("contexto cancelado: %+v %v", d, err)
	}
}

// ---- workflows ----

type fakeWF struct {
	rules map[string]WorkflowRule
	runs  int
	err   error
	runsE error
}

func (f *fakeWF) Workflow(_ context.Context, n string) (WorkflowRule, bool, error) {
	if f.err != nil {
		return WorkflowRule{}, false, f.err
	}
	r, ok := f.rules[n]
	return r, ok, nil
}
func (f *fakeWF) RunsAllowed(context.Context, string, time.Time) (int, error) { return f.runs, f.runsE }

func wfInput(w string) GateInput {
	in := allTrue(StateRehearsing, StateRunning)
	in.Workflow = w
	return in
}

func TestWorkflowPolicy(t *testing.T) {
	src := &fakeWF{rules: map[string]WorkflowRule{
		"checkout": {Name: "checkout", MaxRunsPerDay: 2},
		"refund":   {Name: "refund", MaxRunsPerDay: 5, RequiresApproval: true},
	}}
	ev := &RuleEvaluator{Workflows: src}
	if !authorize(t, ev, wfInput("checkout")).Allow {
		t.Fatal("checkout debía permitirse")
	}
	if authorize(t, ev, wfInput("nada")).Allow {
		t.Fatal("workflow inexistente debe denegar")
	}
	if d := authorize(t, ev, wfInput("refund")); d.Allow || !strings.Contains(d.Reason, "aprobación") {
		t.Fatalf("refund sin aprobación: %+v", d)
	}
	in := wfInput("refund")
	in.ApprovalRecorded = Unknown
	if authorize(t, ev, in).Allow {
		t.Fatal("aprobación unknown debe denegar")
	}
	in.ApprovalRecorded = True
	if !authorize(t, ev, in).Allow {
		t.Fatal("refund con aprobación debía permitirse")
	}
	src.runs = 2
	if d := authorize(t, ev, wfInput("checkout")); d.Allow || !strings.Contains(d.Reason, "cuota") {
		t.Fatalf("cuota agotada: %+v", d)
	}
	// la cuota solo aplica a running
	in = allTrue(StateInferring, StateRehearsing)
	in.Workflow = "checkout"
	if !authorize(t, ev, in).Allow {
		t.Fatal("la cuota diaria solo aplica a running")
	}
	// sin política
	if authorize(t, &RuleEvaluator{}, wfInput("checkout")).Allow {
		t.Fatal("sin fuente de workflows debe denegar")
	}
	// workflow vacío: solo cuenta workflow_allowed
	if !authorize(t, ev, wfInput("")).Allow {
		t.Fatal("workflow vacío con workflow_allowed=true debe permitir")
	}
	// reset y reporte no dependen de la política de workflows
	in = allTrue(StateRunning, StateResetting)
	in.Workflow = "borrado"
	if !authorize(t, ev, in).Allow {
		t.Fatal("el reset siempre debe poder intentarse")
	}
}

func TestWorkflowSourceErrorsDeny(t *testing.T) {
	boom := errors.New("boom")
	for name, src := range map[string]*fakeWF{
		"política ilegible": {err: boom},
		"conteo falla":      {rules: map[string]WorkflowRule{"checkout": {MaxRunsPerDay: 9}}, runsE: boom},
	} {
		d, err := (&RuleEvaluator{Workflows: src}).AuthorizeTransition(context.Background(), wfInput("checkout"))
		if d.Allow || err == nil || d.Reason == "" {
			t.Fatalf("%s: %+v %v", name, d, err)
		}
	}
}

func TestFactJSON(t *testing.T) {
	var in GateInput
	if err := (&in).Confirmed.UnmarshalJSON([]byte(`"maybe"`)); err == nil {
		t.Fatal("valor inválido debe fallar")
	}
	if err := in.Confirmed.UnmarshalJSON([]byte(`true`)); err == nil {
		t.Fatal("booleano JSON debe fallar: los hechos son strings")
	}
}

// F-01: la existencia del workflow se exige en todo destino salvo resetting,
// reporting, done y failed; la cuota y la aprobación solo en running.
func TestWorkflowExistenceByDestination(t *testing.T) {
	src := &fakeWF{rules: map[string]WorkflowRule{"checkout": {Name: "checkout", MaxRunsPerDay: 1}}}
	ev := &RuleEvaluator{Workflows: src}
	exempt := map[State]bool{StateResetting: true, StateReporting: true, StateDone: true, StateFailed: true}
	for _, row := range factTable {
		t.Run(fmt.Sprintf("%s_%s", row.from, row.to), func(t *testing.T) {
			in := allTrue(row.from, row.to)
			in.Workflow = "nada"
			if d := authorize(t, ev, in); d.Allow == !exempt[row.to] {
				t.Fatalf("workflow inexistente en %s: allow=%v (exento=%v)", row.to, d.Allow, exempt[row.to])
			}
			in.Workflow = "checkout"
			if d := authorize(t, ev, in); !d.Allow {
				t.Fatalf("workflow existente debe permitir: %s", d.Reason)
			}
			// sin fuente de políticas: igual que inexistente
			in.Workflow = "nada"
			if d := authorize(t, &RuleEvaluator{}, in); d.Allow == !exempt[row.to] {
				t.Fatalf("sin política en %s: allow=%v", row.to, d.Allow)
			}
		})
	}
	// la cuota agotada solo bloquea running
	src.runs = 1
	for _, row := range factTable {
		in := allTrue(row.from, row.to)
		in.Workflow = "checkout"
		if d := authorize(t, ev, in); d.Allow == (row.to == StateRunning) {
			t.Fatalf("cuota agotada en %s: allow=%v", row.to, d.Allow)
		}
	}
}
