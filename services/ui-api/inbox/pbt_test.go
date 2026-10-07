package inbox_test

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/ui-api/inbox"
	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/gen"
)

func sameEvent(a, b inbox.NotifyCreated) bool {
	if !a.OccurredAt.Equal(b.OccurredAt) {
		return false
	}
	a.OccurredAt, b.OccurredAt = time.Time{}, time.Time{}
	return reflect.DeepEqual(a, b)
}

// Para toda notificación generada: ParseNotifyCreated(MarshalEvent(e)) == e (el
// instante se compara con Equal: el desplazamiento horario no se conserva), y el
// JSON producido valida siempre contra notify.created.schema.json.
func TestPBT_NotifyCreatedRoundTrip(t *testing.T) {
	schema := gen.NotifySchema(t)
	rapid.Check(t, func(t *rapid.T) {
		e := gen.NotifyCreated().Draw(t, "event")
		raw, err := gen.MarshalEvent(e)
		if err != nil {
			t.Fatal(err)
		}
		if err := gen.ValidateJSON(schema, raw); err != nil {
			t.Fatalf("el JSON producido no valida contra el esquema: %v\n%s", err, raw)
		}
		got, err := inbox.ParseNotifyCreated(raw)
		if err != nil {
			t.Fatalf("el parser rechaza un evento válido: %v\n%s", err, raw)
		}
		if !sameEvent(got, e) {
			t.Fatalf("round-trip: %+v, quiero %+v", got, e)
		}
	})
}

// path es una ruta de claves dentro del evento ({} = raíz, {"data"}, {"data","artifact"}).
func objAt(m map[string]any, path []string) map[string]any {
	for _, k := range path {
		m = m[k].(map[string]any)
	}
	return m
}

var objPaths = [][]string{{}, {"data"}, {"data", "artifact"}}

