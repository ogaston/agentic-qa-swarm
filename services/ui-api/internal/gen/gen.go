package gen

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/ui-api/inbox"
)

const hexLower = "0123456789abcdef"

// Owner es un dueño de repositorio de GitHub (1 a 39 caracteres alfanuméricos y guiones).
func Owner() *rapid.Generator[string] {
	return rapid.StringMatching(`[A-Za-z0-9][A-Za-z0-9-]{0,38}`)
}

// RepoName es un nombre de repositorio (1 a 100 caracteres de [A-Za-z0-9_.-]).
func RepoName() *rapid.Generator[string] {
	return rapid.OneOf(
		rapid.StringMatching(`[A-Za-z0-9_.-]{1,100}`),
		rapid.StringMatching(`[A-Za-z0-9_.-]`),
		rapid.StringMatching(`[A-Za-z0-9_.-]{100}`),
	)
}

// Repo es "owner/name".
func Repo() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		return Owner().Draw(t, "owner") + "/" + RepoName().Draw(t, "name")
	})
}

// Sha40 es un SHA de commit: 40 hex en minúsculas.
func Sha40() *rapid.Generator[string] {
	return rapid.StringOfN(rapid.SampledFrom([]rune(hexLower)), 40, 40, -1)
}

// Tag es un nombre de tag válido (largos 1 y 128 incluidos, distinto de latest).
func Tag() *rapid.Generator[string] {
	return rapid.OneOf(
		rapid.StringMatching(`v[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}`),
		rapid.StringMatching(`[A-Za-z0-9_][A-Za-z0-9._-]{0,127}`),
		rapid.StringMatching(`[A-Za-z0-9_][A-Za-z0-9._-]{127}`),
	).Filter(func(s string) bool { return !strings.EqualFold(s, "latest") })
}

// InvalidTag son variantes inválidas controladas de Tag.
func InvalidTag() *rapid.Generator[string] {
	return rapid.OneOf(
		rapid.Just(""),
		rapid.SampledFrom([]string{"latest", "LATEST", "Latest"}),
		rapid.StringMatching(`[-.][A-Za-z0-9._-]{0,20}`),
		rapid.StringMatching(`[A-Za-z0-9_][A-Za-z0-9._-]{128,140}`),
		rapid.SampledFrom([]string{"a b", "a/b", "a:b", "é", "日本"}),
	)
}

// Free es una cadena arbitraria (Unicode, vacía y de largo límite).
func Free() *rapid.Generator[string] {
	return rapid.OneOf(
		rapid.Just(""),
		rapid.StringN(0, 40, -1),
		rapid.StringN(1, 1, -1),
		rapid.SampledFrom([]string{"ñandú", "日本語", "a\u0000b", "<&>\"", "\u2028", "😀", "x y"}),
		rapid.StringN(500, 500, -1),
	)
}

// NonEmpty es como Free sin la cadena vacía (campos con minLength 1).
func NonEmpty() *rapid.Generator[string] {
	return Free().Filter(func(s string) bool { return s != "" })
}

// Events es el conjunto de github_event.
var Events = []string{"commit", "pull_request", "tag"}

// Kinds es el conjunto de artifact.kind.
var Kinds = []string{"build-from-repo", "published-image"}

// States son los tres valores del enum State del contrato.
var States = []inbox.State{inbox.StatePending, inbox.StateConfirmed, inbox.StateRejected}

