package gen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/artifact"
	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/intake"
)

const (
	hexLower = "0123456789abcdef"
	// ZeroSHA es el "after" de un borrado de rama o tag.
	ZeroSHA = "0000000000000000000000000000000000000000"
)

// Owner es un dueño de repositorio de GitHub (1 a 39 caracteres alfanuméricos y guiones).
func Owner() *rapid.Generator[string] {
	return rapid.StringMatching(`[A-Za-z0-9][A-Za-z0-9-]{0,38}`)
}

// RepoName es un nombre de repositorio (1 a 100 caracteres de [A-Za-z0-9_.-]),
// con los largos límite y nombres como "." o ".." que el patrón permite.
func RepoName() *rapid.Generator[string] {
	return rapid.OneOf(
		rapid.StringMatching(`[A-Za-z0-9_.-]{1,100}`),
		rapid.StringMatching(`[A-Za-z0-9_.-]`),
		rapid.StringMatching(`[A-Za-z0-9_.-]{100}`),
		rapid.SampledFrom([]string{".", "..", "-", "_"}),
	)
}

// Repo es "owner/name" con mayúsculas y minúsculas mezcladas.
func Repo() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		return Owner().Draw(t, "owner") + "/" + RepoName().Draw(t, "name")
	})
}

// Sha40 es un SHA de commit: 40 hex en minúsculas.
func Sha40() *rapid.Generator[string] {
	return rapid.StringOfN(rapid.SampledFrom([]rune(hexLower)), 40, 40, -1)
}

func isLatest(s string) bool { return strings.EqualFold(s, "latest") }

// Tag es un tag válido para artifact.Resolve (^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$,
// distinto de latest sin importar mayúsculas), incluidos los largos 1 y 128.
func Tag() *rapid.Generator[string] {
	return rapid.OneOf(
		rapid.StringMatching(`v[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}`),
		rapid.StringMatching(`[A-Za-z0-9_][A-Za-z0-9._-]{0,127}`),
		rapid.StringMatching(`[A-Za-z0-9_]`),
		rapid.StringMatching(`[A-Za-z0-9_][A-Za-z0-9._-]{127}`),
		rapid.SampledFrom([]string{"latest-1", "mylatest", "latest.1", "_latest", "V1"}),
	).Filter(func(s string) bool { return !isLatest(s) })
}

// InvalidTag son variantes inválidas controladas: vacío, latest en cualquier
// capitalización, comienzo prohibido, 129 caracteres y caracteres fuera del patrón.
func InvalidTag() *rapid.Generator[string] {
	return rapid.OneOf(
		rapid.Just(""),
		rapid.SampledFrom([]string{"latest", "LATEST", "Latest", "lAtEsT"}),
		rapid.StringMatching(`[-.][A-Za-z0-9._-]{0,20}`),
		rapid.StringMatching(`[A-Za-z0-9_][A-Za-z0-9._-]{128,140}`),
		rapid.Custom(func(t *rapid.T) string {
			bad := rapid.SampledFrom([]string{" ", "/", ":", "@", "\t", "\n", "é", "日本", "+", "#"}).Draw(t, "bad")
			return rapid.StringMatching(`[A-Za-z0-9_]{0,10}`).Draw(t, "pre") + bad + rapid.StringMatching(`[A-Za-z0-9._-]{0,10}`).Draw(t, "suf")
		}),
	)
}

// Free es una cadena arbitraria no vacía (Unicode, largos límite).
func Free() *rapid.Generator[string] {
	return rapid.OneOf(
		rapid.StringN(1, 40, -1),
		rapid.StringN(1, 1, -1),
		rapid.SampledFrom([]string{"ñandú", "日本語", "a\u0000b", "<&>\"", " ", "😀", "x y"}),
		rapid.StringN(500, 500, -1),
	)
}

// GitHubCase es un webhook de GitHub generado y su clasificación esperada.
type GitHubCase struct {
	// Class: push-branch, push-tag, pr-same-repo, pr-fork, pr-null-head-repo, release.
	Class string
	// Event es el valor de X-GitHub-Event.
	Event string
	Body  []byte
	Want  intake.Classified
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("gen: json: %v", err))
	}
	return b
}