func keysOf(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// replacements son valores de reemplazo de un campo: tipos erróneos y bordes.
var replacements = []any{nil, true, false, 0.0, 1.0, 2.0, -1.0, 1.5, "", " ", "x", "1", []any{}, map[string]any{}, []any{"a"}}

// variants son variantes de texto por campo (algunas siguen siendo válidas).
var variants = map[string][]string{
	"event_id":        {"", "no-es-uuid", "3F2B8C1E-6A4D-4E7B-9C10-5D2E8A7F1B34", "3f2b8c1e6a4d4e7b9c105d2e8a7f1b34", "3f2b8c1e-6a4d-4e7b-9c10-5d2e8a7f1b3", "3f2b8c1e-6a4d-4e7b-9c10-5d2e8a7f1b34\n", "x3f2b8c1e-6a4d-4e7b-9c10-5d2e8a7f1b34", " 3f2b8c1e-6a4d-4e7b-9c10-5d2e8a7f1b34", "3f2b8c1e-6a4d-4e7b-9c10-5d2e8a7f1b34x", "3f2b8c1e-6a4d-4e7b-9c10-5d2e8a7f1b345", "3f2b8c1e-6a4d-4e7b-9c10-5d2e8a7f1b", "3f2b8c1e-6a4d-4e7b-9c10-5d2e8a7f1b3g"},
	"type":            {"notify.created ", "Notify.Created", "notify.created\n", "run.confirmed"},
	"occurred_at":     {"", "ayer", "2026-01-15", "2026-01-15T10:00:00", "2026-13-01T00:00:00Z", "2026-02-30T00:00:00Z", "2026-01-15 10:00:00Z", "2026-01-15T24:00:00Z", "2026-01-15T10:00:60Z", "2026-01-15T23:59:60Z", "2026-01-15t10:00:00z", "2026-01-15T10:00:00.123456789012Z", "2026-01-15T10:00:00+0100", "2026-01-15T10:00:00+24:00", "2026-01-15T10:00:00-24:00", "2026-01-15T10:00:00+23:60", "2026-01-15T10:00:00+24:01", "2026-01-15T10:00:00+00:60", "2026-01-15T10:00:00,5Z", "2026-01-15T10:00:00-23:60", "0001-01-01T00:00:00Z", "0000-01-01T00:00:00Z", "2026-01-15T10:00:00Z\n"},
	"github_event":    {"", "Commit", "push", "commit ", "TAG"},
	"sha":             {"", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "aaaa", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n", "gggggggggggggggggggggggggggggggggggggggg"},
	"kind":            {"", "otro", "Build-From-Repo", "published-image "},
	"ref":             {"", " "},
	"trace_id":        {"", " "},
	"repo":            {"", " "},
	"notification_id": {"", " "},
}

func mutate(t *rapid.T, m map[string]any) string {
	path := rapid.SampledFrom(objPaths).Draw(t, "path")
	obj := objAt(m, path)
	key := rapid.SampledFrom(keysOf(obj)).Draw(t, "key")
	where := strings.Join(append(append([]string{}, path...), key), ".")
	switch rapid.IntRange(0, 4).Draw(t, "mutation") {
	case 0:
		delete(obj, key)
		return "borrar " + where
	case 1:
		extra := rapid.OneOf(rapid.SampledFrom([]string{"x", "extra", "Version", "$schema", ""}), gen.Free()).Draw(t, "extra")
		obj[extra] = rapid.SampledFrom(replacements).Draw(t, "extraval")
		return "agregar " + extra + " en " + strings.Join(path, ".")
	case 2:
		nk := rapid.SampledFrom([]string{strings.ToUpper(key), strings.ToUpper(key[:1]) + key[1:], key + " ", " " + key}).Draw(t, "newkey")
		if nk != key {
			obj[nk] = obj[key]
			delete(obj, key)
		}
		return "renombrar " + where + " a " + nk
	case 3:
		v := rapid.SampledFrom(replacements).Draw(t, "value")
		obj[key] = v
		return "reemplazar " + where
	default:
		if vs, ok := variants[key]; ok {
			if _, isStr := obj[key].(string); isStr {
				obj[key] = rapid.SampledFrom(vs).Draw(t, "variant")
				return "variante de " + where
			}
		}
		return "sin cambio"
	}
}

// documentedStricter indica si un documento que el esquema acepta pero el parser
// rechaza cae en una de las diferencias documentadas en ParseNotifyCreated:
// "t"/"z" minúsculas en occurred_at, el segundo 60 (segundo intercalar, que
// time.Parse no acepta), el instante cero (0001-01-01T00:00:00Z, que
// el parser trata como ausente; ver TestPBT_Limit_ZeroInstantRejected) o version
// que no es el entero 1.
func documentedStricter(m map[string]any) bool {
	at, _ := m["occurred_at"].(string)
	if strings.ContainsAny(at, "tz") || strings.Contains(at, ":60") {
		return true
	}
	if tm, err := time.Parse(time.RFC3339, at); err == nil && tm.IsZero() {
		return true
	}
	return false
}

// El parser nunca es más laxo que el esquema: para todo evento válido mutado de
// modo que el esquema lo rechace (campo borrado, clave extra o con otra
// capitalización, tipo o valor erróneo), ParseNotifyCreated también lo rechaza.
// Y el evento sin mutar siempre se acepta. Cuando el esquema acepta y el parser
// rechaza, debe ser una de las diferencias documentadas.
func TestPBT_ParserNeverLaxerThanSchema(t *testing.T) {
	schema := gen.NotifySchema(t)
	gen.AtLeastChecks(t, 500) // la mutación elige entre muchos campos: más sorteos
	rapid.Check(t, func(t *rapid.T) {
		e := gen.NotifyCreated().Draw(t, "event")
		raw, err := gen.MarshalEvent(e)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := inbox.ParseNotifyCreated(raw); err != nil {
			t.Fatalf("evento válido rechazado: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		what := mutate(t, m)
		mraw, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		schemaErr := gen.ValidateJSON(schema, mraw)
		_, parseErr := inbox.ParseNotifyCreated(mraw)
		if at, _ := m["occurred_at"].(string); schemaErr != nil && knownLax(at) {
			// H-1 (ver TestPBT_Limit_ParserAcceptsDateTimeSchemaRejects): se fija que hoy el
			// parser lo acepta; si se corrige, esta rama falla y hay que quitarla.
			if parseErr != nil && onlyOccurredAtWrong(schema, m) {
				t.Fatalf("H-1 corregido (%q rechazado): invertir los Limit y quitar knownLax", at)
			}
			if parseErr == nil {
				return
			}
		}
		if schemaErr != nil && parseErr == nil {
			t.Fatalf("parser más laxo que el esquema tras %q:\n%s\nesquema: %v", what, mraw, schemaErr)
		}
		if schemaErr == nil && parseErr != nil && !documentedStricter(m) {
			t.Fatalf("parser más estricto que el esquema fuera de lo documentado tras %q:\n%s\nparser: %v", what, mraw, parseErr)
		}
	})
}

// knownLax reconoce los valores de H-1: desplazamientos fuera de rango y coma decimal.
var knownLaxRE = regexp.MustCompile(`^2026-01-15T10:00:00(,5Z|[+-](24:0[01]|23:60|00:60))$`)

func knownLax(at string) bool { return knownLaxRE.MatchString(at) }

// onlyOccurredAtWrong indica que, al poner un occurred_at válido, el esquema acepta
// el documento (es decir, el único defecto del documento es el instante).
func onlyOccurredAtWrong(schema *jsonschema.Schema, m map[string]any) bool {
	c := map[string]any{}
	for k, v := range m {
		c[k] = v
	}
	c["occurred_at"] = "2026-01-15T10:00:00Z"
	raw, _ := json.Marshal(c)
	return gen.ValidateJSON(schema, raw) == nil
}

var runIDRE = regexp.MustCompile(`^run-[0-9a-f]{32}$`)

type fileRecord struct {
	inbox.Receipt
	Flows []string `json:"flows"`
}

func readReceipts(t interface {
	Helper()
	Fatal(args ...any)
	Fatalf(format string, args ...any)
}, dir string) []fileRecord {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, inbox.ConfirmationsFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []fileRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for sc.Scan() {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var r fileRecord
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatalf("línea ilegible %q: %v", sc.Text(), err)
		}
		out = append(out, r)
	}
	return out
}

func tempDir(base string, t *rapid.T) string {
	dir, err := os.MkdirTemp(base, "it-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func distinctEvents() *rapid.Generator[[]inbox.NotifyCreated] {
	return rapid.SliceOfNDistinct(gen.NotifyCreated(), 0, 10, func(e inbox.NotifyCreated) string { return e.Data.NotificationID })
}

// Para toda secuencia generada de confirmaciones: escribir, cerrar, reabrir y
// leer devuelve los mismos recibos en el mismo orden (en el archivo y como
// estado confirmed tras reabrir), y confirmar de nuevo da ErrAlreadyConfirmed
// sin añadir líneas. Se reabre también a mitad de la secuencia.
func TestPBT_ReceiptStoreRoundTrip(t *testing.T) {
	base := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		dir := tempDir(base, t)
		events := distinctEvents().Draw(t, "events")
		clock := time.Unix(1_700_000_000, 0)
		now := func() time.Time { clock = clock.Add(1500 * time.Millisecond); return clock }
		st, err := inbox.OpenStore(dir, now)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range events {
			st.Apply(e)
		}
		var want []fileRecord
		confirmed, usedRuns := map[string]bool{}, map[string]bool{}
		for _, i := range rapid.SliceOfN(rapid.IntRange(0, max(len(events)-1, 0)), 0, 12).Draw(t, "order") {
			if len(events) == 0 {
				break
			}
			id := events[i].Data.NotificationID
			flows, by := gen.Flows().Draw(t, "flows"), gen.NonEmpty().Draw(t, "by")
			r, err := st.Confirm(id, by, flows)
			if confirmed[id] {
				if !errors.Is(err, inbox.ErrAlreadyConfirmed) {
					t.Fatalf("segunda confirmación de %q: %v", id, err)
				}
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			if r.NotificationID != id || r.ConfirmedBy != by || !runIDRE.MatchString(r.RunID) || usedRuns[r.RunID] || !r.ConfirmedAt.Equal(clock.UTC().Truncate(time.Second)) {
				t.Fatalf("recibo incoherente con la confirmación (%q por %q): %+v", id, by, r)
			}
			usedRuns[r.RunID] = true
			confirmed[id] = true
			want = append(want, fileRecord{Receipt: r, Flows: flows})
			if rapid.IntRange(0, 3).Draw(t, "reopen") == 0 {
				if st, err = inbox.OpenStore(dir, now); err != nil {
					t.Fatal(err)
				}
				for _, e := range events {
					st.Apply(e)
				}
			}
		}
		got := readReceipts(t, dir)
		if len(got) != len(want) {
			t.Fatalf("archivo: %d recibos, quiero %d", len(got), len(want))
		}
		for i := range want {
			if !got[i].ConfirmedAt.Equal(want[i].ConfirmedAt) {
				t.Fatalf("recibo %d: confirmed_at %v, quiero %v", i, got[i].ConfirmedAt, want[i].ConfirmedAt)
			}
			got[i].ConfirmedAt, want[i].ConfirmedAt = time.Time{}, time.Time{}
			if !reflect.DeepEqual(got[i], want[i]) {
				t.Fatalf("recibo %d: %+v, quiero %+v", i, got[i], want[i])
			}
		}
		st2, err := inbox.OpenStore(dir, now)
		if err != nil || st2.Skipped != 0 {
			t.Fatalf("reabrir: %v (ilegibles %d)", err, st2.Skipped)
		}
		for _, e := range events {
			st2.Apply(e)
		}
		for _, n := range st2.List("") {
			wantState := inbox.StatePending
			if confirmed[n.ID] {
				wantState = inbox.StateConfirmed
			}
			if n.State != wantState {
				t.Fatalf("%s: estado %q tras reabrir, quiero %q", n.ID, n.State, wantState)
			}
		}
		if got := len(st2.List(inbox.StateConfirmed)); got != len(confirmed) {
			t.Fatalf("confirmadas tras reabrir: %d, quiero %d", got, len(confirmed))
		}
		if _, err := st2.Confirm("n-no-existe-"+gen.UUID().Draw(t, "ghost"), "x", nil); !errors.Is(err, inbox.ErrNotFound) {
			t.Fatalf("confirmar un id inexistente: %v", err)
		}
		for id := range confirmed {
			if _, err := st2.Confirm(id, "x", nil); !errors.Is(err, inbox.ErrAlreadyConfirmed) {
				t.Fatalf("reconfirmar %q tras reabrir: %v", id, err)
			}
		}
		if after := readReceipts(t, dir); len(after) != len(want) {
			t.Fatalf("reconfirmar añadió líneas: %d, quiero %d", len(after), len(want))
		}
	})
}

// Apply es idempotente (primera gana por event_id y por notification_id) y List
// ordena de la más reciente a la más antigua, con empate por orden de llegada
// inverso; la proyección conserva los campos del evento.
func TestPBT_ListOrderAndIdempotence(t *testing.T) {
	base := t.TempDir()
	instants := []time.Time{time.Unix(1000, 0), time.Unix(2000, 0), time.Unix(3000, 0)}
	rapid.Check(t, func(t *rapid.T) {
		st, err := inbox.OpenStore(tempDir(base, t), nil)
		if err != nil {
			t.Fatal(err)
		}
		pool := rapid.SliceOfN(gen.NotifyCreatedAt(rapid.SampledFrom(instants)), 1, 6).Draw(t, "pool")
		stream := rapid.SliceOfN(rapid.IntRange(0, len(pool)-1), 0, 20).Draw(t, "stream")
		seenEvent, seenID := map[string]bool{}, map[string]bool{}
		var order []inbox.NotifyCreated
		for _, i := range stream {
			e := pool[i]
			switch rapid.IntRange(0, 5).Draw(t, "clone") {
			case 0: // mismo notification_id con otro event_id
				e.EventID = gen.UUID().Draw(t, "other_event_id")
			case 1: // mismo event_id con otro notification_id
				e.Data.NotificationID = "n-" + gen.UUID().Draw(t, "other_nid")
			}
			created := st.Apply(e)
			wantCreated := !seenEvent[e.EventID] && !seenID[e.Data.NotificationID]
			seenEvent[e.EventID] = true
			if created != wantCreated {
				t.Fatalf("Apply(%s) = %v, quiero %v", e.Data.NotificationID, created, wantCreated)
			}
			if created {
				seenID[e.Data.NotificationID] = true
				order = append(order, e)
			}
		}
		idx := make([]int, len(order))
		for i := range idx {
			idx[i] = i
		}
		sort.SliceStable(idx, func(a, b int) bool {
			ea, eb := order[idx[a]], order[idx[b]]
			if !ea.OccurredAt.Equal(eb.OccurredAt) {
				return ea.OccurredAt.After(eb.OccurredAt)
			}
			return idx[a] > idx[b]
		})
		got := st.List("")
		if len(got) != len(order) {
			t.Fatalf("List: %d notificaciones, quiero %d", len(got), len(order))
		}
		for pos, i := range idx {
			e := order[i]
			want := inbox.Notification{ID: e.Data.NotificationID, GithubEvent: e.Data.GithubEvent, Repo: e.Data.Repo,
				SHA: e.Data.SHA, Artifact: e.Data.Artifact, State: inbox.StatePending}
			if !reflect.DeepEqual(got[pos], want) {
				t.Fatalf("posición %d: %+v, quiero %+v", pos, got[pos], want)
			}
		}
	})
}

// Límite conocido: el parser trata el instante cero como ausente, aunque el
// esquema lo acepta (más estricto que el esquema, sin consecuencias: un evento
// real nunca ocurre en el año 1). Fijado para que un cambio sea visible.
func TestPBT_Limit_ZeroInstantRejected(t *testing.T) {
	e := gen.NotifyCreatedAt(rapid.Just(time.Date(2, 1, 1, 0, 0, 0, 0, time.UTC))).Example(0)
	raw, _ := gen.MarshalEvent(e)
	raw = []byte(strings.Replace(string(raw), `"0002-01-01T00:00:00Z"`, `"0001-01-01T00:00:00Z"`, 1))
	if err := gen.ValidateJSON(gen.NotifySchema(t), raw); err != nil {
		t.Fatalf("el esquema debería aceptar el instante cero: %v", err)
	}
	if _, err := inbox.ParseNotifyCreated(raw); !errors.Is(err, inbox.ErrInvalidEvent) {
		t.Fatalf("el parser debería rechazar el instante cero: %v", err)
	}
}

// Defecto de producción hallado por TestPBT_ParserNeverLaxerThanSchema (H-1):
// el parser acepta occurred_at que el esquema rechaza porque time.Parse es más
// permisivo que el formato date-time: desplazamientos +24:00/-24:00 y hh:60, y
// coma como separador de la fracción. Contraejemplo reducido: occurred_at
// "2026-01-15T10:00:00+24:00". Fuera de alcance de U1-T05 (solo se permite tocar
// go.mod/go.sum en producción): se fija el comportamiento actual y se propone
// una tarea candidata. Es la prueba de regresión del hallazgo: si se corrige
// el parser, debe invertirse (el parser debe rechazar estos valores).
func TestPBT_Limit_ParserAcceptsDateTimeSchemaRejects(t *testing.T) {
	schema := gen.NotifySchema(t)
	const tpl = `{"event_id":"3f2b8c1e-6a4d-4e7b-9c10-5d2e8a7f1b34","type":"notify.created","version":1,"occurred_at":"AT","trace_id":"t",` +
		`"data":{"notification_id":"n","github_event":"commit","repo":"r","sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","artifact":{"kind":"build-from-repo","ref":"r"}}}`
	for _, at := range []string{"2026-01-15T10:00:00+24:00", "2026-01-15T10:00:00-24:00", "2026-01-15T10:00:00+23:60", "2026-01-15T10:00:00+24:01", "2026-01-15T10:00:00-23:60", "2026-01-15T10:00:00+00:60", "2026-01-15T10:00:00,5Z"} {
		raw := []byte(strings.Replace(tpl, "AT", at, 1))
		if gen.ValidateJSON(schema, raw) == nil {
			t.Errorf("%s: el esquema ya no rechaza este valor; revisar el límite", at)
		}
		if _, err := inbox.ParseNotifyCreated(raw); err != nil {
			t.Errorf("%s: el parser ya rechaza este valor (defecto corregido): %v; invertir esta prueba", at, err)
		}
	}
}

// Ejemplos fijos (canónicos) de las propiedades de este paquete.
func TestPBT_Fixed_NotifyCreatedRoundTrip(t *testing.T) {
	a := inbox.Artifact{Kind: "build-from-repo", Ref: "acme/shop@" + strings.Repeat("a", 40)}
	var e inbox.NotifyCreated
	e.EventID, e.Type, e.Version, e.TraceID = "3f2b8c1e-6a4d-4e7b-9c10-5d2e8a7f1b34", "notify.created", 1, "4bf92f3577b34da6a3ce929d0e0e4736"
	e.OccurredAt = time.Date(2026, 1, 15, 10, 0, 0, 123, time.FixedZone("", 3600))
	e.Data.NotificationID, e.Data.GithubEvent, e.Data.Repo, e.Data.SHA, e.Data.Artifact = "n-1", "commit", "acme/shop", strings.Repeat("a", 40), &a
	raw, _ := gen.MarshalEvent(e)
	if err := gen.ValidateJSON(gen.NotifySchema(t), raw); err != nil {
		t.Fatal(err)
	}
	got, err := inbox.ParseNotifyCreated(raw)
	if err != nil || !sameEvent(got, e) {
		t.Fatalf("%+v %v", got, err)
	}
	// Mayúsculas en un nombre de campo: el esquema y el parser lo rechazan.
	bad := []byte(strings.Replace(string(raw), `"sha"`, `"SHA"`, 1))
	if gen.ValidateJSON(gen.NotifySchema(t), bad) == nil {
		t.Fatal("el esquema acepta SHA")
	}
	if _, err := inbox.ParseNotifyCreated(bad); err == nil {
		t.Fatal("el parser acepta SHA")
	}
}

func TestPBT_Fixed_ReceiptStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	a := inbox.Artifact{Kind: "published-image", Ref: "ghcr.io/acme/shop:v1"}
	var e inbox.NotifyCreated
	e.EventID, e.Type, e.Version, e.TraceID = "3f2b8c1e-6a4d-4e7b-9c10-5d2e8a7f1b34", "notify.created", 1, "t"
	e.OccurredAt = time.Unix(1000, 0)
	e.Data.NotificationID, e.Data.GithubEvent, e.Data.Repo, e.Data.SHA, e.Data.Artifact = "n-1", "tag", "acme/shop", strings.Repeat("b", 40), &a
	st, err := inbox.OpenStore(dir, func() time.Time { return time.Unix(2000, 5) })
	if err != nil {
		t.Fatal(err)
	}
	st.Apply(e)
	r, err := st.Confirm("n-1", "u-1", []string{"checkout"})
	if err != nil {
		t.Fatal(err)
	}
	got := readReceipts(t, dir)
	if len(got) != 1 || got[0].RunID != r.RunID || !got[0].ConfirmedAt.Equal(time.Unix(2000, 0)) || !reflect.DeepEqual(got[0].Flows, []string{"checkout"}) {
		t.Fatalf("%+v", got)
	}
	st2, err := inbox.OpenStore(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	st2.Apply(e)
	if l := st2.List(inbox.StateConfirmed); len(l) != 1 || l[0].ID != "n-1" {
		t.Fatalf("%+v", l)
	}
	if _, err := st2.Confirm("n-1", "u-2", nil); !errors.Is(err, inbox.ErrAlreadyConfirmed) {
		t.Fatal(err)
	}
}

func TestPBT_Fixed_ListOrder(t *testing.T) {
	st, err := inbox.OpenStore(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	a := inbox.Artifact{Kind: "build-from-repo", Ref: "r"}
	mk := func(id string, at int64) inbox.NotifyCreated {
		var e inbox.NotifyCreated
		e.EventID, e.Type, e.Version, e.TraceID = "00000000-0000-4000-8000-00000000000"+id, "notify.created", 1, "t"
		e.OccurredAt = time.Unix(at, 0)
		e.Data.NotificationID, e.Data.GithubEvent, e.Data.Repo, e.Data.SHA, e.Data.Artifact = "n-"+id, "commit", "a/b", strings.Repeat("a", 40), &a
		return e
	}
	for _, e := range []inbox.NotifyCreated{mk("1", 10), mk("2", 20), mk("3", 20), mk("1", 99)} {
		st.Apply(e)
	}
	var ids []string
	for _, n := range st.List("") {
		ids = append(ids, n.ID)
	}
	if strings.Join(ids, ",") != "n-3,n-2,n-1" {
		t.Fatalf("orden %v", ids)
	}
}

// Ejemplo fijo de TestPBT_ParserNeverLaxerThanSchema: una mutación de cada clase
// (campo borrado, clave extra, capitalización, tipo erróneo, sha inválido) que el
// esquema y el parser rechazan a la vez.
func TestPBT_Fixed_ParserNeverLaxerThanSchema(t *testing.T) {
	schema := gen.NotifySchema(t)
	const valid = `{"event_id":"3f2b8c1e-6a4d-4e7b-9c10-5d2e8a7f1b34","type":"notify.created","version":1,"occurred_at":"2026-01-15T10:00:00Z","trace_id":"t",` +
		`"data":{"notification_id":"n","github_event":"commit","repo":"r","sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","artifact":{"kind":"build-from-repo","ref":"r"}}}`
	if _, err := inbox.ParseNotifyCreated([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	for name, mut := range map[string][2]string{
		"borrar trace_id":  {`"trace_id":"t",`, ``},
		"clave extra":      {`"trace_id"`, `"x":1,"trace_id"`},
		"clave extra data": {`"repo"`, `"x":1,"repo"`},
		"mayúsculas":       {`"sha"`, `"SHA"`},
		"sha corto":        {`aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`, `aaaa"`},
		"sha mayúsculas":   {`aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`, `AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"`},
		"version string":   {`"version":1`, `"version":"1"`},
		"kind":             {`"build-from-repo"`, `"otro"`},
		"fecha":            {`2026-01-15T10:00:00Z`, `ayer`},
		"artifact null":    {`"artifact":{"kind":"build-from-repo","ref":"r"}`, `"artifact":null`},
	} {
		raw := []byte(strings.Replace(valid, mut[0], mut[1], 1))
		if string(raw) == valid {
			t.Fatalf("%s: la mutación no cambió nada", name)
		}
		if gen.ValidateJSON(schema, raw) == nil {
			t.Errorf("%s: el esquema acepta", name)
		}
		if _, err := inbox.ParseNotifyCreated(raw); err == nil {
			t.Errorf("%s: el parser acepta", name)
		}
	}
}

// Para toda notificación REST generada (los tres estados del enum, con y sin
// artefacto), JSON ida y vuelta la devuelve igual y el estado es válido.
func TestPBT_NotificationJSONRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := gen.Notification().Draw(t, "n")
		raw, err := json.Marshal(n)
		if err != nil {
			t.Fatal(err)
		}
		var back inbox.Notification
		if err := json.Unmarshal(raw, &back); err != nil || !reflect.DeepEqual(back, n) {
			t.Fatalf("%+v / %v, quiero %+v\n%s", back, err, n, raw)
		}
		if !back.State.Valid() {
			t.Fatalf("estado inválido %q", back.State)
		}
		if odd := inbox.State(gen.Free().Draw(t, "odd")); odd.Valid() != (odd == "pending" || odd == "confirmed" || odd == "rejected") {
			t.Fatalf("State(%q).Valid() = %v", odd, odd.Valid())
		}
		if (n.Artifact == nil) != !strings.Contains(string(raw), `"artifact"`) {
			t.Fatalf("omitempty de artifact: %s", raw)
		}
	})
}

// Para todo recibo generado, JSON ida y vuelta lo devuelve igual (el instante con Equal).
func TestPBT_ReceiptJSONRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		r := gen.ConfirmationReceipt().Draw(t, "r")
		raw, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		var back inbox.Receipt
		if err := json.Unmarshal(raw, &back); err != nil || !back.ConfirmedAt.Equal(r.ConfirmedAt) {
			t.Fatalf("%+v / %v, quiero %+v", back, err, r)
		}
		back.ConfirmedAt, r.ConfirmedAt = time.Time{}, time.Time{}
		if back != r {
			t.Fatalf("%+v, quiero %+v", back, r)
		}
	})
}

