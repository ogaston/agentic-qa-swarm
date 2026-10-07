package warmmanager

import (
	"fmt"
	"time"
)

// DefaultDeployerImage es un PLACEHOLDER: la imagen aqs-warm-deployer aun no existe (su construccion,
// la SA warm-deployer y el RBAC quedan para U2-T07). Tag fijado para pasar las politicas.
const DefaultDeployerImage = "ghcr.io/ogaston/aqs-warm-deployer:0.1.0"

// JobDeadline es activeDeadlineSeconds del Job; WARM_JOB_TIMEOUT debe superarlo.
const JobDeadline = 600 * time.Second

// Constantes del Job (una linea de razon cada una).
const (
	jobDeadlineSeconds = int64(600)  // tope duro de un intento de deploy (JobDeadline)
	jobTTLSeconds      = int64(3600) // el Job terminado se limpia a la hora
	jobCPURequest      = "100m"      // el deployer solo aplica un parche
	jobMemRequest      = "128Mi"
	jobCPULimit        = "500m"
	jobMemLimit        = "512Mi"
	jobUID             = int64(65532) // mismo usuario no root que el resto de imagenes
)

// JobConfig parametriza el constructor del Job.
type JobConfig struct {
	DeployerImage     string
	AllowedRegistries []string
	ServiceAccount    string // por defecto warm-deployer
}

// DefaultAllowedRegistries son los registros permitidos si WARM_ALLOWED_REGISTRIES no se define.
var DefaultAllowedRegistries = []string{"ghcr.io"}

// JobName es el nombre del intento n (0 = inicial) del deploy de la corrida.
func JobName(runID string, attempt int) string { return fmt.Sprintf("deploy-%s-%d", runID, attempt) }

// BuildDeployJob construye, sin tocar nada, el Job deploy-{run}-{n} que aplica el artefacto
// sobre el Deployment warm-app. Función pura y sin credenciales: sin envFrom ni Secrets,
// sin token de ServiceAccount, no root, rootfs de solo lectura, sin hostNetwork, tag fijado.
func BuildDeployJob(cfg JobConfig, runID string, attempt int, a Artifact) (Manifest, error) {
	if err := ValidateRunID(runID); err != nil {
		return nil, err
	}
	if err := ValidateArtifact(a, cfg.AllowedRegistries); err != nil {
		return nil, err
	}
	img := cfg.DeployerImage
	if img == "" {
		img = DefaultDeployerImage
	}
	if err := ValidateArtifact(Artifact{Kind: KindPublishedImage, Ref: img}, cfg.AllowedRegistries); err != nil {
		return nil, fmt.Errorf("imagen del deployer: %w", err)
	}
	sa := cfg.ServiceAccount
	if sa == "" {
		sa = "warm-deployer"
	}
	labels := map[string]any{
		"app.kubernetes.io/name":      "warm-deploy",
		"app.kubernetes.io/component": "deploy",
		"aqs.io/run-id":               runID,
	}
	return Manifest{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]any{
			"name":      JobName(runID, attempt),
			"namespace": Namespace,
			"labels":    labels,
			"annotations": map[string]any{ // el artefacto viaja en el Job: permite reconstruir el deploy tras un reinicio
				"aqs.io/artifact-kind": a.Kind,
				"aqs.io/artifact-ref":  a.Ref,
			},
		},
		"spec": map[string]any{
			"backoffLimit":            int64(0), // los reintentos los decide go-warm-manager (V8)
			"activeDeadlineSeconds":   jobDeadlineSeconds,
			"ttlSecondsAfterFinished": jobTTLSeconds,
			"template": map[string]any{
				"metadata": map[string]any{"labels": labels},
				"spec": map[string]any{
					"serviceAccountName":           sa,
					"automountServiceAccountToken": false,
					"hostNetwork":                  false,
					"restartPolicy":                "Never",
					"securityContext": map[string]any{
						"runAsNonRoot":   true,
						"runAsUser":      jobUID,
						"runAsGroup":     jobUID,
						"seccompProfile": map[string]any{"type": "RuntimeDefault"},
					},
					"containers": []any{map[string]any{
						"name":  "deploy",
						"image": img,
						"args": []any{"apply", "--deployment", "warm-app", "--namespace", Namespace,
							"--artifact-kind", a.Kind, "--artifact-ref", a.Ref},
						"securityContext": map[string]any{
							"allowPrivilegeEscalation": false,
							"readOnlyRootFilesystem":   true,
							"capabilities":             map[string]any{"drop": []any{"ALL"}},
						},
						"resources": map[string]any{
							"requests": map[string]any{"cpu": jobCPURequest, "memory": jobMemRequest},
							"limits":   map[string]any{"cpu": jobCPULimit, "memory": jobMemLimit},
						},
						"volumeMounts": []any{map[string]any{"name": "tmp", "mountPath": "/tmp"}},
					}},
					"volumes": []any{map[string]any{"name": "tmp", "emptyDir": map[string]any{}}},
				},
			},
		},
	}, nil
}
