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
	// LogsUnavailable: pods/log falló; logs.txt va vacío por eso y no porque el runner no escribiera nada.
	LogsUnavailable bool `json:"logs_unavailable,omitempty"`
}

// startKey es el origen persistente del plazo de la corrida: se escribe UNA vez, antes de crear el
// primer Job, y no depende de ningún Job (los Jobs vencidos se borran, el origen no se mueve).
func startKey(runID string) string { return "runs/" + runID + "/started-at" }

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
	for _, j := range jobs {
		byFlow[j.Labels[LabelFlow]] = j
	}
	first, started, err := l.readStart(ctx, run.ID)
	if err != nil {
		return runctl.RunOutcome{}, err
	}
	now := l.now()
	runExpired := started && now.Sub(first) > lim.RunTimeout

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
				l.settle(ctx, run, f.FlowID, ResTimeout, nil, false, resolved, failed)
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
		logs, lerr := l.Kube.JobLogs(ctx, j.Name) // sin logs legibles: el resultado se guarda igual, marcado
		ok := l.settle(ctx, run, f.FlowID, status, logs, lerr != nil, resolved, failed)
		if expired && ok { // nunca queda un Job colgado, pero no se borra hasta que su flujo esté resuelto
			if err := l.Kube.DeleteJob(ctx, j.Name); err != nil {
				return runctl.RunOutcome{}, fmt.Errorf("eliminando el Job vencido: %w", err)
			}
		}
	}

	if len(pending) > 0 && active < max {
		cfg := l.Cfg
		cfg.DeadlineSeconds = int64(lim.FlowTimeout / time.Second)
		// Toda la ola se construye y valida (funciones puras) ANTES de crear ningún Job.
		var wave []*batchv1.Job
		for _, f := range pending[:min(len(pending), max-active)] {
			spec, err := l.Exec.Plan(f)
			if err != nil {
				return runctl.RunOutcome{}, fmt.Errorf("motor: %w", err)
			}
			job, err := BuildJob(cfg, run.ID, f.FlowID, spec, now)
			if err != nil {
				return runctl.RunOutcome{}, err
			}
			wave = append(wave, job)
		}
		if !started { // primera ola: cuota y aprobación del workflow (gate de U4), una vez por corrida
			if err := l.authorize(ctx, run, fp.Workflow); err != nil {
				return runctl.RunOutcome{}, err
			}
			if _, err := PutVerified(ctx, l.Evidence, startKey(run.ID), []byte(now.UTC().Format(time.RFC3339))); err != nil {
				return runctl.RunOutcome{}, errors.New("no se pudo registrar el inicio de la corrida; no se lanza nada")
			}
			first = now
		}
		var created []string
		for _, job := range wave {
			err := l.Kube.CreateJob(ctx, job)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				for _, name := range created { // sin lanzamientos parciales (best-effort)
					_ = l.Kube.DeleteJob(ctx, name)
				}
				return runctl.RunOutcome{}, fmt.Errorf("creando el Job runner: %w", err)
			}
			if err == nil {
				created = append(created, job.Name)
			}
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
	if started {
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

// readStart lee el origen del plazo de la corrida. Solo ErrNotFound significa «aún no empezó».
func (l *Launcher) readStart(ctx context.Context, runID string) (time.Time, bool, error) {
	b, err := l.Evidence.Get(ctx, startKey(runID))
	if errors.Is(err, ErrNotFound) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("leyendo el inicio de la corrida: %w", err)
	}
	t, perr := time.Parse(time.RFC3339, string(b))
	if perr != nil {
		return time.Time{}, false, errors.New("inicio de la corrida ilegible")
	}
	return t, true, nil
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

// settle guarda logs y luego result.json (el marcador, siempre al final), cada uno leído de vuelta y
// comparado por hash. Si cualquier escritura falla, el flujo cuenta como fallido, no se declara
// evidencia y devuelve false (el llamador no borra el Job).
func (l *Launcher) settle(ctx context.Context, run runctl.Run, flowID, status string, logs []byte, logsUnavailable bool, resolved map[string]bool, failed map[string]string) bool {
	lg := l.log().With("run_id", run.ID, "flow_id", flowID, "trace_id", run.TraceID)
	h := sha256Hex(logs)
	res, _ := json.Marshal(flowResult{FlowID: flowID, Status: status, LogsSHA256: h, LogsUnavailable: logsUnavailable})
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
		return false
	}
	resolved[flowID] = true
	l.metric(func(o Observer) { o.EvidenceObject(ResStored) })
	lg.Info("flujo resuelto", "status", status)
	return true
}
