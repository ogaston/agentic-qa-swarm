package artifact

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var sha40 = strings.Repeat("a", 40)

func TestArtifactResolve(t *testing.T) {
	cases := []struct {
		name     string
		registry string
		ev       Event
		kind     string
		ref      string
	}{
		{"commit", "", Event{GithubEvent: EventCommit, Repo: "acme/shop", SHA: sha40}, KindBuildFromRepo, "acme/shop@" + sha40},
		{"PR opened", "", Event{GithubEvent: EventPullRequest, Repo: "acme/shop", SHA: sha40, HeadRepo: "acme/shop"}, KindBuildFromRepo, "acme/shop@" + sha40},
		{"PR synchronize", "", Event{GithubEvent: EventPullRequest, Repo: "acme/shop", SHA: strings.Repeat("c", 40), HeadRepo: "acme/shop"}, KindBuildFromRepo, "acme/shop@" + strings.Repeat("c", 40)},
		{"PR reopened", "", Event{GithubEvent: EventPullRequest, Repo: "acme/shop", SHA: sha40, HeadRepo: "ACME/Shop"}, KindBuildFromRepo, "acme/shop@" + sha40},
		{"push tag", "", Event{GithubEvent: EventTag, Repo: "acme/shop", SHA: sha40, Tag: "v1.2.0"}, KindPublishedImage, "ghcr.io/acme/shop:v1.2.0"},
		{"release published", "", Event{GithubEvent: EventTag, Repo: "acme/shop", SHA: sha40, Tag: "v1.2.0"}, KindPublishedImage, "ghcr.io/acme/shop:v1.2.0"},
		{"registro personalizado", "Registry.Example.com:5000", Event{GithubEvent: EventTag, Repo: "acme/shop", SHA: sha40, Tag: "v2"}, KindPublishedImage, "registry.example.com:5000/acme/shop:v2"},
		{"repo con mayusculas a imagen en minusculas", "", Event{GithubEvent: EventTag, Repo: "Acme/Shop", SHA: sha40, Tag: "V1.0"}, KindPublishedImage, "ghcr.io/acme/shop:V1.0"},
		{"commit conserva mayusculas del repo", "", Event{GithubEvent: EventCommit, Repo: "Acme/Shop", SHA: sha40}, KindBuildFromRepo, "Acme/Shop@" + sha40},
	}
	schema := compileSchema(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Resolver{Registry: c.registry}.Resolve(c.ev)
			if err != nil {
				t.Fatal(err)
			}
			if got.Kind != c.kind || got.Ref != c.ref {
				t.Fatalf("got %+v, quiero %s %s", got, c.kind, c.ref)
			}
			// El ref sale valido contra el esquema de notify.created.
			ev := map[string]any{
				"event_id": "8a1f7d3e-5b2c-4c1d-9e0f-123456789abc", "type": "notify.created", "version": 1,
				"occurred_at": "2026-10-05T00:00:00Z", "trace_id": "t",
				"data": map[string]any{
					"notification_id": "n-1", "github_event": c.ev.GithubEvent, "repo": c.ev.Repo, "sha": c.ev.SHA,
					"artifact": map[string]any{"kind": got.Kind, "ref": got.Ref},
				},
			}
			raw, _ := json.Marshal(ev)
			var v any
			_ = json.Unmarshal(raw, &v)
			if err := schema.Validate(v); err != nil {
				t.Fatalf("no cumple notify.created: %v", err)
			}
		})
	}
}

func TestArtifactRejections(t *testing.T) {
	pr := func(head string) Event {
		return Event{GithubEvent: EventPullRequest, Repo: "acme/shop", SHA: sha40, HeadRepo: head}
	}
	tag := func(tag string) Event { return Event{GithubEvent: EventTag, Repo: "acme/shop", SHA: sha40, Tag: tag} }
	cases := []struct {
		name string
		ev   Event
	}{
		{"PR de fork", pr("mallory/shop")},
		{"PR sin repo de origen", pr("")},
		{"tag latest", tag("latest")},
		{"tag LATEST", tag("LATEST")},
		{"tag con espacios", tag("v1 2")},
		{"tag vacio", tag("")},
		{"tag que empieza con punto", tag(".hidden")},
		{"tag de 129 caracteres", tag(strings.Repeat("a", 129))},
		{"repo con caracteres invalidos", Event{GithubEvent: EventCommit, Repo: "acme/sh op", SHA: sha40}},
		{"repo sin owner", Event{GithubEvent: EventCommit, Repo: "shop", SHA: sha40}},
		{"SHA corto", Event{GithubEvent: EventCommit, Repo: "acme/shop", SHA: "abc1234"}},
		{"SHA con mayusculas", Event{GithubEvent: EventCommit, Repo: "acme/shop", SHA: strings.Repeat("A", 40)}},
		{"evento desconocido", Event{GithubEvent: "deployment", Repo: "acme/shop", SHA: sha40}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Resolver{}.Resolve(c.ev)
			if !errors.Is(err, ErrUnresolvableArtifact) {
				t.Fatalf("err=%v, quiero ErrUnresolvableArtifact", err)
			}
			if got != (Ref{}) {
				t.Fatalf("un rechazo no devuelve ref: %+v", got)
			}
		})
	}
	t.Run("registro invalido", func(t *testing.T) {
		if _, err := (Resolver{Registry: "bad registry"}).Resolve(tag("v1")); !errors.Is(err, ErrUnresolvableArtifact) {
			t.Fatalf("err=%v", err)
		}
	})
}

func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "contracts", "events", "notify.created.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("notify.created.schema.json", doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("notify.created.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	return s
}
