package runctl

import (
	"context"
	"errors"
)

// Fact es un hecho tri-estado hacia el gate. El valor cero es Unknown: lo que el controlador
// no conoce nunca se envía como true.
type Fact string

// Valores de Fact (los del OpenAPI GateRequest).
const (
	Unknown Fact = "unknown"
	True    Fact = "true"
	False   Fact = "false"
)

// FactOf convierte un booleano conocido.
func FactOf(b bool) Fact {
	if b {
		return True
	}
	return False
}

// GateRequest es components.schemas.GateRequest.
type GateRequest struct {
	RunID           string `json:"run_id"`
	From            State  `json:"from"`
	To              State  `json:"to"`
	TargetNamespace string `json:"target_namespace"`
	Workflow        string `json:"workflow,omitempty"`
	Confirmed       Fact   `json:"confirmed"`
	ResetVerified   Fact   `json:"reset_verified"`
	EnsayoPassed    Fact   `json:"ensayo_passed"`
	WorkflowAllowed Fact   `json:"workflow_allowed"`
	TraceID         string `json:"-"`
}

// GateDecision es components.schemas.GateDecision.
type GateDecision struct {
	Allow    bool   `json:"allow"`
	Reason   string `json:"reason"`
	AuditRef string `json:"audit_ref"`
}

// GateClient pregunta a go-governance. Un error nunca equivale a permitir.
type GateClient interface {
	Authorize(ctx context.Context, req GateRequest) (GateDecision, error)
}

// Run es la corrida persistida.
type Run struct {
	ID           string          `json:"id"`
	State        State           `json:"state"`
	TraceID      string          `json:"trace_id,omitempty"`
	ConfirmedBy  string          `json:"confirmed_by,omitempty"`
	Flows        []string        `json:"flows,omitempty"`
	Workflow     string          `json:"workflow,omitempty"`
	EnsayoPassed Fact            `json:"ensayo_passed,omitempty"` // de rehearsal.passed / rehearsal.failed
	Attempts     map[string]int  `json:"attempts,omitempty"`      // fallos por fase (V8)
	FailReason   string          `json:"fail_reason,omitempty"`   // si está, la corrida termina en failed tras el reset
	Halted       bool            `json:"halted,omitempty"`        // sin salida segura: el controlador no la toca más
	Evidence     []string        `json:"evidence_uris,omitempty"`
	DonePublish  bool            `json:"done_published,omitempty"`
	Seen         []string        `json:"seen_events,omitempty"` // event_id ya aplicados (idempotencia ante el replay del archivo)
	Started      map[string]int  `json:"started,omitempty"`     // lanzamientos iniciados por fase (se persiste ANTES de lanzar; tope 3)
	HandedOff    map[string]bool `json:"handed_off,omitempty"`  // fases cuyo handoff ya se emitió (a lo sumo uno)
	Launched     map[string]bool `json:"launched,omitempty"`    // fase ya lanzada con éxito en el estado actual
}

func (r Run) clone() Run {
	c := r
	c.Flows = append([]string(nil), r.Flows...)
	c.Evidence = append([]string(nil), r.Evidence...)
	c.Seen = append([]string(nil), r.Seen...)
	c.Attempts = copyMap(r.Attempts)
	c.Launched = copyMap(r.Launched)
	c.Started = copyMap(r.Started)
	c.HandedOff = copyMap(r.HandedOff)
	return c
}

