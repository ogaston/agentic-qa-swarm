package sessions_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/adapters/sessions"
	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/core"
)

func TestSessionStoreLastWinsAndSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	s, _ := sessions.New(dir)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	_ = s.Save(ctx, core.Session{RunID: "r-1", Status: core.SessionActive, LastActivity: now, Plan: json.RawMessage(`{"a":1}`), State: "executing"})
	_ = s.Save(ctx, core.Session{RunID: "r-1", Status: core.SessionIncomplete, LastActivity: now, Plan: json.RawMessage(`{"a":1}`), State: "executing", WarmState: "ready"})
	s2, _ := sessions.New(dir)
	l, err := s2.List(ctx)
	if err != nil || len(l) != 1 || l[0].Status != core.SessionIncomplete || string(l[0].Plan) != `{"a":1}` || l[0].State != "executing" {
		t.Fatalf("%+v %v", l, err)
	}
}
