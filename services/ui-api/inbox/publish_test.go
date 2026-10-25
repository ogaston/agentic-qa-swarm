package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type recPub struct {
	mu    sync.Mutex
	fails int
	got   []RunConfirmed
}

func (p *recPub) Publish(_ context.Context, ev RunConfirmed) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fails > 0 {
		p.fails--
		return errors.New("transporte caido")
	}
	p.got = append(p.got, ev)
	return nil
}

func storeWithConfirmation(t *testing.T, dir string) (*Store, Receipt) {
	t.Helper()
	st, err := OpenStore(dir, func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	var e NotifyCreated
	e.EventID, e.Type, e.Version, e.OccurredAt, e.TraceID = "e1", "notify.created", 1, time.Now(), "t"
	e.Data.NotificationID, e.Data.Artifact = "n-1", &Artifact{Kind: "build-from-repo", Ref: "x"}
	st.Apply(e)
	r, err := st.ConfirmTraced("n-1", "marta", []string{"checkout"}, "4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatal(err)
	}
	return st, r
}

func TestEventIDDeterministicUUIDv5(t *testing.T) {
	a, b := EventID("run-1"), EventID("run-1")
	if a != b || a == EventID("run-2") {
		t.Fatal("misma entrada, mismo UUID; entrada distinta, UUID distinto")
	}
	if !uuidRe.MatchString(a) || a[14] != '5' || !strings.ContainsRune("89ab", rune(a[19])) {
		t.Fatalf("no es UUIDv5: %s", a)
	}
}

func TestRepublishPendingSendsExactlyOnceEvenIfCalledTwice(t *testing.T) {
	st, _ := storeWithConfirmation(t, t.TempDir())
	rp := &recPub{fails: 2}
	p := NewPublisher(st, rp, nil)
	if n := p.RepublishPending(context.Background()); n != 0 || st.PublishPendingCount() != 1 {
		t.Fatalf("n=%d pendientes=%d", n, st.PublishPendingCount())
	}
	_ = p.Publish(context.Background(), "n-1") // segundo fallo
	if n := p.RepublishPending(context.Background()); n != 1 {
		t.Fatalf("n=%d", n)
	}
	p.RepublishPending(context.Background())
	if len(rp.got) != 1 || st.PublishPendingCount() != 0 {
		t.Fatalf("eventos=%d pendientes=%d", len(rp.got), st.PublishPendingCount())
	}
}

func TestPendingSurvivesRestartAndPublishedDoesNotRepeat(t *testing.T) {
	dir := t.TempDir()
	_, r := storeWithConfirmation(t, dir)
	st2, err := OpenStore(dir, nil)
	if err != nil || st2.PublishPendingCount() != 1 {
		t.Fatalf("err=%v pendientes=%d", err, st2.PublishPendingCount())
	}
	rp := &recPub{}
	NewPublisher(st2, rp, nil).RepublishPending(context.Background())
	if len(rp.got) != 1 || rp.got[0].TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" || rp.got[0].Data.RunID != r.RunID {
		t.Fatalf("%+v", rp.got)
	}
	st3, _ := OpenStore(dir, nil)
	NewPublisher(st3, rp, nil).RepublishPending(context.Background())
	if len(rp.got) != 1 || st3.PublishPendingCount() != 0 {
		t.Fatal("lo ya publicado no se repite tras reiniciar")
	}
}

