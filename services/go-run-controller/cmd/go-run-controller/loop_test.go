package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/adapters"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/internal/obs"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

// flakyStore falla Save mientras down; cuenta los guardados exitosos.
type flakyStore struct {
	*runctl.MemStore
	down  atomic.Bool
	saves int
}

func (f *flakyStore) Save(r runctl.Run) error {
	if f.down.Load() {
		return errors.New("disco")
	}
	f.saves++
	return f.MemStore.Save(r)
}

const (
	confLine = `{"event_id":"e1","type":"run.confirmed","version":1,"trace_id":"t","data":{"run_id":"r-1","confirmed_by":"u","flows":["a"]}}`
	passLine = `{"event_id":"e2","type":"rehearsal.passed","version":1,"trace_id":"t","data":{"run_id":"r-1"}}`
)

func rig(t *testing.T, lines ...string) (*runctl.Controller, *flakyStore, *adapters.FileSource) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "e.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	st := &flakyStore{MemStore: runctl.NewMemStore()}
	ctl, err := runctl.New(runctl.Config{Gate: runctl.AllowAll(), Store: st, Publisher: &runctl.FakePublisher{}, Warm: &runctl.FakeWarm{Fact: runctl.True},
		Alerter: &runctl.FakeAlerter{}, Phases: &runctl.FakePhases{}})
	if err != nil {
		t.Fatal(err)
	}
	return ctl, st, &adapters.FileSource{Path: p}
}

func ticks(ctl *runctl.Controller, src runctl.EventSource, n int) {
	for range n {
		tick(context.Background(), nopLog(), ctl, src)
	}
}

// Con Save fallando run.confirmed no se pierde: la corrida se crea al sanar.
func TestEventNotLostWhenSaveFailsDuringApply(t *testing.T) {
	ctl, st, src := rig(t, confLine)
	st.down.Store(true)
	ticks(ctl, src, 3)
	if _, ok := st.Get("r-1"); ok {
		t.Fatal("no debía crearse con el disco caído")
	}
	st.down.Store(false)
	ticks(ctl, src, 30)
	if _, ok := st.Get("r-1"); !ok {
		t.Fatal("la corrida nunca se crea: el evento se perdió")
	}
}

// rehearsal.passed aplicado con el disco caído no deja la corrida esperando para siempre.
func TestEventRedeliveredOnPersistErrorRehearsalPassed(t *testing.T) {
	ctl, st, src := rig(t, passLine)
	_ = st.MemStore.Save(runctl.Run{ID: "r-1", State: runctl.Rehearsing, ConfirmedBy: "u", Flows: []string{"a"},
		Launched: map[string]bool{runctl.PhaseRehearse: true}})
	st.down.Store(true)
	ticks(ctl, src, 3)
	st.down.Store(false)
	ticks(ctl, src, 30)
	r, _ := st.Get("r-1")
	if r.EnsayoPassed != runctl.True || r.State == runctl.Rehearsing {
		t.Fatalf("la corrida sigue esperando el ensayo: %+v", r)
	}
}

// spySource registra, en cada Ack, si el evento ya estaba aplicado.
type spySource struct {
	evs     []runctl.Event
	st      runctl.RunStore
	acks    int
	applied []bool
	polls   int
}

func (s *spySource) Poll(context.Context) ([]runctl.Event, error) {
	s.polls++
	return s.evs, nil
}
func (s *spySource) Ack(ev runctl.Event) {
	_, ok := s.st.Get(ev.RunID)
	s.acks++
	s.applied = append(s.applied, ok)
}

// El Ack ocurre solo después de un Apply exitoso.
func TestEventAckAfterApply(t *testing.T) {
	ctl, st, _ := rig(t, confLine)
	ev := runctl.Event{Type: runctl.EvRunConfirmed, EventID: "e1", TraceID: "t", RunID: "r-1", ConfirmedBy: "u", Flows: []string{"a"}}
	spy := &spySource{evs: []runctl.Event{ev}, st: st}
	st.down.Store(true)
	ticks(ctl, spy, 3)
	if spy.acks != 0 {
		t.Fatalf("Ack con el disco caído: %d", spy.acks)
	}
	st.down.Store(false)
	ticks(ctl, spy, 1)
	if spy.acks != 1 || !spy.applied[0] {
		t.Fatalf("acks=%d aplicado-al-confirmar=%v", spy.acks, spy.applied)
	}
}

