package gen_test

import (
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/gen"
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

// En 500 sorteos aparece cada estado del enum, cada github_event, cada tipo de
// artefacto (con y sin artefacto en la notificación), cada tipo de instante y
// cada variante de tag.
func TestPBT_GeneratorCoverage(t *testing.T) {
	gen.AtLeastChecks(t, 500)
	nots, evs, tags, rcs := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	n := 0
	distinct := map[string]map[string]bool{"sha": {}, "repo": {}, "tag": {}, "event_id": {}}
	rapid.Check(t, func(t *rapid.T) {
		n++
		no := gen.Notification().Draw(t, "n")
		nots["state:"+string(no.State)]++
		nots["event:"+no.GithubEvent]++
		if no.Artifact == nil {
			nots["sin-artefacto"]++
		} else {
			nots["kind:"+no.Artifact.Kind]++
		}
		e := gen.NotifyCreated().Draw(t, "e")
		distinct["sha"][gen.Sha40().Draw(t, "sha")] = true
		distinct["repo"][gen.Repo().Draw(t, "repo")] = true
		distinct["tag"][gen.Tag().Draw(t, "vtag")] = true
		distinct["event_id"][e.EventID] = true
		evs["event:"+e.Data.GithubEvent]++
		evs["kind:"+e.Data.Artifact.Kind]++
		_, off := e.OccurredAt.Zone()
		switch {
		case off != 0:
			evs["offset"]++
		default:
			evs["utc"]++
		}
		if e.OccurredAt.Nanosecond() != 0 {
			evs["fraccion"]++
		} else {
			evs["entero"]++
		}
		if id := e.EventID; id == strings.ToUpper(id) {
			evs["uuid-mayus"]++
		} else if id == strings.ToLower(id) {
			evs["uuid-minus"]++
		} else {
			evs["uuid-mixto"]++
		}
		switch tag := gen.Tag().Draw(t, "tag"); len(tag) {
		case 1:
			tags["valido-1"]++
		case 128:
			tags["valido-128"]++
		default:
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
		r := gen.ConfirmationReceipt().Draw(t, "r")
		if r.ConfirmedAt.Nanosecond() == 0 && r.ConfirmedAt.Location().String() == "UTC" {
			rcs["utc-al-segundo"]++
		}
		if len(gen.Flows().Draw(t, "flows")) == 0 {
			rcs["flows-vacio"]++
		} else {
			rcs["flows-con-datos"]++
		}
	})
	if n < 500 {
		t.Fatalf("solo %d sorteos", n)
	}
	// Entropía mínima: un generador constante o casi constante no cubre nada.
	for what, set := range distinct {
		if min := n * 9 / 10; len(set) < min && what != "tag" {
			t.Errorf("%s: solo %d valores distintos en %d sorteos (mínimo %d)", what, len(set), n, min)
		}
	}
	if len(distinct["tag"]) < 100 {
		t.Errorf("tag: solo %d valores distintos en %d sorteos", len(distinct["tag"]), n)
	}
	missing(t, "notificaciones", nots, "state:pending", "state:confirmed", "state:rejected", "event:commit", "event:pull_request",
		"event:tag", "sin-artefacto", "kind:build-from-repo", "kind:published-image")
	missing(t, "eventos", evs, "event:commit", "event:pull_request", "event:tag", "kind:build-from-repo", "kind:published-image",
		"offset", "utc", "fraccion", "entero", "uuid-mayus", "uuid-minus", "uuid-mixto")
	missing(t, "tags", tags, "valido-1", "valido-128", "valido-otro", "vacio", "latest", "largo", "invalido-otro")
	missing(t, "recibos", rcs, "utc-al-segundo", "flows-vacio", "flows-con-datos")
}
