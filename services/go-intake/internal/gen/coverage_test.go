package gen_test

import (
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/artifact"
	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/gen"
	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/intake"
)

// TestMain fija y registra el seed de rapid (PBT-08).
func TestMain(m *testing.M) { gen.Main(m) }

func missing(t *testing.T, what string, seen map[string]int, want ...string) {
	t.Helper()
	for _, w := range want {
		if seen[w] == 0 {
			t.Errorf("%s: la clase %q no apareció en 500 sorteos (%v)", what, w, seen)
		}
	}
}

// En 500 sorteos aparece cada clase de evento de GitHub, cada github_event,
// cada tipo de artefacto, con y sin artefacto, cada rechazo y cada variante de tag.
func TestPBT_GeneratorCoverage(t *testing.T) {
	gen.AtLeastChecks(t, 500)
	gh, rej, rec, ev, tags, res := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	n := 0
	rapid.Check(t, func(t *rapid.T) {
		n++
		c := gen.GitHubValid().Draw(t, "gh")
		gh[c.Class]++
		gh["event:"+c.Want.GithubEvent]++
		r := gen.GitHubRejected().Draw(t, "rej")
		rej[r.Class]++
		rec["event:"+gen.Notification().Draw(t, "n").GithubEvent]++
		record := gen.Record().Draw(t, "rec")
		if record.Artifact == nil {
			rec["sin-artefacto"]++
		} else {
			rec["artefacto:"+record.Artifact.Kind]++
		}
		rec["pending:"+map[bool]string{true: "si", false: "no"}[record.PublishPending]]++
		rec["state:"+record.State]++
		e := gen.Event().Draw(t, "event")
		ev["event:"+e.Data.GithubEvent]++
		ev["kind:"+e.Data.Artifact.Kind]++
		switch {
		case strings.HasSuffix(e.OccurredAt, "Z") && !strings.Contains(e.OccurredAt, "."):
			ev["fecha-Z"]++
		case strings.HasSuffix(e.OccurredAt, "Z"):
			ev["fecha-fraccion"]++
		default:
			ev["fecha-offset"]++
		}
		if tag := gen.Tag().Draw(t, "tag"); len(tag) == 1 {
			tags["valido-1"]++
		} else if len(tag) == 128 {
			tags["valido-128"]++
		} else {
			tags["valido-otro"]++
		}
		switch it := gen.InvalidTag().Draw(t, "itag"); {
		case it == "":
			tags["vacio"]++
		case strings.EqualFold(it, "latest"):
			tags["latest"]++
		case len(it) > 128:
			tags["largo"]++
		default:
			tags["invalido-otro"]++
		}
		rc := gen.ResolvableEvent().Draw(t, "rc")
		res["ok:"+rc.Event.GithubEvent]++
		if rc.Registry != "" {
			res["registro-propio"]++
		}
		res["bad:"+gen.UnresolvableEvent().Draw(t, "bad").Event.GithubEvent]++
	})
	if n < 500 {
		t.Fatalf("solo %d sorteos", n)
	}
	missing(t, "github", gh, "push-branch", "push-tag", "pr-same-repo", "pr-fork", "pr-null-head-repo", "release",
		"event:commit", "event:pull_request", "event:tag")
	missing(t, "rechazos", rej, "push-delete-branch", "push-delete-tag", "push-other-ref", "pr-action", "release-action",
		"event-name", "push-bad-sha", "pr-bad-sha", "push-no-repo", "release-branch-commitish")
	missing(t, "registros", rec, "event:commit", "event:pull_request", "event:tag", "sin-artefacto",
		"artefacto:"+artifact.KindBuildFromRepo, "artefacto:"+artifact.KindPublishedImage, "pending:si", "pending:no", "state:"+intake.StatePending)
	missing(t, "eventos", ev, "event:commit", "event:pull_request", "event:tag", "kind:"+artifact.KindBuildFromRepo,
		"kind:"+artifact.KindPublishedImage, "fecha-Z", "fecha-fraccion", "fecha-offset")
	missing(t, "tags", tags, "valido-1", "valido-128", "valido-otro", "vacio", "latest", "largo", "invalido-otro")
	missing(t, "resolver", res, "ok:commit", "ok:pull_request", "ok:tag", "registro-propio", "bad:commit", "bad:pull_request", "bad:tag")
}
