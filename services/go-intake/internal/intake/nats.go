package intake

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// EventsStream es el stream JetStream de los eventos de la plataforma (U8-T01, nats-init).
const EventsStream = "AQS_EVENTS"

// eventsSubjectPrefix es el prefijo de los subjects de eventos: aqs.events.<tipo>.
const eventsSubjectPrefix = "aqs.events."

// NATSPublisher publica notify.created en JetStream (C-45, U8-T02). Nats-Msg-Id = event_id: el stream
// descarta un reenvío con el mismo event_id dentro de su ventana de deduplicación (2 min).
type NATSPublisher struct {
	js jetstream.JetStream
}

var _ EventPublisher = (*NATSPublisher)(nil)

// NewNATSPublisher devuelve el publicador sobre js.
func NewNATSPublisher(js jetstream.JetStream) *NATSPublisher { return &NATSPublisher{js: js} }

// Publish implementa EventPublisher. Devuelve error si el stream no confirma la publicación; el
// llamador (el handler del webhook) trata el fallo como hoy con el archivo.
func (p *NATSPublisher) Publish(ctx context.Context, ev Event) error {
	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if _, err := p.js.Publish(ctx, eventsSubjectPrefix+ev.Type, line, jetstream.WithMsgID(ev.EventID)); err != nil {
		return fmt.Errorf("publicando %s en NATS: %w", ev.Type, err)
	}
	return nil
}

// ConnectNATS abre la conexión a url. RetryOnFailedConnect: no falla si NATS aún no responde (nats-init
// reinicia mientras el clúster arranca); la conexión se establece en segundo plano. La reconexión tiene
// espera fija acotada (2 s con jitter de hasta 1 s) y sin tope de intentos.
func ConnectNATS(url string) (*nats.Conn, error) {
	if url == "" {
		return nil, errors.New("NATS_URL es obligatorio con EVENTS_TRANSPORT=nats")
	}
	return nats.Connect(url,
		nats.Name("go-intake"),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
		nats.ReconnectJitter(200*time.Millisecond, time.Second),
		nats.Timeout(5*time.Second),
	)
}

// NATSReady es el chequeo de /readyz del transporte: falla mientras la conexión no esté CONNECTED.
func NATSReady(nc *nats.Conn) error {
	if nc == nil || nc.Status() != nats.CONNECTED {
		return errors.New("sin conexión con NATS")
	}
	return nil
}