func TestPBT_Fixed_NotificationAndReceiptJSON(t *testing.T) {
	a := inbox.Artifact{Kind: "tag", Ref: "r"}
	for _, n := range []inbox.Notification{
		{ID: "n-1", GithubEvent: "commit", Repo: "a/b", SHA: strings.Repeat("a", 40), State: inbox.StateRejected},
		{ID: "n-2", GithubEvent: "tag", Repo: "a/b", SHA: strings.Repeat("b", 40), State: inbox.StateConfirmed, Artifact: &a},
	} {
		raw, _ := json.Marshal(n)
		var back inbox.Notification
		if err := json.Unmarshal(raw, &back); err != nil || !reflect.DeepEqual(back, n) {
			t.Fatalf("%+v %v", back, err)
		}
	}
	r := inbox.Receipt{RunID: "run-" + strings.Repeat("0", 32), NotificationID: "n-1", ConfirmedBy: "u", ConfirmedAt: time.Unix(1700000000, 0).UTC()}
	raw, _ := json.Marshal(r)
	var back inbox.Receipt
	if err := json.Unmarshal(raw, &back); err != nil || !back.ConfirmedAt.Equal(r.ConfirmedAt) || back.RunID != r.RunID {
		t.Fatalf("%+v %v", back, err)
	}
}