func TestOutboxIsIdempotentByEventIDAndValidatesAgainstSchema(t *testing.T) {
	st, _ := storeWithConfirmation(t, t.TempDir())
	c, _ := st.Confirmation("n-1")
	ev := NewRunConfirmed(c)
	path := filepath.Join(t.TempDir(), "out.jsonl")
	o := NewOutbox(path)
	for i := 0; i < 3; i++ {
		if err := o.Publish(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(path)
	if n := strings.Count(string(b), "\n"); n != 1 {
		t.Fatalf("lineas=%d", n)
	}
	sch, err := os.ReadFile("../../../contracts/events/run.confirmed.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	_ = json.Unmarshal(sch, &doc)
	cc := jsonschema.NewCompiler()
	cc.DefaultDraft(jsonschema.Draft2020)
	cc.AssertFormat()
	_ = cc.AddResource("mem:///rc.json", doc)
	s, err := cc.Compile("mem:///rc.json")
	if err != nil {
		t.Fatal(err)
	}
	var inst any
	_ = json.Unmarshal([]byte(strings.TrimSpace(string(b))), &inst)
	if err := s.Validate(inst); err != nil {
		t.Fatalf("run.confirmed invalido: %v", err)
	}
}

func TestPublishPendingCountIgnoresOrphanPublishedIDs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, PublishedFile), []byte("{\"notification_id\":\"huerfano-1\"}\n{\"notification_id\":\"huerfano-2\"}\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	st, err := OpenStore(dir, nil)
	if err != nil || st.PublishPendingCount() != 0 {
		t.Fatalf("err=%v pendientes=%d", err, st.PublishPendingCount())
	}
}

// validateRunConfirmed valida el JSON de un run.confirmed contra el esquema real del repositorio.
func validateRunConfirmed(t *testing.T, raw []byte) {
	t.Helper()
	sch, err := os.ReadFile("../../../contracts/events/run.confirmed.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	_ = json.Unmarshal(sch, &doc)
	cc := jsonschema.NewCompiler()
	cc.DefaultDraft(jsonschema.Draft2020)
	cc.AssertFormat()
	if err := cc.AddResource("mem:///rc.json", doc); err != nil {
		t.Fatal(err)
	}
	s, err := cc.Compile("mem:///rc.json")
	if err != nil {
		t.Fatal(err)
	}
	var inst any
	if err := json.Unmarshal(raw, &inst); err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(inst); err != nil {
		t.Fatalf("run.confirmed invalido: %v\n%s", err, raw)
	}
}

// El run.confirmed publicado lleva el artefacto de la notificación confirmada (CA-4a):
// el controlador despliega ese artefacto y no el estático de RUN_ARTIFACT_REF.
func TestRunConfirmedCarriesNotificationArtifact(t *testing.T) {
	sha := strings.Repeat("a", 40)
	want := Artifact{Kind: "published-image", Ref: "ghcr.io/ogaston/demo-target-app-qa-swarm:" + sha}
	st, err := OpenStore(t.TempDir(), func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	var e NotifyCreated
	e.EventID, e.Type, e.Version, e.OccurredAt, e.TraceID = "e1", "notify.created", 1, time.Now(), "t"
	e.Data.NotificationID, e.Data.GithubEvent, e.Data.Repo, e.Data.SHA = "n-1", "commit", "ogaston/demo-target-app-qa-swarm", sha
	e.Data.Artifact = &want
	st.Apply(e)
	if _, err := st.ConfirmTraced("n-1", "marta", []string{"checkout"}, "4bf92f3577b34da6a3ce929d0e0e4736"); err != nil {
		t.Fatal(err)
	}
	rp := &recPub{}
	if err := NewPublisher(st, rp, nil).Publish(context.Background(), "n-1"); err != nil {
		t.Fatal(err)
	}
	if len(rp.got) != 1 {
		t.Fatalf("eventos=%d", len(rp.got))
	}
	got := rp.got[0].Data.Artifact
	if got == nil || *got != want {
		t.Fatalf("artefacto=%+v, quiero %+v", got, want)
	}
	raw, _ := json.Marshal(rp.got[0])
	validateRunConfirmed(t, raw)
}

// Sin artefacto en la Confirmed (recibo antiguo), run.confirmed no lleva el campo: es opcional en el contrato.
func TestRunConfirmedWithoutArtifactOmitsField(t *testing.T) {
	c := Confirmed{
		Receipt: Receipt{RunID: "run-1", NotificationID: "n-1", ConfirmedBy: "marta", ConfirmedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
		Flows:   []string{"checkout"}, TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
	}
	raw, _ := json.Marshal(NewRunConfirmed(c))
	if strings.Contains(string(raw), `"artifact"`) {
		t.Fatalf("sin artefacto no debe salir el campo: %s", raw)
	}
	validateRunConfirmed(t, raw)
}

// F-01 (a): el artefacto persiste en el registro de confirmaciones. Tras reiniciar, sin que
// la notificación se haya re-proyectado (items vacío), run.confirmed sale con el artefacto.
func TestRunConfirmedArtifactSobreviveReinicio(t *testing.T) {
	dir := t.TempDir()
	sha := strings.Repeat("a", 40)
	want := Artifact{Kind: "published-image", Ref: "ghcr.io/ogaston/demo-target-app-qa-swarm:" + sha}
	st, err := OpenStore(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	var e NotifyCreated
	e.EventID, e.Type, e.Version, e.OccurredAt, e.TraceID = "e1", "notify.created", 1, time.Now(), "t"
	e.Data.NotificationID, e.Data.GithubEvent, e.Data.Repo, e.Data.SHA = "n-1", "commit", "ogaston/demo-target-app-qa-swarm", sha
	e.Data.Artifact = &want
	st.Apply(e)
	if _, err := st.ConfirmTraced("n-1", "marta", []string{"checkout"}, "4bf92f3577b34da6a3ce929d0e0e4736"); err != nil {
		t.Fatal(err)
	}
	// Reinicio antes de publicar: el recibo se relee, las notificaciones aún no (replay asíncrono).
	st2, err := OpenStore(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	rp := &recPub{}
	if n := NewPublisher(st2, rp, nil).RepublishPending(context.Background()); n != 1 {
		t.Fatalf("republicados=%d, quiero 1", n)
	}
	if got := rp.got[0].Data.Artifact; got == nil || *got != want {
		t.Fatalf("run.confirmed republicado con artefacto=%+v, quiero %+v", got, want)
	}
}

// F-01 (b), fail-closed: un recibo sin artefacto disponible (registro anterior, sin campo artifact,
// y notificación aún sin re-proyectar) NO se publica ni se marca publicado; queda pendiente.
func TestPublishSinArtefactoDisponibleNoMarcaPublicado(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"run_id":"run-1","notification_id":"n-1","confirmed_by":"marta","confirmed_at":"2026-01-02T03:04:05Z","flows":["checkout"],"trace_id":"4bf92f3577b34da6a3ce929d0e0e4736"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, ConfirmationsFile), []byte(legacy), 0o640); err != nil {
		t.Fatal(err)
	}
	st, err := OpenStore(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	rp := &recPub{}
	p := NewPublisher(st, rp, nil)
	if err := p.Publish(context.Background(), "n-1"); err == nil {
		t.Fatal("sin artefacto disponible, Publish debe fallar")
	}
	if len(rp.got) != 0 || st.IsPublished("n-1") || st.PublishPendingCount() != 1 {
		t.Fatalf("publicado=%v eventos=%d pendientes=%d", st.IsPublished("n-1"), len(rp.got), st.PublishPendingCount())
	}
	// Cuando la notificación llega (replay), el reintento sí publica con artefacto.
	var e NotifyCreated
	e.EventID, e.Type, e.Version, e.OccurredAt, e.TraceID = "e1", "notify.created", 1, time.Now(), "t"
	e.Data.NotificationID, e.Data.GithubEvent, e.Data.Repo, e.Data.SHA = "n-1", "commit", "a/b", strings.Repeat("a", 40)
	e.Data.Artifact = &Artifact{Kind: "published-image", Ref: "ghcr.io/a/b:" + strings.Repeat("a", 40)}
	st.Apply(e)
	if err := p.Publish(context.Background(), "n-1"); err != nil {
		t.Fatalf("tras el replay debe publicar: %v", err)
	}
	if len(rp.got) != 1 || rp.got[0].Data.Artifact == nil || !st.IsPublished("n-1") {
		t.Fatalf("eventos=%d publicado=%v", len(rp.got), st.IsPublished("n-1"))
	}
}