func repoField(t *rapid.T) string {
	return rapid.OneOf(Repo(), Free()).Draw(t, "repo")
}

// GitHubPushBranch es un push a una rama (github_event commit).
func GitHubPushBranch() *rapid.Generator[GitHubCase] {
	return rapid.Custom(func(t *rapid.T) GitHubCase {
		repo, sha := repoField(t), Sha40().Draw(t, "sha")
		branch := rapid.OneOf(rapid.Just("main"), rapid.StringN(0, 30, -1)).Draw(t, "branch")
		body := mustJSON(map[string]any{
			"ref": "refs/heads/" + branch, "before": Sha40().Draw(t, "before"), "after": sha,
			"repository": map[string]any{"full_name": repo, "id": rapid.IntRange(0, 1<<30).Draw(t, "rid")},
			"pusher":     map[string]any{"name": Free().Draw(t, "pusher")},
		})
		return GitHubCase{Class: "push-branch", Event: "push", Body: body,
			Want: intake.Classified{GithubEvent: intake.EventCommit, Repo: repo, SHA: sha}}
	})
}

// GitHubPushTag es un push de un tag (github_event tag). El nombre del tag no
// se valida al clasificar, así que puede ser cualquier cadena.
func GitHubPushTag() *rapid.Generator[GitHubCase] {
	return rapid.Custom(func(t *rapid.T) GitHubCase {
		repo, sha := repoField(t), Sha40().Draw(t, "sha")
		tag := rapid.OneOf(Tag(), InvalidTag(), rapid.StringN(0, 30, -1)).Draw(t, "tag")
		body := mustJSON(map[string]any{
			"ref": "refs/tags/" + tag, "after": sha, "repository": map[string]any{"full_name": repo},
		})
		return GitHubCase{Class: "push-tag", Event: "push", Body: body,
			Want: intake.Classified{GithubEvent: intake.EventTag, Repo: repo, SHA: sha, Tag: tag}}
	})
}

// GitHubPullRequest es un pull_request opened/synchronize/reopened, con la
// rama origen en el mismo repositorio, en un fork o con head.repo nulo (fork borrado).
func GitHubPullRequest() *rapid.Generator[GitHubCase] {
	return rapid.Custom(func(t *rapid.T) GitHubCase {
		base, sha := Repo().Draw(t, "base"), Sha40().Draw(t, "sha")
		action := rapid.SampledFrom([]string{"opened", "synchronize", "reopened"}).Draw(t, "action")
		class := rapid.SampledFrom([]string{"pr-same-repo", "pr-fork", "pr-null-head-repo"}).Draw(t, "class")
		head, headRepo := any(nil), ""
		switch class {
		case "pr-same-repo":
			headRepo = base
			if rapid.Bool().Draw(t, "swapcase") {
				headRepo = strings.ToUpper(base)
			}
		case "pr-fork":
			headRepo = Owner().Filter(func(o string) bool { return !strings.HasPrefix(strings.ToLower(base), strings.ToLower(o)+"/") }).Draw(t, "forkowner") + "/" + RepoName().Draw(t, "forkname")
		}
		if headRepo != "" {
			head = map[string]any{"full_name": headRepo}
		}
		body := mustJSON(map[string]any{
			"action": action, "number": rapid.IntRange(1, 99999).Draw(t, "number"),
			"pull_request": map[string]any{"head": map[string]any{"sha": sha, "repo": head, "ref": Free().Draw(t, "headref")}},
			"repository":   map[string]any{"full_name": base},
		})
		return GitHubCase{Class: class, Event: "pull_request", Body: body,
			Want: intake.Classified{GithubEvent: intake.EventPullRequest, Repo: base, SHA: sha, HeadRepo: headRepo}}
	})
}

// GitHubRelease es un release published cuyo target_commitish es un SHA.
func GitHubRelease() *rapid.Generator[GitHubCase] {
	return rapid.Custom(func(t *rapid.T) GitHubCase {
		repo, sha := repoField(t), Sha40().Draw(t, "sha")
		tag := rapid.OneOf(Tag(), InvalidTag(), rapid.StringN(0, 30, -1)).Draw(t, "tag")
		body := mustJSON(map[string]any{
			"action":     "published",
			"release":    map[string]any{"tag_name": tag, "target_commitish": sha, "draft": false},
			"repository": map[string]any{"full_name": repo},
		})
		return GitHubCase{Class: "release", Event: "release", Body: body,
			Want: intake.Classified{GithubEvent: intake.EventTag, Repo: repo, SHA: sha, Tag: tag}}
	})
}