// path de cada campo con variantes dentro del evento.
var variantPaths = map[string][]string{
	"version": {}, "event_id": {}, "type": {}, "occurred_at": {}, "trace_id": {}, "notification_id": {"data"}, "github_event": {"data"},
	"repo": {"data"}, "sha": {"data"}, "kind": {"data", "artifact"}, "ref": {"data", "artifact"},
}

const canonicalUUID = "3f2b8c1e-6a4d-4e7b-9c10-5d2e8a7f1b34"

// edits enumera, para un valor base, todas las ediciones de un carácter: cambiar
// cada posición por cada carácter de repl, borrar cada posición e insertar cada
// carácter de repl en cada posición (incluidos los extremos). Es el recorrido
// determinista de los límites de un patrón.
func edits(base, repl string) []string {
	var out []string
	for i := 0; i <= len(base); i++ {
		if i < len(base) {
			out = append(out, base[:i]+base[i+1:])
		}
		for _, c := range repl {
			if i < len(base) && rune(base[i]) != c {
				out = append(out, base[:i]+string(c)+base[i+1:])
			}
			out = append(out, base[:i]+string(c)+base[i:])
		}
	}
	return out
}

// bigVariants son las variantes deterministas por campo: uuid y sha por posición
// (guion ausente, primer grupo de 9, g en cada grupo, sha de 39 y de 41...).
func bigVariants() map[string][]string {
	m := map[string][]string{}
	for k, v := range variants {
		m[k] = append([]string(nil), v...)
	}
	m["event_id"] = append(m["event_id"], edits(canonicalUUID, "gG- a")...)
	m["event_id"] = append(m["event_id"], strings.ToUpper(canonicalUUID), canonicalUUID[:23]+strings.ToUpper(canonicalUUID[23:]),
		strings.ReplaceAll(canonicalUUID, "-", ""), "3f2b8c1e-6a4d-4e7b-9c10-5d2e8a7f1b34-3f2b")
	sha := strings.Repeat("a", 40)
	m["sha"] = append(m["sha"], edits(sha, "gGA \n-")...)
	m["sha"] = append(m["sha"], strings.ToUpper(sha), strings.Repeat("A", 40), strings.Repeat("F", 40), sha[:39], sha+"a", sha+sha, "0x"+sha[:38],
		strings.Repeat("٣", 40))
	return m
}

