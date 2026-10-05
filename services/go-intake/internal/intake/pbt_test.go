package intake_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/gen"
	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/intake"
)

// marshalPayload es el "Marshal" del modelo normalizado: reconstruye un webhook
// canónico a partir de intake.Classified. Un tag admite dos formas (push de tag
// y release published); una rama, un push.
func marshalPayload(c intake.Classified, asRelease bool) (string, []byte) {
	repository := map[string]any{"full_name": c.Repo}
	switch c.GithubEvent {
	case intake.EventCommit:
		b, _ := json.Marshal(map[string]any{"ref": "refs/heads/main", "after": c.SHA, "repository": repository})
		return "push", b
	case intake.EventTag:
		if asRelease {
			b, _ := json.Marshal(map[string]any{"action": "published", "release": map[string]any{"tag_name": c.Tag, "target_commitish": c.SHA}, "repository": repository})
			return "release", b
		}
		b, _ := json.Marshal(map[string]any{"ref": "refs/tags/" + c.Tag, "after": c.SHA, "repository": repository})
		return "push", b
	default:
		var head any
		if c.HeadRepo != "" {
			head = map[string]any{"full_name": c.HeadRepo}
		}
		b, _ := json.Marshal(map[string]any{"action": "opened", "pull_request": map[string]any{"head": map[string]any{"sha": c.SHA, "repo": head}}, "repository": repository})
		return "pull_request", b
	}
}

// Para todo payload generado de push, pull_request o release (PBT-02):
// Classify(Marshal(Classify(p))) == Classify(p), y Classify coincide con la
// clasificación esperada del generador; el modelo también hace round-trip JSON.
func TestPBT_GitHubPayloadRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		c := gen.GitHubValid().Draw(t, "case")
		first, err := intake.Classify(c.Event, c.Body)
		if err != nil {
			t.Fatalf("Classify(%s) = %v\n%s", c.Event, err, c.Body)
		}
		if first != c.Want {
			t.Fatalf("Classify = %+v, quiero %+v", first, c.Want)
		}
		for _, asRelease := range []bool{false, true} {
			ev, body := marshalPayload(first, asRelease)
			second, err := intake.Classify(ev, body)
			if err != nil || second != first {
				t.Fatalf("round-trip (release=%v): %+v / %v, quiero %+v\n%s", asRelease, second, err, first, body)
			}
		}
		raw, err := json.Marshal(first)
		if err != nil {
			t.Fatal(err)
		}
		var back intake.Classified
		if err := json.Unmarshal(raw, &back); err != nil || back != first {
			t.Fatalf("JSON del modelo: %+v / %v", back, err)
		}
	})
}

// Todo webhook generado como no soportado o inválido se rechaza con el error
// esperado (nunca produce una notificación).
func TestPBT_ClassifyRejects(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		r := gen.GitHubRejected().Draw(t, "case")
		c, err := intake.Classify(r.Event, r.Body)
		if !errors.Is(err, r.Want) {
			t.Fatalf("%s: Classify = %+v, %v; quiero %v\n%s", r.Class, c, err, r.Want, r.Body)
		}
	})
}

// Para toda notificación generada: UnmarshalEvent(MarshalEvent(e)) == e y el JSON
// producido valida siempre contra notify.created.schema.json. go-intake no tiene
// funciones MarshalEvent/UnmarshalEvent: son json.Marshal/json.Unmarshal de
// intake.Event, que es lo que escribe el outbox.
func TestPBT_NotifyCreatedRoundTrip(t *testing.T) {
	schema := gen.NotifySchema(t)
	rapid.Check(t, func(t *rapid.T) {
		e := gen.Event().Draw(t, "event")
		raw, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		if err := gen.ValidateJSON(schema, raw); err != nil {
			t.Fatalf("el JSON producido no valida contra el esquema: %v\n%s", err, raw)
		}
		var back intake.Event
		if err := json.Unmarshal(raw, &back); err != nil || back != e {
			t.Fatalf("round-trip: %+v / %v, quiero %+v", back, err, e)
		}
	})
}

// tempDir crea un subdirectorio por iteración dentro de base (t.TempDir() de la
// prueba; rapid.T no tiene TempDir) y lo borra al terminar la iteración.
func tempDir(base string, t *rapid.T) string {
	dir, err := os.MkdirTemp(base, "it-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func readLines(t interface {
	Helper()
	Fatal(args ...any)
	Fatalf(format string, args ...any)
}, path string) []intake.Record {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []intake.Record
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if l == "" {
			continue
		}
		var r intake.Record
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			t.Fatalf("línea ilegible %q: %v", l, err)
		}
		out = append(out, r)
	}
	return out
}