// GitHubValid sortea cualquiera de las cinco clases de webhook válido.
func GitHubValid() *rapid.Generator[GitHubCase] {
	return rapid.OneOf(GitHubPushBranch(), GitHubPushTag(), GitHubPullRequest(), GitHubRelease())
}

// RejectedCase es un webhook que Classify debe rechazar con Want.
type RejectedCase struct {
	Class string
	Event string
	Body  []byte
	Want  error
}

// GitHubRejected son webhooks no soportados (ErrUnsupported) o sin repo/sha
// válidos (ErrInvalidPayload).
func GitHubRejected() *rapid.Generator[RejectedCase] {
	return rapid.Custom(func(t *rapid.T) RejectedCase {
		repo, sha := Repo().Draw(t, "repo"), Sha40().Draw(t, "sha")
		badSHA := rapid.OneOf(
			rapid.StringMatching(`[0-9a-f]{0,39}`), rapid.StringMatching(`[0-9a-f]{41,45}`),
			rapid.StringMatching(`[0-9A-F]{40}`).Filter(func(s string) bool { return s != strings.ToLower(s) }),
			rapid.Just(""), rapid.StringMatching(`[g-z]{40}`),
		).Draw(t, "badsha")
		repository := map[string]any{"full_name": repo}
		switch rapid.IntRange(0, 8).Draw(t, "kind") {
		case 0:
			return RejectedCase{"push-delete-branch", "push", mustJSON(map[string]any{"ref": "refs/heads/x", "after": ZeroSHA, "repository": repository}), intake.ErrUnsupported}
		case 1:
			return RejectedCase{"push-delete-tag", "push", mustJSON(map[string]any{"ref": "refs/tags/v1", "after": ZeroSHA, "repository": repository}), intake.ErrUnsupported}
		case 2:
			ref := rapid.SampledFrom([]string{"", "refs/notes/x", "refs/pull/1/head", "heads/main", "refs/HEADS/x"}).Draw(t, "ref")
			return RejectedCase{"push-other-ref", "push", mustJSON(map[string]any{"ref": ref, "after": sha, "repository": repository}), intake.ErrUnsupported}
		case 3:
			act := rapid.SampledFrom([]string{"closed", "edited", "labeled", "assigned", "", "OPENED", "ready_for_review"}).Draw(t, "act")
			return RejectedCase{"pr-action", "pull_request", mustJSON(map[string]any{"action": act, "pull_request": map[string]any{"head": map[string]any{"sha": sha}}, "repository": repository}), intake.ErrUnsupported}
		case 4:
			act := rapid.SampledFrom([]string{"created", "edited", "prereleased", "released", "deleted", ""}).Draw(t, "act")
			return RejectedCase{"release-action", "release", mustJSON(map[string]any{"action": act, "release": map[string]any{"tag_name": "v1", "target_commitish": sha}, "repository": repository}), intake.ErrUnsupported}
		case 5:
			ev := rapid.SampledFrom([]string{"ping", "issues", "star", "", "Push", "PUSH"}).Draw(t, "event")
			return RejectedCase{"event-name", ev, mustJSON(map[string]any{"ref": "refs/heads/x", "after": sha, "repository": repository}), intake.ErrUnsupported}
		case 6:
			return RejectedCase{"push-bad-sha", "push", mustJSON(map[string]any{"ref": "refs/heads/x", "after": badSHA, "repository": repository}), intake.ErrInvalidPayload}
		case 7:
			return RejectedCase{"pr-bad-sha", "pull_request", mustJSON(map[string]any{"action": "opened", "pull_request": map[string]any{"head": map[string]any{"sha": badSHA}}, "repository": repository}), intake.ErrInvalidPayload}
		default:
			// release con rama como target_commitish (sin SHA) o repo ausente.
			if rapid.Bool().Draw(t, "norepo") {
				return RejectedCase{"push-no-repo", "push", mustJSON(map[string]any{"ref": "refs/heads/x", "after": sha, "repository": map[string]any{"full_name": ""}}), intake.ErrInvalidPayload}
			}
			return RejectedCase{"release-branch-commitish", "release", mustJSON(map[string]any{"action": "published", "release": map[string]any{"tag_name": "v1", "target_commitish": "main"}, "repository": repository}), intake.ErrInvalidPayload}
		}
	})
}

