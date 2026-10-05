package artifact_test

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/artifact"
	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/gen"
)

var (
	buildRefRE = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+@[0-9a-f]{40}$`)
	imageRefRE = regexp.MustCompile(`^[a-z0-9.-]+(:[0-9]+)?(/[a-z0-9._-]+)*/[a-z0-9_.-]+/[a-z0-9_.-]+:[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$`)
)

// Para todo evento válido, Resolve produce un ref que cumple el patrón de
// U1-T03 (repo@sha de 40 hex, o registro/repo-en-minúsculas:tag) y nunca termina
// en :latest (sin importar mayúsculas).
func TestPBT_ArtifactRef(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		rc := gen.ResolvableEvent().Draw(t, "rc")
		ref, err := artifact.Resolver{Registry: rc.Registry}.Resolve(rc.Event)
		if err != nil {
			t.Fatalf("evento válido no resuelto: %v", err)
		}
		if strings.HasSuffix(strings.ToLower(ref.Ref), ":latest") {
			t.Fatalf("ref móvil: %q", ref.Ref)
		}
		switch rc.Event.GithubEvent {
		case artifact.EventTag:
			if ref.Kind != artifact.KindPublishedImage || !imageRefRE.MatchString(ref.Ref) || !strings.HasSuffix(ref.Ref, ":"+rc.Event.Tag) {
				t.Fatalf("ref de imagen inválido: %+v", ref)
			}
			if want := strings.ToLower(rc.Event.Repo); !strings.Contains(ref.Ref, "/"+want+":") {
				t.Fatalf("el repo debe ir en minúsculas: %q", ref.Ref)
			}
		default:
			if ref.Kind != artifact.KindBuildFromRepo || !buildRefRE.MatchString(ref.Ref) || ref.Ref != rc.Event.Repo+"@"+rc.Event.SHA {
				t.Fatalf("ref de build inválido: %+v", ref)
			}
		}
	})
}

// Fail-closed: todo evento inválido (tag vacío/latest/mal formado, PR de fork o
// sin origen, SHA corto, repo malformado, evento desconocido) es no resoluble.
func TestPBT_ArtifactRejectsInvalid(t *testing.T) {
	gen.AtLeastChecks(t, 500) // muchas clases de rechazo: más sorteos
	rapid.Check(t, func(t *rapid.T) {
		rc := gen.UnresolvableEvent().Draw(t, "rc")
		ref, err := artifact.Resolver{Registry: rc.Registry}.Resolve(rc.Event)
		if !errors.Is(err, artifact.ErrUnresolvableArtifact) {
			t.Fatalf("evento %+v: err=%v ref=%+v, quiero ErrUnresolvableArtifact", rc.Event, err, ref)
		}
	})
}

// Ejemplo fijo: latest en cualquier capitalización se rechaza; un tag normal resuelve.
func TestPBT_Fixed_ArtifactRef(t *testing.T) {
	sha := strings.Repeat("a", 40)
	for _, tag := range []string{"latest", "LATEST", "Latest", ""} {
		if _, err := (artifact.Resolver{}).Resolve(artifact.Event{GithubEvent: artifact.EventTag, Repo: "acme/shop", SHA: sha, Tag: tag}); !errors.Is(err, artifact.ErrUnresolvableArtifact) {
			t.Fatalf("tag %q: %v", tag, err)
		}
	}
	ref, err := artifact.Resolver{}.Resolve(artifact.Event{GithubEvent: artifact.EventTag, Repo: "Acme/Shop", SHA: sha, Tag: "v1.0.0"})
	if err != nil || ref.Ref != "ghcr.io/acme/shop:v1.0.0" {
		t.Fatalf("%+v %v", ref, err)
	}
}

// Ejemplo fijo de TestPBT_ArtifactRejectsInvalid: los límites que la propiedad
// recorre (SHA de 41, con basura alrededor, tag de 129 y con barra final, registro
// y repositorio inválidos).
func TestPBT_Fixed_ArtifactRejectsInvalid(t *testing.T) {
	sha := strings.Repeat("a", 40)
	bad := []struct {
		reg string
		ev  artifact.Event
	}{
		{"", artifact.Event{GithubEvent: artifact.EventCommit, Repo: "a/b", SHA: sha + "a"}},
		{"", artifact.Event{GithubEvent: artifact.EventCommit, Repo: "a/b", SHA: "x" + sha}},
		{"", artifact.Event{GithubEvent: artifact.EventCommit, Repo: "a/b", SHA: sha + "\n"}},
		{"", artifact.Event{GithubEvent: artifact.EventCommit, Repo: "a/b", SHA: sha[:39]}},
		{"", artifact.Event{GithubEvent: artifact.EventTag, Repo: "a/b", SHA: sha, Tag: "a" + strings.Repeat("b", 128)}},
		{"", artifact.Event{GithubEvent: artifact.EventTag, Repo: "a/b", SHA: sha, Tag: "v1/"}},
		{"ghcr.io/", artifact.Event{GithubEvent: artifact.EventTag, Repo: "a/b", SHA: sha, Tag: "v1"}},
		{"a_b.io", artifact.Event{GithubEvent: artifact.EventTag, Repo: "a/b", SHA: sha, Tag: "v1"}},
		{"", artifact.Event{GithubEvent: artifact.EventTag, Repo: "a/b/c", SHA: sha, Tag: "v1"}},
		{"", artifact.Event{GithubEvent: artifact.EventPullRequest, Repo: "a/b", SHA: sha, HeadRepo: "c/d"}},
	}
	for i, c := range bad {
		if _, err := (artifact.Resolver{Registry: c.reg}).Resolve(c.ev); !errors.Is(err, artifact.ErrUnresolvableArtifact) {
			t.Fatalf("caso %d (%+v): %v", i, c, err)
		}
	}
	// Límite superior válido: tag de 128 caracteres.
	if _, err := (artifact.Resolver{}).Resolve(artifact.Event{GithubEvent: artifact.EventTag, Repo: "a/b", SHA: sha, Tag: "a" + strings.Repeat("b", 127)}); err != nil {
		t.Fatal(err)
	}
}