// UUID es un uuid en minúsculas, mayúsculas o mezclado.
func UUID() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		s := rapid.StringMatching(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`).Draw(t, "uuid")
		switch rapid.IntRange(0, 2).Draw(t, "case") {
		case 0:
			return strings.ToUpper(s)
		case 1:
			return s[:18] + strings.ToUpper(s[18:])
		}
		return s
	})
}

// Artifact es {kind, ref} con ref no vacío.
func Artifact() *rapid.Generator[inbox.Artifact] {
	return rapid.Custom(func(t *rapid.T) inbox.Artifact {
		return inbox.Artifact{Kind: rapid.SampledFrom(Kinds).Draw(t, "kind"), Ref: NonEmpty().Draw(t, "ref")}
	})
}

// Notification es la forma REST, en cualquiera de los tres estados del enum, con y sin artefacto.
func Notification() *rapid.Generator[inbox.Notification] {
	return rapid.Custom(func(t *rapid.T) inbox.Notification {
		n := inbox.Notification{
			ID: "n-" + UUID().Draw(t, "id"), GithubEvent: rapid.SampledFrom(Events).Draw(t, "event"),
			Repo: Repo().Draw(t, "repo"), SHA: Sha40().Draw(t, "sha"), State: rapid.SampledFrom(States).Draw(t, "state"),
		}
		if rapid.IntRange(0, 3).Draw(t, "hasart") > 0 {
			a := Artifact().Draw(t, "artifact")
			n.Artifact = &a
		}
		return n
	})
}

// Instant es un instante RFC 3339 representable (años 2 a 9999, sin el instante
// cero que el parser rechaza), en UTC o con desplazamiento, con y sin fracción.
func Instant() *rapid.Generator[time.Time] {
	return rapid.Custom(func(t *rapid.T) time.Time {
		zone := time.UTC
		switch rapid.IntRange(0, 2).Draw(t, "zone") {
		case 1:
			zone = time.FixedZone("", rapid.IntRange(-23*60, 23*60).Draw(t, "off")*60)
		}
		ns := 0
		if rapid.Bool().Draw(t, "fraction") {
			ns = rapid.IntRange(0, 999999999).Draw(t, "ns")
		}
		return time.Date(rapid.IntRange(2, 9999).Draw(t, "year"), time.Month(rapid.IntRange(1, 12).Draw(t, "month")),
			rapid.IntRange(1, 28).Draw(t, "day"), rapid.IntRange(0, 23).Draw(t, "h"), rapid.IntRange(0, 59).Draw(t, "m"),
			rapid.IntRange(0, 59).Draw(t, "s"), ns, zone)
	})
}

// NotifyCreated es un notify.created v1 válido contra el esquema y el parser.
func NotifyCreated() *rapid.Generator[inbox.NotifyCreated] {
	return NotifyCreatedAt(Instant())
}

// NotifyCreatedAt es como NotifyCreated con el instante de at.
func NotifyCreatedAt(at *rapid.Generator[time.Time]) *rapid.Generator[inbox.NotifyCreated] {
	return rapid.Custom(func(t *rapid.T) inbox.NotifyCreated {
		var e inbox.NotifyCreated
		e.EventID, e.Type, e.Version = UUID().Draw(t, "event_id"), "notify.created", 1
		e.OccurredAt = at.Draw(t, "occurred_at")
		e.TraceID = rapid.OneOf(rapid.StringMatching(`[0-9a-f]{32}`), NonEmpty()).Draw(t, "trace_id")
		e.Data.NotificationID = "n-" + UUID().Draw(t, "nid")
		e.Data.GithubEvent = rapid.SampledFrom(Events).Draw(t, "github_event")
		e.Data.Repo = rapid.OneOf(Repo(), NonEmpty()).Draw(t, "repo")
		e.Data.SHA = Sha40().Draw(t, "sha")
		a := Artifact().Draw(t, "artifact")
		e.Data.Artifact = &a
		return e
	})
}

// ConfirmationReceipt es un recibo de confirmación (confirmed_at al segundo, en UTC,
// como lo escribe el almacén).
func ConfirmationReceipt() *rapid.Generator[inbox.Receipt] {
	return rapid.Custom(func(t *rapid.T) inbox.Receipt {
		return inbox.Receipt{
			RunID:          "run-" + rapid.StringMatching(`[0-9a-f]{32}`).Draw(t, "run"),
			NotificationID: "n-" + UUID().Draw(t, "nid"), ConfirmedBy: NonEmpty().Draw(t, "by"),
			ConfirmedAt: Instant().Draw(t, "at").UTC().Truncate(time.Second),
		}
	})
}

// Flows es la lista de flujos de una confirmación (nil, vacía o con cadenas arbitrarias).
func Flows() *rapid.Generator[[]string] {
	return rapid.OneOf(rapid.Just[[]string](nil), rapid.Just([]string{}), rapid.SliceOfN(Free(), 1, 4))
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

// MarshalEvent serializa el evento (json.Marshal; occurred_at sale en RFC 3339).
func MarshalEvent(e inbox.NotifyCreated) ([]byte, error) { return json.Marshal(e) }