func uuid() *rapid.Generator[string] {
	return rapid.StringMatching(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
}

// Artifact es un {kind, ref} de la notificación (ref no vacío).
func Artifact() *rapid.Generator[intake.Artifact] {
	return rapid.Custom(func(t *rapid.T) intake.Artifact {
		kind := rapid.SampledFrom([]string{artifact.KindBuildFromRepo, artifact.KindPublishedImage}).Draw(t, "kind")
		return intake.Artifact{Kind: kind, Ref: Free().Draw(t, "ref")}
	})
}

// Notification es una notificación pendiente, con y sin artefacto.
func Notification() *rapid.Generator[intake.Notification] {
	return rapid.Custom(func(t *rapid.T) intake.Notification {
		n := intake.Notification{
			ID:          "n-" + uuid().Draw(t, "id"),
			GithubEvent: rapid.SampledFrom([]string{intake.EventCommit, intake.EventPullRequest, intake.EventTag}).Draw(t, "event"),
			Repo:        Repo().Draw(t, "repo"), SHA: Sha40().Draw(t, "sha"), State: intake.StatePending,
		}
		if rapid.IntRange(0, 3).Draw(t, "hasart") > 0 {
			a := Artifact().Draw(t, "artifact")
			n.Artifact = &a
		}
		return n
	})
}

// Record es lo que persiste el almacén: la notificación con su entrega.
func Record() *rapid.Generator[intake.Record] {
	return rapid.Custom(func(t *rapid.T) intake.Record {
		return intake.Record{
			Notification: Notification().Draw(t, "n"), DeliveryID: uuid().Draw(t, "delivery"),
			PublishPending: rapid.Bool().Draw(t, "pending"),
		}
	})
}

// OccurredAt es un date-time RFC 3339 válido: Z o desplazamiento, con y sin fracción.
func OccurredAt() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		ts := time.Date(rapid.IntRange(1, 9999).Draw(t, "year"), time.Month(rapid.IntRange(1, 12).Draw(t, "month")),
			rapid.IntRange(1, 28).Draw(t, "day"), rapid.IntRange(0, 23).Draw(t, "h"), rapid.IntRange(0, 59).Draw(t, "m"),
			rapid.IntRange(0, 59).Draw(t, "s"), rapid.IntRange(0, 999999999).Draw(t, "ns"), time.UTC)
		switch rapid.IntRange(0, 2).Draw(t, "layout") {
		case 0:
			return ts.Format(time.RFC3339)
		case 1:
			return ts.Format(time.RFC3339Nano)
		}
		zone := time.FixedZone("", rapid.IntRange(-23*60, 23*60).Draw(t, "off")*60)
		return ts.In(zone).Format(time.RFC3339Nano)
	})
}

// Event es un notify.created v1 válido contra el esquema.
func Event() *rapid.Generator[intake.Event] {
	return rapid.Custom(func(t *rapid.T) intake.Event {
		return intake.Event{
			EventID: uuid().Draw(t, "event_id"), Type: "notify.created", Version: 1,
			OccurredAt: OccurredAt().Draw(t, "occurred_at"),
			TraceID:    rapid.OneOf(rapid.StringMatching(`[0-9a-f]{32}`), Free()).Draw(t, "trace_id"),
			Data: intake.EventData{
				NotificationID: "n-" + uuid().Draw(t, "nid"),
				GithubEvent:    rapid.SampledFrom([]string{intake.EventCommit, intake.EventPullRequest, intake.EventTag}).Draw(t, "ge"),
				Repo:           rapid.OneOf(Repo(), Free()).Draw(t, "repo"), SHA: Sha40().Draw(t, "sha"),
				Artifact: Artifact().Draw(t, "artifact"),
			},
		}
	})
}

