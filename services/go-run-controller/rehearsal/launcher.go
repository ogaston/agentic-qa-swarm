package rehearsal

import (
	"context"
	"crypto/sha1" //nolint:gosec // UUIDv5 (RFC 4122) exige SHA-1; no es un uso criptográfico.
	"errors"
	"fmt"
	"sort"
	"strconv"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/plan"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

// KubeAPI es el puerto mínimo hacia Kubernetes: crear y listar Jobs por etiquetas.
type KubeAPI interface {
	CreateJob(ctx context.Context, j *batchv1.Job) error // AlreadyExists se devuelve como apierrors.IsAlreadyExists
	ListJobs(ctx context.Context, selector string) ([]batchv1.Job, error)
}

// ClientGo adapta kubernetes.Interface (real o fake) al puerto.
type ClientGo struct {
	CS kubernetes.Interface
	NS string
}

// CreateJob implementa KubeAPI.
func (c ClientGo) CreateJob(ctx context.Context, j *batchv1.Job) error {
	_, err := c.CS.BatchV1().Jobs(c.NS).Create(ctx, j, metav1.CreateOptions{})
	return err
}

// ListJobs implementa KubeAPI.
func (c ClientGo) ListJobs(ctx context.Context, selector string) ([]batchv1.Job, error) {
	l, err := c.CS.BatchV1().Jobs(c.NS).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	return l.Items, nil
}

// FlowSource entrega el FlowPlan de la corrida (U3; hoy un stub o un adaptador que falla cerrado).
type FlowSource interface {
	Flows(runID string) (plan.FlowPlan, error)
}

// Launcher implementa runctl.PhaseLauncher (fase rehearse) y runctl.RehearsalResults.
type Launcher struct {
	Kube  KubeAPI
	Cfg   Config
	Plans FlowSource
}

var (
	_ runctl.PhaseLauncher    = (*Launcher)(nil)
	_ runctl.RehearsalResults = (*Launcher)(nil)
)

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

func attemptOf(j batchv1.Job) int { n, _ := strconv.Atoi(j.Labels[LabelAtt]); return n }

// Launch es idempotente por (corrida, fase): valida el plan ANTES de tocar Kubernetes (un plan roto
// no crea ningún Job); adopta el Job vivo por etiquetas; solo si no hay ninguno crea
// rehearsal-<run>-<Started>. AlreadyExists cuenta como éxito. Nunca crea más de 3 Jobs por corrida.
func (l *Launcher) Launch(ctx context.Context, phase string, run runctl.Run) ([]string, error) {
	if phase != runctl.PhaseRehearse {
		return nil, fmt.Errorf("fase %q no es de este lanzador", phase)
	}
	fp, err := l.Plans.Flows(run.ID)
	if err != nil {
		return nil, fmt.Errorf("plan de flujos: %w", err)
	}
	if err := plan.ValidateFlowPlan(fp); err != nil {
		return nil, err
	}
	if fp.RunID != run.ID {
		return nil, errors.New("flow-plan de otra corrida")
	}
	jobs, err := l.Kube.ListJobs(ctx, selector(run.ID))
	if err != nil {
		return nil, fmt.Errorf("listando Jobs de ensayo: %w", err)
	}
	for _, j := range jobs {
		if done, _ := jobState(j); !done {
			return nil, nil // Job vivo: se adopta
		}
	}
	if len(jobs) >= runctl.MaxRetries+1 {
		return nil, errors.New("tope de 3 Jobs de ensayo alcanzado")
	}
	attempt := run.Started[phase]
	if attempt < 1 {
		attempt = 1
	}
	job, err := BuildJob(l.Cfg, run.ID, fp.Flows[0], attempt)
	if err != nil {
		return nil, err
	}
	if err := l.Kube.CreateJob(ctx, job); err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("creando el Job de ensayo: %w", err)
	}
	return nil, nil
}

// Result lee el estado del Job más reciente de la corrida. Done=false mientras corre (o no existe).
// Passed solo sale de un Job completado con éxito; un error de lectura nunca es passed.
func (l *Launcher) Result(ctx context.Context, run runctl.Run) (runctl.RehearsalOutcome, error) {
	jobs, err := l.Kube.ListJobs(ctx, selector(run.ID))
	if err != nil {
		return runctl.RehearsalOutcome{}, err
	}
	if len(jobs) == 0 {
		return runctl.RehearsalOutcome{}, nil
	}
	sort.Slice(jobs, func(i, k int) bool { return attemptOf(jobs[i]) < attemptOf(jobs[k]) })
	last := jobs[len(jobs)-1]
	done, passed := jobState(last)
	if !done {
		return runctl.RehearsalOutcome{}, nil
	}
	return runctl.RehearsalOutcome{Done: true, Passed: passed, EventID: EventID(run.ID, last.Name)}, nil
}

// EventID es un UUIDv5 determinista por (corrida, Job).
func EventID(runID, jobName string) string {
	h := sha1.New() //nolint:gosec // UUIDv5
	h.Write([]byte("aqs-rehearsal:" + runID + ":" + jobName))
	b := h.Sum(nil)[:16]
	b[6] = (b[6] & 0x0f) | 0x50
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
