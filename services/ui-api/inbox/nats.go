package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// EventsStreamName es el stream JetStream de los eventos (U8-T01, nats-init).
const EventsStreamName = "AQS_EVENTS"

// durableInbox es el consumidor durable de notify.created de ui-api (U8-T02).
const durableInbox = "ui-api-inbox"

const (
	subjectPrefix   = "aqs.events."
	subjectNotify   = subjectPrefix + "notify.created"
	consumerAckWait = 30 * time.Second
	retryBackoff    = time.Second
)

// NATSPublisher publica run.confirmed en JetStream (C-45, U8-T02). Nats-Msg-Id = event_id: el stream
// descarta un reenvío con el mismo event_id dentro de su ventana de deduplicación (2 min). Tras la
// ventana, el republicador de Publisher ya no reenvía (publish_pending se cierra con MarkPublished).
type NATSPublisher struct {
	js jetstream.JetStream
}

var _ EventPublisher = (*NATSPublisher)(nil)

// NewNATSPublisher devuelve el publicador sobre js.
func NewNATSPublisher(js jetstream.JetStream) *NATSPublisher { return &NATSPublisher{js: js} }

// Publish implementa EventPublisher. Un error deja la confirmación como publish_pending.
func (p *NATSPublisher) Publish(ctx context.Context, ev RunConfirmed) error {
	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if _, err := p.js.Publish(ctx, subjectPrefix+ev.Type, line, jetstream.WithMsgID(ev.EventID)); err != nil {
		return fmt.Errorf("publicando %s en NATS: %w", ev.Type, err)
	}
	return nil
}

// NATSSubscriber entrega notify.created desde JetStream (C-45, U8-T02). Dos fases:
//
//  1. Replay (una vez por proceso): un consumidor efímero lee el stream desde el principio y entrega
//     cada evento válido a handle. La proyección del inbox vive en memoria y se reconstruye así en cada
//     arranque, igual que el FileSubscriber relee el archivo desde 0.
//  2. Durable ui-api-inbox (AckExplicit): entrega lo que aún no tiene ack. Cada mensaje se hace Ack solo
//     después de handle; un evento que no cumple notify.created.schema.json se hace Term y no se aplica.
//
// La idempotencia por event_id la aplica el Store, así que el replay y el durable pueden solaparse.
type NATSSubscriber struct {
	js       jetstream.JetStream
	log      *slog.Logger
	replayed bool
}

var _ EventSubscriber = (*NATSSubscriber)(nil)

// NewNATSSubscriber devuelve el suscriptor sobre js. log puede ser nil.
func NewNATSSubscriber(js jetstream.JetStream, log *slog.Logger) *NATSSubscriber {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &NATSSubscriber{js: js, log: log}
}

// Run implementa EventSubscriber. Bloquea hasta que ctx termina. Si NATS no responde o el stream aún
// no existe (nats-init reinicia al arrancar el clúster), reintenta con espera fija; no termina.
func (s *NATSSubscriber) Run(ctx context.Context, handle func(NotifyCreated)) error {
	for {
		err := s.session(ctx, handle)
		if ctx.Err() != nil {
			return nil
		}
		s.log.WarnContext(ctx, "suscripción a NATS reintentando", "error", err.Error())
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(retryBackoff):
		}
	}
}

// session hace una pasada completa: stream, replay (si falta) y durable hasta que ctx o un error.
func (s *NATSSubscriber) session(ctx context.Context, handle func(NotifyCreated)) error {
	stream, err := s.js.Stream(ctx, EventsStreamName)
	if err != nil {
		return err
	}
	if !s.replayed {
		if err := s.replay(ctx, stream, handle); err != nil {
			return err
		}
		s.replayed = true
	}
	cons, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       durableInbox,
		FilterSubject: subjectNotify,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       consumerAckWait,
		MaxDeliver:    -1,
	})
	if err != nil {
		return err
	}
	cc, err := cons.Consume(func(msg jetstream.Msg) {
		ev, perr := ParseNotifyCreated(msg.Data())
		if perr != nil {
			// Sin contenido en el log: el error de ParseNotifyCreated no cita valores del evento.
			s.log.WarnContext(ctx, "evento notify.created descartado (no cumple el contrato)", "error", perr.Error())
			_ = msg.Term()
			return
		}
		handle(ev)
		_ = msg.Ack()
	})
	if err != nil {
		return err
	}
	defer cc.Stop()
	<-ctx.Done()
	return nil
}

// replay entrega al handle todo el histórico de notify.created del stream, en orden, hasta alcanzar
// NumPending == 0 del consumidor efímero. Los inválidos se omiten (el durable los hará Term).
func (s *NATSSubscriber) replay(ctx context.Context, stream jetstream.Stream, handle func(NotifyCreated)) error {
	cons, err := stream.OrderedConsumer(ctx, jetstream.OrderedConsumerConfig{
		FilterSubjects: []string{subjectNotify},
		DeliverPolicy:  jetstream.DeliverAllPolicy,
	})
	if err != nil {
		return err
	}
	for {
		info, err := cons.Info(ctx)
		if err != nil {
			return err
		}
		if info.NumPending == 0 {
			return nil
		}
		msgs, err := cons.Fetch(256, jetstream.FetchMaxWait(time.Second))
		if err != nil {
			return err
		}
		for m := range msgs.Messages() {
			ev, perr := ParseNotifyCreated(m.Data())
			if perr != nil {
				s.log.WarnContext(ctx, "replay: evento notify.created descartado", "error", perr.Error())
				continue
			}
			handle(ev)
		}
		if err := msgs.Error(); err != nil && !errors.Is(err, nats.ErrTimeout) {
			return err
		}
	}
}

// NATSReady es el chequeo de /readyz del transporte: falla mientras la conexión no esté CONNECTED.
func NATSReady(nc *nats.Conn) error {
	if nc == nil || nc.Status() != nats.CONNECTED {
		return errors.New("sin conexión con NATS")
	}
	return nil
}

// ConnectNATS abre la conexión a url con reconexión acotada (espera fija de 2 s con jitter) y sin
// fallar si NATS aún no responde (RetryOnFailedConnect).
func ConnectNATS(url string) (*nats.Conn, error) {
	if url == "" {
		return nil, errors.New("NATS_URL es obligatorio con EVENTS_TRANSPORT=nats")
	}
	return nats.Connect(url,
		nats.Name("ui-api"),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
		nats.ReconnectJitter(200*time.Millisecond, time.Second),
		nats.Timeout(5*time.Second),
	)
}
