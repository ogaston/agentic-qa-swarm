package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/plan"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

// FlowSource entrega el FlowPlan de la corrida (U3; hoy falla cerrado).
type FlowSource interface {
	Flows(runID string) (plan.FlowPlan, error)
}

// Limits son los plazos de la política del workflow.
type Limits struct{ FlowTimeout, RunTimeout time.Duration }

// Policy entrega los plazos del workflow. Un error es fail-closed: no se lanza nada.
type Policy interface {
	Limits(ctx context.Context, workflow string) (Limits, error)
}

// StaticPolicy son plazos fijos (de la configuración del servicio).
type StaticPolicy struct{ L Limits }

// Limits implementa Policy.
func (s StaticPolicy) Limits(context.Context, string) (Limits, error) {
	if s.L.FlowTimeout <= 0 || s.L.RunTimeout <= 0 {
		return Limits{}, errors.New("política de plazos ilegible")
	}
	return s.L, nil
}

// Observer recibe métricas (nil-seguro vía el launcher).
type Observer interface {
	RunnerJob(result string)
	EvidenceObject(result string)
	RunDuration(seconds float64)
}

// Resultados de aqs_runner_jobs_total y aqs_evidence_objects_total.
const (
	ResPassed  = "passed"
	ResFailed  = "failed"
	ResTimeout = "timeout"
	ResStored  = "stored"
	ResError   = "error"
)

// Launcher implementa runctl.PhaseLauncher (fase run) y runctl.RunnerResults.
type Launcher struct {
	Kube        KubeAPI
	Cfg         Config // DeadlineSeconds lo fija la política en cada Job
	Plans       FlowSource
	Exec        Executor
	Evidence    Evidence
	Gate        runctl.GateClient // cuota y aprobación del workflow (U4)
	Policy      Policy
	MaxParallel int
	Now         func() time.Time
	Obs         Observer
	Log         *slog.Logger

	mu     sync.Mutex
	evFail map[string]bool // (corrida, flujo) cuya evidencia no pudo escribirse: flujo fallido
}

var (
	_ runctl.PhaseLauncher = (*Launcher)(nil)
	_ runctl.RunnerResults = (*Launcher)(nil)
)

func (l *Launcher) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

func (l *Launcher) log() *slog.Logger {
	if l.Log != nil {
		return l.Log
	}
	return slog.New(slog.DiscardHandler)
}

func (l *Launcher) metric(f func(Observer)) {
	if l.Obs != nil {
		f(l.Obs)
	}
}

func selector(runID string) string {
	return fmt.Sprintf("%s=%s,%s=%s", LabelRun, runID, LabelPhase, PhaseValue)
}

func jobState(j batchv1.Job) (done, passed bool) {
	for _, c := range j.Status.Conditions {
		if c.Status != corev1.ConditionTrue {
			continue
		}
		switch c.Type {
		case batchv1.JobComplete:
			return true, true
		case batchv1.JobFailed:
			return true, false
		}
	}
	if j.Status.Failed > 0 {
		return true, false
	}
	return false, false
}

func key(runID, flowID, name string) string { return "runs/" + runID + "/" + flowID + "/" + name }

type flowResult struct {
	FlowID     string `json:"flow_id"`
	Status     string `json:"status"` // passed | failed | timeout
	LogsSHA256 string `json:"logs_sha256,omitempty"`
}

// Launch implementa runctl.PhaseLauncher: solo la fase run, solo con ensayo_passed registrado.
// Es idempotente por (corrida, flujo): el Job vivo se adopta (por nombre determinista y etiquetas)
// y AlreadyExists cuenta como éxito.
func (l *Launcher) Launch(ctx context.Context, phase string, run runctl.Run) ([]string, error) {
	if phase != runctl.PhaseRun {
		return nil, fmt.Errorf("fase %q no es de este lanzador", phase)
	}
	_, err := l.step(ctx, run)
	return nil, err
}

// Progress implementa runctl.RunnerResults.
func (l *Launcher) Progress(ctx context.Context, run runctl.Run) (runctl.RunOutcome, error) {
	return l.step(ctx, run)
}

