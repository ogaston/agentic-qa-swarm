package obs

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

// keys cuenta las claves de primer nivel de una línea JSON (detecta duplicados).
func keys(t *testing.T, line string) map[string]int {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(line))
	if tok, _ := dec.Token(); tok != json.Delim('{') {
		t.Fatalf("no es un objeto: %s", line)
	}
	out := map[string]int{}
	for dec.More() {
		k, _ := dec.Token()
		out[k.(string)]++
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestLogLinesHaveSingleTraceIDWithRunTrace(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(&buf, "go-run-controller", 0)
	gate := &runctl.FakeGate{} // deniega: genera logs de gate y de handoff
	st := runctl.NewMemStore()
	k, _ := runctl.New(runctl.Config{Gate: gate, Store: st, Publisher: &runctl.FakePublisher{}, Warm: &runctl.FakeWarm{Fact: runctl.True},
		Alerter: &runctl.FakeAlerter{}, Phases: &runctl.FakePhases{}, Log: log})
	_ = k.Apply(t.Context(), runctl.Event{Type: runctl.EvRunConfirmed, EventID: "e", TraceID: "trace-de-la-corrida", RunID: "r-1", ConfirmedBy: "u", Flows: []string{"f"}})
	_ = k.Apply(t.Context(), runctl.Event{Type: runctl.EvRehearsalPassed, EventID: "e2", RunID: "r-1"})
	_ = k.Drive(t.Context(), "r-1")
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("pocos logs: %q", buf.String())
	}
	for _, l := range lines {
		if n := keys(t, l)["trace_id"]; n != 1 {
			t.Errorf("trace_id aparece %d veces: %s", n, l)
		}
		var m map[string]any
		_ = json.Unmarshal([]byte(l), &m)
		if m["trace_id"] != "trace-de-la-corrida" {
			t.Errorf("trace_id = %v: %s", m["trace_id"], l)
		}
	}
}
