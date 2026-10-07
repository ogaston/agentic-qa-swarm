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
	Err   error // error de ScaleUp
}

func (f *FakeRuntime) ScaleUp(context.Context) error {
	f.mu.Lock()
	f.Calls++
	cb, err := f.OnUp, f.Err
	f.mu.Unlock()
	if cb != nil {
		cb()
	}
	return err
}

// FakeDeployer registra los parches de imagen y decide cuando el rollout completa.
type FakeDeployer struct {
	mu sync.Mutex
	// Images son las imagenes parcheadas, en orden.
	Images []string
	// SetErr decide el error del k-esimo SetImage (1-based).
	SetErr func(call int) error
	// Rollout decide la k-esima consulta (1-based) de RolloutComplete; nil = completa de inmediato.
	Rollout func(call int) (bool, error)
	// Gate, si no es nil, bloquea RolloutComplete hasta cerrarse (o ctx).
	Gate   chan struct{}
	rCalls int
	sCalls int
}

// Patches devuelve cuantos parches de imagen se aplicaron.
func (f *FakeDeployer) Patches() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.Images) }

func (f *FakeDeployer) SetImage(_ context.Context, ref string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sCalls++
	if f.SetErr != nil {
		if err := f.SetErr(f.sCalls); err != nil {
			return err
		}
	}
	f.Images = append(f.Images, ref)
	return nil
}

func (f *FakeDeployer) RolloutComplete(ctx context.Context, _ string) (bool, error) {
	f.mu.Lock()
	gate := f.Gate
	f.rCalls++
	call, fn := f.rCalls, f.Rollout
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
	if fn == nil {
		return true, nil
	}
	return fn(call)
}

// FakeProber responde con un mapa ruta -> (status, cuerpo).
type FakeProber struct {
	Base   string
	Routes map[string]FakeResponse
	Err    error // si no es nil, todo Get falla (app inalcanzable)
}

// FakeResponse es una respuesta del FakeProber.
type FakeResponse struct {
	Status int
	Body   string
}

func (f *FakeProber) BaseURL() string { return f.Base }
func (f *FakeProber) Get(_ context.Context, p string) (int, []byte, error) {
	if f.Err != nil {
		return 0, nil, f.Err
	}
	r, ok := f.Routes[p]
	if !ok {
		return 404, nil, nil
	}
	return r.Status, []byte(r.Body), nil
}

// MemObjects es un ObjectStore en memoria.
type MemObjects struct {
	mu     sync.Mutex
	M      map[string][]byte
	PutErr error
}

func (m *MemObjects) Put(_ context.Context, key string, data []byte) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.PutErr != nil {
		return "", m.PutErr
	}
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