// step es el único camino que crea runner-*: ensayo_passed, plan, política y cuota se verifican
// ANTES de tocar Kubernetes; cualquier error de esas lecturas es fail-closed (nada se lanza).
func (l *Launcher) step(ctx context.Context, run runctl.Run) (runctl.RunOutcome, error) {
	if run.EnsayoPassed != runctl.True {
		return runctl.RunOutcome{}, errors.New("sin ensayo_passed registrado: no se lanzan runners")
	}
	fp, err := l.Plans.Flows(run.ID)
	if err != nil {
		return runctl.RunOutcome{}, fmt.Errorf("plan de flujos: %w", err)
	}
	if err := plan.ValidateFlowPlan(fp); err != nil {
		return runctl.RunOutcome{}, err
	}
	if fp.RunID != run.ID {
		return runctl.RunOutcome{}, errors.New("flow-plan de otra corrida")
	}
	lim, err := l.Policy.Limits(ctx, fp.Workflow)
	if err != nil {
		return runctl.RunOutcome{}, fmt.Errorf("política de workflow: %w", err)
	}
	max := l.MaxParallel
	if max < 1 {
		max = 3
	}
	jobs, err := l.Kube.ListJobs(ctx, selector(run.ID))
	if err != nil {
		return runctl.RunOutcome{}, fmt.Errorf("listando Jobs runner: %w", err)
	}
	byFlow := map[string]batchv1.Job{}
	var first time.Time
	for _, j := range jobs {
		byFlow[j.Labels[LabelFlow]] = j
		if t, err := time.Parse(time.RFC3339, j.Annotations[AnnotCreated]); err == nil && (first.IsZero() || t.Before(first)) {
			first = t
		}
	}
	now := l.now()
	runExpired := !first.IsZero() && now.Sub(first) > lim.RunTimeout

	resolved := map[string]bool{} // flujo con evidencia leída de vuelta (cualquier estado del runner)
	failed := map[string]string{} // flujo sin evidencia
	var pending []plan.Flow
	active := 0
	for _, f := range fp.Flows {
		if _, found, err := l.readResult(ctx, run.ID, f.FlowID); err != nil {
			return runctl.RunOutcome{}, err // lectura ilegible: se vuelve a leer, nunca se asume
		} else if found {
			resolved[f.FlowID] = true
			continue
		}
		if l.hasEvFail(run.ID, f.FlowID) {
			failed[f.FlowID] = "evidencia no escrita"
			continue
		}
		j, has := byFlow[f.FlowID]
		if !has {
			if runExpired { // el plazo de la corrida venció antes de lanzarlo
				l.settle(ctx, run, f.FlowID, ResTimeout, nil, resolved, failed)
				continue
			}
			pending = append(pending, f)
			continue
		}
		done, passed := jobState(j)
		expired := false
		if !done {
			created, perr := time.Parse(time.RFC3339, j.Annotations[AnnotCreated])
			expired = runExpired || perr != nil || now.Sub(created) > lim.FlowTimeout
		}
		if !done && !expired {
			active++
			continue
		}
		status := ResFailed
		switch {
		case expired:
			status = ResTimeout
		case passed:
			status = ResPassed
		}
		logs, _ := l.Kube.JobLogs(ctx, j.Name) // sin logs legibles: el resultado se guarda igual
		l.settle(ctx, run, f.FlowID, status, logs, resolved, failed)
		if expired { // nunca queda un Job colgado
			if err := l.Kube.DeleteJob(ctx, j.Name); err != nil {
				return runctl.RunOutcome{}, fmt.Errorf("eliminando el Job vencido: %w", err)
			}
		}
	}

	if len(pending) > 0 && active < max {
		if len(jobs) == 0 { // primera ola: cuota y aprobación del workflow (gate de U4)
			if err := l.authorize(ctx, run, fp.Workflow); err != nil {
				return runctl.RunOutcome{}, err
			}
		}
		cfg := l.Cfg
		cfg.DeadlineSeconds = int64(lim.FlowTimeout / time.Second)
		for _, f := range pending {
			if active >= max {
				break
			}
			spec, err := l.Exec.Plan(f)
			if err != nil {
				return runctl.RunOutcome{}, fmt.Errorf("motor: %w", err)
			}
			job, err := BuildJob(cfg, run.ID, f.FlowID, spec, now)
			if err != nil {
				return runctl.RunOutcome{}, err
			}
			if err := l.Kube.CreateJob(ctx, job); err != nil && !apierrors.IsAlreadyExists(err) {
				return runctl.RunOutcome{}, fmt.Errorf("creando el Job runner: %w", err)
			}
			active++
		}
		return runctl.RunOutcome{}, nil
	}
	if len(pending) > 0 || active > 0 {
		return runctl.RunOutcome{}, nil
	}

	out := runctl.RunOutcome{Done: true}
	for _, f := range fp.Flows {
		if !resolved[f.FlowID] {
			out.FailReason = fmt.Sprintf("flujo %s: %s", f.FlowID, failed[f.FlowID])
			continue
		}
		out.URIs = append(out.URIs, l.Evidence.URI(key(run.ID, f.FlowID, "logs.txt")), l.Evidence.URI(key(run.ID, f.FlowID, "result.json")))
	}
	if !first.IsZero() {
		l.metric(func(o Observer) { o.RunDuration(now.Sub(first).Seconds()) })
	}
	return out, nil
}

