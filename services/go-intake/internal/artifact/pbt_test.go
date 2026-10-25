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
	imageRefRE = regexp.MustCompile(`^[a-z0-9.-]+(:[0-9]+)?(/[a-z0-9._-]+)*/[a-z0-9_.-]+/[a-z0-9_.-]+:[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$`)
)

func checkResolvable(t interface{ Fatalf(string, ...any) }, rc gen.ResolveCase) {
	ref, err := artifact.Resolver{Registry: rc.Registry}.Resolve(rc.Event)
	if err != nil {
		t.Fatalf("evento válido no resuelto (%+v): %v", rc, err)
	}
	if strings.HasSuffix(strings.ToLower(ref.Ref), ":latest") {
		t.Fatalf("ref móvil: %q", ref.Ref)
	}
	// PBT-03: commit y PR nunca resuelven a build-from-repo; van a imagen publicada por SHA.
	reg := strings.ToLower(rc.Registry)
	if reg == "" {
		reg = artifact.DefaultRegistry
	}
	var tag string
	switch rc.Event.GithubEvent {
	case artifact.EventTag:
		tag = rc.Event.Tag
	default:
		tag = rc.Event.SHA
	}
	if ref.Kind != artifact.KindPublishedImage || ref.Kind == artifact.KindBuildFromRepo {
		t.Fatalf("kind inesperado: %+v (%+v)", ref, rc)
	}
	if !imageRefRE.MatchString(ref.Ref) || !strings.HasSuffix(ref.Ref, ":"+tag) {
		t.Fatalf("ref de imagen inválido: %+v (%+v)", ref, rc)
	}
	if want := strings.ToLower(rc.Event.Repo); !strings.Contains(ref.Ref, "/"+want+":") {
		t.Fatalf("el repo debe ir en minúsculas: %q", ref.Ref)
	}
	if want := reg + "/" + strings.ToLower(rc.Event.Repo) + ":" + tag; ref.Ref != want {
		t.Fatalf("ref=%q, quiero %q", ref.Ref, want)
	}
}

// Para todo evento válido, Resolve produce un ref de imagen publicada
// (registro/repo-en-minúsculas:tag, donde el tag es el tag del evento o el SHA de
// 40 hex en commit y PR) y nunca build-from-repo, y nunca termina
// en :latest (sin importar mayúsculas). Antes del muestreo se recorren, siempre,
// todos los casos de frontera de gen.FixedResolvable (repos con . _ -, tags con
// latest como prefijo o sufijo, 1 y 128 caracteres, registros con puerto y ruta).
func TestPBT_ArtifactRef(t *testing.T) {
	for _, rc := range gen.FixedResolvable() {
		checkResolvable(t, rc)
	}
	rapid.Check(t, func(t *rapid.T) { checkResolvable(t, gen.ResolvableEvent().Draw(t, "rc")) })
}

// Fail-closed: todo evento inválido (tag vacío/latest/mal formado, PR de fork o
// sin origen, SHA corto o largo o con basura o en mayúsculas, repo malformado,
// registro inválido, evento desconocido) es no resoluble. Antes del muestreo se
// recorren, siempre, todos los casos de gen.FixedUnresolvable.
func TestPBT_ArtifactRejectsInvalid(t *testing.T) {
	check := func(t interface{ Fatalf(string, ...any) }, rc gen.ResolveCase) {
		ref, err := artifact.Resolver{Registry: rc.Registry}.Resolve(rc.Event)
		if !errors.Is(err, artifact.ErrUnresolvableArtifact) {
			t.Fatalf("evento %+v (registro %q): err=%v ref=%+v, quiero ErrUnresolvableArtifact", rc.Event, rc.Registry, err, ref)
		}
	}
	for _, rc := range gen.FixedUnresolvable() {
		check(t, rc)
	}
	gen.AtLeastChecks(t, 500)
	rapid.Check(t, func(t *rapid.T) { check(t, gen.UnresolvableEvent().Draw(t, "rc")) })
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
	// Commit: el tag de la imagen es el SHA completo y el repo va en minúsculas.
	ref, err = artifact.Resolver{}.Resolve(artifact.Event{GithubEvent: artifact.EventCommit, Repo: "Ogaston/Demo-Target-App-QA-Swarm", SHA: sha})
	if err != nil || ref.Kind != artifact.KindPublishedImage || ref.Ref != "ghcr.io/ogaston/demo-target-app-qa-swarm:"+sha {
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