// versionValues son los valores de version recorridos siempre (el esquema pide const 1).
var versionValues = []any{0.0, 2.0, -1.0, 1.5, 0.5, 3.0, 100.0, 1e300, "1", "", true, false, nil, []any{}, []any{1.0}, map[string]any{}}

// versionRaw son formas textuales de números que json.Marshal de un map no produce.
var versionRaw = []string{"0", "2", "-1", "-0", "1.0", "1.00", "1e0", "1E0", "1e-0", "0.1e1", "10e-1", "1.5", "1e1", "9007199254740993", "0.9999999999999999999"}

func agree(t interface{ Fatalf(string, ...any) }, schema *jsonschema.Schema, label string, mraw []byte, stricterOK func() bool) {
	schemaErr := gen.ValidateJSON(schema, mraw)
	_, parseErr := inbox.ParseNotifyCreated(mraw)
	if schemaErr != nil && parseErr == nil {
		// H-1 conocido: el parser acepta hoy esos occurred_at (fijado en el Limit).
		v := strings.Trim(strings.TrimPrefix(label, "occurred_at="), `"`)
		if !strings.HasPrefix(label, "occurred_at=") || !knownLax(v) {
			t.Fatalf("parser más laxo que el esquema con %s: %v\n%s", label, schemaErr, mraw)
		}
	}
	if schemaErr == nil && parseErr != nil && !stricterOK() {
		t.Fatalf("parser más estricto que el esquema fuera de lo documentado con %s: %v\n%s", label, parseErr, mraw)
	}
}

