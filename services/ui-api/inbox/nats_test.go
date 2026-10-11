package inbox

import (
	"context"
	"sync"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func startNATS(t *testing.T) *natsserver.Server {
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

func jsConn(t *testing.T, srv *natsserver.Server) (*nats.Conn, jetstream.JetStream) {
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
	return nc, js
}

// streamAQS crea AQS_EVENTS como nats-init (aqs.events.>, dedupe 2 min).
func streamAQS(t *testing.T, js jetstream.JetStream) jetstream.Stream {
	t.Helper()
	s, err := js.CreateStream(context.Background(), jetstream.StreamConfig{
		Name: EventsStreamName, Subjects: []string{"aqs.events.>"}, Storage: jetstream.FileStorage,
		Duplicates: 2 * time.Minute, Replicas: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// notifyLine es un notify.created válido contra su esquema (mismo formato que el outbox de U6).
func notifyLine(eventID, notifID string) []byte {
	return []byte(`{"event_id":"` + eventID + `","type":"notify.created","version":1,` +
		`"occurred_at":"2026-10-11T10:00:00Z","trace_id":"0af7651916cd43dd8448eb211c80319c",` +
		`"data":{"notification_id":"` + notifID + `","github_event":"commit","repo":"acme/shop",` +
		`"sha":"1111111111111111111111111111111111111111","artifact":{"kind":"build-from-repo","ref":"main"}}}`)
}

func TestNATSPublisherRunConfirmedConMsgIDYDeduplica(t *testing.T) {
	srv := startNATS(t)
	_, js := jsConn(t, srv)
	st := streamAQS(t, js)
	p := NewNATSPublisher(js)
	ctx := context.Background()
	ev := NewRunConfirmed(Confirmed{Receipt: Receipt{RunID: "r-1", NotificationID: "n-1", ConfirmedBy: "demo", ConfirmedAt: time.Unix(0, 0)}, Flows: []string{"f"}})
	if err := p.Publish(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if err := p.Publish(ctx, ev); err != nil {
		t.Fatal(err)
	}
	info, _ := st.Info(ctx)
	if info.State.Msgs != 1 {
		t.Fatalf("mensajes = %d, quiero 1 (dedupe por event_id)", info.State.Msgs)
	}
	m, err := st.GetMsg(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if m.Subject != "aqs.events.run.confirmed" || m.Header.Get("Nats-Msg-Id") != ev.EventID {
		t.Fatalf("subject=%q msg-id=%q", m.Subject, m.Header.Get("Nats-Msg-Id"))
	}
}

// recolector guarda los eventos que entrega el suscriptor.
type recolector struct {
	mu  sync.Mutex
	ids []string
}

func (r *recolector) handle(ev NotifyCreated) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ids = append(r.ids, ev.EventID)
}

func (r *recolector) cuenta() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.ids)
}

// esperar hace polling hasta que cond() o falla tras 10 s.
func esperar(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("tiempo agotado esperando: %s", what)
}

func TestNATSSubscriberAckSoloTrasAplicarYTermInvalido(t *testing.T) {
	srv := startNATS(t)
	_, js := jsConn(t, srv)
	st := streamAQS(t, js)
	ctx := context.Background()
	const e1, e2 = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	if _, err := js.Publish(ctx, "aqs.events.notify.created", notifyLine(e1, "n-1")); err != nil {
		t.Fatal(err)
	}
	// Inválido: falta data.artifact. Debe hacerse Term y no entregarse a handle.
	if _, err := js.Publish(ctx, "aqs.events.notify.created", []byte(`{"event_id":"`+e2+`","type":"notify.created"}`)); err != nil {
		t.Fatal(err)
	}

	var rec recolector
	sub := NewNATSSubscriber(js, nil)
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- sub.Run(runCtx, rec.handle) }()
	// El durable (no el replay) es quien hace Ack/Term: se espera a que no quede nada pendiente y a que
	// haya entregado los dos mensajes del stream.
	var info jetstream.ConsumerInfo
	esperar(t, "durable sin pendientes tras el válido y el inválido", func() bool {
		c, err := st.Consumer(ctx, "ui-api-inbox")
		if err != nil {
			return false
		}
		i, err := c.Info(ctx)
		if err != nil {
			return false
		}
		info = *i
		return i.NumPending == 0 && i.NumAckPending == 0 && i.Delivered.Stream >= 2
	})
	stop()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if rec.cuenta() < 1 || rec.ids[0] != e1 {
		t.Fatalf("entregados = %v, quiero primero %s", rec.ids, e1)
	}
	for _, id := range rec.ids {
		if id == e2 {
			t.Fatalf("el inválido %s llegó a handle", e2)
		}
	}
	if info.NumRedelivered != 0 {
		t.Fatalf("redelivered = %d, quiero 0 (el inválido se hizo Term, no se reentrega)", info.NumRedelivered)
	}
}

func TestNATSSubscriberReplayReconstruyeTrasReinicio(t *testing.T) {
	srv := startNATS(t)
	_, js := jsConn(t, srv)
	streamAQS(t, js)
	ctx := context.Background()
	if _, err := js.Publish(ctx, "aqs.events.notify.created", notifyLine("33333333-3333-4333-8333-333333333333", "n-1")); err != nil {
		t.Fatal(err)
	}

	// Primera vida: aplica y ack. Segunda vida (reinicio, store nuevo): el durable ya está al día,
	// pero el replay debe volver a entregar el evento para reconstruir la proyección en memoria.
	for vida := 1; vida <= 2; vida++ {
		var rec recolector
		runCtx, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- NewNATSSubscriber(js, nil).Run(runCtx, rec.handle) }()
		esperar(t, "replay en vida "+string(rune('0'+vida)), func() bool { return rec.cuenta() >= 1 })
		stop()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

func TestNATSSubscriberDedupePorEventIDEnStore(t *testing.T) {
	srv := startNATS(t)
	_, js := jsConn(t, srv)
	streamAQS(t, js)
	ctx := context.Background()
	const e = "44444444-4444-4444-8444-444444444444"
	// Dos mensajes distintos (sin Nats-Msg-Id, no los dedupea el stream) con el mismo event_id.
	for i := 0; i < 2; i++ {
		if _, err := js.Publish(ctx, "aqs.events.notify.created", notifyLine(e, "n-1")); err != nil {
			t.Fatal(err)
		}
	}
	st, err := OpenStore(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	sub := NewNATSSubscriber(js, nil)
	go func() {
		done <- sub.Run(runCtx, func(ev NotifyCreated) {
			mu.Lock()
			defer mu.Unlock()
			st.Apply(ev)
		})
	}()
	esperar(t, "ambos mensajes confirmados", func() bool {
		cons, err := js.Stream(ctx, EventsStreamName)
		if err != nil {
			return false
		}
		c, err := cons.Consumer(ctx, "ui-api-inbox")
		if err != nil {
			return false
		}
		info, err := c.Info(ctx)
		return err == nil && info.AckFloor.Stream == 2
	})
	stop()
	<-done
	mu.Lock()
	defer mu.Unlock()
	if n := len(st.List(StatePending)); n != 1 {
		t.Fatalf("notificaciones = %d, quiero 1 (dedupe por event_id)", n)
	}
}
