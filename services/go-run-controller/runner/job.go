package runner

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Etiquetas y anotación con las que se busca y adopta el Job de un (corrida, flujo).
const (
	LabelRun      = "aqs.io/run-id"
	LabelPhase    = "aqs.io/phase"
	LabelFlow     = "aqs.io/flow-id"
	PhaseValue    = "run"
	AnnotCreated  = "aqs.io/created-at"
	maxNameLength = 63
)

// Config del Job runner. El aislamiento de red lo da el namespace aqs-test (NetworkPolicy default-deny).
type Config struct {
	Namespace       string // aqs-test
	Image           string // tag fijado, nunca latest
	ServiceAccount  string // explícito; sin token montado
	DeadlineSeconds int64  // activeDeadlineSeconds = timeout del flujo (de la política)
}

var idRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,40}[a-z0-9])?$`)

// ErrConfig marca una configuración inválida del Job.
var ErrConfig = errors.New("configuración del Job runner inválida")

// Validate exige imagen con tag fijado (o digest), namespace, SA y plazo.
func (c Config) Validate() error {
	i := c.Image
	tagged := strings.Contains(i, "@sha256:") || (strings.LastIndex(i, ":") > strings.LastIndex(i, "/") && !strings.HasSuffix(i, ":latest"))
	switch {
	case i == "" || !tagged:
		return fmt.Errorf("%w: imagen sin tag fijado (o latest)", ErrConfig)
	case c.Namespace == "" || c.ServiceAccount == "":
		return fmt.Errorf("%w: namespace y serviceAccount son obligatorios", ErrConfig)
	case c.DeadlineSeconds <= 0:
		return fmt.Errorf("%w: el plazo debe salir de la política", ErrConfig)
	}
	return nil
}

// JobName es `runner-<run>-<flow>`.
func JobName(runID, flowID string) string { return "runner-" + runID + "-" + flowID }

// BuildJob es el constructor puro: sin credenciales ni Secrets, sin token de ServiceAccount, no root,
// sistema de archivos de solo lectura, recursos y plazo completos. El Executor solo aporta
// command/args/env NO sensibles; cualquier otra cosa se rechaza.
func BuildJob(c Config, runID, flowID string, spec JobSpec, now time.Time) (*batchv1.Job, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if !idRe.MatchString(runID) || !idRe.MatchString(flowID) || len(JobName(runID, flowID)) > maxNameLength {
		return nil, fmt.Errorf("%w: run_id/flow_id no aptos para un nombre de Job", ErrConfig)
	}
	if err := checkSpec(spec); err != nil {
		return nil, err
	}
	env := make([]corev1.EnvVar, 0, len(spec.Env))
	for _, e := range spec.Env {
		env = append(env, corev1.EnvVar{Name: e.Name, Value: e.Value})
	}
	dl := c.DeadlineSeconds
	f, t, ro := false, true, true
	var user int64 = 65532
	var backoff int32
	labels := map[string]string{LabelRun: runID, LabelPhase: PhaseValue, LabelFlow: flowID,
		"app.kubernetes.io/name": "runner", "aqs.io/role": "runner"}
	return &batchv1.Job{
		TypeMeta: metav1.TypeMeta{APIVersion: "batch/v1", Kind: "Job"},
		ObjectMeta: metav1.ObjectMeta{Name: JobName(runID, flowID), Namespace: c.Namespace, Labels: labels,
			Annotations: map[string]string{AnnotCreated: now.UTC().Format(time.RFC3339)}},
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
						Name: "runner", Image: c.Image, Command: spec.Command, Args: spec.Args, Env: env,
						SecurityContext: &corev1.SecurityContext{ReadOnlyRootFilesystem: &ro, AllowPrivilegeEscalation: &f,
							Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("64Mi")},
							Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("256Mi")},
						},
					}},
				},
			},
		},
	}, nil
}
