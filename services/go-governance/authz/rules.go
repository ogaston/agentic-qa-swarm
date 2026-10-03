package authz

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// DefaultTestNamespace es el único namespace donde se permite correr.
const DefaultTestNamespace = "aqs-test"

// legal lista las transiciones legales (sin la regla "→ failed desde no terminal").
var legal = map[State][]State{
	StateConfirmed:  {StateWarmReady},
	StateWarmReady:  {StateDeploying},
	StateDeploying:  {StateInferring, StateResetting},
	StateInferring:  {StateRehearsing, StateResetting},
	StateRehearsing: {StateRunning, StateResetting},
	StateRunning:    {StateResetting},
	StateResetting:  {StateReporting},
	StateReporting:  {StateDone},
}

// IsTerminal indica si el estado es terminal (done, failed).
func IsTerminal(s State) bool { return s == StateDone || s == StateFailed }

// LegalTransition indica si from→to es una transición legal.
func LegalTransition(from, to State) bool {
	if !from.Valid() || !to.Valid() {
		return false
	}
	if to == StateFailed {
		return !IsTerminal(from)
	}
	for _, t := range legal[from] {
		if t == to {
			return true
		}
	}
	return false
}

// WorkflowRule es la regla de un workflow en la política `workflows`.
type WorkflowRule struct {
	Name             string
	MaxRunsPerDay    int
	RequiresApproval bool
	Version          int // versión de la política de la que sale
}

// WorkflowSource da al evaluador la política de workflows y el consumo del día.
// Cualquier error se traduce en Deny.
type WorkflowSource interface {
	// Workflow devuelve la regla del workflow; found=false si la política o el nombre no existen.
	Workflow(ctx context.Context, name string) (rule WorkflowRule, found bool, err error)
	// RunsAllowed cuenta las decisiones allow hacia running del workflow en el día UTC de day.
	RunsAllowed(ctx context.Context, workflow string, day time.Time) (int, error)
}

// RuleEvaluator es el evaluador real de authorizeTransition: fail-closed, evalúa
// los hechos tri-estado (Unknown cuenta como False), el namespace y el workflow.
type RuleEvaluator struct {
	// TestNamespace es el único namespace permitido (comparación exacta). Vacío: DefaultTestNamespace.
	TestNamespace string
	// Workflows es opcional; sin él un workflow no vacío se deniega.
	Workflows WorkflowSource
	// Now es el reloj (por defecto time.Now).
	Now func() time.Time
}

var _ Evaluator = (*RuleEvaluator)(nil)

func deny(format string, a ...any) Decision {
	return Decision{Reason: fmt.Sprintf(format, a...)}
}

// AuthorizeTransition decide. Un error interno devuelve Allow=false con razón y el error.
func (e *RuleEvaluator) AuthorizeTransition(ctx context.Context, in GateInput) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return deny("contexto cancelado"), err
	}
	if err := in.Validate(); err != nil {
		return deny("entrada inválida: %s", err), nil
	}
	if !LegalTransition(in.From, in.To) {
		return deny("transición ilegal %s->%s", in.From, in.To), nil
	}
	var missing []string
	need := func(name string, f Fact) {
		if !f.IsTrue() {
			missing = append(missing, fmt.Sprintf("%s=%s", name, f))
		}
	}
	ns := e.TestNamespace
	if ns == "" {
		ns = DefaultTestNamespace
	}
	needNS := func() {
		if in.TargetNamespace != ns {
			missing = append(missing, fmt.Sprintf("namespace %q no es el de prueba %q", in.TargetNamespace, ns))
		}
	}
	gatedWorkflow := false
	switch in.To {
	case StateWarmReady:
		need("confirmed", in.Confirmed)
		need("reset_verified", in.ResetVerified)
		needNS()
		need("workflow_allowed", in.WorkflowAllowed) // la matriz de U4-T01 lo exige también aquí
		gatedWorkflow = true
	case StateDeploying:
		need("confirmed", in.Confirmed)
		need("reset_verified", in.ResetVerified)
		needNS()
		need("workflow_allowed", in.WorkflowAllowed)
		gatedWorkflow = true
	case StateInferring:
		need("confirmed", in.Confirmed)
		needNS()
		gatedWorkflow = true
	case StateRehearsing:
		need("confirmed", in.Confirmed)
		needNS()
		need("workflow_allowed", in.WorkflowAllowed)
		gatedWorkflow = true
	case StateRunning:
		need("confirmed", in.Confirmed)
		need("ensayo_passed", in.EnsayoPassed)
		needNS()
		need("workflow_allowed", in.WorkflowAllowed)
		gatedWorkflow = true
	case StateResetting, StateReporting:
		needNS()
	}
	if len(missing) > 0 {
		return deny("hechos exigidos no cumplidos: %s", strings.Join(missing, ", ")), nil
	}
	if gatedWorkflow && in.Workflow != "" {
		if d, err := e.checkWorkflow(ctx, in); d != nil || err != nil {
			if d == nil {
				return deny("error evaluando la política de workflows"), err
			}
			return *d, err
		}
	}
	return Decision{Allow: true, Reason: "permitido"}, nil
}

// checkWorkflow devuelve nil,nil si el workflow está permitido.
func (e *RuleEvaluator) checkWorkflow(ctx context.Context, in GateInput) (*Decision, error) {
	if e.Workflows == nil {
		d := deny("workflow %q no permitido: sin política de workflows", in.Workflow)
		return &d, nil
	}
	rule, found, err := e.Workflows.Workflow(ctx, in.Workflow)
	if err != nil {
		return nil, fmt.Errorf("política de workflows ilegible: %w", err)
	}
	if !found {
		d := deny("workflow %q no está en la política workflows", in.Workflow)
		return &d, nil
	}
	if in.To != StateRunning {
		return nil, nil
	}
	if rule.RequiresApproval && !in.ApprovalRecorded.IsTrue() {
		d := deny("workflow %q exige aprobación (approval_recorded=%s)", in.Workflow, in.ApprovalRecorded)
		return &d, nil
	}
	now := time.Now
	if e.Now != nil {
		now = e.Now
	}
	n, err := e.Workflows.RunsAllowed(ctx, in.Workflow, now().UTC())
	if err != nil {
		return nil, fmt.Errorf("no se pudo contar el consumo diario: %w", err)
	}
	if n >= rule.MaxRunsPerDay {
		d := deny("workflow %q alcanzó la cuota diaria (%d de %d)", in.Workflow, n, rule.MaxRunsPerDay)
		return &d, nil
	}
	return nil, nil
}
