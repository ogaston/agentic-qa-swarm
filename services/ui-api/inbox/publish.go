package inbox

import (
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // UUIDv5 (RFC 4122) exige SHA-1; no es un uso criptografico.
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

// RunConfirmed es el evento run.confirmed v1 (contracts/events/run.confirmed.schema.json).
type RunConfirmed struct {
	EventID    string           `json:"event_id"`
	Type       string           `json:"type"`
	Version    int              `json:"version"`
	OccurredAt time.Time        `json:"occurred_at"`
	TraceID    string           `json:"trace_id"`
	Data       RunConfirmedData `json:"data"`
}

// RunConfirmedData es el cuerpo de run.confirmed.
type RunConfirmedData struct {
	RunID          string   `json:"run_id"`
	NotificationID string   `json:"notification_id"`
	ConfirmedBy    string   `json:"confirmed_by"`
	Flows          []string `json:"flows"`
}

// eventNamespace es el espacio de nombres UUIDv5 de los event_id de run.confirmed.
var eventNamespace = [16]byte{0x7a, 0x1c, 0x52, 0x0e, 0x9b, 0x3d, 0x4f, 0x6a, 0x8e, 0x21, 0xc4, 0x5d, 0x90, 0x0b, 0x6e, 0x17}

// EventID es el UUIDv5 determinista de run.confirmed para runID (misma entrada, mismo UUID).
func EventID(runID string) string {
	h := sha1.New() //nolint:gosec
	h.Write(eventNamespace[:])
	h.Write([]byte(runID))
	b := h.Sum(nil)[:16]
	b[6] = (b[6] & 0x0f) | 0x50
	b[8] = (b[8] & 0x3f) | 0x80
	x := hex.EncodeToString(b)
	return x[0:8] + "-" + x[8:12] + "-" + x[12:16] + "-" + x[16:20] + "-" + x[20:]
}

// NewRunConfirmed arma el evento de una confirmacion. Sin trace_id (recibos antiguos) deriva
// uno estable de run_id para cumplir minLength 1.
func NewRunConfirmed(c Confirmed) RunConfirmed {
	trace := c.TraceID
	if trace == "" {
		sum := sha256.Sum256([]byte(c.RunID))
		trace = hex.EncodeToString(sum[:16])
	}
	return RunConfirmed{
		EventID: EventID(c.RunID), Type: "run.confirmed", Version: 1,
		OccurredAt: c.ConfirmedAt.UTC(), TraceID: trace,
		Data: RunConfirmedData{RunID: c.RunID, NotificationID: c.NotificationID, ConfirmedBy: c.ConfirmedBy, Flows: c.Flows},
	}
}

// EventPublisher publica run.confirmed. Debe ser idempotente por event_id.
type EventPublisher interface {
	Publish(ctx context.Context, ev RunConfirmed) error
}

// Outbox es el EventPublisher de transicion (C-45): una linea JSON por evento en un archivo
// (fsync). Omite un event_id ya presente, de modo que reintentos tras un fallo parcial no duplican.
type Outbox struct {
	mu   sync.Mutex
	path string
}

var _ EventPublisher = (*Outbox)(nil)

// NewOutbox devuelve un Outbox sobre path (se crea en el primer evento).
func NewOutbox(path string) *Outbox { return &Outbox{path: path} }

// Publish implementa EventPublisher.
func (o *Outbox) Publish(_ context.Context, ev RunConfirmed) error {
	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if b, err := os.ReadFile(o.path); err == nil {
		if bytes.Contains(b, []byte(`"event_id":"`+ev.EventID+`"`)) {
			return nil
		}
		if len(b) > 0 && b[len(b)-1] != '\n' {
			line = append([]byte("\n"), line...)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return appendFile(o.path, append(line, '\n'))
}

// Publisher publica run.confirmed de las confirmaciones del Store y marca lo publicado.
type Publisher struct {
	store *Store
	pub   EventPublisher
	log   *slog.Logger
	mu    sync.Mutex
}

// NewPublisher crea el Publisher. log puede ser nil.
func NewPublisher(store *Store, pub EventPublisher, log *slog.Logger) *Publisher {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Publisher{store: store, pub: pub, log: log}
}

// Publish publica el run.confirmed de la notificacion id (ya confirmada). Si ya se publico, no hace nada.
func (p *Publisher) Publish(ctx context.Context, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.store.IsPublished(id) {
		return nil
	}
	c, ok := p.store.Confirmation(id)
	if !ok {
		return ErrNotFound
	}
	if err := p.pub.Publish(ctx, NewRunConfirmed(c)); err != nil {
		return fmt.Errorf("publicando run.confirmed: %w", err)
	}
	return p.store.MarkPublished(id)
}

// RepublishPending reintenta todas las publicaciones pendientes; devuelve cuantas envio.
func (p *Publisher) RepublishPending(ctx context.Context) int {
	n := 0
	for _, c := range p.store.PublishPending() {
		if err := p.Publish(ctx, c.NotificationID); err != nil {
			p.log.WarnContext(ctx, "republicacion de run.confirmed fallo", "notification_id", c.NotificationID, "error", err.Error())
			continue
		}
		n++
	}
	return n
}

// Run republica al arrancar y luego cada interval hasta que ctx termine.
func (p *Publisher) Run(ctx context.Context, interval time.Duration) {
	p.RepublishPending(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.RepublishPending(ctx)
		}
	}
}
