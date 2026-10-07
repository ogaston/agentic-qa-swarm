// Package outbox es el EventPublisher: una línea JSON por evento en WARM_OUTBOX_FILE, idempotente por event_id.
package outbox

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"sync"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
)

// File publica a un archivo JSONL.
type File struct {
	mu   sync.Mutex
	Path string
	seen map[string]bool
}

// Publish añade el evento si su event_id no estaba ya.
func (f *File) Publish(_ context.Context, e wm.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.seen == nil {
		f.seen = map[string]bool{}
		if fh, err := os.Open(f.Path); err == nil {
			sc := bufio.NewScanner(fh)
			sc.Buffer(make([]byte, 1<<20), 1<<20)
			for sc.Scan() {
				var p struct {
					ID string `json:"event_id"`
				}
				if json.Unmarshal(sc.Bytes(), &p) == nil {
					f.seen[p.ID] = true
				}
			}
			fh.Close()
		}
	}
	if f.seen[e.EventID] {
		return nil
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	fh, err := os.OpenFile(f.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer fh.Close()
	if _, err := fh.Write(append(raw, '\n')); err != nil {
		return err
	}
	if err := fh.Sync(); err != nil {
		return err
	}
	f.seen[e.EventID] = true
	return nil
}
