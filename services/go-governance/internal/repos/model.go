package repos

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	// ErrInvalid indica entrada no válida (HTTP 422).
	ErrInvalid = errors.New("entrada de repositorio inválida")
	// ErrNotFound indica repositorio no encontrado (HTTP 404).
	ErrNotFound = errors.New("repositorio no encontrado")
	// ErrAlreadyExists indica repositorio duplicado (HTTP 409).
	ErrAlreadyExists = errors.New("repositorio ya registrado")
	// ErrWarmCapExceeded indica que se alcanzó el límite de entornos warm (HTTP 429).
	ErrWarmCapExceeded = errors.New("límite de entornos warm alcanzado (WARM_MAX_ENVIRONMENTS)")
	// ErrForbidden indica que el usuario no tiene permisos sobre el repositorio (HTTP 403).
	ErrForbidden = errors.New("rol o permisos insuficientes sobre el repositorio")
)

var repoRegex = regexp.MustCompile(`^[a-z0-9-]+/[a-z0-9._-]+$`)

const (
	ResourcePostgreSQL = "postgresql"
	ResourceRedis      = "redis"
)

// WarmEnvironment representa el entorno warm asociado a un repositorio.
type WarmEnvironment struct {
	Repo                string `json:"repo,omitempty"`
	Namespace           string `json:"namespace"`
	WarmID              string `json:"warm_id"`
	State               string `json:"state"`
	ResetVerified       bool   `json:"reset_verified,omitempty"`
	BaselineVersion     string `json:"baseline_version,omitempty"`
	InfrastructureReady bool   `json:"infrastructure_ready"`
}

// Repository es el modelo registrado en go-governance.
type Repository struct {
	Repo                 string          `json:"repo"`
	Name                 string          `json:"name"`
	Description          string          `json:"description,omitempty"`
	Resources            []string        `json:"resources"`
	WebhookSecretVersion int             `json:"webhook_secret_version"`
	ImageRepo            string          `json:"image_repo"`
	Enabled              bool            `json:"enabled"`
	Owner                string          `json:"owner"`
	CreatedAt            time.Time       `json:"created_at"`
	Warm                 WarmEnvironment `json:"warm"`
}

// RepositoryInput es el cuerpo para crear un repositorio.
type RepositoryInput struct {
	Repo        string   `json:"repo"`
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Resources   []string `json:"resources,omitempty"`
	ImageRepo   string   `json:"image_repo,omitempty"`
}

// RepositoryPatch es el cuerpo para actualizar un repositorio.
type RepositoryPatch struct {
	Enabled   *bool   `json:"enabled,omitempty"`
	ImageRepo *string `json:"image_repo,omitempty"`
}

// NormalizeRepository aplica los defaults de registros anteriores al onboarding por proyecto.
func NormalizeRepository(r Repository) Repository {
	r.Repo = strings.ToLower(strings.TrimSpace(r.Repo))
	if strings.TrimSpace(r.Name) == "" {
		r.Name = r.Repo
	}
	r.Description = strings.TrimSpace(r.Description)
	r.Resources = NormalizeResources(r.Resources)
	return r
}

// NormalizeResources devuelve ambos recursos para nil (clientes legacy) y conserva una lista vacía explícita.
func NormalizeResources(resources []string) []string {
	if resources == nil {
		return []string{ResourcePostgreSQL, ResourceRedis}
	}
	out := make([]string, 0, len(resources))
	seen := map[string]bool{}
	for _, canonical := range []string{ResourcePostgreSQL, ResourceRedis} {
		for _, resource := range resources {
			if resource == canonical && !seen[canonical] {
				out = append(out, canonical)
				seen[canonical] = true
			}
		}
	}
	return out
}

// ValidateProjectDetails valida metadatos y recursos antes de registrar el proyecto.
func ValidateProjectDetails(name, description string, resources []string) error {
	if len(strings.TrimSpace(name)) > 120 {
		return fmt.Errorf("%w: name no puede superar 120 caracteres", ErrInvalid)
	}
	if len(strings.TrimSpace(description)) > 500 {
		return fmt.Errorf("%w: description no puede superar 500 caracteres", ErrInvalid)
	}
	seen := map[string]bool{}
	for _, resource := range resources {
		if resource != ResourcePostgreSQL && resource != ResourceRedis {
			return fmt.Errorf("%w: recurso no soportado %q", ErrInvalid, resource)
		}
		if seen[resource] {
			return fmt.Errorf("%w: recurso duplicado %q", ErrInvalid, resource)
		}
		seen[resource] = true
	}
	return nil
}

// ValidateRepoName verifica el patrón de nombre de repositorio: ^[a-z0-9-]+/[a-z0-9._-]+$.
func ValidateRepoName(repo string) error {
	if !repoRegex.MatchString(repo) {
		return fmt.Errorf("%w: el repositorio %q debe coincidir con ^[a-z0-9-]+/[a-z0-9._-]+$", ErrInvalid, repo)
	}
	return nil
}

// ValidateImageRepo verifica que imageRepo empiece con ghcr.io/<repo>.
func ValidateImageRepo(repo, imageRepo string) error {
	expectedPrefix := "ghcr.io/" + repo
	if imageRepo != expectedPrefix && !strings.HasPrefix(imageRepo, expectedPrefix) {
		return fmt.Errorf("%w: image_repo %q debe ser igual o comenzar con %q", ErrInvalid, imageRepo, expectedPrefix)
	}
	return nil
}

// Slug genera un slug seguro según la fórmula dorada de contracts/examples/slugs.json:
// minúsculas, reemplaza no-alfanuméricos por '-', colapsa guiones repetidos, máx 40 caracteres sin guión final.
func Slug(repo string) string {
	r := strings.ToLower(strings.TrimSpace(repo))
	var sb strings.Builder
	lastHyphen := false
	for _, ch := range r {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			sb.WriteRune(ch)
			lastHyphen = false
		} else {
			if !lastHyphen && sb.Len() > 0 {
				sb.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	s := strings.Trim(sb.String(), "-")
	if len(s) > 40 {
		s = strings.TrimRight(s[:40], "-")
	}
	return s
}

// NamespaceForRepo genera el namespace para un repo: aqs-w-<slug>.
func NamespaceForRepo(repo string) string {
	return "aqs-w-" + Slug(repo)
}

// WarmIDForRepo genera el warm_id para un repo: w-<slug>.
func WarmIDForRepo(repo string) string {
	return "w-" + Slug(repo)
}