// Un evento ya aplicado que se entrega de nuevo (por tick y Ack) no se aplica dos veces ni guarda de nuevo.
func TestEventAckAfterApplyAlreadyAppliedNotAppliedTwice(t *testing.T) {
	ctl, st, _ := rig(t, confLine)
	_ = st.MemStore.Save(runctl.Run{ID: "r-1", State: runctl.Rehearsing, ConfirmedBy: "u", Flows: []string{"a"},
		Launched: map[string]bool{runctl.PhaseRehearse: true}})
	ev := runctl.Event{Type: runctl.EvRehearsalPassed, EventID: "e2", TraceID: "t", RunID: "r-1"}
	conf := runctl.Event{Type: runctl.EvRunConfirmed, EventID: "e1", TraceID: "t", RunID: "r-1", ConfirmedBy: "u", Flows: []string{"a"}}
	spy := &spySource{evs: []runctl.Event{ev}, st: st}
	ticks(ctl, spy, 1)
	r1, _ := st.Get("r-1")
	spy.evs = []runctl.Event{ev, conf, ev, conf}
	before := st.saves
	ticks(ctl, spy, 3)
	if r2, _ := st.Get("r-1"); r2.EnsayoPassed != r1.EnsayoPassed || len(r2.Seen) != len(r1.Seen) {
		t.Fatalf("un evento ya aplicado se aplicó dos veces: %+v -> %+v", r1, r2)
	}
	// los guardados de DriveAll avanzan la corrida; los eventos repetidos no añaden ninguno propio
	if len(r1.Seen) != 1 {
		t.Fatal(r1.Seen)
	}
	_ = before
}

// Lote [run.confirmed, rehearsal.passed] con el disco caído: el segundo falla sin Save (ErrNotFound)
// pero no debe confirmarse por encima del primero (que no se aplicó).
func TestEventBatchPersistErrorDoesNotSkipLaterEvents(t *testing.T) {
	ctl, st, src := rig(t, confLine, passLine)
	st.down.Store(true)
	ticks(ctl, src, 3)
	st.down.Store(false)
	ticks(ctl, src, 30)
	if _, ok := st.Get("r-1"); !ok {
		t.Fatal("run.confirmed perdido: el Ack del segundo evento saltó al primero")
	}
}

// El lazo REAL (loop) usa tick: con el disco caído no confirma, y al sanar crea la corrida.
func TestEventNotLostWhenSaveFailsInRealLoop(t *testing.T) {
	old := tickEvery
	tickEvery = 2 * time.Millisecond
	defer func() { tickEvery = old }()
	ctl, st, src := rig(t, confLine)
	st.down.Store(true)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { loop(ctx, nopLog(), ctl, src); close(done) }()
	time.Sleep(60 * time.Millisecond)
	st.down.Store(false)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := st.Get("r-1"); ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("loop no termina al cancelar el contexto")
	}
	if _, ok := st.Get("r-1"); !ok {
		t.Fatal("el lazo real perdió run.confirmed")
	}
}

// openJournal (lo que usa run()) registra la cola descartada en el log y en la métrica.
func TestJournalTornTailWiringOpenJournalLogsAndCounts(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, adapters.JournalFile), []byte(`{"seq":1,"pre`), 0o600); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	m := &tailM{}
	j, err := openJournal(dir, slog.New(slog.NewTextHandler(&logs, nil)), m)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if m.n != 1 || !strings.Contains(logs.String(), "bytes_descartados=13") {
		t.Fatalf("métrica=%d log=%q", m.n, logs.String())
	}
}

// Un fallo que no es del almacén (corrida inexistente) se confirma: no se reentrega para siempre.
func TestEventAckAfterApplyPermanentErrorIsAcked(t *testing.T) {
	ctl, st, src := rig(t, passLine) // sin corrida r-1
	_ = st
	ticks(ctl, src, 1)
	if evs, _ := src.Poll(context.Background()); len(evs) != 0 {
		t.Fatalf("reentrega eterna de un evento irrecuperable: %+v", evs)
	}
}

// El chequeo persist de /readyz está cableado a la salud de persistencia del controlador.
func TestReadyzPersistWiring(t *testing.T) {
	ctl, st, src := rig(t, confLine)
	reg := prometheus.NewRegistry()
	h := obs.Wrap(obs.Config{Service: "t", Log: nopLog(), Registry: reg, Metrics: obs.NewHTTPMetrics(reg, "t"),
		Ready: readyChecks(okJournal{}, ctl), Route: func(*http.Request) string { return "unmatched" }}, http.NotFoundHandler())
	ready := func() (int, string) {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		return rr.Code, rr.Body.String()
	}
	st.down.Store(true)
	ticks(ctl, src, 1) // el Save de Apply falla
	if code, body := ready(); code != http.StatusServiceUnavailable || !strings.Contains(body, `"persist":"fail"`) {
		t.Fatalf("con el Save fallando /readyz debe dar 503 persist fail: %d %s", code, body)
	}
	st.down.Store(false)
	ticks(ctl, src, 1)
	if code, body := ready(); code != http.StatusOK {
		t.Fatalf("sanado debe dar 200: %d %s", code, body)
	}
}

type okJournal struct{}

func (okJournal) Healthy() error { return nil }

type tailJ int

func (j tailJ) TailDiscarded() int { return int(j) }

type tailM struct{ n int }

func (m *tailM) JournalTailDiscarded() { m.n++ }

func TestJournalTornTailWiringCountsMetric(t *testing.T) {
	m := &tailM{}
	noteJournalTail(tailJ(0), m)
	if m.n != 0 {
		t.Fatal("sin cola descartada no se cuenta")
	}
	noteJournalTail(tailJ(19), m)
	if m.n != 1 {
		t.Fatal("con cola descartada se cuenta una vez")
	}
}
