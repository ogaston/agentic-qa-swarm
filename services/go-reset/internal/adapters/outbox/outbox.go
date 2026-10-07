// Package outbox es el EventPublisher JSONL idempotente por event_id.
package outbox

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"sync"

	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/core"
)

// Outbox añade eventos a un archivo JSONL.
type Outbox struct {
	mu   sync.Mutex
	path string
}

func New(path string) *Outbox { return &Outbox{path: path} }

func (o *Outbox) Publish(_ context.Context, e core.Event) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if ids, err := o.ids(); err != nil {
		return err
	} else if ids[e.EventID] {
		return nil
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(o.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func (o *Outbox) ids() (map[string]bool, error) {
	ids := map[string]bool{}
	f, err := os.Open(o.path)
	if os.IsNotExist(err) {
		return ids, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var h struct {
			ID string `json:"event_id"`
		}
		if json.Unmarshal(line, &h) == nil {
			ids[h.ID] = true
		}
	}
	return ids, sc.Err()
}
