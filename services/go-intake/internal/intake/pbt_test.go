package intake_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
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

func checkRoundTrip(t interface {
	Fatalf(string, ...any)
	Fatal(...any)
}, c gen.GitHubCase) {
	first, err := intake.Classify(c.Event, c.Body)
	if err != nil {
		t.Fatalf("Classify(%s) = %v\n%s", c.Event, err, c.Body)
	}
	if first != c.Want {
		t.Fatalf("Classify = %+v, quiero %+v\n%s", first, c.Want, c.Body)
	}
	for _, asRelease := range []bool{false, true} {
		if !asRelease && first.SHA == gen.ZeroSHA && first.GithubEvent == intake.EventTag {
			continue // un push con el SHA cero es un borrado: ese tag solo se expresa como release
		}
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
}

// Para todo payload generado de push, pull_request o release (PBT-02):
// Classify(Marshal(Classify(p))) == Classify(p), y Classify coincide con la
// clasificación esperada del generador; el modelo también hace round-trip JSON.
// Antes del muestreo se recorren los casos de frontera de gen.FixedValidClassified.
func TestPBT_GitHubPayloadRoundTrip(t *testing.T) {
	for _, c := range gen.FixedValidClassified() {
		checkRoundTrip(t, c)
	}
	rapid.Check(t, func(t *rapid.T) { checkRoundTrip(t, gen.GitHubValid().Draw(t, "case")) })
}

// Todo webhook no soportado o inválido se rechaza con el error esperado (nunca
// produce una notificación). Antes del muestreo se recorren, siempre, todas las
// acciones, eventos, refs y SHA rechazados de gen.FixedRejected.
func TestPBT_ClassifyRejects(t *testing.T) {
	check := func(t interface{ Fatalf(string, ...any) }, r gen.RejectedCase) {
		c, err := intake.Classify(r.Event, r.Body)
		if !errors.Is(err, r.Want) {
			t.Fatalf("%s (evento %q): Classify = %+v, %v; quiero %v\n%s", r.Class, r.Event, c, err, r.Want, r.Body)
		}
	}
	for _, r := range gen.FixedRejected() {
		check(t, r)
	}
	// Un cuerpo que no es un objeto JSON es un error de JSON (ni ErrUnsupported ni ErrInvalidPayload).
	for _, body := range []string{"", " ", "{", "}", "not json", "[]", "[1]", `"x"`, "1", "true", `{"ref":`, `{"ref":"refs/heads/x","after":"` + strings.Repeat("a", 40) + `"`, "\x00", "{'ref':1}"} {
		for _, ev := range []string{"push", "pull_request", "release"} {
			c, err := intake.Classify(ev, []byte(body))
			if err == nil || errors.Is(err, intake.ErrUnsupported) || errors.Is(err, intake.ErrInvalidPayload) {
				t.Fatalf("cuerpo %q (%s): Classify = %+v, %v; quiero un error de JSON", body, ev, c, err)
			}
		}
	}
	rapid.Check(t, func(t *rapid.T) { check(t, gen.GitHubRejected().Draw(t, "case")) })
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
			keys := make([]int, 0, len(last))
			for i := range last {
				keys = append(keys, i)
			}
			sort.Ints(keys) // orden fijo: el mensaje de fallo debe ser idéntico con el mismo seed
			for _, i := range keys {
				want := last[i]
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

func TestPBT_Fixed_ClassifyRejects(t *testing.T) {
	for _, ref := range []string{"refs/headsX", "refs/heads", "refs/tagsX/v1", "refs/tags", ""} {
		body := `{"ref":"` + ref + `","after":"` + strings.Repeat("a", 40) + `","repository":{"full_name":"a/b"}}`
		if _, err := intake.Classify("push", []byte(body)); !errors.Is(err, intake.ErrUnsupported) {
			t.Fatalf("ref %q: %v", ref, err)
		}
	}
	if _, err := intake.Classify("push", []byte(`{"ref":"refs/heads/x","after":"abc","repository":{"full_name":"a/b"}}`)); !errors.Is(err, intake.ErrInvalidPayload) {
		t.Fatalf("sha corto: %v", err)
	}
	// El tag con barra final se conserva tal cual (Resolve lo rechaza después).
	c, err := intake.Classify("push", []byte(`{"ref":"refs/tags/v1/","after":"`+strings.Repeat("a", 40)+`","repository":{"full_name":"a/b"}}`))
	if err != nil || c.Tag != "v1/" {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestPBT_Fixed_StoreLastWinsPerID(t *testing.T) {
	dir := t.TempDir()
	st, err := intake.OpenJSONLStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := intake.Record{Notification: intake.Notification{ID: "n-a", GithubEvent: "commit", Repo: "a/b", SHA: strings.Repeat("a", 40), State: intake.StatePending}, DeliveryID: "d-a", PublishPending: true}
	b := intake.Record{Notification: intake.Notification{ID: "n-b", GithubEvent: "tag", Repo: "a/b", SHA: strings.Repeat("b", 40), State: intake.StatePending}, DeliveryID: "d-b", PublishPending: true}
	for _, r := range []intake.Record{a, b} {
		if err := st.Put(r); err != nil {
			t.Fatal(err)
		}
	}
	a.PublishPending = false
	if err := st.Put(a); err != nil {
		t.Fatal(err)
	}
	st2, err := intake.OpenJSONLStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range []*intake.JSONLStore{st, st2} {
		if got, ok := st.GetByDelivery("d-a"); !ok || got.PublishPending {
			t.Fatalf("gana la última línea: %+v %v", got, ok)
		}
		if got, ok := st.GetByDelivery("d-b"); !ok || !got.PublishPending {
			t.Fatalf("%+v %v", got, ok)
		}
	}
}

// Líneas ilegibles del almacén: una línea rota, sin id o sin delivery_id, o un
// final truncado sin salto de línea, no impide abrir ni pierde los registros
// buenos, y el registro escrito después de un final truncado no se pega a él.
func TestPBT_StoreIgnoresUnreadableLines(t *testing.T) {
	base := t.TempDir()
	garbage := []string{"", "   ", "{", "not json", "[]", "null", `{"id":"n-x"}`, `{"delivery_id":"d-x"}`, `{"id":"","delivery_id":"d-y"}`, `{"id":"n-z","delivery_id":""}`, `{"id":1}`}
	rapid.Check(t, func(t *rapid.T) {
		dir := tempDir(base, t)
		good := rapid.SliceOfNDistinct(gen.Record(), 1, 6, func(r intake.Record) string { return r.ID + "|" + r.DeliveryID }).Draw(t, "good")
		var sb strings.Builder
		for _, r := range good {
			for _, g := range rapid.SliceOfN(rapid.SampledFrom(garbage), 0, 2).Draw(t, "garbage") {
				sb.WriteString(g + "\n")
			}
			line, _ := json.Marshal(r)
			sb.WriteString(string(line) + "\n")
		}
		truncated := rapid.Bool().Draw(t, "truncated")
		if truncated {
			sb.WriteString(`{"id":"n-trunc","delivery_id":"d-tr`)
		}
		if err := os.WriteFile(filepath.Join(dir, "notifications.jsonl"), []byte(sb.String()), 0o600); err != nil {
			t.Fatal(err)
		}
		st, err := intake.OpenJSONLStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range good {
			if got, ok := st.GetByDelivery(r.DeliveryID); !ok || !reflect.DeepEqual(got, r) {
				t.Fatalf("GetByDelivery(%q) = %+v, %v; quiero %+v", r.DeliveryID, got, ok, r)
			}
		}
		for _, d := range []string{"d-x", "d-y", "d-z", "d-tr", ""} {
			if _, ok := st.GetByDelivery(d); ok {
				t.Fatalf("la entrega %q de una línea ilegible está indexada", d)
			}
		}
		extra := good[0]
		extra.ID, extra.DeliveryID = "n-extra", "d-extra"
		if err := st.Put(extra); err != nil {
			t.Fatal(err)
		}
		st2, err := intake.OpenJSONLStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := st2.GetByDelivery("d-extra"); !ok || !reflect.DeepEqual(got, extra) {
			t.Fatalf("el registro escrito tras una línea %s se perdió: %+v, %v", map[bool]string{true: "truncada", false: "completa"}[truncated], got, ok)
		}
	})
}

func TestPBT_Fixed_StoreIgnoresUnreadableLines(t *testing.T) {
	dir := t.TempDir()
	ok := `{"id":"n-1","github_event":"commit","repo":"a/b","sha":"` + strings.Repeat("a", 40) + `","state":"pending","delivery_id":"d-1","publish_pending":false}`
	body := "not json\n" + `{"id":"n-x"}` + "\n" + `{"delivery_id":"d-x"}` + "\n" + ok + "\n" + `{"id":"n-2","delivery_id":"d-tr`
	if err := os.WriteFile(filepath.Join(dir, "notifications.jsonl"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := intake.OpenJSONLStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := st.GetByDelivery("d-1"); !found {
		t.Fatal("falta d-1")
	}
	if _, found := st.GetByDelivery("d-x"); found {
		t.Fatal("d-x indexada")
	}
	if err := st.Put(intake.Record{Notification: intake.Notification{ID: "n-3"}, DeliveryID: "d-3"}); err != nil {
		t.Fatal(err)
	}
	st2, _ := intake.OpenJSONLStore(dir)
	if _, found := st2.GetByDelivery("d-3"); !found {
		t.Fatal("el registro tras el final truncado se perdió")
	}
}
