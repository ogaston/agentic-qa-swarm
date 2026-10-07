package runctl

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
)

// DefaultNamespace es el único namespace donde el controlador emite transiciones.
const DefaultNamespace = "aqs-test"

// MaxRetries es el tope de reintentos por fase (V8): a la tercera falla, handoff.
const MaxRetries = 2

// Config agrupa los puertos del controlador. Todos salvo Observer y Log son obligatorios.
type Config struct {
	Namespace string // vacío: aqs-test
	Gate      GateClient
	Store     RunStore
	Publisher EventPublisher
	Warm      WarmStateReader
	Alerter   Alerter
	Phases    PhaseLauncher
	Observer  Observer
	Log       *slog.Logger
}

// Controller es el controlador de corridas. Serializa todas sus operaciones.
type Controller struct {
	c  Config
	mu sync.Mutex
}

// New valida la configuración (puertos presentes).
func New(c Config) (*Controller, error) {
	if c.Namespace == "" {
		c.Namespace = DefaultNamespace
	}
	switch {
	case c.Gate == nil:
		return nil, errors.New("falta el puerto GateClient")
	case c.Store == nil:
		return nil, errors.New("falta el puerto RunStore")
	case c.Publisher == nil:
		return nil, errors.New("falta el puerto EventPublisher")
	case c.Warm == nil:
		return nil, errors.New("falta el puerto WarmStateReader")
	case c.Alerter == nil:
		return nil, errors.New("falta el puerto Alerter")
	case c.Phases == nil:
		return nil, errors.New("falta el puerto PhaseLauncher")
	}
	if c.Log == nil {
		c.Log = slog.New(slog.DiscardHandler)
	}
	return &Controller{c: c}, nil
}

// Namespace devuelve el namespace fijo de las transiciones.
func (k *Controller) Namespace() string { return k.c.Namespace }

func (k *Controller) obsTransition(from, to State, res string) {
	if k.c.Observer != nil {
		k.c.Observer.Transition(from, to, res)
	}
}

// Apply procesa un evento de entrada. run.confirmed es idempotente: una corrida existente
// nunca se recrea ni vuelve a confirmed.
func (k *Controller) Apply(ctx context.Context, ev Event) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	switch ev.Type {
	case EvRunConfirmed:
		if ev.RunID == "" {
			return errors.New("run.confirmed sin run_id")
		}
		if _, ok := k.c.Store.Get(ev.RunID); ok {
			return nil
		}
		return k.c.Store.Save(Run{ID: ev.RunID, State: Confirmed, TraceID: ev.TraceID,
			ConfirmedBy: ev.ConfirmedBy, Flows: ev.Flows, Seen: []string{ev.EventID}})
	case EvRehearsalPassed, EvRehearsalFailed:
		r, ok := k.c.Store.Get(ev.RunID)
		if !ok {
			return fmt.Errorf("%s: %w", ev.Type, ErrNotFound)
		}
		if ev.EventID != "" && slices.Contains(r.Seen, ev.EventID) {
			return nil
		}
		if ev.EventID != "" {
			r.Seen = append(r.Seen, ev.EventID)
		}
		if r.State.Terminal() {
			return k.c.Store.Save(r)
		}
		r.EnsayoPassed = FactOf(ev.Type == EvRehearsalPassed)
		return k.c.Store.Save(r)
	}
	return fmt.Errorf("tipo de evento no soportado %q", ev.Type)
}

// request arma el GateRequest con los hechos que el controlador conoce; el resto va unknown.
func (k *Controller) request(ctx context.Context, r Run, to State) GateRequest {
	req := GateRequest{
		RunID: r.ID, From: r.State, To: to, TargetNamespace: k.c.Namespace, Workflow: r.Workflow,
		TraceID: r.TraceID, Confirmed: Unknown, ResetVerified: Unknown, EnsayoPassed: Unknown, WorkflowAllowed: Unknown,
	}
	if r.ConfirmedBy != "" { // run.confirmed trae quién confirmó: solo entonces se sabe
		req.Confirmed = True
	}
	if len(r.Flows) > 0 { // los flujos los confirmó el humano; go-governance no lo contrasta en warm_ready
		req.WorkflowAllowed = True
	}
	if r.EnsayoPassed == True || r.EnsayoPassed == False {
		req.EnsayoPassed = r.EnsayoPassed
	}
	if to == WarmReady || to == Deploying {
		if f, err := k.c.Warm.ResetVerified(ctx); err == nil && (f == True || f == False) {
			req.ResetVerified = f
		}
	}
	return req
}