// Acuerdo con el esquema en TODAS las variantes de cada campo, recorridas sin
// azar: uuid y sha por posición (guion ausente, 9 caracteres en el primer grupo,
// g en cada grupo, sha de 39, 41 y en mayúsculas), enums, fechas, vacíos y los
// valores de version (0, 2, -1, 1.0, 1e0...). Donde el esquema rechaza, el parser
// rechaza; donde el esquema acepta y el parser rechaza, es una diferencia
// documentada. Después se repite con eventos generados y las variantes estáticas.
func TestPBT_ParserAgreesOnEveryFieldVariant(t *testing.T) {
	schema := gen.NotifySchema(t)
	run := func(t interface{ Fatalf(string, ...any) }, raw []byte, vs map[string][]string, withVersion bool) {
		keys := make([]string, 0, len(vs))
		for k := range vs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		set := func(k string, v any) (map[string]any, []byte) {
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatalf("%v", err)
			}
			objAt(m, variantPaths[k])[k] = v
			mraw, _ := json.Marshal(m)
			return m, mraw
		}
		for _, k := range keys {
			for _, v := range vs[k] {
				m, mraw := set(k, v)
				agree(t, schema, fmt.Sprintf("%s=%q", k, v), mraw, func() bool { return documentedStricter(m) })
			}
		}
		if !withVersion {
			return
		}
		for _, v := range versionValues {
			m, mraw := set("version", v)
			agree(t, schema, fmt.Sprintf("version=%v", v), mraw, func() bool { return documentedStricter(m) })
		}
		for _, v := range versionRaw {
			mraw := []byte(strings.Replace(string(raw), `"version":1`, `"version":`+v, 1))
			if string(mraw) == string(raw) {
				t.Fatalf("no encuentro version en %s", raw)
			}
			// 1.0, 1e0 y similares: el esquema acepta el 1 numérico; el parser exige el entero 1 (documentado).
			agree(t, schema, "version="+v, mraw, func() bool { return strings.ContainsAny(v, ".eE") })
		}
	}
	canonical := []byte(`{"event_id":"` + canonicalUUID + `","type":"notify.created","version":1,"occurred_at":"2026-01-15T10:00:00Z","trace_id":"t",` +
		`"data":{"notification_id":"n","github_event":"commit","repo":"r","sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","artifact":{"kind":"build-from-repo","ref":"r"}}}`)
	run(t, canonical, bigVariants(), true)
	rapid.Check(t, func(t *rapid.T) {
		e := gen.NotifyCreated().Draw(t, "event")
		raw, err := gen.MarshalEvent(e)
		if err != nil {
			t.Fatal(err)
		}
		run(t, raw, variants, true)
	})
}

