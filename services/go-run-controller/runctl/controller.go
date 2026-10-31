package runctl

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"
)

// DefaultNamespace es el único namespace donde el controlador emite transiciones.
const DefaultNamespace = "aqs-test"

// MaxRetries es el tope de reintentos por fase (V8): a la tercera falla, handoff.
const MaxRetries = 2

// DefaultExecutionApprovalTTL es cuánto espera una corrida ensayada la confirmación humana
// antes de salir por la ruta de fallo normal (U12-T02).
const DefaultExecutionApprovalTTL = 30 * time.Minute

// Config agrupa los puertos del controlador. Todos salvo Observer y Log son obligatorios.
type Config struct {
	Namespace string // vacío: aqs-test
	Gate      GateClient
	Store     RunStore
	Publisher EventPublisher
	Warm      WarmStateReader
	Alerter   Alerter
	Phases    PhaseLauncher
	Results   RehearsalResults // opcional: lee el resultado del Job de ensayo (sin él, rehearsal.* llega por eventos)
	Runners   RunnerResults    // opcional: avance de la fase run (runners, evidencia, run.done al terminar)
	Observer  Observer
	Log       *slog.Logger
	Now       func() time.Time // reloj (nil: time.Now); inyectable en pruebas
	// ExecutionApprovalTTL es el plazo para aprobar la ejecución tras el ensayo (<=0: 30 min).
	ExecutionApprovalTTL time.Duration
}

// Controller es el controlador de corridas. Serializa todas sus operaciones.
type Controller struct {
	c  Config
	mu sync.Mutex

	healthMu    sync.Mutex
	lastSaveErr error
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
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.ExecutionApprovalTTL <= 0 {
		c.ExecutionApprovalTTL = DefaultExecutionApprovalTTL
	}
	return &Controller{c: c}, nil
}

// now lee el reloj configurado (New siempre lo deja no nulo).
func (k *Controller) now() time.Time {
	if k.c.Now != nil {
		return k.c.Now()
	}
	return time.Now()
}

// AwaitingExecution indica si la corrida espera la confirmación humana para ejecutar.
func AwaitingExecution(r Run) bool {
	return r.State == Rehearsing && r.EnsayoPassed == True && r.ExecutionApprovedBy == ""
}

// ApproveExecution registra la aprobación humana de la ejecución (U12-T02). Serializada con k.mu:
// ErrNotFound si no existe; ErrForbidden si no es admin ni el dueño; ErrNotAwaiting si la corrida
// no espera aprobación o ya la aprobó otra persona. Repetirla con el mismo principal es idempotente.
func (k *Controller) ApproveExecution(ctx context.Context, id, principalID string, admin bool) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	r, ok := k.c.Store.Get(id)
	if !ok {
		return ErrNotFound
	}
	ctx = withTrace(ctx, r.TraceID)
	if !admin && principalID != r.ConfirmedBy {
		return ErrForbidden
	}
	if r.ExecutionApprovedBy != "" {
		if r.ExecutionApprovedBy == principalID {
			return nil // idempotente: el mismo principal puede repetir la aprobación
		}
		return ErrNotAwaiting // ya la aprobó otra persona
	}
	if !AwaitingExecution(r) {
		return ErrNotAwaiting
	}
	r.ExecutionApprovedBy = principalID
	if err := k.save(r); err != nil {
		return err
	}
	k.c.Log.InfoContext(ctx, "ejecución aprobada por el humano", "run_id", id, "principal_id", principalID, "admin", admin)
	return nil
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
		return k.save(Run{ID: ev.RunID, State: Confirmed, TraceID: ev.TraceID,
			ConfirmedBy: ev.ConfirmedBy, Flows: ev.Flows, Seen: []string{ev.EventID}, Artifact: copyArtifact(ev.Artifact)})
	case EvRehearsalPassed, EvRehearsalFailed:
		r, ok := k.c.Store.Get(ev.RunID)
		if !ok {
			return fmt.Errorf("%s: %w", ev.Type, ErrNotFound)
		}
		if ev.EventID != "" && slices.Contains(r.Seen, ev.EventID) {
			return nil
		}
		r = markSeen(r, ev.EventID)
		if r.State != Rehearsing || !r.Launched[PhaseRehearse] {
			// Fuera de la fase de ensayo el hecho no se observó: se descarta (y no se aplicará en un replay).
			k.c.Log.WarnContext(withTrace(ctx, r.TraceID), "evento de ensayo fuera de estado o antes del lanzamiento descartado",
				"run_id", r.ID, "type", ev.Type, "state", r.State)
			if k.c.Observer != nil {
				k.c.Observer.EventDropped(ev.Type)
			}
			return k.save(r)
		}
		r.EnsayoPassed = FactOf(ev.Type == EvRehearsalPassed)
		return k.save(r)
	}
	return fmt.Errorf("tipo de evento no soportado %q", ev.Type)
}