// Para toda secuencia generada de registros: escribir, cerrar (descartar el
// almacén), reabrir y leer devuelve la misma secuencia en el mismo orden (en el
// archivo y por delivery_id), también si se reabre varias veces a mitad.
func TestPBT_StoreRoundTrip(t *testing.T) {
	base := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		dir := tempDir(base, t)
		seq := rapid.SliceOfNDistinct(gen.Record(), 0, 12, func(r intake.Record) string { return r.ID + "|" + r.DeliveryID }).Draw(t, "seq")
		st, err := intake.OpenJSONLStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range seq {
			if err := st.Put(r); err != nil {
				t.Fatal(err)
			}
			if rapid.IntRange(0, 3).Draw(t, "reopen") == 0 { // cerrar y reabrir a mitad
				if st, err = intake.OpenJSONLStore(dir); err != nil {
					t.Fatal(err)
				}
			}
		}
		st, err = intake.OpenJSONLStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(seq) > 0 { // el archivo se crea en la primera escritura
			if got := readLines(t, filepath.Join(dir, "notifications.jsonl")); !reflect.DeepEqual(got, seq) {
				t.Fatalf("archivo: %+v, quiero %+v", got, seq)
			}
		}
		for _, r := range seq {
			if got, ok := st.GetByDelivery(r.DeliveryID); !ok || !reflect.DeepEqual(got, r) {
				t.Fatalf("GetByDelivery(%q) = %+v, %v; quiero %+v", r.DeliveryID, got, ok, r)
			}
		}
	})
}

// De-duplicación: con IDs repetidos (el handler escribe cada registro dos veces:
// pendiente y publicado) gana la última línea de cada ID, antes y después de
// reabrir; y todas las entregas distintas siguen indexadas.
func TestPBT_StoreLastWinsPerID(t *testing.T) {
	base := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		dir := tempDir(base, t)
		pool := rapid.SliceOfN(gen.Record(), 1, 5).Draw(t, "pool")
		for i := range pool { // un ID por posición y su entrega fija
			pool[i].ID = "n-" + string(rune('a'+i))
			pool[i].DeliveryID = "d-" + string(rune('a'+i))
		}
		idx := rapid.SliceOfN(rapid.IntRange(0, len(pool)-1), 0, 20).Draw(t, "order")
		st, err := intake.OpenJSONLStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		last := map[int]intake.Record{}
		for _, i := range idx {
			r := pool[i]
			r.PublishPending = rapid.Bool().Draw(t, "pending")
			if rapid.Bool().Draw(t, "noart") {
				r.Artifact = nil
			}
			if err := st.Put(r); err != nil {
				t.Fatal(err)
			}
			last[i] = r
		}
		check := func(st *intake.JSONLStore) {
			for i, want := range last {
				if got, ok := st.GetByDelivery(pool[i].DeliveryID); !ok || !reflect.DeepEqual(got, want) {
					t.Fatalf("GetByDelivery(%q) = %+v, %v; quiero %+v", pool[i].DeliveryID, got, ok, want)
				}
			}
		}
		check(st)
		st2, err := intake.OpenJSONLStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		check(st2)
	})
}

// Ejemplos fijos (canónicos) de las propiedades de este paquete.
func TestPBT_Fixed_GitHubPayloadRoundTrip(t *testing.T) {
	sha := strings.Repeat("b", 40)
	want := intake.Classified{GithubEvent: intake.EventTag, Repo: "acme/shop", SHA: sha, Tag: "v1.2.0"}
	for _, asRelease := range []bool{false, true} {
		ev, body := marshalPayload(want, asRelease)
		got, err := intake.Classify(ev, body)
		if err != nil || got != want {
			t.Fatalf("%v: %+v %v", asRelease, got, err)
		}
	}
	if _, err := intake.Classify("push", []byte(`{"ref":"refs/heads/x","after":"`+gen.ZeroSHA+`","repository":{"full_name":"a/b"}}`)); !errors.Is(err, intake.ErrUnsupported) {
		t.Fatalf("borrado de rama: %v", err)
	}
}

func TestPBT_Fixed_NotifyCreatedRoundTrip(t *testing.T) {
	e := intake.Event{EventID: "3f2b8c1e-6a4d-4e7b-9c10-5d2e8a7f1b34", Type: "notify.created", Version: 1,
		OccurredAt: "2026-01-15T10:00:00Z", TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		Data: intake.EventData{NotificationID: "n-1", GithubEvent: "commit", Repo: "acme/shop", SHA: strings.Repeat("a", 40),
			Artifact: intake.Artifact{Kind: "build-from-repo", Ref: "acme/shop@" + strings.Repeat("a", 40)}}}
	raw, _ := json.Marshal(e)
	if err := gen.ValidateJSON(gen.NotifySchema(t), raw); err != nil {
		t.Fatal(err)
	}
	var back intake.Event
	if err := json.Unmarshal(raw, &back); err != nil || back != e {
		t.Fatalf("%+v %v", back, err)
	}
}

func TestPBT_Fixed_StoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st, err := intake.OpenJSONLStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	art := intake.Artifact{Kind: "build-from-repo", Ref: "acme/shop@" + strings.Repeat("a", 40)}
	rec := intake.Record{Notification: intake.Notification{ID: "n-1", GithubEvent: "commit", Repo: "acme/shop", SHA: strings.Repeat("a", 40), State: intake.StatePending, Artifact: &art}, DeliveryID: "d-1", PublishPending: true}
	if err := st.Put(rec); err != nil {
		t.Fatal(err)
	}
	rec.PublishPending = false
	if err := st.Put(rec); err != nil {
		t.Fatal(err)
	}
	st2, err := intake.OpenJSONLStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := st2.GetByDelivery("d-1"); !ok || !reflect.DeepEqual(got, rec) {
		t.Fatalf("%+v %v", got, ok)
	}
}
