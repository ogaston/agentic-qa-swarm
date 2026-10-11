package adapters

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

const contractsEvents = "../../../contracts/events"

func natsTestServer(t *testing.T) *natsserver.Server {
	t.Helper()
	srv, err := natsserver.NewServer(&natsserver.Options{Host: "127.0.0.1", Port: -1, JetStream: true, StoreDir: t.TempDir(), NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(10 * time.Second) {
		t.Fatal("nats-server no arrancó")
	}
	t.Cleanup(srv.Shutdown)
	return srv
}

func natsTestJS(t *testing.T, srv *natsserver.Server) jetstream.JetStream {
	t.Helper()
	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := js.CreateStream(context.Background(), jetstream.StreamConfig{
		Name: "AQS_EVENTS", Subjects: []string{"aqs.events.>"}, Storage: jetstream.FileStorage,
		Duplicates: 2 * time.Minute, Replicas: 1,
	}); err != nil {
		t.Fatal(err)
	}
	return js
}

// runConfirmedLine es un run.confirmed válido contra su esquema.
func runConfirmedLine(eventID, runID string) []byte {
	return []byte(`{"event_id":"` + eventID + `","type":"run.confirmed","version":1,` +
		`"occurred_at":"2026-10-11T10:00:00Z","trace_id":"0af7651916cd43dd8448eb211c80319c",` +
		`"data":{"run_id":"` + runID + `","notification_id":"n-1","confirmed_by":"demo","flows":["login"]}}`)
}

func compilarEsquema(t *testing.T, nombre string) *jsonschema.Schema {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(contractsEvents, nombre))
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()
	if err := c.AddResource("mem:///e.json", doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("mem:///e.json")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestParseRunConfirmedAgreesWithSchema exige que el validador de NATS coincida con
// contracts/events/run.confirmed.schema.json en el ejemplo válido y en los inválidos del contrato.
func TestParseRunConfirmedAgreesWithSchema(t *testing.T) {
	s := compilarEsquema(t, "run.confirmed.schema.json")
	valido := runConfirmedLine("55555555-5555-4555-8555-555555555555", "r-1")
	var doc any
	if err := json.Unmarshal(valido, &doc); err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(doc); err != nil {
		t.Fatalf("el ejemplo válido no cumple el esquema: %v", err)
	}
	if _, err := ParseRunConfirmed(valido); err != nil {
		t.Fatalf("ParseRunConfirmed rechaza un válido: %v", err)
	}

	invalidos, _ := filepath.Glob(filepath.Join(contractsEvents, "examples", "invalid", "run.confirmed.*.json"))
	if len(invalidos) == 0 {
		t.Fatal("no hay ejemplos inválidos de run.confirmed")
	}
	for _, f := range invalidos {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var d any
		if err := json.Unmarshal(b, &d); err != nil {
			t.Fatal(err)
		}
		if s.Validate(d) == nil {
			t.Fatalf("%s: el esquema lo acepta y debía rechazarlo", filepath.Base(f))
		}
		if _, err := ParseRunConfirmed(b); err == nil {
			t.Errorf("%s: ParseRunConfirmed lo acepta y el esquema lo rechaza", filepath.Base(f))
		}
	}

	// Sin notification_id: el esquema lo exige y el parser debe rechazarlo también.
	sin := strings.Replace(string(valido), `"notification_id":"n-1",`, "", 1)
	if _, err := ParseRunConfirmed([]byte(sin)); err == nil {
		t.Fatal("ParseRunConfirmed acepta run.confirmed sin notification_id")
	}
}

// TestNATSOutboxRunDoneEsMismoJSONQueArchivo: run.done por NATS lleva la misma línea que el outbox de
// archivo y Nats-Msg-Id = event_id.
func TestNATSOutboxRunDoneEsMismoJSONQueArchivo(t *testing.T) {
	srv := natsTestServer(t)
	js := natsTestJS(t, srv)
	fixed := time.Date(2026, 10, 11, 10, 0, 0, 0, time.UTC)
	ev := runctl.OutEvent{Type: "run.done", RunID: "r-9", TraceID: "0af7651916cd43dd8448eb211c80319c", EvidenceURIs: []string{"s3://evidence/r-9/a.json"}}

	path := filepath.Join(t.TempDir(), "outbox.jsonl")
	if err := (&Outbox{Path: path, Now: func() time.Time { return fixed }}).Publish(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	archivo, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pub := &NATSOutbox{JS: js, Now: func() time.Time { return fixed }}
	if err := pub.Publish(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	st, err := js.Stream(context.Background(), "AQS_EVENTS")
	if err != nil {
		t.Fatal(err)
	}
	m, err := st.GetMsg(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSuffix(string(archivo), "\n") != string(m.Data) {
		t.Fatalf("JSON distinto:\narchivo=%s\nnats=%s", archivo, m.Data)
	}
	if m.Subject != "aqs.events.run.done" || m.Header.Get("Nats-Msg-Id") != OutEventID("run.done", "r-9") {
		t.Fatalf("subject=%q msg-id=%q", m.Subject, m.Header.Get("Nats-Msg-Id"))
	}
}

// pollHasta llama a Poll hasta que cond(evs) o falla tras 10 s; devuelve lo acumulado.
func pollHasta(t *testing.T, src *NATSSource, what string, cond func([]runctl.Event) bool) []runctl.Event {
	t.Helper()
	var all []runctl.Event
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		evs, err := src.Poll(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, evs...)
		if cond(all) {
			return all
		}
	}
	t.Fatalf("tiempo agotado: %s", what)
	return nil
}

func TestNATSSourceAckYReentregaSinAck(t *testing.T) {
	srv := natsTestServer(t)
	js := natsTestJS(t, srv)
	ctx := context.Background()
	if _, err := js.Publish(ctx, "aqs.events.run.confirmed", runConfirmedLine("66666666-6666-4666-8666-666666666666", "r-1")); err != nil {
		t.Fatal(err)
	}
	src := &NATSSource{JS: js, AckWait: time.Second}

	evs := pollHasta(t, src, "entrega de run.confirmed", func(e []runctl.Event) bool { return len(e) >= 1 })
	ev := evs[0]
	if ev.Type != runctl.EvRunConfirmed || ev.RunID != "r-1" || ev.ConfirmedBy != "demo" || len(ev.Flows) != 1 {
		t.Fatalf("evento mal parseado: %+v", ev)
	}
	// Sin Ack: pasado AckWait el servidor lo vuelve a entregar (at-least-once).
	again := pollHasta(t, src, "reentrega tras AckWait sin Ack", func(e []runctl.Event) bool { return len(e) >= 2 })
	src.Ack(again[len(again)-1])

	// Con Ack no vuelve a salir.
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		evs, err := src.Poll(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(evs) != 0 {
			t.Fatalf("tras Ack salió de nuevo: %+v", evs)
		}
	}
}

func TestNATSSourceInvalidoSeHaceTermYNoSeEntrega(t *testing.T) {
	srv := natsTestServer(t)
	js := natsTestJS(t, srv)
	ctx := context.Background()
	// Inválido: flows vacío.
	bad := []byte(`{"event_id":"77777777-7777-4777-8777-777777777777","type":"run.confirmed","version":1,` +
		`"occurred_at":"2026-10-11T10:00:00Z","trace_id":"t","data":{"run_id":"r-2","notification_id":"n-2","confirmed_by":"demo","flows":[]}}`)
	if _, err := js.Publish(ctx, "aqs.events.run.confirmed", bad); err != nil {
		t.Fatal(err)
	}
	src := &NATSSource{JS: js, AckWait: time.Second}
	// Un válido después para saber que el consumidor ya pasó por el inválido.
	if _, err := js.Publish(ctx, "aqs.events.run.confirmed", runConfirmedLine("88888888-8888-4888-8888-888888888888", "r-3")); err != nil {
		t.Fatal(err)
	}
	evs := pollHasta(t, src, "válido tras el inválido", func(e []runctl.Event) bool { return len(e) >= 1 })
	for _, ev := range evs {
		if ev.RunID == "r-2" {
			t.Fatal("el inválido llegó al controlador")
		}
	}
	src.Ack(evs[0])
	if src.Discarded() != 1 {
		t.Fatalf("descartados = %d, quiero 1", src.Discarded())
	}
	st, _ := js.Stream(ctx, "AQS_EVENTS")
	c, err := st.Consumer(ctx, "go-run-controller-confirmed")
	if err != nil {
		t.Fatal(err)
	}
	info, err := c.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.NumRedelivered != 0 {
		t.Fatalf("redelivered = %d, quiero 0 (Term, no reentrega)", info.NumRedelivered)
	}
}