// copyArtifact devuelve una copia del artefacto del evento (nil si no lo trae).
func copyArtifact(a *Artifact) *Artifact {
	if a == nil {
		return nil
	}
	c := *a
	return &c
}

func markSeen(r Run, eventID string) Run {
	if eventID != "" && !slices.Contains(r.Seen, eventID) {
		r.Seen = append(r.Seen, eventID)
	}
	return r
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
	ctx = withTrace(ctx, r.TraceID)
	from := r.State
	if !Legal(from, to) {
		k.obsTransition(from, to, ResIllegal)
		return fmt.Errorf("%w: %s -> %s", ErrIllegal, from, to)
	}
	if from == Rehearsing && to == Running {
		// Gate duro y no omitible: sin ensayo_passed=true registrado en el almacén no hay runner.
		if r.EnsayoPassed != True {
			k.obsTransition(from, to, ResIllegal)
			return fmt.Errorf("%w: %s -> %s sin ensayo_passed registrado", ErrIllegal, from, to)
		}
		// U12-T02: tampoco hay runner sin la confirmación humana de la ejecución.
		if r.ExecutionApprovedBy == "" {
			k.obsTransition(from, to, ResIllegal)
			return fmt.Errorf("%w: %s -> %s sin aprobación de ejecución", ErrIllegal, from, to)
		}
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
			"reason", dec.Reason, "audit_ref", dec.AuditRef)
		return fmt.Errorf("%w: %s", ErrDenied, dec.Reason)
	}
	k.gateCall(ResAllow)
	r.State, r.Launched = to, nil
	if err := k.save(r); err != nil {
		k.obsTransition(from, to, ResError)
		return err
	}
	k.obsTransition(from, to, ResApplied)
	k.c.Log.InfoContext(ctx, "transición aplicada", "run_id", id, "from", from, "to", to)
	return nil
}

func (k *Controller) gateCall(res string) {
	if k.c.Observer != nil {
		k.c.Observer.GateCall(res)
	}
}

// failRun lleva la corrida a resetting (si ya desplegó) o a failed, pasando por el gate. Si el
// gate lo deniega, la corrida queda detenida (Halted) con un handoff; si está caído o el disco
// falla, la salida se reintenta en cada paso (nunca se avanza sin permiso del gate).
func (k *Controller) failRun(ctx context.Context, id, reason string) error {
	r, ok := k.c.Store.Get(id)
	if !ok {
		return ErrNotFound
	}
	ctx = withTrace(ctx, r.TraceID)
	if err := k.markFail(r, reason); err != nil {
		return err
	}
	target := Failed
	if r.State.Resettable() {
		target = Resetting
	}
	if err := k.transition(ctx, id, target); err != nil {
		if errors.Is(err, ErrGate) || errors.Is(err, ErrPersist) {
			return err
		}
		r, _ = k.c.Store.Get(id)
		r.Halted = true // el gate denegó la salida: no se reintenta
		if herr := k.handoffOnce(ctx, r, PhaseTransport, fmt.Sprintf("sin salida segura hacia %s: %v", target, err)); herr != nil {
			return errors.Join(err, herr)
		}
		return err
	}
	return nil
}

// markFail fija FailReason (si no estaba) y lo persiste.
func (k *Controller) markFail(r Run, reason string) error {
	if r.FailReason != "" {
		return nil
	}
	r.FailReason = reason
	return k.save(r)
}

// save persiste vía el puerto, registra el fallo (métrica y readyz) y lo marca como ErrPersist.
func (k *Controller) save(r Run) error {
	err := k.c.Store.Save(r)
	k.healthMu.Lock()
	k.lastSaveErr = err
	k.healthMu.Unlock()
	if err != nil {
		if k.c.Observer != nil {
			k.c.Observer.PersistError()
		}
		return fmt.Errorf("%w: %v", ErrPersist, err)
	}
	return nil
}

// PersistHealthy es nil si el último Save salió bien (usado por /readyz).
func (k *Controller) PersistHealthy() error {
	k.healthMu.Lock()
	defer k.healthMu.Unlock()
	return k.lastSaveErr
}

// handoffOnce persiste que el handoff de la fase se emitió ANTES de alertar: un Save que falla
// impide el efecto, y uno que sale bien garantiza que no se repite (a lo sumo una vez por fase).
func (k *Controller) handoffOnce(ctx context.Context, r Run, phase, reason string) error {
	if r.HandedOff[phase] {
		return nil
	}
	if r.HandedOff == nil {
		r.HandedOff = map[string]bool{}
	}
	r.HandedOff[phase] = true
	if err := k.save(r); err != nil {
		return err
	}
	k.c.Alerter.Handoff(ctx, r.ID, phase, reason)
	if k.c.Observer != nil {
		k.c.Observer.Handoff(phase)
	}
	k.c.Log.ErrorContext(ctx, "handoff humano", "run_id", r.ID, "phase", phase, "reason", reason)
	return nil
}

