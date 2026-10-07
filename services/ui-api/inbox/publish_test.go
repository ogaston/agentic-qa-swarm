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
