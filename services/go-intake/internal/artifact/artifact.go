// Package artifact implementa la resolucion de artefacto V5: a partir de un
// evento de GitHub ya clasificado decide si se despliega construyendo desde el
// repositorio (commit, PR) o una imagen publicada (tag, release). Es pura: no
// hace llamadas de red, no verifica que la imagen o el commit existan (eso lo
// hace go-warm-manager al desplegar) y no lee el entorno.
package artifact

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Kinds de artefacto (contracts/events/notify.created).
const (
	KindBuildFromRepo  = "build-from-repo"
	KindPublishedImage = "published-image"
)

// Valores de Event.GithubEvent.
const (
	EventCommit      = "commit"
	EventPullRequest = "pull_request"
	EventTag         = "tag"
)

// DefaultRegistry es el registro por defecto de las imagenes publicadas.
const DefaultRegistry = "ghcr.io"

// ErrUnresolvableArtifact indica que el evento no se puede fijar a un
// artefacto desplegable. El webhook responde 422 unresolvable_artifact.
var ErrUnresolvableArtifact = errors.New("artifact: artefacto no resoluble")

var (
	shaRE  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	repoRE = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	tagRE  = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$`)
	regRE  = regexp.MustCompile(`^[A-Za-z0-9.-]+(:[0-9]+)?(/[A-Za-z0-9._-]+)*$`)
)

// Event es el evento clasificado que se resuelve.
type Event struct {
	// GithubEvent: commit, pull_request o tag.
	GithubEvent string
	// Repo es owner/repo del repositorio conectado (base en un PR).
	Repo string
	// SHA es el commit (head del PR) en 40 hex minusculas.
	SHA string
	// Tag es el nombre del tag (solo para GithubEvent == tag).
	Tag string
	// HeadRepo es owner/repo de la rama origen (solo para pull_request).
	HeadRepo string
}

// Ref es el ArtifactRef {kind, ref}.
type Ref struct {
	Kind string
	Ref  string
}

// Resolver resuelve eventos a artefactos con un registro de imagenes fijo.
type Resolver struct {
	// Registry es el registro de imagenes; vacio equivale a DefaultRegistry.
	Registry string
}

func unresolvable(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrUnresolvableArtifact, fmt.Sprintf(format, a...))
}

// Resolve devuelve el ArtifactRef del evento o un error que cumple
// errors.Is(err, ErrUnresolvableArtifact). Fail-closed.
func (r Resolver) Resolve(ev Event) (Ref, error) {
	if !repoRE.MatchString(ev.Repo) {
		return Ref{}, unresolvable("repositorio invalido")
	}
	switch ev.GithubEvent {
	case EventCommit, EventPullRequest:
		if !shaRE.MatchString(ev.SHA) {
			return Ref{}, unresolvable("sha debe ser 40 hex en minusculas")
		}
		if ev.GithubEvent == EventPullRequest &&
			(ev.HeadRepo == "" || !strings.EqualFold(ev.HeadRepo, ev.Repo)) {
			return Ref{}, unresolvable("PR desde un fork o sin repositorio de origen")
		}
		return Ref{Kind: KindBuildFromRepo, Ref: ev.Repo + "@" + ev.SHA}, nil
	case EventTag:
		if strings.EqualFold(ev.Tag, "latest") || !tagRE.MatchString(ev.Tag) {
			return Ref{}, unresolvable("tag vacio, latest o invalido")
		}
		reg := strings.ToLower(r.Registry)
		if reg == "" {
			reg = DefaultRegistry
		}
		if !regRE.MatchString(reg) {
			return Ref{}, unresolvable("registro invalido")
		}
		return Ref{Kind: KindPublishedImage, Ref: reg + "/" + strings.ToLower(ev.Repo) + ":" + ev.Tag}, nil
	default:
		return Ref{}, unresolvable("evento %q no soportado", ev.GithubEvent)
	}
}
