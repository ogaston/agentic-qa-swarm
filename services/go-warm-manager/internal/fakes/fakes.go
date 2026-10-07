// Package fakes: dobles en memoria de los puertos de go-warm-manager (solo pruebas).
package fakes

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
)

// Fakes en memoria de todos los puertos (pruebas y modo WARM_KUBE=fake).

// MemState es un StateStore en memoria.
type MemState struct {
	mu sync.Mutex
	S  wm.WarmState
}

func (m *MemState) Get(context.Context) (wm.WarmState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.S, nil
}
func (m *MemState) CompareAndSwap(_ context.Context, expect, next wm.WarmState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.S != expect {
		return wm.ErrStateConflict
	}
	m.S = next
	return nil
}
func (m *MemState) Put(_ context.Context, s wm.WarmState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.S = s
	return nil
}

// FakeProbe devuelve Err en cada Check (o nil).
type FakeProbe struct {
	mu  sync.Mutex
	Err error
}

func (f *FakeProbe) Check(context.Context) error { f.mu.Lock(); defer f.mu.Unlock(); return f.Err }
func (f *FakeProbe) Set(err error)               { f.mu.Lock(); f.Err = err; f.mu.Unlock() }

// FakeRuntime cuenta los ScaleUp.
type FakeRuntime struct {
	mu    sync.Mutex
	Calls int
	OnUp  func()
}

func (f *FakeRuntime) ScaleUp(context.Context) error {
	f.mu.Lock()
	f.Calls++
	cb := f.OnUp
	f.mu.Unlock()
	if cb != nil {
		cb()
	}
	return nil
}

// FakeJobs crea Jobs que terminan según Outcome(n) (n = Jobs ya creados antes de este).
type FakeJobs struct {
	mu      sync.Mutex
	Created []wm.Manifest
	Outcome func(n int) wm.JobPhase
	// StatusErr decide el error de la k-esima consulta de Status (1-based); CreateErr el de la k-esima creacion.
	StatusErr func(call int) error
	CreateErr func(call int) error
	// Latency simula una API lenta en Create (amplia las ventanas de carrera).
	Latency time.Duration
	status  map[string]wm.JobPhase
	sCalls  int
	cCalls  int
}

// Count devuelve los Jobs creados.
func (f *FakeJobs) Count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.Created) }

func (f *FakeJobs) Create(_ context.Context, m wm.Manifest) error {
	if f.Latency > 0 {
		time.Sleep(f.Latency)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cCalls++
	if f.CreateErr != nil {
		if err := f.CreateErr(f.cCalls); err != nil {
			return err
		}
	}
	if f.status == nil {
		f.status = map[string]wm.JobPhase{}
	}
	name := m["metadata"].(map[string]any)["name"].(string)
	p := wm.JobSucceeded
	if f.Outcome != nil {
		p = f.Outcome(len(f.Created))
	}
	f.Created = append(f.Created, m)
	f.status[name] = p
	return nil
}

func (f *FakeJobs) Status(_ context.Context, name string) (wm.JobPhase, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sCalls++
	if f.StatusErr != nil {
		if err := f.StatusErr(f.sCalls); err != nil {
			return "", "", err
		}
	}
	p, ok := f.status[name]
	if !ok {
		return "", "", errors.New("job inexistente")
	}
	return p, "simulado", nil
}

// FakeProber responde con un mapa ruta -> (status, cuerpo).
type FakeProber struct {
	Base   string
	Routes map[string]FakeResponse
}

// FakeResponse es una respuesta del FakeProber.
type FakeResponse struct {
	Status int
	Body   string
}

func (f *FakeProber) BaseURL() string { return f.Base }
func (f *FakeProber) Get(_ context.Context, p string) (int, []byte, error) {
	r, ok := f.Routes[p]
	if !ok {
		return 404, nil, nil
	}
	return r.Status, []byte(r.Body), nil
}

// MemObjects es un ObjectStore en memoria.
type MemObjects struct {
	mu sync.Mutex
	M  map[string][]byte
}

func (m *MemObjects) Put(_ context.Context, key string, data []byte) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.M == nil {
		m.M = map[string][]byte{}
	}
	m.M["mem://"+key] = append([]byte(nil), data...)
	return "mem://" + key, nil
}

func (m *MemObjects) Get(_ context.Context, uri string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.M[uri]
	if !ok {
		return nil, errors.New("no existe")
	}
	return b, nil
}

// FakeAlerter registra los handoffs.
type FakeAlerter struct {
	mu    sync.Mutex
	Calls []string
}

func (f *FakeAlerter) Handoff(_ context.Context, phase, runID, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, phase+"/"+runID+"/"+reason)
	return nil
}

// MemPublisher acumula eventos.
type MemPublisher struct {
	mu     sync.Mutex
	Events []wm.Event
}

// Snapshot devuelve una copia de los eventos publicados.
func (p *MemPublisher) Snapshot() []wm.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]wm.Event(nil), p.Events...)
}

// SyncBuffer es un bytes.Buffer seguro para uso concurrente (logs en pruebas).
type SyncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *SyncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

// String devuelve lo escrito.
func (s *SyncBuffer) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func (p *MemPublisher) Publish(_ context.Context, e wm.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Events = append(p.Events, e)
	return nil
}

// FakeClock avanza solo con Sleep.
type FakeClock struct {
	mu sync.Mutex
	T  time.Time
}

func (c *FakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.T }
func (c *FakeClock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	c.T = c.T.Add(d)
	c.mu.Unlock()
	return nil
}
