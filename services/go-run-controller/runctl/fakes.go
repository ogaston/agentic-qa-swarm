package runctl

import (
	"context"
	"sort"
	"sync"
)

// FakeGate es un GateClient programable. Decide es la lógica; Calls registra cada petición.
type FakeGate struct {
	mu     sync.Mutex
	Decide func(GateRequest) (GateDecision, error)
	Calls  []GateRequest
}

// Authorize implementa GateClient. Sin Decide, deniega.
func (g *FakeGate) Authorize(_ context.Context, req GateRequest) (GateDecision, error) {
	g.mu.Lock()
	g.Calls = append(g.Calls, req)
	f := g.Decide
	g.mu.Unlock()
	if f == nil {
		return GateDecision{Reason: "sin regla", AuditRef: "fake"}, nil
	}
	return f(req)
}

// AllowAll devuelve un FakeGate que permite todo.
func AllowAll() *FakeGate {
	return &FakeGate{Decide: func(GateRequest) (GateDecision, error) {
		return GateDecision{Allow: true, Reason: "permitido", AuditRef: "fake"}, nil
	}}
}

// MemStore es un RunStore en memoria.
type MemStore struct {
	mu   sync.Mutex
	runs map[string]Run
}

// NewMemStore crea un MemStore vacío.
func NewMemStore() *MemStore { return &MemStore{runs: map[string]Run{}} }

// Get implementa RunStore.
func (s *MemStore) Get(id string) (Run, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	return r.clone(), ok
}

// List implementa RunStore (ordenado por id).
func (s *MemStore) List() []Run {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Run, 0, len(s.runs))
	for _, r := range s.runs {
		out = append(out, r.clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Save implementa RunStore.
func (s *MemStore) Save(r Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[r.ID] = r.clone()
	return nil
}

// FakeWarm es un WarmStateReader fijo.
type FakeWarm struct {
	Fact Fact
	Err  error
}

// ResetVerified implementa WarmStateReader.
func (w *FakeWarm) ResetVerified(context.Context) (Fact, error) { return w.Fact, w.Err }

// FakeAlerter registra los handoffs.
type FakeAlerter struct {
	mu    sync.Mutex
	Calls []string // "run/phase"
}

// Handoff implementa Alerter.
func (a *FakeAlerter) Handoff(_ context.Context, runID, phase, _ string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Calls = append(a.Calls, runID+"/"+phase)
}

// FakePublisher registra los eventos publicados.
type FakePublisher struct {
	mu     sync.Mutex
	Events []OutEvent
	Err    error
}

// Publish implementa EventPublisher.
func (p *FakePublisher) Publish(_ context.Context, ev OutEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.Err != nil {
		return p.Err
	}
	p.Events = append(p.Events, ev)
	return nil
}

// FakePhases es un PhaseLauncher: FailFirst[fase] = cuántos lanzamientos fallan antes de tener éxito.
type FakePhases struct {
	mu        sync.Mutex
	FailFirst map[string]int
	Launches  map[string]int
}

// Launch implementa PhaseLauncher. La fase report devuelve una URI de evidencia.
func (f *FakePhases) Launch(_ context.Context, phase string, run Run) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Launches == nil {
		f.Launches = map[string]int{}
	}
	f.Launches[phase]++
	if f.Launches[phase] <= f.FailFirst[phase] {
		return nil, errFake
	}
	if phase == PhaseReport {
		return []string{"s3://evidence/" + run.ID + "/logs.tgz"}, nil
	}
	return nil, nil
}

type fakeErr struct{}

func (fakeErr) Error() string { return "fallo simulado de la fase" }

var errFake error = fakeErr{}