// Transition pide el gate y, solo si permite, aplica from -> to y lo persiste. Cualquier otra
// cosa (ilegal, deniega, error, timeout, respuesta inválida) deja el estado intacto y devuelve
// ErrIllegal, ErrDenied o ErrGate. Nunca reintenta un gate denegado.
func (k *Controller) Transition(ctx context.Context, id string, to State) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.transition(ctx, id, to)
}

func (k *Controller) transition(ctx context.Context, id string, to State) error {
	r, ok := k.c.Store.Get(id)
	if !ok {
		return ErrNotFound
	}
	from := r.State
	if !Legal(from, to) {
		k.obsTransition(from, to, ResIllegal)
		return fmt.Errorf("%w: %s -> %s", ErrIllegal, from, to)
	}
	dec, err := k.c.Gate.Authorize(ctx, k.request(ctx, r, to))
	switch {
	case err != nil:
		k.gateCall(ResError)
		k.obsTransition(from, to, ResError)
		return fmt.Errorf("%w: %v", ErrGate, err)
	case !dec.Allow:
		k.gateCall(ResDeny)
		k.obsTransition(from, to, ResDenied)
		k.c.Log.WarnContext(ctx, "gate denegó la transición", "run_id", id, "from", from, "to", to,
			"reason", dec.Reason, "audit_ref", dec.AuditRef, "trace_id", r.TraceID)
		return fmt.Errorf("%w: %s", ErrDenied, dec.Reason)
	}
	k.gateCall(ResAllow)
	r.State, r.Launched = to, nil
	if err := k.c.Store.Save(r); err != nil {
		k.obsTransition(from, to, ResError)
		return fmt.Errorf("persistiendo la transición: %w", err)
	}
	k.obsTransition(from, to, ResApplied)
	k.c.Log.InfoContext(ctx, "transición aplicada", "run_id", id, "from", from, "to", to, "trace_id", r.TraceID)
	return nil
}

func (k *Controller) gateCall(res string) {
	if k.c.Observer != nil {
		k.c.Observer.GateCall(res)
	}
}

// failRun lleva la corrida a resetting (si ya desplegó) o a failed, pasando por el gate. Si el
// gate tampoco lo permite, la corrida queda detenida (Halted) con handoff: sin salida segura.
func (k *Controller) failRun(ctx context.Context, id, reason string) error {
	r, ok := k.c.Store.Get(id)
	if !ok {
		return ErrNotFound
	}
	if r.FailReason == "" {
		r.FailReason = reason
		if err := k.c.Store.Save(r); err != nil {
			return err
		}
	}
	target := Failed
	if r.State.Resettable() {
		target = Resetting
	}
	if err := k.transition(ctx, id, target); err != nil {
		r, _ = k.c.Store.Get(id)
		r.Halted = true
		k.handoff(ctx, r, PhaseTransport, fmt.Sprintf("sin salida segura hacia %s: %v", target, err))
		return errors.Join(err, k.c.Store.Save(r))
	}
	return nil
}

func (k *Controller) handoff(ctx context.Context, r Run, phase, reason string) {
	k.c.Alerter.Handoff(ctx, r.ID, phase, reason)
	if k.c.Observer != nil {
		k.c.Observer.Handoff(phase)
	}
	k.c.Log.ErrorContext(ctx, "handoff humano", "run_id", r.ID, "phase", phase, "reason", reason, "trace_id", r.TraceID)
}

// DriveAll avanza un paso cada corrida no terminal (y reintenta publicar run.done pendientes).
func (k *Controller) DriveAll(ctx context.Context) {
	for _, r := range k.c.Store.List() {
		if err := k.Drive(ctx, r.ID); err != nil {
			k.c.Log.WarnContext(ctx, "la corrida no avanzó", "run_id", r.ID, "state", r.State, "error", err.Error(), "trace_id", r.TraceID)
		}
	}
}

