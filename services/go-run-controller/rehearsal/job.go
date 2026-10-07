// Package rehearsal construye y lanza el Job `rehearsal-<run>-<n>` del ensayo bloqueante y lee su resultado.
package rehearsal

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/plan"
)

// Etiquetas con las que se busca y adopta el Job vivo de una (corrida, fase).
const (
	LabelRun   = "aqs.io/run-id"
	LabelPhase = "aqs.io/phase"
	LabelAtt   = "aqs.io/attempt"
	PhaseValue = "rehearse"
)

// Config del Job de ensayo.
type Config struct {
	Namespace       string // aqs-test
	Image           string // tag fijado, nunca latest
	ServiceAccount  string // explícito; sin token montado
	TargetURL       string // Service del warm
	DeadlineSeconds int64  // 0: 120
}

var idRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,40}[a-z0-9])?$`)

// ErrConfig marca una configuración inválida del Job.
var ErrConfig = errors.New("configuración del Job de ensayo inválida")

// Validate exige imagen con tag fijado (o digest), namespace, SA y destino.
func (c Config) Validate() error {
	i := c.Image
	tagged := strings.Contains(i, "@sha256:") || (strings.LastIndex(i, ":") > strings.LastIndex(i, "/") && !strings.HasSuffix(i, ":latest"))
	switch {
	case i == "" || !tagged || strings.HasSuffix(i, ":latest"):
		return fmt.Errorf("%w: imagen sin tag fijado (o latest)", ErrConfig)
	case c.Namespace == "" || c.ServiceAccount == "" || c.TargetURL == "":
		return fmt.Errorf("%w: namespace, serviceAccount y destino son obligatorios", ErrConfig)
	}
	return nil
}

// JobName es `rehearsal-<run>-<n>`.
func JobName(runID string, attempt int) string { return fmt.Sprintf("rehearsal-%s-%d", runID, attempt) }

// BuildJob es el constructor puro del Job: sin credenciales ni Secrets, sin token de ServiceAccount,
// no root, sistema de archivos de solo lectura, recursos y plazo completos. Ejecuta UN flujo.
func BuildJob(c Config, runID string, flow plan.Flow, attempt int) (*batchv1.Job, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if !idRe.MatchString(runID) {
		return nil, fmt.Errorf("%w: run_id no apto para un nombre de Job", ErrConfig)
	}
	if attempt < 1 || attempt > 3 {
		return nil, fmt.Errorf("%w: intento fuera de 1..3", ErrConfig)
	}
	steps, err := json.Marshal(flow.Steps)
	if err != nil {
		return nil, err
	}
	dl := c.DeadlineSeconds
	if dl <= 0 {
		dl = 120
	}
	f, t, ro := false, true, true
	var user int64 = 65532
	var backoff int32
	labels := map[string]string{LabelRun: runID, LabelPhase: PhaseValue, LabelAtt: fmt.Sprint(attempt),
		"app.kubernetes.io/name": "rehearsal", "aqs.io/role": "rehearsal"}
	return &batchv1.Job{
		TypeMeta:   metav1.TypeMeta{APIVersion: "batch/v1", Kind: "Job"},
		ObjectMeta: metav1.ObjectMeta{Name: JobName(runID, attempt), Namespace: c.Namespace, Labels: labels},
		Spec: batchv1.JobSpec{
			BackoffLimit:          &backoff,
			ActiveDeadlineSeconds: &dl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					RestartPolicy:                corev1.RestartPolicyNever,
					ServiceAccountName:           c.ServiceAccount,
					AutomountServiceAccountToken: &f,
					SecurityContext: &corev1.PodSecurityContext{RunAsNonRoot: &t, RunAsUser: &user, RunAsGroup: &user,
						SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
					Containers: []corev1.Container{{
						Name: "rehearsal", Image: c.Image,
						Args: []string{"rehearse", "--run", runID, "--flow", flow.FlowID, "--target", c.TargetURL},
						Env: []corev1.EnvVar{
							{Name: "REHEARSAL_STEPS", Value: string(steps)},
							{Name: "REHEARSAL_INVARIANT", Value: flow.Invariant},
						},
						SecurityContext: &corev1.SecurityContext{ReadOnlyRootFilesystem: &ro, AllowPrivilegeEscalation: &f,
							Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("64Mi")},
							Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("250m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
						},
					}},
				},
			},
		},
	}, nil
}
