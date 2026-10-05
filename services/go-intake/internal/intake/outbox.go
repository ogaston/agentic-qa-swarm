package intake

import (
	"context"
	"encoding/json"
	"os"
	"sync"
)

// Outbox es el EventPublisher de transicion: agrega una linea JSON por evento
// a un archivo (con fsync). Marcador de posicion hasta que C-45 decida el
// transporte de eventos; no es un broker.
type Outbox struct {
	mu   sync.Mutex
	path string
}

var _ EventPublisher = (*Outbox)(nil)

// NewOutbox devuelve un Outbox sobre path (se crea en el primer evento).
func NewOutbox(path string) *Outbox { return &Outbox{path: path} }

// Publish implementa EventPublisher.
func (o *Outbox) Publish(_ context.Context, ev Event) error {
	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	o.mu.Lock()
	defer o.mu.Unlock()
	f, err := os.OpenFile(o.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	if _, err := f.Write(line); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
