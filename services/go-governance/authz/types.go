package authz

import (
	"context"
	"encoding/json"
	"fmt"
)

// Role es el rol de un principal. Solo user y admin son válidos.
type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// Valid indica si el rol es uno de los dos permitidos.
func (r Role) Valid() bool { return r == RoleUser || r == RoleAdmin }

// Principal es la identidad que pide una transición.
type Principal struct {
	ID   string `json:"id"`
	Role Role   `json:"role"`
}

// State es un estado de Run.state (enum del OpenAPI).
type State string

const (
	StateConfirmed  State = "confirmed"
	StateWarmReady  State = "warm_ready"
	StateDeploying  State = "deploying"
	StateInferring  State = "inferring"
	StateRehearsing State = "rehearsing"
	StateRunning    State = "running"
	StateResetting  State = "resetting"
	StateReporting  State = "reporting"
	StateDone       State = "done"
	StateFailed     State = "failed"
)

// AllStates lista los estados válidos.
var AllStates = []State{StateConfirmed, StateWarmReady, StateDeploying, StateInferring,
	StateRehearsing, StateRunning, StateResetting, StateReporting, StateDone, StateFailed}

// Valid indica si el estado pertenece al enum.
func (s State) Valid() bool {
	for _, v := range AllStates {
		if s == v {
			return true
		}
	}
	return false
}

// Fact es un hecho tri-estado. El valor cero es Unknown.
type Fact int

const (
	Unknown Fact = iota
	True
	False
)

// IsTrue es verdadero solo para True: Unknown se lee como False.
func (f Fact) IsTrue() bool { return f == True }

func (f Fact) String() string {
	switch f {
	case True:
		return "true"
	case False:
		return "false"
	}
	return "unknown"
}

// UnmarshalJSON acepta "true", "false" y "unknown".
func (f *Fact) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("hecho debe ser string: %w", err)
	}
	switch s {
	case "true":
		*f = True
	case "false":
		*f = False
	case "unknown":
		*f = Unknown
	default:
		return fmt.Errorf("hecho inválido %q", s)
	}
	return nil
}

// MarshalJSON es el inverso de UnmarshalJSON.
func (f Fact) MarshalJSON() ([]byte, error) { return json.Marshal(f.String()) }

// Gate identifica una transición (From, To) sujeta a evaluación.
type Gate struct {
	From State `json:"from"`
	To   State `json:"to"`
}

// Transition es un alias semántico de Gate para quien pide el cambio.
type Transition = Gate

// GateInput son la transición pedida y los hechos reportados por el llamante.
type GateInput struct {
	RunID           string `json:"run_id"`
	From            State  `json:"from"`
	To              State  `json:"to"`
	Confirmed       Fact   `json:"confirmed"`
	ResetVerified   Fact   `json:"reset_verified"`
	EnsayoPassed    Fact   `json:"ensayo_passed"`
	TargetNamespace string `json:"target_namespace"`
	WorkflowAllowed Fact   `json:"workflow_allowed"`
	// Workflow es el nombre del workflow de la corrida (opcional). Si no está
	// vacío, el evaluador real lo contrasta con la política `workflows`.
	Workflow string `json:"workflow,omitempty"`
	// ApprovalRecorded indica que existe aprobación registrada para el workflow.
	ApprovalRecorded Fact `json:"approval_recorded"`
}

// Validate comprueba la forma de la entrada (RunID no vacío, estados del enum).
func (in GateInput) Validate() error {
	if in.RunID == "" {
		return fmt.Errorf("run_id vacío")
	}
	if !in.From.Valid() {
		return fmt.Errorf("estado origen desconocido %q", in.From)
	}
	if !in.To.Valid() {
		return fmt.Errorf("estado destino desconocido %q", in.To)
	}
	return nil
}

// Decision es el veredicto. Allow=false siempre lleva Reason no vacío.
type Decision struct {
	Allow    bool   `json:"allow"`
	Reason   string `json:"reason"`
	AuditRef string `json:"audit_ref"`
}

// Evaluator decide si una transición está autorizada. Un error nunca es permitir.
type Evaluator interface {
	AuthorizeTransition(ctx context.Context, in GateInput) (Decision, error)
}
