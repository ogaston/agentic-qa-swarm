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

// WarmEnvironment representa el entorno warm asociado a un repositorio.
type WarmEnvironment struct {
	Repo            string `json:"repo,omitempty"`
	Namespace       string `json:"namespace"`
	WarmID          string `json:"warm_id"`
	State           string `json:"state"`
	ResetVerified   bool   `json:"reset_verified,omitempty"`
	BaselineVersion string `json:"baseline_version,omitempty"`
}

// Repository es el modelo registrado en go-governance.
type Repository struct {
	Repo      string          `json:"repo"`
	ImageRepo string          `json:"image_repo"`
	Enabled   bool            `json:"enabled"`
	Owner     string          `json:"owner"`
	CreatedAt time.Time       `json:"created_at"`
	Warm      WarmEnvironment `json:"warm"`
}

// RepositoryInput es el cuerpo para crear un repositorio.
type RepositoryInput struct {
	Repo      string `json:"repo"`
	ImageRepo string `json:"image_repo,omitempty"`
}

// RepositoryPatch es el cuerpo para actualizar un repositorio.
type RepositoryPatch struct {
	Enabled   *bool   `json:"enabled,omitempty"`
	ImageRepo *string `json:"image_repo,omitempty"`
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
