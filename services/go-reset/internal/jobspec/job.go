// Package jobspec construye, de forma pura y sin credenciales, el Job reset-{run}.
package jobspec

import (
	"fmt"
	"regexp"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

const (
	Namespace      = "aqs-test"
	ServiceAccount = "aqs-reset-job"
	// DefaultImage se sobreescribe con RESET_JOB_IMAGE o --image (siempre con tag fijado).
	DefaultImage = "ghcr.io/ogaston/agentic-qa-swarm/go-reset:0.0.0"

	// Decisiones de dimensionado del Job (no son datos del entorno):
	jobDeadlineSeconds int64 = 600     // un reset debe acabar en 10 min o se aborta
	jobTTLSeconds      int32 = 3600    // el Job terminado se conserva 1 h para diagnóstico
	jobUID             int64 = 65532   // usuario no root de la imagen (patrón go-identity)
	cpuRequest               = "50m"   // resources: mismos valores que los CronJobs de deploy/
	memRequest               = "64Mi"  // idem
	cpuLimit                 = "250m"  // idem
	memLimit                 = "128Mi" // idem
)

var runIDRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,38}[a-z0-9])?$`)

// Build devuelve el Job reset-{run}. Rechaza run_id no DNS-seguros e imágenes sin tag fijado.
func Build(runID, image string) (*batchv1.Job, error) {
	if !runIDRe.MatchString(runID) {
		return nil, fmt.Errorf("run_id %q inválido (minúsculas, dígitos y '-', hasta 40)", runID)
	}
	if image == "" {
		image = DefaultImage
	}
	i := strings.LastIndex(image, ":")
	if i < 0 || i < strings.LastIndex(image, "/") || image[i+1:] == "" || image[i+1:] == "latest" {
		return nil, fmt.Errorf("imagen %q sin tag fijado", image)
	}
	f, t := false, true
	uid, back, ttl, deadline := jobUID, int32(0), jobTTLSeconds, jobDeadlineSeconds // backoffLimit 0: el reintento lo decide go-reset
	return &batchv1.Job{
		TypeMeta:   metav1.TypeMeta{APIVersion: "batch/v1", Kind: "Job"},
		ObjectMeta: metav1.ObjectMeta{Name: "reset-" + runID, Namespace: Namespace, Labels: map[string]string{"app.kubernetes.io/name": "reset", "aqs/run-id": runID}},
		Spec: batchv1.JobSpec{
			BackoffLimit: &back, TTLSecondsAfterFinished: &ttl, ActiveDeadlineSeconds: &deadline,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app.kubernetes.io/name": "reset"}},
				Spec: corev1.PodSpec{
					RestartPolicy:                corev1.RestartPolicyNever,
					ServiceAccountName:           ServiceAccount,
					AutomountServiceAccountToken: &f,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &t, RunAsUser: &uid, RunAsGroup: &uid,
						SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					},
					Containers: []corev1.Container{{
						Name:  "reset",
						Env:   []corev1.EnvVar{{Name: "TZ", Value: "UTC"}},
						Image: image, Args: []string{"reset-run", "--run", runID},
						SecurityContext: &corev1.SecurityContext{
							RunAsNonRoot: &t, ReadOnlyRootFilesystem: &t, AllowPrivilegeEscalation: &f,
							Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(cpuRequest), corev1.ResourceMemory: resource.MustParse(memRequest)},
							Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(cpuLimit), corev1.ResourceMemory: resource.MustParse(memLimit)},
						},
					}},
				},
			},
		},
	}, nil
}

// YAML renderiza el Job; se quita el status vacío.
func YAML(j *batchv1.Job) ([]byte, error) {
	b, err := yaml.Marshal(j)
	if err != nil {
		return nil, err
	}
	out := strings.ReplaceAll(string(b), "status: {}\n", "")
	out = strings.ReplaceAll(out, "creationTimestamp: null\n", "")
	return []byte(strings.ReplaceAll(out, "  creationTimestamp: null\n", "")), nil
}