// El parser rechaza toda clave repetida, en la raíz, en data y en artifact, con
// cualquier valor (incluso null o el mismo valor): es más estricto que el esquema,
// que solo ve el último valor, y es deliberado.
func TestPBT_ParserRejectsDuplicateKeys(t *testing.T) {
	type site struct{ open, key string }
	sites := []site{
		{`{"event_id"`, "event_id"}, {`{"event_id"`, "type"}, {`{"event_id"`, "version"}, {`{"event_id"`, "occurred_at"},
		{`{"event_id"`, "trace_id"}, {`{"event_id"`, "data"},
		{`"data":{"notification_id"`, "notification_id"}, {`"data":{"notification_id"`, "github_event"},
		{`"data":{"notification_id"`, "repo"}, {`"data":{"notification_id"`, "sha"}, {`"data":{"notification_id"`, "artifact"},
		{`"artifact":{"kind"`, "kind"}, {`"artifact":{"kind"`, "ref"},
	}
	check := func(t interface{ Fatalf(string, ...any) }, doc string, s site, val []byte) {
		pos := strings.Index(doc, s.open)
		if pos < 0 {
			t.Fatalf("no encuentro %q en %s", s.open, doc)
		}
		cut := pos + strings.Index(s.open, "{") + 1
		mut := doc[:cut] + `"` + s.key + `":` + string(val) + `,` + doc[cut:]
		if _, err := inbox.ParseNotifyCreated([]byte(mut)); !errors.Is(err, inbox.ErrInvalidEvent) {
			t.Fatalf("clave repetida %q aceptada (%v):\n%s", s.key, err, mut)
		}
	}
	vals := append([]any{}, replacements...)
	vals = append(vals, "dup")
	// Recorrido determinista: toda clave de cada nivel repetida con todos los valores.
	canonical, _ := gen.MarshalEvent(gen.NotifyCreatedAt(rapid.Just(time.Unix(1000, 0).UTC())).Example(0))
	for _, st := range sites {
		for _, v := range vals {
			val, _ := json.Marshal(v)
			check(t, string(canonical), st, val)
		}
	}
	rapid.Check(t, func(t *rapid.T) {
		e := gen.NotifyCreated().Draw(t, "event")
		raw, err := gen.MarshalEvent(e)
		if err != nil {
			t.Fatal(err)
		}
		val, _ := json.Marshal(rapid.SampledFrom(vals).Draw(t, "value"))
		check(t, string(raw), rapid.SampledFrom(sites).Draw(t, "site"), val)
	})
}

