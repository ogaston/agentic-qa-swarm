package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

// Transporte NATS JetStream (C-45, U8-T02). Los nombres y el contrato siguen a U8-T01 (stream
// AQS_EVENTS, subjects aqs.events.<tipo>, dedupe por Nats-Msg-Id de 2 min).
const (
	natsStream           = "AQS_EVENTS"
	natsSubjectPrefix    = "aqs.events."
	natsDurable          = "go-run-controller-confirmed"
	natsSubjectConfirmed = natsSubjectPrefix + "run.confirmed"
	natsDefaultAckWait   = 30 * time.Second
	natsFetchBatch       = 16
	natsFetchWait        = 200 * time.Millisecond
)

// ConnectNATS abre la conexión a url con reconexión acotada (espera fija de 2 s con jitter) y sin
// fallar si NATS aún no responde (RetryOnFailedConnect): el consumidor se crea cuando el stream existe.
func ConnectNATS(url string) (*nats.Conn, error) {
	if url == "" {
		return nil, errors.New("NATS_URL es obligatorio con EVENTS_TRANSPORT=nats")
	}
	return nats.Connect(url,
		nats.Name("go-run-controller"),
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

// uuidRe valida el formato uuid de event_id (el esquema lo declara format: uuid).
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// runConfirmedWire es la forma exacta de run.confirmed v1 (contracts/events/run.confirmed.schema.json).
type runConfirmedWire struct {
	EventID    string `json:"event_id"`
	Type       string `json:"type"`
	Version    int    `json:"version"`
	OccurredAt string `json:"occurred_at"`
	TraceID    string `json:"trace_id"`
	Data       struct {
		RunID          string          `json:"run_id"`
		NotificationID string          `json:"notification_id"`
		ConfirmedBy    string          `json:"confirmed_by"`
		Flows          []string        `json:"flows"`
		Artifact       json.RawMessage `json:"artifact,omitempty"`
		Warm           json.RawMessage `json:"warm,omitempty"`
	} `json:"data"`
}

// parseArtifact valida el artefacto opcional de run.confirmed (U11-T01) con las reglas de
// notify.created: kind de un enum, ref no vacío y sin campos extra. Ausente (vacío) = nil, sin error.
// Un null explícito se rechaza: el esquema exige un objeto.
func parseArtifact(raw json.RawMessage) (*runctl.Artifact, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if string(raw) == "null" {
		return nil, errors.New("run.confirmed: artifact no puede ser null")
	}
	var a struct {
		Kind string `json:"kind"`
		Ref  string `json:"ref"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return nil, errors.New("run.confirmed: artifact malformado o con campos desconocidos")
	}
	if a.Kind != "build-from-repo" && a.Kind != "published-image" {
		return nil, errors.New("run.confirmed: artifact.kind desconocido")
	}
	if a.Ref == "" {
		return nil, errors.New("run.confirmed: artifact.ref vacío")
	}
	return &runctl.Artifact{Kind: a.Kind, Ref: a.Ref}, nil
}

// parseWarm valida el destino warm opcional de run.confirmed (U12) con las reglas del esquema:
// warm_id y namespace strings no vacíos y sin campos extra. Ausente (vacío) = nil, sin error.
// Un null explícito se rechaza: el esquema exige un objeto.
func parseWarm(raw json.RawMessage) (*runctl.WarmTarget, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if string(raw) == "null" {
		return nil, errors.New("run.confirmed: warm no puede ser null")
	}
	var w struct {
		WarmID    string `json:"warm_id"`
		Namespace string `json:"namespace"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&w); err != nil {
		return nil, errors.New("run.confirmed: warm malformado o con campos desconocidos")
	}
	if w.WarmID == "" || w.Namespace == "" {
		return nil, errors.New("run.confirmed: warm_id o namespace vacío")
	}
	return &runctl.WarmTarget{WarmID: w.WarmID, Namespace: w.Namespace}, nil
}

// ParseRunConfirmed valida una línea run.confirmed contra el esquema: campos exactos (sin extras),
// tipos, requeridos y no vacíos. Es más estricto que ParseEvent (que el transporte de archivo sigue
// usando sin cambios) porque aquí no hay línea de archivo que tolerar.
func ParseRunConfirmed(line []byte) (runctl.Event, error) {
	var w runConfirmedWire
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&w); err != nil {
		return runctl.Event{}, errors.New("run.confirmed: json inválido o con campos desconocidos")
	}
	if _, err := dec.Token(); err == nil {
		return runctl.Event{}, errors.New("run.confirmed: datos tras el objeto")
	}
	if _, err := time.Parse(time.RFC3339, w.OccurredAt); err != nil {
		return runctl.Event{}, errors.New("run.confirmed: occurred_at no es date-time")
	}
	switch {
	case !uuidRe.MatchString(w.EventID):
		return runctl.Event{}, errors.New("run.confirmed: event_id no es UUID")
	case w.Type != runctl.EvRunConfirmed:
		return runctl.Event{}, errors.New("run.confirmed: type distinto")
	case w.Version != 1:
		return runctl.Event{}, errors.New("run.confirmed: version debe ser 1")
	case w.TraceID == "":
		return runctl.Event{}, errors.New("run.confirmed: falta trace_id")
	case w.Data.RunID == "" || w.Data.NotificationID == "" || w.Data.ConfirmedBy == "":
		return runctl.Event{}, errors.New("run.confirmed: falta run_id, notification_id o confirmed_by")
	case len(w.Data.Flows) == 0:
		return runctl.Event{}, errors.New("run.confirmed: flows vacío")
	}
	for _, f := range w.Data.Flows {
		if f == "" {
			return runctl.Event{}, errors.New("run.confirmed: flows con elemento vacío")
		}
	}
	art, err := parseArtifact(w.Data.Artifact)
	if err != nil {
		return runctl.Event{}, err
	}
	warm, err := parseWarm(w.Data.Warm)
	if err != nil {
		return runctl.Event{}, err
	}
	return runctl.Event{Type: w.Type, EventID: w.EventID, TraceID: w.TraceID, RunID: w.Data.RunID,
		ConfirmedBy: w.Data.ConfirmedBy, Flows: w.Data.Flows, Artifact: art, Warm: warm}, nil
}

// NATSSource entrega run.confirmed desde el consumidor durable go-run-controller-confirmed (AckExplicit).
// Ack solo confirma el mensaje que el controlador aplicó; lo no confirmado se reentrega tras AckWait.
// Un mensaje inválido se hace Term y no llega al controlador.
type NATSSource struct {
	JS      jetstream.JetStream
	AckWait time.Duration // 0: 30 s
	Log     *slog.Logger  // nil: sin log

	mu        sync.Mutex
	cons      jetstream.Consumer
	pending   map[int64]jetstream.Msg
	discarded int
}

var _ runctl.EventSource = (*NATSSource)(nil)

// Discarded cuenta los mensajes inválidos descartados (Term).
func (n *NATSSource) Discarded() int { n.mu.Lock(); defer n.mu.Unlock(); return n.discarded }

func (n *NATSSource) logger() *slog.Logger {
	if n.Log == nil {
		return slog.New(slog.DiscardHandler)
	}
	return n.Log
}

// ensureConsumer crea o actualiza el durable. Falla mientras el stream no exista (nats-init aún corre).
func (n *NATSSource) ensureConsumer(ctx context.Context) error {
	if n.cons != nil {
		return nil
	}
	ackWait := n.AckWait
	if ackWait <= 0 {
		ackWait = natsDefaultAckWait
	}
	st, err := n.JS.Stream(ctx, natsStream)
	if err != nil {
		return err
	}
	c, err := st.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       natsDurable,
		FilterSubject: natsSubjectConfirmed,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       ackWait,
		MaxDeliver:    -1,
	})
	if err != nil {
		return err
	}
	n.cons = c
	n.pending = map[int64]jetstream.Msg{}
	return nil
}