// exhausted indica que la fase ya no puede lanzarse: más de MaxRetries fallos o 3 lanzamientos
// iniciados (contados en el almacén ANTES de lanzar: cota dura aunque falle el guardado posterior).
func exhausted(r Run, phase string) bool {
	return r.Attempts[phase] > MaxRetries || r.Started[phase] >= MaxRetries+1
}

// exhaust hace el handoff (una vez) y saca la corrida por failRun.
func (k *Controller) exhaust(ctx context.Context, r Run, phase, reason string) error {
	if err := k.handoffOnce(ctx, r, phase, reason); err != nil {
		return err
	}
	r, _ = k.c.Store.Get(r.ID)
	return k.failRun(ctx, r.ID, fmt.Sprintf("fase %s agotó sus reintentos: %s", phase, reason))
}

// DriveAll avanza un paso cada corrida no terminal (y reintenta publicar run.done pendientes).
func (k *Controller) DriveAll(ctx context.Context) {
	for _, r := range k.c.Store.List() {
		if err := k.Drive(ctx, r.ID); err != nil {
			k.c.Log.WarnContext(ctx, "la corrida no avanzó", "run_id", r.ID, "state", r.State, "error", err.Error())
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
	ctx = withTrace(ctx, r.TraceID)
	if r.Halted {
		return nil
	}
	if r.State == Done && !r.DonePublish {
		return k.publishDone(ctx, r)
	}
	if r.State.Terminal() {
		return nil
	}
	if r.FailReason != "" && (r.State != Resetting || exhausted(r, PhaseReset)) {
		// Salida pendiente (gate caído o disco): solo se reintenta la transición, nunca la fase agotada.
		return k.failRun(ctx, id, r.FailReason)
	}
	if r.State == Confirmed || r.State == WarmReady {
		return k.advance(ctx, r, chain[r.State])
	}
	if r.State == Running && r.EnsayoPassed != True {
		return k.failRun(ctx, id, "corrida en running sin ensayo_passed registrado")
	}
	phase, next := phaseOf(r.State)
	if phase == PhaseReset && r.FailReason != "" {
		next = Failed // la corrida fallida cierra en failed tras el reset verificado
	}
	if !r.Launched[phase] {
		if exhausted(r, phase) {
			return k.exhaust(ctx, r, phase, "reintentos agotados")
		}
		// Se persiste la intención de lanzar ANTES de lanzar: sin Save no hay efecto externo.
		if r.Started == nil {
			r.Started = map[string]int{}
		}
		r.Started[phase]++
		if err := k.save(r); err != nil {
			return err
		}
		if rl, ok := k.c.Phases.(ReportLauncher); ok && phase == PhaseReport {
			// La fase report real devuelve el veredicto y conserva la evidencia que dejó la fase run.
			res, err := rl.LaunchReport(ctx, r.clone())
			if err != nil {
				return k.phaseFailed(ctx, r, phase, err.Error())
			}
			r.Verdict, r.ReportURI = res.Verdict, res.ReportURI
		} else {
			uris, err := k.c.Phases.Launch(ctx, phase, r.clone())
			k.adoptPlan(&r) // StoredPlans guarda el plan dentro de Launch: no lo pisa el save de abajo
			if err != nil {
				return k.phaseFailed(ctx, r, phase, err.Error())
			}
			if phase == PhaseReport {
				r.Evidence = uris
			}
		}
		if r.Launched == nil {
			r.Launched = map[string]bool{}
		}
		r.Launched[phase] = true
		if err := k.save(r); err != nil {
			return err
		}
	}
	if phase == PhaseRun && k.c.Runners != nil {
		if done, err := k.runnersStep(ctx, r); err != nil || !done {
			return err
		}
		r, _ = k.c.Store.Get(id)
	}
	if phase == PhaseRehearse {
		if r.EnsayoPassed != True && r.EnsayoPassed != False && k.c.Results != nil {
			out, err := k.c.Results.Result(ctx, r.clone())
			if err != nil {
				return fmt.Errorf("leyendo el resultado del ensayo: %w", err) // no se asume nada: se vuelve a leer
			}
			if out.Done {
				r = markSeen(r, out.EventID)
				r.EnsayoPassed = FactOf(out.Passed)
				if err := k.save(r); err != nil {
					return err
				}
			}
		}
		switch r.EnsayoPassed {
		case False:
			return k.phaseFailed(ctx, r, phase, "rehearsal.failed")
		case True:
			// U12-T02: el ensayo aprobado no ejecuta nada por sí solo. La corrida espera a que su
			// dueño (o un admin) confirme la ejecución; sin confirmación en el TTL sale por fallo.
			if r.ExecutionApprovedBy == "" {
				now := k.now()
				if r.AwaitingSince.IsZero() {
					r.AwaitingSince = now
					if err := k.save(r); err != nil {
						return err
					}
				} else if now.Sub(r.AwaitingSince) > k.c.ExecutionApprovalTTL {
					return k.failRun(ctx, id, "ejecución no aprobada a tiempo")
				}
				return nil
			}
		default:
			return nil // esperando rehearsal.passed: no hay nada que preguntar al gate todavía
		}
	}
	if err := k.advance(ctx, r, next); err != nil {
		return err
	}
	if next == Done {
		r, _ = k.c.Store.Get(id)
		return k.publishDone(ctx, r)
	}
	return nil
}

// advance aplica el avance normal. Gate caído: se fija FailReason y la salida se reintenta en el
// siguiente paso (una sola llamada al gate por paso). Gate deniega: salida inmediata. Error de
// persistencia: no pasa nada (no avanza, no falla, no hace handoff) y se reintenta.
func (k *Controller) advance(ctx context.Context, r Run, to State) error {
	err := k.transition(ctx, r.ID, to)
	switch {
	case err == nil, errors.Is(err, ErrIllegal), errors.Is(err, ErrNotFound), errors.Is(err, ErrPersist):
		return err
	case errors.Is(err, ErrGate):
		return errors.Join(err, k.markFail(r, err.Error()))
	}
	return errors.Join(err, k.failRun(ctx, r.ID, err.Error()))
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
	if err := k.save(r); err != nil {
		return err
	}
	if r.Attempts[phase] <= MaxRetries {
		k.c.Log.WarnContext(ctx, "fase fallida; se reintentará", "run_id", r.ID, "phase", phase,
			"attempt", r.Attempts[phase])
		return nil
	}
	return k.exhaust(ctx, r, phase, reason)
}

// adoptPlan copia el plan que el lanzador acaba de persistir en la corrida. Drive trabaja con una copia
// tomada antes de Launch; sin esto el save posterior borraría Run.Plan.
func (k *Controller) adoptPlan(r *Run) {
	if cur, ok := k.c.Store.Get(r.ID); ok {
		r.Plan = cur.Plan
	}
}

// runnersStep avanza los runners. Devuelve done=true cuando la evidencia de TODOS los flujos está
// guardada y run.done ya se publicó (solo con >=1 URI y sin FailReason). Un fallo de evidencia
// saca la corrida por reset sin run.done.
func (k *Controller) runnersStep(ctx context.Context, r Run) (bool, error) {
	if r.DonePublish {
		return true, nil
	}
	if len(r.Evidence) == 0 {
		out, err := k.c.Runners.Progress(ctx, r.clone())
		if err != nil {
			return false, fmt.Errorf("avance de los runners: %w", err) // no se asume nada: se vuelve a leer
		}
		if !out.Done {
			return false, nil
		}
		if out.FailReason != "" || len(out.URIs) == 0 {
			reason := out.FailReason
			if reason == "" {
				reason = "los runners no dejaron evidencia"
			}
			return false, k.failRun(ctx, r.ID, reason)
		}
		r.Evidence = out.URIs
		if err := k.save(r); err != nil {
			return false, err
		}
	}
	if err := k.c.Publisher.Publish(ctx, OutEvent{Type: "run.done", RunID: r.ID, TraceID: r.TraceID, EvidenceURIs: r.Evidence}); err != nil {
		return false, fmt.Errorf("publicando run.done: %w", err)
	}
	r.DonePublish = true
	return true, k.save(r)
}

func (k *Controller) publishDone(ctx context.Context, r Run) error {
	if r.FailReason != "" || r.DonePublish { // run.done ya salió al terminar los runners
		return nil
	}
	if err := k.c.Publisher.Publish(ctx, OutEvent{Type: "run.done", RunID: r.ID, TraceID: r.TraceID, EvidenceURIs: r.Evidence}); err != nil {
		return fmt.Errorf("publicando run.done: %w", err)
	}
	r.DonePublish = true
	return k.save(r)
}

type traceKey struct{}

// withTrace pone el trace_id de la corrida en el contexto: el logger lo añade una sola vez.
func withTrace(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, traceKey{}, id)
}

// TraceFrom devuelve el trace_id de la corrida puesto por el controlador ("" si no hay).
func TraceFrom(ctx context.Context) string { s, _ := ctx.Value(traceKey{}).(string); return s }