func copyMap[V any](m map[string]V) map[string]V {
	if m == nil {
		return nil
	}
	c := make(map[string]V, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

// RunStore persiste las corridas. Save añade una versión; Get/List devuelven la última.
type RunStore interface {
	Get(id string) (Run, bool)
	List() []Run
	Save(r Run) error
}

// Event es un evento de entrada ya validado.
type Event struct {
	Type        string // run.confirmed | rehearsal.passed | rehearsal.failed
	EventID     string
	TraceID     string
	RunID       string
	ConfirmedBy string
	Flows       []string
	Pos         int64 // posición opaca en la fuente (fin de la línea); la usa EventSource.Ack
}

// Tipos de evento que consume el controlador.
const (
	EvRunConfirmed    = "run.confirmed"
	EvRehearsalPassed = "rehearsal.passed"
	EvRehearsalFailed = "rehearsal.failed"
)

// EventSource entrega eventos (marcador de transición C-45). Poll devuelve los que aún no se
// confirmaron con Ack; un archivo inexistente no es error. Quien consume llama a Ack solo cuando
// Apply salió bien (o falló sin remedio): ante ErrPersist el evento se vuelve a entregar.
type EventSource interface {
	Poll(ctx context.Context) ([]Event, error)
	Ack(ev Event)
}

// OutEvent es un evento que publica el controlador (run.done).
type OutEvent struct {
	Type         string
	RunID        string
	TraceID      string
	EvidenceURIs []string
}

// EventPublisher publica eventos; debe ser idempotente por (tipo, run_id).
type EventPublisher interface {
	Publish(ctx context.Context, ev OutEvent) error
}

// WarmStateReader informa si el warm quedó con reset verificado (adaptador real: U2-T03/T06).
type WarmStateReader interface {
	ResetVerified(ctx context.Context) (Fact, error)
}

// Alerter registra el handoff humano de una fase agotada (sin esquema de evento: candidata).
type Alerter interface {
	Handoff(ctx context.Context, runID, phase, reason string)
}

// Fases lanzables.
const (
	PhaseDeploy    = "deploy"
	PhaseInfer     = "infer"
	PhaseRehearse  = "rehearse"
	PhaseRun       = "run"
	PhaseReset     = "reset"
	PhaseReport    = "report"
	PhaseTransport = "gate" // solo para el handoff cuando no hay salida segura
)

// PhaseLauncher ejecuta una fase que crea Jobs (adaptadores reales: T03 a T06).
// Devuelve las URIs de evidencia (solo la fase report las llena).
//
// Launch DEBE ser idempotente por (corrida, fase): ante un reintento (un crash entre Launch y el
// guardado de Launched repite el lanzamiento, hasta 3 veces por fase) el lanzador debe ADOPTAR el
// trabajo vivo de esa (corrida, fase) en vez de crear otro. Run.Started[fase] es solo el número de
// intento (tope 3) y NO sirve como clave de deduplicación: cambia en cada intento.
type PhaseLauncher interface {
	Launch(ctx context.Context, phase string, run Run) ([]string, error)
}

// RehearsalOutcome es el resultado leído del Job de ensayo. Done=false: aún corre (o no existe).
// Passed solo puede ser true si el Job terminó con éxito.
type RehearsalOutcome struct {
	Done    bool
	Passed  bool
	EventID string // idempotencia (se guarda en Seen)
}

// RehearsalResults lee el resultado del ensayo de la corrida (adaptador real: estado del Job vía KubeAPI).
// Un error nunca equivale a passed.
type RehearsalResults interface {
	Result(ctx context.Context, run Run) (RehearsalOutcome, error)
}

// Observer recibe métricas; todos los métodos deben ser seguros con nil en el controlador.
type Observer interface {
	Transition(from, to State, result string)
	GateCall(result string)
	Handoff(phase string)
	EventDropped(eventType string)
	PersistError()
}

// Resultados de aqs_run_transitions_total{result} y aqs_gate_calls_total{result}.
const (
	ResApplied = "applied"
	ResDenied  = "denied"
	ResError   = "error"
	ResIllegal = "illegal"
	ResAllow   = "allow"
	ResDeny    = "deny"
	ResInvalid = "invalid"
)

// Errores del controlador.
var (
	ErrNotFound = errors.New("corrida inexistente")
	ErrIllegal  = errors.New("transición ilegal")
	ErrDenied   = errors.New("gate denegó la transición")
	ErrGate     = errors.New("gate no disponible o respuesta inválida")
	ErrPersist  = errors.New("no se pudo persistir")
)
