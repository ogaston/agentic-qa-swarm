package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/authz"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/audit"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/gen"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/policy"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/service"
)

// memAudit es un AuditLog en memoria (sin disco) que puede fallar.
type memAudit struct {
	fail    bool
	used    int
	entries []audit.Entry
}

func (m *memAudit) Append(e audit.Entry) (audit.Entry, error) {
	if m.fail {
		return audit.Entry{}, errors.New("disco lleno")
	}
	e.Hash = "h"
	m.entries = append(m.entries, e)
	return e, nil
}
func (m *memAudit) Query(string, int) []audit.Entry       { return m.entries }
func (m *memAudit) RunsAllowed(_ string, _ time.Time) int { return m.used }

// memStore es un policy.Store en memoria que cuenta las escrituras.
type memStore struct {
	latest map[string]policy.Version
	puts   int
}

func newStore() *memStore { return &memStore{latest: map[string]policy.Version{}} }

func (s *memStore) Get(_ context.Context, n string) (policy.Version, bool, error) {
	v, ok := s.latest[n]
	return v, ok, nil
}
func (s *memStore) Plan(_ context.Context, n string, v json.RawMessage) (int, bool, error) {
	if p, ok := s.latest[n]; ok {
		return p.Version + 1, true, nil
	}
	return 1, true, nil
}
func (s *memStore) Put(_ context.Context, n string, v json.RawMessage, a string) (int, bool, error) {
	s.puts++
	ver := s.latest[n].Version + 1
	s.latest[n] = policy.Version{Name: n, Version: ver, Value: v, Actor: a}
	return ver, true, nil
}

var now = func() time.Time { return time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC) }

func TestPBT_Invariant_AuditFailureNeverAllow(t *testing.T) {
	gen.AtLeastChecks(t, 500)
	allowed := 0
	rapid.Check(t, func(t *rapid.T) {
		in := gen.GateInput().Draw(t, "in")
		src := gen.WorkflowSource().Draw(t, "src")
		pol, _ := json.Marshal(src.Policy())
		mk := func(fail bool) *service.Service {
			st := newStore()
			st.latest[policy.Workflows] = policy.Version{Name: policy.Workflows, Version: 1, Value: pol}
			return service.New(service.Config{Policies: st, Audit: &memAudit{fail: fail, used: src.Used}, Now: now})
		}
		ok, _ := mk(false).Authorize(context.Background(), "actor", in)
		d, err := mk(true).Authorize(context.Background(), "actor", in)
		if d.Allow {
			t.Fatalf("Allow con auditoría caída: %+v", in)
		}
		if d.Reason == "" || err == nil {
			t.Fatalf("Deny sin razón o sin error: %+v, %v", d, err)
		}
		if ok.Allow {
			allowed++ // la propiedad no es vacía: con auditoría sana esta entrada se permitía
			if !errors.Is(err, service.ErrAuditFailed) {
				t.Fatalf("error sin ErrAuditFailed: %v", err)
			}
		}
	})
	if allowed == 0 {
		t.Fatal("ninguna entrada generada se permitió con auditoría sana: propiedad vacía")
	}
}

func TestPBT_PolicyRejectsInvalid(t *testing.T) {
	st, au := newStore(), &memAudit{}
	svc := service.New(service.Config{Policies: st, Audit: au, Now: now})
	// una versión previa válida por política, para comprobar que no cambia.
	for _, n := range []string{policy.Events, policy.WarmQuotas, policy.Workflows, policy.ConfirmRequired} {
		var raw json.RawMessage
		switch n {
		case policy.Events:
			raw = json.RawMessage(`{"enabled_events":["tag"]}`)
		case policy.WarmQuotas:
			raw = json.RawMessage(`{"max_runs_per_day":1,"rebuild_cadence_hours":1,"idle_scale_down_minutes":1,"housekeeping_grace_hours":1}`)
		case policy.Workflows:
			raw = json.RawMessage(`{"workflows":[]}`)
		default:
			raw = json.RawMessage(`{"required":true}`)
		}
		if _, err := svc.SetPolicy(context.Background(), "admin", n, raw); err != nil {
			t.Fatal(err)
		}
	}
	rapid.Check(t, func(t *rapid.T) {
		p := gen.InvalidPolicyValue().Draw(t, "invalid")
		before, _, _ := st.Get(context.Background(), p.Name)
		puts := st.puts
		ver, err := svc.SetPolicy(context.Background(), "admin", p.Name, p.Raw)
		if !errors.Is(err, policy.ErrInvalid) || ver != 0 {
			t.Fatalf("valor inválido (%s) no rechazado: ver=%d err=%v: %s", p.Why, ver, err, p.Raw)
		}
		after, _, _ := st.Get(context.Background(), p.Name)
		if st.puts != puts || after.Version != before.Version {
			t.Fatalf("se creó una versión con un valor inválido (%s): %d -> %d", p.Why, before.Version, after.Version)
		}
	})
}

// Ejemplo fijo de las dos propiedades.
func TestPBT_Examples_AuditAndPolicy(t *testing.T) {
	st := newStore()
	svc := service.New(service.Config{Policies: st, Audit: &memAudit{fail: true}, Now: now})
	in := authz.GateInput{RunID: "r", From: authz.StateReporting, To: authz.StateDone}
	if d, err := svc.Authorize(context.Background(), "a", in); d.Allow || !errors.Is(err, service.ErrAuditFailed) {
		t.Errorf("auditoría caída: %+v %v", d, err)
	}
	if _, err := svc.SetPolicy(context.Background(), "a", policy.WarmQuotas, json.RawMessage(`{"max_runs_per_day":0,"rebuild_cadence_hours":1,"idle_scale_down_minutes":1,"housekeeping_grace_hours":1}`)); !errors.Is(err, policy.ErrInvalid) || st.puts != 0 {
		t.Errorf("cuota 0: %v puts=%d", err, st.puts)
	}
}
