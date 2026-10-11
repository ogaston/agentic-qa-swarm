package intake

import (
	"context"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// startNATS levanta un nats-server embebido con JetStream en un directorio temporal.
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

// streamAQS crea el stream AQS_EVENTS como lo hace nats-init (subjects aqs.events.>, dedupe 2 min).
func streamAQS(t *testing.T, js jetstream.JetStream) jetstream.Stream {
	t.Helper()
	s, err := js.CreateStream(context.Background(), jetstream.StreamConfig{
		Name: "AQS_EVENTS", Subjects: []string{"aqs.events.>"}, Storage: jetstream.FileStorage,
		Duplicates: 2 * time.Minute, Replicas: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func evento(id string) Event {
	return Event{
		EventID: id, Type: "notify.created", Version: 1,
		OccurredAt: "2026-10-11T10:00:00Z", TraceID: "0af7651916cd43dd8448eb211c80319c",
		Data: EventData{NotificationID: "n-1", GithubEvent: "commit", Repo: "acme/shop",
			SHA: "1111111111111111111111111111111111111111", Artifact: Artifact{Kind: "build-from-repo", Ref: "main"}},
	}
}

func TestNATSPublisherPublicaConNatsMsgIDYDeduplica(t *testing.T) {
	srv := startNATS(t)
	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	st := streamAQS(t, js)

	p := NewNATSPublisher(js)
	ctx := context.Background()
	ev := evento("8f0c5f5e-1d9e-4f7e-9a3b-2c6d1e0f4a11")
	if err := p.Publish(ctx, ev); err != nil {
		t.Fatalf("primera publicación: %v", err)
	}
	if err := p.Publish(ctx, ev); err != nil { // mismo event_id: la ventana de dedupe lo descarta
		t.Fatalf("reenvío: %v", err)
	}
	info, err := st.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.State.Msgs != 1 {
		t.Fatalf("mensajes en el stream = %d, quiero 1", info.State.Msgs)
	}
	m, err := st.GetMsg(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if m.Subject != "aqs.events.notify.created" {
		t.Fatalf("subject = %q", m.Subject)
	}
	if got := m.Header.Get("Nats-Msg-Id"); got != ev.EventID {
		t.Fatalf("Nats-Msg-Id = %q, quiero %q", got, ev.EventID)
	}
}

func TestNATSPublisherErrorSinStream(t *testing.T) {
	srv := startNATS(t)
	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, _ := jetstream.New(nc)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := NewNATSPublisher(js).Publish(ctx, evento("8f0c5f5e-1d9e-4f7e-9a3b-2c6d1e0f4a12")); err == nil {
		t.Fatal("sin stream AQS_EVENTS debe fallar la publicación (el llamador reintenta)")
	}
}