func (l *Launcher) authorize(ctx context.Context, run runctl.Run, workflow string) error {
	req := runctl.GateRequest{RunID: run.ID, From: runctl.Rehearsing, To: runctl.Running, TargetNamespace: l.Cfg.Namespace,
		Workflow: workflow, TraceID: run.TraceID, Confirmed: runctl.Unknown, ResetVerified: runctl.Unknown,
		EnsayoPassed: run.EnsayoPassed, WorkflowAllowed: runctl.True}
	if run.ConfirmedBy != "" {
		req.Confirmed = runctl.True
	}
	dec, err := l.Gate.Authorize(ctx, req)
	if err != nil {
		return fmt.Errorf("cuota del workflow ilegible: %w", err)
	}
	if !dec.Allow {
		return fmt.Errorf("cuota o aprobación del workflow denegada: %s", dec.Reason)
	}
	return nil
}

func (l *Launcher) hasEvFail(runID, flowID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.evFail[runID+"/"+flowID]
}

// readResult lee de vuelta result.json. Solo ErrNotFound significa «aún no hay resultado».
func (l *Launcher) readResult(ctx context.Context, runID, flowID string) (flowResult, bool, error) {
	b, err := l.Evidence.Get(ctx, key(runID, flowID, "result.json"))
	if errors.Is(err, ErrNotFound) {
		return flowResult{}, false, nil
	}
	if err != nil {
		return flowResult{}, false, fmt.Errorf("leyendo el resultado guardado: %w", err)
	}
	var r flowResult
	if json.Unmarshal(b, &r) != nil || r.FlowID != flowID {
		return flowResult{}, false, errors.New("resultado guardado ilegible")
	}
	return r, true, nil
}

// settle guarda logs y luego result.json (el marcador), cada uno leído de vuelta y comparado por hash.
// Si cualquier escritura falla, el flujo cuenta como fallido y no se declara evidencia.
func (l *Launcher) settle(ctx context.Context, run runctl.Run, flowID, status string, logs []byte, resolved map[string]bool, failed map[string]string) {
	lg := l.log().With("run_id", run.ID, "flow_id", flowID, "trace_id", run.TraceID)
	h := sha256Hex(logs)
	res, _ := json.Marshal(flowResult{FlowID: flowID, Status: status, LogsSHA256: h})
	_, err := PutVerified(ctx, l.Evidence, key(run.ID, flowID, "logs.txt"), logs)
	if err == nil {
		_, err = PutVerified(ctx, l.Evidence, key(run.ID, flowID, "result.json"), res)
	}
	l.metric(func(o Observer) { o.RunnerJob(status) })
	if err != nil {
		l.mu.Lock()
		if l.evFail == nil {
			l.evFail = map[string]bool{}
		}
		l.evFail[run.ID+"/"+flowID] = true
		l.mu.Unlock()
		failed[flowID] = "evidencia no escrita"
		l.metric(func(o Observer) { o.EvidenceObject(ResError) })
		lg.Warn("evidencia no escrita; el flujo cuenta como fallido")
		return
	}
	resolved[flowID] = true
	l.metric(func(o Observer) { o.EvidenceObject(ResStored) })
	lg.Info("flujo resuelto", "status", status)
}