// Poll implementa EventSource: trae un lote (espera acotada) y devuelve los run.confirmed válidos.
func (n *NATSSource) Poll(ctx context.Context) ([]runctl.Event, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if err := n.ensureConsumer(ctx); err != nil {
		return nil, err
	}
	batch, err := n.cons.Fetch(natsFetchBatch, jetstream.FetchMaxWait(natsFetchWait))
	if err != nil {
		return nil, err
	}
	var out []runctl.Event
	for m := range batch.Messages() {
		ev, perr := ParseRunConfirmed(m.Data())
		if perr != nil {
			n.discarded++
			n.logger().WarnContext(ctx, "evento descartado en el consumidor", "error", perr.Error())
			_ = m.Term()
			continue
		}
		meta, err := m.Metadata()
		if err != nil {
			return out, err
		}
		ev.Pos = int64(meta.Sequence.Stream)
		n.pending[ev.Pos] = m
		out = append(out, ev)
	}
	if err := batch.Error(); err != nil && !errors.Is(err, nats.ErrTimeout) {
		return out, err
	}
	return out, nil
}

// Ack confirma que ev se aplicó (Pos es la secuencia del stream que devolvió Poll).
func (n *NATSSource) Ack(ev runctl.Event) {
	n.mu.Lock()
	defer n.mu.Unlock()
	m, ok := n.pending[ev.Pos]
	if !ok {
		return
	}
	delete(n.pending, ev.Pos)
	_ = m.Ack()
}

// NATSOutbox publica run.done en JetStream (C-45, U8-T02). Nats-Msg-Id = event_id determinista de
// (tipo, run_id): un reenvío dentro de la ventana de 2 min lo descarta el stream.
type NATSOutbox struct {
	JS  jetstream.JetStream
	Now func() time.Time
}

var _ runctl.EventPublisher = (*NATSOutbox)(nil)

// Publish implementa EventPublisher. Solo run.done: el controlador no publica otros tipos por NATS.
func (o *NATSOutbox) Publish(ctx context.Context, ev runctl.OutEvent) error {
	if ev.Type != "run.done" {
		return errors.New("el publicador NATS del controlador solo publica run.done")
	}
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	id, line, err := runDoneLine(ev, now())
	if err != nil {
		return err
	}
	if _, err := o.JS.Publish(ctx, natsSubjectPrefix+ev.Type, line, jetstream.WithMsgID(id)); err != nil {
		return fmt.Errorf("publicando run.done en NATS: %w", err)
	}
	return nil
}