// phaseOf asocia cada estado con la fase que hay que lanzar en él y el estado siguiente.
func phaseOf(s State) (phase string, next State) {
	switch s {
	case Deploying:
		return PhaseDeploy, Inferring
	case Inferring:
		return PhaseInfer, Rehearsing
	case Rehearsing:
		return PhaseRehearse, Running
	case Running:
		return PhaseRun, Resetting
	case Resetting:
		return PhaseReset, Reporting
	case Reporting:
		return PhaseReport, Done
	}
	return "", ""
}

// Drive da un paso a la corrida id según su estado persistido.
func (k *Controller) Drive(ctx context.Context, id string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	r, ok := k.c.Store.Get(id)
	if !ok {
		return ErrNotFound
	}
	if r.Halted {
		return nil
	}
	if r.State == Done && !r.DonePublish {
		return k.publishDone(ctx, r)
	}
	if r.State.Terminal() {
		return nil
	}
	if r.State == Confirmed || r.State == WarmReady {
		if err := k.transition(ctx, id, chain[r.State]); err != nil {
			return errors.Join(err, k.failRun(ctx, id, err.Error()))
		}
		return nil
	}
	phase, next := phaseOf(r.State)
	if !r.Launched[phase] {
		uris, err := k.c.Phases.Launch(ctx, phase, r.clone())
		if err != nil {
			return k.phaseFailed(ctx, r, phase, err.Error())
		}
		if r.Launched == nil {
			r.Launched = map[string]bool{}
		}
		r.Launched[phase] = true
		if phase == PhaseReport {
			r.Evidence = uris
		}
		if err := k.c.Store.Save(r); err != nil {
			return err
		}
	}
	if phase == PhaseRehearse {
		switch r.EnsayoPassed {
		case False:
			return k.phaseFailed(ctx, r, phase, "rehearsal.failed")
		case True:
		default:
			return nil // esperando rehearsal.passed: no hay nada que preguntar al gate todavía
		}
	}
	if err := k.transition(ctx, id, next); err != nil {
		if errors.Is(err, ErrIllegal) || errors.Is(err, ErrNotFound) {
			return err
		}
		return errors.Join(err, k.failRun(ctx, id, err.Error()))
	}
	if next == Done {
		r, _ = k.c.Store.Get(id)
		return k.publishDone(ctx, r)
	}
	if next == Resetting && r.State == Running {
		return nil
	}
	return nil
}

// phaseFailed cuenta un fallo de la fase; hasta MaxRetries se reintenta en el próximo paso, al
// siguiente se hace handoff y la corrida sale (resetting o failed).
func (k *Controller) phaseFailed(ctx context.Context, r Run, phase, reason string) error {
	if r.Attempts == nil {
		r.Attempts = map[string]int{}
	}
	r.Attempts[phase]++
	delete(r.Launched, phase)
	if phase == PhaseRehearse {
		r.EnsayoPassed = Unknown
	}
	if err := k.c.Store.Save(r); err != nil {
		return err
	}
	if r.Attempts[phase] <= MaxRetries {
		k.c.Log.WarnContext(ctx, "fase fallida; se reintentará", "run_id", r.ID, "phase", phase,
			"attempt", r.Attempts[phase], "trace_id", r.TraceID)
		return nil
	}
	k.handoff(ctx, r, phase, reason)
	return k.failRun(ctx, r.ID, fmt.Sprintf("fase %s agotó sus reintentos: %s", phase, reason))
}

func (k *Controller) publishDone(ctx context.Context, r Run) error {
	if r.FailReason != "" {
		return nil
	}
	if err := k.c.Publisher.Publish(ctx, OutEvent{Type: "run.done", RunID: r.ID, TraceID: r.TraceID, EvidenceURIs: r.Evidence}); err != nil {
		return fmt.Errorf("publicando run.done: %w", err)
	}
	r.DonePublish = true
	return k.c.Store.Save(r)
}
