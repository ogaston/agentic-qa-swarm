package authz

import (
	"context"
	"fmt"
	"sync"
)

type ruleKind int

const (
	kindAllow ruleKind = iota
	kindDeny
	kindError
)

type rule struct {
	kind   ruleKind
	reason string
	err    error
}

// FakeEvaluator es un Evaluator en memoria, programable y fail-closed.
// Sin regla para (From, To) devuelve Deny. Un estado vacío en la regla es comodín.
type FakeEvaluator struct {
	mu    sync.Mutex
	rules map[Gate]rule
	calls []GateInput
}

// NewFakeEvaluator crea un fake sin reglas (todo Deny).
func NewFakeEvaluator() *FakeEvaluator {
	return &FakeEvaluator{rules: map[Gate]rule{}}
}

func (f *FakeEvaluator) set(from, to State, r rule) *FakeEvaluator {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rules[Gate{From: from, To: to}] = r
	return f
}

// Allow programa permitir (from, to).
func (f *FakeEvaluator) Allow(from, to State) *FakeEvaluator {
	return f.set(from, to, rule{kind: kindAllow})
}

// Deny programa denegar (from, to) con una razón.
func (f *FakeEvaluator) Deny(from, to State, reason string) *FakeEvaluator {
	return f.set(from, to, rule{kind: kindDeny, reason: reason})
}

// Error programa que (from, to) devuelva err.
func (f *FakeEvaluator) Error(from, to State, err error) *FakeEvaluator {
	return f.set(from, to, rule{kind: kindError, err: err})
}

// Calls devuelve una copia de las llamadas registradas, en orden.
func (f *FakeEvaluator) Calls() []GateInput {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]GateInput(nil), f.calls...)
}

func (f *FakeEvaluator) lookup(from, to State) (rule, bool) {
	for _, k := range []Gate{{from, to}, {from, ""}, {"", to}, {"", ""}} {
		if r, ok := f.rules[k]; ok {
			return r, true
		}
	}
	return rule{}, false
}

// AuthorizeTransition registra la llamada y aplica la regla. Falla cerrado:
// entrada inválida, contexto cancelado o sin regla implican Allow=false.
func (f *FakeEvaluator) AuthorizeTransition(ctx context.Context, in GateInput) (Decision, error) {
	f.mu.Lock()
	f.calls = append(f.calls, in)
	r, ok := f.lookup(in.From, in.To)
	f.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return Decision{Reason: "contexto cancelado"}, err
	}
	if err := in.Validate(); err != nil {
		return Decision{Reason: "entrada inválida: " + err.Error()}, nil
	}
	if !ok {
		return Decision{Reason: fmt.Sprintf("sin regla para %s->%s (deny por defecto)", in.From, in.To)}, nil
	}
	switch r.kind {
	case kindAllow:
		return Decision{Allow: true, Reason: "permitido", AuditRef: "fake-" + in.RunID}, nil
	case kindError:
		return Decision{Reason: "error del evaluador"}, r.err
	}
	reason := r.reason
	if reason == "" {
		reason = "denegado por regla"
	}
	return Decision{Reason: reason}, nil
}

var _ Evaluator = (*FakeEvaluator)(nil)