// Ejemplo fijo de las dos propiedades anteriores.
func TestPBT_Fixed_ParserVariantsAndDuplicates(t *testing.T) {
	const valid = `{"event_id":"3f2b8c1e-6a4d-4e7b-9c10-5d2e8a7f1b34","type":"notify.created","version":1,"occurred_at":"2026-01-15T10:00:00Z","trace_id":"t",` +
		`"data":{"notification_id":"n","github_event":"commit","repo":"r","sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","artifact":{"kind":"build-from-repo","ref":"r"}}}`
	schema := gen.NotifySchema(t)
	for name, mut := range map[string]string{
		"uuid con basura": strings.Replace(valid, `"3f2b8c1e-6a4d-4e7b-9c10-5d2e8a7f1b34"`, `"x3f2b8c1e-6a4d-4e7b-9c10-5d2e8a7f1b34"`, 1),
		"uuid largo":      strings.Replace(valid, `5d2e8a7f1b34"`, `5d2e8a7f1b345"`, 1),
		"sha mayúsculas":  strings.Replace(valid, `"aaaaaaaa`, `"AAAAAAAA`, 1),
		"sha 41":          strings.Replace(valid, `aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`, `aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`, 1),
		"type":            strings.Replace(valid, `"notify.created"`, `"notify.created "`, 1),
	} {
		if gen.ValidateJSON(schema, []byte(mut)) == nil {
			t.Errorf("%s: el esquema acepta", name)
		}
		if _, err := inbox.ParseNotifyCreated([]byte(mut)); err == nil {
			t.Errorf("%s: el parser acepta", name)
		}
	}
	for _, mut := range []string{
		strings.Replace(valid, `{"event_id"`, `{"event_id":"x","event_id"`, 1),
		strings.Replace(valid, `"data":{`, `"data":{"repo":null,`, 1),
		strings.Replace(valid, `"artifact":{`, `"artifact":{"kind":"x",`, 1),
	} {
		if _, err := inbox.ParseNotifyCreated([]byte(mut)); !errors.Is(err, inbox.ErrInvalidEvent) {
			t.Errorf("clave repetida aceptada: %v\n%s", err, mut)
		}
	}
}

// Líneas ilegibles del registro de confirmaciones: una línea rota, sin run_id, sin
// confirmed_by o sin notification_id, o un final truncado, no impide abrir, no
// pierde los recibos buenos, se cuenta en Skipped (las líneas en blanco, no) y el
// recibo escrito después de un final truncado no se pega a él.
func TestPBT_ReceiptStoreIgnoresUnreadableLines(t *testing.T) {
	base := t.TempDir()
	type junk struct {
		line    string
		skipped int
	}
	garbage := []junk{{"", 0}, {"   ", 0}, {"{", 1}, {"not json", 1}, {"[]", 1}, {`{"notification_id":"n-x"}`, 1},
		{`{"run_id":"run-x","confirmed_by":"u"}`, 1}, {`{"run_id":"run-x","notification_id":"n-x"}`, 1}, {`{"notification_id":"n-x","confirmed_by":"u"}`, 1},
		{`{"run_id":"","notification_id":"n-x","confirmed_by":"u"}`, 1}}
	rapid.Check(t, func(t *rapid.T) {
		dir := tempDir(base, t)
		events := rapid.SliceOfNDistinct(gen.NotifyCreated(), 1, 5, func(e inbox.NotifyCreated) string { return e.Data.NotificationID }).Draw(t, "events")
		var sb strings.Builder
		skipped := 0
		for _, e := range events {
			for _, g := range rapid.SliceOfN(rapid.SampledFrom(garbage), 0, 2).Draw(t, "garbage") {
				sb.WriteString(g.line + "\n")
				skipped += g.skipped
			}
			line, _ := json.Marshal(map[string]any{"run_id": "run-" + e.Data.NotificationID, "notification_id": e.Data.NotificationID,
				"confirmed_by": "u", "confirmed_at": "2026-01-15T10:00:00Z", "flows": []string{}})
			sb.WriteString(string(line) + "\n")
		}
		truncated := rapid.Bool().Draw(t, "truncated")
		if truncated {
			sb.WriteString(`{"run_id":"run-tr","notification_id":"n-tr","confirmed_by":"u","confirmed_at":"2026-01-15T10`)
			skipped++
		}
		if err := os.WriteFile(filepath.Join(dir, inbox.ConfirmationsFile), []byte(sb.String()), 0o600); err != nil {
			t.Fatal(err)
		}
		st, err := inbox.OpenStore(dir, nil)
		if err != nil {
			t.Fatal(err)
		}
		if st.Skipped != skipped {
			t.Fatalf("Skipped = %d, quiero %d", st.Skipped, skipped)
		}
		for _, e := range events {
			st.Apply(e)
		}
		for _, n := range st.List("") {
			if n.State != inbox.StateConfirmed {
				t.Fatalf("%s no quedó confirmada tras reabrir", n.ID)
			}
		}
		extra := gen.NotifyCreated().Draw(t, "extra")
		extra.Data.NotificationID = "n-extra"
		st.Apply(extra)
		if _, err := st.Confirm("n-extra", "u2", nil); err != nil {
			t.Fatal(err)
		}
		st2, err := inbox.OpenStore(dir, nil)
		if err != nil {
			t.Fatal(err)
		}
		st2.Apply(extra)
		if n := st2.List(inbox.StateConfirmed); len(n) != 1 || n[0].ID != "n-extra" {
			t.Fatalf("el recibo escrito tras una línea %s se perdió: %+v", map[bool]string{true: "truncada", false: "completa"}[truncated], n)
		}
		if st2.Skipped != skipped {
			t.Fatalf("tras reescribir, Skipped = %d, quiero %d (el recibo nuevo no debe pegarse a una línea truncada)", st2.Skipped, skipped)
		}
	})
}

func TestPBT_Fixed_ReceiptStoreIgnoresUnreadableLines(t *testing.T) {
	dir := t.TempDir()
	body := "not json\n\n" + `{"notification_id":"n-x"}` + "\n" + `{"run_id":"run-1","notification_id":"n-1","confirmed_by":"u","confirmed_at":"2026-01-15T10:00:00Z"}` + "\n" + `{"run_id":"run-2","notification_id":"n-2","confirmed_by`
	if err := os.WriteFile(filepath.Join(dir, inbox.ConfirmationsFile), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := inbox.OpenStore(dir, nil)
	if err != nil || st.Skipped != 3 {
		t.Fatalf("%v, Skipped=%d", err, st.Skipped)
	}
}