// ResolveCase es una entrada de artifact.Resolver que debe resolverse.
type ResolveCase struct {
	Registry string
	Event    artifact.Event
}

// Registry es un registro de imágenes válido (vacío = ghcr.io), con mayúsculas, puerto y ruta.
func Registry() *rapid.Generator[string] {
	return rapid.OneOf(
		rapid.Just(""), rapid.Just("ghcr.io"), rapid.Just("Registry.Example.com:5000"),
		rapid.StringMatching(`[A-Za-z0-9.-]{1,30}(:[0-9]{1,5})?(/[A-Za-z0-9._-]{1,10}){0,3}`),
	)
}

// ResolvableEvent es un evento clasificado que artifact.Resolve debe fijar:
// commit, PR del mismo repositorio (sin importar mayúsculas) o tag válido.
func ResolvableEvent() *rapid.Generator[ResolveCase] {
	return rapid.Custom(func(t *rapid.T) ResolveCase {
		repo, sha := Repo().Draw(t, "repo"), Sha40().Draw(t, "sha")
		rc := ResolveCase{Registry: Registry().Draw(t, "registry")}
		switch rapid.IntRange(0, 2).Draw(t, "kind") {
		case 0:
			rc.Event = artifact.Event{GithubEvent: artifact.EventCommit, Repo: repo, SHA: sha}
		case 1:
			head := repo
			if rapid.Bool().Draw(t, "case") {
				head = strings.ToUpper(repo)
			}
			rc.Event = artifact.Event{GithubEvent: artifact.EventPullRequest, Repo: repo, SHA: sha, HeadRepo: head}
		default:
			rc.Event = artifact.Event{GithubEvent: artifact.EventTag, Repo: repo, SHA: sha, Tag: Tag().Draw(t, "tag")}
		}
		return rc
	})
}

// UnresolvableEvent es un evento que Resolve debe rechazar (fail-closed).
func UnresolvableEvent() *rapid.Generator[ResolveCase] {
	return rapid.Custom(func(t *rapid.T) ResolveCase {
		repo, sha := Repo().Draw(t, "repo"), Sha40().Draw(t, "sha")
		rc := ResolveCase{}
		switch rapid.IntRange(0, 5).Draw(t, "kind") {
		case 0:
			rc.Event = artifact.Event{GithubEvent: artifact.EventTag, Repo: repo, SHA: sha, Tag: InvalidTag().Draw(t, "tag")}
		case 1:
			head := (Owner().Draw(t, "o") + "x/" + RepoName().Draw(t, "n"))
			if strings.EqualFold(head, repo) {
				head += "z"
			}
			rc.Event = artifact.Event{GithubEvent: artifact.EventPullRequest, Repo: repo, SHA: sha, HeadRepo: head}
		case 2:
			rc.Event = artifact.Event{GithubEvent: artifact.EventPullRequest, Repo: repo, SHA: sha}
		case 3:
			rc.Event = artifact.Event{GithubEvent: artifact.EventCommit, Repo: repo, SHA: sha[:rapid.IntRange(0, 39).Draw(t, "len")]}
		case 4:
			rc.Event = artifact.Event{GithubEvent: artifact.EventCommit, Repo: rapid.SampledFrom([]string{"", "solo", "a/b/c", "a b/c", "ñ/x", "a/"}).Draw(t, "badrepo"), SHA: sha}
		default:
			rc.Event = artifact.Event{GithubEvent: rapid.SampledFrom([]string{"", "branch", "Commit", "release"}).Draw(t, "ev"), Repo: repo, SHA: sha, Tag: "v1"}
		}
		return rc
	})
}

// NotifySchema compila contracts/events/notify.created.schema.json (la ruta se
// resuelve respecto de este archivo, no del directorio de la prueba).
func NotifySchema(tb testing.TB) *jsonschema.Schema {
	tb.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		tb.Fatal("gen: no se pudo ubicar el esquema")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "contracts", "events", "notify.created.schema.json")
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()
	s, err := c.Compile(path)
	if err != nil {
		tb.Fatalf("compilando el esquema: %v", err)
	}
	return s
}

// ValidateJSON valida un documento JSON crudo contra el esquema.
func ValidateJSON(s *jsonschema.Schema, raw []byte) error {
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	return s.Validate(inst)
}
