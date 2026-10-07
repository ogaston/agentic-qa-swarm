package outbox_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/adapters/outbox"
	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/core"
)

var at = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

func TestOutboxIdempotentByEventID(t *testing.T) {
	p := filepath.Join(t.TempDir(), "o.jsonl")
	o := outbox.New(p)
	e := core.ResetVerifiedEvent("r-1", "w", "tr-1", []string{"app-ready", "db-baseline", "cache-empty"}, at)
	for i := 0; i < 3; i++ {
		if err := o.Publish(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(p)
	if n := strings.Count(string(b), "\n"); n != 1 {
		t.Fatalf("%d líneas", n)
	}
}

func TestEventsShape(t *testing.T) {
	e := core.ResetVerifiedEvent("r-1", "w", "tr-1", []string{"a", "b", "c"}, at)
	b, _ := json.Marshal(e)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"event_id", "type", "version", "occurred_at", "trace_id", "data"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("falta %s", k)
		}
	}
	for _, c := range m["data"].(map[string]any)["checks"].([]any) {
		if c.(map[string]any)["ok"] != true {
			t.Fatal("check ok != true")
		}
	}
	td := core.TeardownVerifiedEvent("w", "teardown", "tr", at)
	if td.Data["verified"] != true || td.Data["mode"] != "teardown" {
		t.Fatal(td)
	}
}

// Vuelca eventos reales a $EVENTS_DUMP_DIR para validarlos con ajv (CA-5).
func TestEventsDump(t *testing.T) {
	d := os.Getenv("EVENTS_DUMP_DIR")
	if d == "" {
		t.Skip("EVENTS_DUMP_DIR no definido")
	}
	for name, e := range map[string]core.Event{
		"reset.verified.json":    core.ResetVerifiedEvent("r-1", "warm-1", "tr-r-1", []string{"app-ready", "db-baseline", "cache-empty"}, at),
		"teardown.verified.json": core.TeardownVerifiedEvent("warm-1", "rebuild", "tr-1", at),
	} {
		b, _ := json.Marshal(e)
		if err := os.WriteFile(filepath.Join(d, name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
