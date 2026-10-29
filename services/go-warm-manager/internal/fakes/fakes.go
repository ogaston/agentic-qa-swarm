// Package fakes: dobles en memoria de los puertos de go-warm-manager (solo pruebas).
package fakes

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/warmtarget"
)

// Fakes en memoria de todos los puertos (pruebas y modo WARM_KUBE=fake).

// MemState es un StateStore en memoria.
type MemState struct {
	mu     sync.Mutex
	S      wm.WarmState
	States map[string]wm.WarmState
}

func (m *MemState) Get(context.Context) (wm.WarmState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.S, nil
}

func (m *MemState) GetForTarget(_ context.Context, target warmtarget.WarmTarget) (wm.WarmState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if target.Namespace == "" || target.Namespace == wm.LegacyNamespace {
		return m.S, nil
	}
	if m.States != nil {
		if st, ok := m.States[target.Namespace]; ok {
			return st, nil
		}
		if st, ok := m.States[target.WarmID]; ok {
			return st, nil
		}
		if st, ok := m.States[target.Repo]; ok {
			return st, nil
		}
	}
	// Si no existe, devuelve la semilla / estado base con datos del target
	seed := m.S
	seed.Namespace = target.Namespace
	seed.WarmID = target.WarmID
	seed.Repo = target.Repo
	return seed, nil
}

func (m *MemState) CompareAndSwap(_ context.Context, expect, next wm.WarmState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if next.Namespace != "" && next.Namespace != wm.LegacyNamespace {
		return m.casLocked(next.Namespace, expect, next)
	}
	if m.S != expect {
		return wm.ErrStateConflict
	}
	m.S = next
	return nil
}

func (m *MemState) CompareAndSwapForTarget(_ context.Context, target warmtarget.WarmTarget, expect, next wm.WarmState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ns := target.Namespace
	if ns == "" {
		ns = wm.LegacyNamespace
	}
	return m.casLocked(ns, expect, next)
}

func (m *MemState) casLocked(ns string, expect, next wm.WarmState) error {
	if ns == wm.LegacyNamespace {
		if m.S != expect {
			return wm.ErrStateConflict
		}
		m.S = next
		return nil
	}
	if m.States == nil {
		m.States = make(map[string]wm.WarmState)
	}
	cur, ok := m.States[ns]
	if !ok {
		cur = m.S
		cur.Namespace = ns
		cur.WarmID = expect.WarmID
		cur.Repo = expect.Repo
	}
	if cur != expect {
		return wm.ErrStateConflict
	}
	m.States[ns] = next
	if next.WarmID != "" {
		m.States[next.WarmID] = next
	}
	if next.Repo != "" {
		m.States[next.Repo] = next
	}
	return nil
}

func (m *MemState) Put(_ context.Context, s wm.WarmState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.Namespace != "" && s.Namespace != wm.LegacyNamespace {
		if m.States == nil {
			m.States = make(map[string]wm.WarmState)
		}
		m.States[s.Namespace] = s
		if s.WarmID != "" {
			m.States[s.WarmID] = s
		}
		if s.Repo != "" {
			m.States[s.Repo] = s
		}
		return nil
	}
	m.S = s
	return nil
}

func (m *MemState) PutForTarget(_ context.Context, target warmtarget.WarmTarget, s wm.WarmState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ns := target.Namespace
	if ns == "" || ns == wm.LegacyNamespace {
		m.S = s
		return nil
	}
	if m.States == nil {
		m.States = make(map[string]wm.WarmState)
	}
	m.States[ns] = s
	if s.WarmID != "" {
		m.States[s.WarmID] = s
	}
	if s.Repo != "" {
		m.States[s.Repo] = s
	}
	return nil
}

func (m *MemState) LookupState(ctx context.Context, idOrNS string) (wm.WarmState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if idOrNS == "" || idOrNS == wm.LegacyNamespace || idOrNS == warmtarget.LegacyWarmID {
		return m.S, nil
	}
	if m.States != nil {
		if st, ok := m.States[idOrNS]; ok {
			return st, nil
		}
	}
	return m.S, nil
}

// FakeProbe devuelve Err en cada Check (o nil).
type FakeProbe struct {
	mu     sync.Mutex
	Err    error
	NSErrs map[string]error
}

func (f *FakeProbe) Check(context.Context) error { f.mu.Lock(); defer f.mu.Unlock(); return f.Err }
func (f *FakeProbe) Set(err error)               { f.mu.Lock(); f.Err = err; f.mu.Unlock() }

func (f *FakeProbe) CheckInNamespace(_ context.Context, ns string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.NSErrs != nil {
		if err, ok := f.NSErrs[ns]; ok {
			return err
		}
	}
	return f.Err
}

// FakeRuntime cuenta los ScaleUp.
type FakeRuntime struct {
	mu      sync.Mutex
	Calls   int
	NSCalls map[string]int
	OnUp    func()
	Err     error // error de ScaleUp
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

func (f *FakeRuntime) ScaleUpInNamespace(ctx context.Context, ns string) error {
	f.mu.Lock()
	if f.NSCalls == nil {
		f.NSCalls = make(map[string]int)
	}
	f.NSCalls[ns]++
	f.mu.Unlock()
	return f.ScaleUp(ctx)
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

func (f *FakeDeployer) SetImageInNamespace(ctx context.Context, _ string, ref string) error {
	return f.SetImage(ctx, ref)
}

func (f *FakeDeployer) RolloutCompleteInNamespace(ctx context.Context, _ string, ref string) (bool, error) {
	return f.RolloutComplete(ctx, ref)
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
func (f *FakeProber) Get(ctx context.Context, p string) (int, []byte, error) {
	return f.GetWithBaseURL(ctx, f.Base, p)
}

func (f *FakeProber) GetWithBaseURL(_ context.Context, baseURL, p string) (int, []byte, error) {
	if f.Err != nil {
		return 0, nil, f.Err
	}
	if f.Routes != nil {
		if r, ok := f.Routes[baseURL+p]; ok {
			return r.Status, []byte(r.Body), nil
		}
		if r, ok := f.Routes[p]; ok {
			return r.Status, []byte(r.Body), nil
		}
	}
	return 404, nil, nil
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
