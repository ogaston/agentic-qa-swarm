package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

func TestJournalResumeLastStateWins(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Save(runctl.Run{ID: "r", State: runctl.Confirmed})
	_ = s.Save(runctl.Run{ID: "r", State: runctl.Deploying, Attempts: map[string]int{"deploy": 1}})
	s.Close()
	s2, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	r, _ := s2.Get("r")
	if r.State != runctl.Deploying || r.Attempts["deploy"] != 1 {
		t.Fatalf("%+v", r)
	}
	_ = s2.Save(runctl.Run{ID: "r", State: runctl.Inferring}) // la cadena continúa tras reanudar
	s2.Close()
	if s3, err := OpenJournal(dir); err != nil {
		t.Fatal(err)
	} else {
		s3.Close()
	}
}

func TestJournalBrokenHashRefusesToStart(t *testing.T) {
	dir := t.TempDir()
	s, _ := OpenJournal(dir)
	_ = s.Save(runctl.Run{ID: "r", State: runctl.Confirmed})
	_ = s.Save(runctl.Run{ID: "r", State: runctl.WarmReady})
	s.Close()
	p := filepath.Join(dir, JournalFile)
	b, _ := os.ReadFile(p)
	b = []byte(strings.Replace(string(b), `"confirmed"`, `"deploying"`, 1)) // alterar la línea 1
	_ = os.WriteFile(p, b, 0o600)
	if _, err := OpenJournal(dir); err == nil {
		t.Fatal("debía rechazar el diario con hash roto")
	}
}

func gateSrv(t *testing.T, h http.HandlerFunc) *HTTPGate {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	g, err := NewHTTPGate(srv.URL, "tok-de-servicio")
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestHTTPGateAllowDenyAndAuthHeader(t *testing.T) {
	var gotAuth, gotBody string
	g := gateSrv(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		b, _ := json.Marshal(m)
		gotBody = string(b)
		_, _ = w.Write([]byte(`{"allow":false,"reason":"no","audit_ref":"a"}`))
	})
	d, err := g.Authorize(t.Context(), runctl.GateRequest{RunID: "r", From: runctl.Confirmed, To: runctl.WarmReady, TargetNamespace: "aqs-test", Confirmed: runctl.True})
	if err != nil || d.Allow || gotAuth != "Bearer tok-de-servicio" || !strings.Contains(gotBody, `"confirmed":"true"`) {
		t.Fatal(d, err, gotAuth, gotBody)
	}
}

func TestHTTPGateInvalidResponsesAreErrors(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"503":       func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) },
		"401":       func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(401) },
		"basura":    func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`nope`)) },
		"sin allow": func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"reason":"x","audit_ref":"a"}`)) },
		"deny mudo": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"allow":false,"reason":"","audit_ref":"a"}`))
		},
		"redirect": func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/x", 302) },
	} {
		g := gateSrv(t, h)
		if d, err := g.Authorize(t.Context(), runctl.GateRequest{RunID: "r"}); err == nil || d.Allow {
			t.Errorf("%s: debía ser error", name)
		}
	}
}

func TestHTTPGateTimeout(t *testing.T) {
	g := gateSrv(t, func(w http.ResponseWriter, r *http.Request) { time.Sleep(300 * time.Millisecond) })
	g.SetTimeout(50 * time.Millisecond)
	if _, err := g.Authorize(t.Context(), runctl.GateRequest{RunID: "r"}); err == nil {
		t.Fatal("debía expirar")
	}
}

func TestNewHTTPGateRejectsBadConfig(t *testing.T) {
	for _, u := range []string{"ftp://x", "", "http://", "http://u:p@h"} {
		if _, err := NewHTTPGate(u, "t"); err == nil {
			t.Errorf("%q aceptada", u)
		}
	}
	if _, err := NewHTTPGate("http://h", ""); err == nil {
		t.Error("token vacío aceptado")
	}
}

func TestFileSourceFullLinesOnlyAndFilters(t *testing.T) {
	p := filepath.Join(t.TempDir(), "e.jsonl")
	conf := `{"event_id":"e1","type":"run.confirmed","version":1,"trace_id":"t","data":{"run_id":"r-1","confirmed_by":"u","flows":["a"]}}`
	other := `{"event_id":"e2","type":"notify.created","version":1,"trace_id":"t","data":{}}`
	bad := `{"event_id":"e3","type":"run.confirmed","version":1,"trace_id":"t","data":{"run_id":"r-2"}}`
	_ = os.WriteFile(p, []byte(conf+"\n"+other+"\n"+bad+"\n"+conf[:20]), 0o600)
	f := &FileSource{Path: p}
	evs, err := f.Poll(t.Context())
	if err != nil || len(evs) != 1 || evs[0].RunID != "r-1" || f.Discarded() != 1 {
		t.Fatal(evs, err, f.Discarded())
	}
	f.Ack(evs[0])
	again, _ := f.Poll(t.Context())
	if len(again) != 0 {
		t.Fatal("reentregó")
	}
	if e, err := (&FileSource{Path: p + ".no"}).Poll(t.Context()); err != nil || e != nil {
		t.Fatal("archivo inexistente no es error")
	}
}

