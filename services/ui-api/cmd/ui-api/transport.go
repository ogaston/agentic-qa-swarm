package main

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/ogaston/agentic-qa-swarm/services/ui-api/inbox"
	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/obs"
)

// transporte agrupa lo que aporta cada modo de EVENTS_TRANSPORT (U8-T02) a run().
type transporte struct {
	pub   inbox.EventPublisher
	file  *inbox.FileSubscriber // EVENTS_TRANSPORT=file (por defecto): carga inicial y sondeo del outbox
	nats  *inbox.NATSSubscriber // EVENTS_TRANSPORT=nats: replay del stream y durable ui-api-inbox
	ready []obs.Check           // chequeos de /readyz del transporte
}

// transportFromEnv elige el transporte de eventos. file (por defecto) usa UIAPI_EVENTS_FILE y
// UIAPI_OUTBOX_FILE y no cambia; nats usa NATS_URL y JetStream.
func transportFromEnv(log *slog.Logger, dataDir string, verifierSet func() bool) (transporte, error) {
	switch os.Getenv("EVENTS_TRANSPORT") {
	case "", "file":
		eventsFile, outboxFile := os.Getenv("UIAPI_EVENTS_FILE"), os.Getenv("UIAPI_OUTBOX_FILE")
		if eventsFile == "" || dataDir == "" || outboxFile == "" {
			return transporte{}, errors.New("UIAPI_EVENTS_FILE, UIAPI_DATA_DIR y UIAPI_OUTBOX_FILE son obligatorios")
		}
		return transporte{
			pub:   inbox.NewOutbox(outboxFile),
			file:  newSubscriber(log, eventsFile),
			ready: obs.ReadyChecks(dataDir, eventsFile, verifierSet),
		}, nil
	case "nats":
		if dataDir == "" {
			return transporte{}, errors.New("UIAPI_DATA_DIR es obligatorio")
		}
		nc, err := inbox.ConnectNATS(os.Getenv("NATS_URL"))
		if err != nil {
			return transporte{}, err
		}
		js, err := jetstream.New(nc)
		if err != nil {
			return transporte{}, err
		}
		return transporte{
			pub:   inbox.NewNATSPublisher(js),
			nats:  inbox.NewNATSSubscriber(js, log),
			ready: natsReadyChecks(dataDir, nc, verifierSet),
		}, nil
	default:
		return transporte{}, errors.New("EVENTS_TRANSPORT debe ser file o nats")
	}
}

// natsReadyChecks son los chequeos de /readyz con NATS: los del servicio sin events_file (el outbox
// de archivo no aplica), más "nats", que falla sin conexión.
func natsReadyChecks(dataDir string, nc *nats.Conn, verifierSet func() bool) []obs.Check {
	var out []obs.Check
	for _, c := range obs.ReadyChecks(dataDir, "", verifierSet) {
		if c.Name != "events_file" {
			out = append(out, c)
		}
	}
	return append(out, obs.Check{Name: "nats", Fn: func(context.Context) error { return inbox.NATSReady(nc) }})
}
