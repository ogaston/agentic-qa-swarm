package outbox_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/adapters/outbox"
)

func TestOutboxIdempotentAcrossRestarts(t *testing.T) {
	p := filepath.Join(t.TempDir(), "o.jsonl")
	ev := wm.Event{EventID: wm.EventID("x"), Type: "warm.ready", Version: 1, OccurredAt: time.Now().UTC().Format(time.RFC3339), TraceID: "t",
		Data: map[string]any{"warm_id": "w", "state": "ready", "baseline_version": "b"}}
	for i := 0; i < 2; i++ { // nueva instancia = reinicio
		if err := (&outbox.File{Path: p}).Publish(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(p)
	if n := strings.Count(string(b), "\n"); n != 1 {
		t.Fatalf("lineas=%d", n)
	}
}
