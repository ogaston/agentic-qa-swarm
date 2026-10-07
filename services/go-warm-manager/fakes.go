package warmmanager

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Fakes en memoria de todos los puertos (pruebas y modo WARM_KUBE=fake).

// MemState es un StateStore en memoria.
type MemState struct {
	mu sync.Mutex
	S  WarmState
}

func (m *MemState) Get(context.Context) (WarmState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.S, nil
}
func (m *MemState) Put(_ context.Context, s WarmState) error {
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
	Created []Manifest
	Outcome func(n int) JobPhase
	status  map[string]JobPhase
}

func (f *FakeJobs) Create(_ context.Context, m Manifest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.status == nil {
		f.status = map[string]JobPhase{}
	}
	name := m["metadata"].(map[string]any)["name"].(string)
	p := JobSucceeded
	if f.Outcome != nil {
		p = f.Outcome(len(f.Created))
	}
	f.Created = append(f.Created, m)
	f.status[name] = p
	return nil
}

func (f *FakeJobs) Status(_ context.Context, name string) (JobPhase, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
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
	Events []Event
}

func (p *MemPublisher) Publish(_ context.Context, e Event) error {
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
