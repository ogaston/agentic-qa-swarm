package warmmanager

import "fmt"

// DefaultDeployerImage es la imagen (tag fijado) que ejecuta el deploy sobre warm-app.
const DefaultDeployerImage = "ghcr.io/ogaston/aqs-warm-deployer:0.1.0"

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
		},
		"spec": map[string]any{
			"backoffLimit":            int64(0), // los reintentos los decide go-warm-manager (V8)
			"activeDeadlineSeconds":   int64(600),
			"ttlSecondsAfterFinished": int64(3600),
			"template": map[string]any{
				"metadata": map[string]any{"labels": labels},
				"spec": map[string]any{
					"serviceAccountName":           sa,
					"automountServiceAccountToken": false,
					"hostNetwork":                  false,
					"restartPolicy":                "Never",
					"securityContext": map[string]any{
						"runAsNonRoot":   true,
						"runAsUser":      int64(65532),
						"runAsGroup":     int64(65532),
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
							"requests": map[string]any{"cpu": "100m", "memory": "128Mi"},
							"limits":   map[string]any{"cpu": "500m", "memory": "512Mi"},
						},
						"volumeMounts": []any{map[string]any{"name": "tmp", "mountPath": "/tmp"}},
					}},
					"volumes": []any{map[string]any{"name": "tmp", "emptyDir": map[string]any{}}},
				},
			},
		},
	}, nil
}