func TestOutboxIdempotentDeterministicID(t *testing.T) {
	p := filepath.Join(t.TempDir(), "o.jsonl")
	o := &Outbox{Path: p}
	ev := runctl.OutEvent{Type: "run.done", RunID: "r-1", TraceID: "t", EvidenceURIs: []string{"s3://x"}}
	for i := 0; i < 2; i++ {
		if err := o.Publish(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(p)
	if strings.Count(string(b), "\n") != 1 || !strings.Contains(string(b), OutEventID("run.done", "r-1")) {
		t.Fatal(string(b))
	}
	if OutEventID("run.done", "r-1") == OutEventID("run.done", "r-2") {
		t.Fatal("ids iguales")
	}
}

type failDoneStore struct{ *runctl.MemStore }

func (s failDoneStore) Save(r runctl.Run) error {
	if r.DonePublish {
		return errors.New("disco")
	}
	return s.MemStore.Save(r)
}

// Publish seguido de un Save que falla: el outbox idempotente deja una sola línea por run.done.
func TestPublishThenSaveFailsWritesOneOutboxLine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "o.jsonl")
	st := failDoneStore{runctl.NewMemStore()}
	_ = st.MemStore.Save(runctl.Run{ID: "r", State: runctl.Done, TraceID: "t", Evidence: []string{"s3://x"}})
	k, _ := runctl.New(runctl.Config{Gate: runctl.AllowAll(), Store: st, Publisher: &Outbox{Path: p}, Warm: &runctl.FakeWarm{Fact: runctl.True},
		Alerter: &runctl.FakeAlerter{}, Phases: &runctl.FakePhases{}})
	for i := 0; i < 10; i++ {
		_ = k.Drive(context.Background(), "r")
	}
	b, _ := os.ReadFile(p)
	if strings.Count(string(b), "\n") != 1 {
		t.Fatalf("run.done escrito %d veces", strings.Count(string(b), "\n"))
	}
}

func writeEvents(t *testing.T, p string, lines ...string) {
	t.Helper()
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, l := range lines {
		_, _ = f.WriteString(l + "\n")
	}
}

const confLine = `{"event_id":"e1","type":"run.confirmed","version":1,"trace_id":"t","data":{"run_id":"r-1","confirmed_by":"u","flows":["a"]}}`

// Sin Ack el evento se vuelve a entregar; con Ack ya no; las líneas inválidas no se recuentan.
func TestFileSourceUnackedIsRedeliveredAckedIsNot(t *testing.T) {
	p := filepath.Join(t.TempDir(), "e.jsonl")
	writeEvents(t, p, `{"event_id":"x","type":"run.confirmed","version":1}`, confLine)
	f := &FileSource{Path: p}
	for i := 0; i < 3; i++ {
		evs, _ := f.Poll(t.Context())
		if len(evs) != 1 || evs[0].EventID != "e1" {
			t.Fatalf("entrega %d: %+v", i, evs)
		}
	}
	if f.Discarded() != 1 {
		t.Fatalf("la línea inválida se recontó al releer: %d", f.Discarded())
	}
	evs, _ := f.Poll(t.Context())
	f.Ack(evs[0])
	if again, _ := f.Poll(t.Context()); len(again) != 0 {
		t.Fatalf("reentregó tras Ack: %+v", again)
	}
	writeEvents(t, p, strings.Replace(confLine, `"e1"`, `"e2"`, 1))
	if next, _ := f.Poll(t.Context()); len(next) != 1 || next[0].EventID != "e2" {
		t.Fatalf("%+v", next)
	}
}

// Líneas ajenas/inválidas al final se confirman solas; un Ack tardío no retrocede el offset; si el
// archivo se reemplaza por uno más corto, se relee desde 0.
func TestFileSourceForeignLinesAckedAndRotationResets(t *testing.T) {
	p := filepath.Join(t.TempDir(), "e.jsonl")
	writeEvents(t, p, confLine, `{"event_id":"o","type":"notify.created","version":1,"trace_id":"t","data":{}}`)
	f := &FileSource{Path: p}
	evs, _ := f.Poll(t.Context())
	f.Ack(evs[0])
	late := evs[0]
	late.Pos = 1
	f.Ack(late) // Ack tardío: no retrocede
	if f.committed != evs[0].Pos {
		t.Fatalf("el Ack tardío retrocedió el offset a %d", f.committed)
	}
	if _, _ = f.Poll(t.Context()); f.committed != int64(len(confLine)+1+len(`{"event_id":"o","type":"notify.created","version":1,"trace_id":"t","data":{}}`)+1) {
		t.Fatalf("offset confirmado %d: debía cubrir también la línea ajena", f.committed)
	}
	_ = os.WriteFile(p, []byte(strings.Replace(confLine, `"e1"`, `"e9"`, 1)+"\n"), 0o600) // archivo reemplazado (más corto)
	if next, _ := f.Poll(t.Context()); len(next) != 1 || next[0].EventID != "e9" {
		t.Fatalf("tras el reemplazo debía releer desde 0: %+v", next)
	}
}
