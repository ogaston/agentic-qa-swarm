// Package fakes son dobles en memoria de todos los puertos (pruebas y RESET_BACKEND=fake).
package fakes

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/core"
)

// Clock es un reloj simulado: Sleep avanza el tiempo.
type Clock struct {
	mu sync.Mutex
	T  time.Time
}

func NewClock(t time.Time) *Clock { return &Clock{T: t} }
func (c *Clock) Now() time.Time   { c.mu.Lock(); defer c.mu.Unlock(); return c.T }
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.T = c.T.Add(d)
	c.mu.Unlock()
}
func (c *Clock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.Advance(d)
	return nil
}

// Kube es un KubeAPI falso. NotReady y los errores simulan fallos; "Ready" es el estado real que se lee de vuelta.
type Kube struct {
	mu         sync.Mutex
	Reps       int32
	Ready      bool
	RestartErr error
	ScaleErr   error // ScaleApp falla sin cambiar las réplicas
	Policy     core.Policy
	Calls      []string
	// ReadyAfterRestart: si es false, el pod nunca queda Ready tras un restart aunque RestartApp "tenga éxito".
	ReadyAfterRestart bool
}

func NewKube() *Kube {
	return &Kube{Reps: 1, Ready: true, ReadyAfterRestart: true,
		Policy: core.Policy{IdleScaleDownAfter: 30 * time.Minute}}
}
func (k *Kube) rec(s string) { k.Calls = append(k.Calls, s) }
func (k *Kube) Replicas(context.Context) (int32, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.Reps, nil
}
func (k *Kube) restart(name string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.rec(name)
	if k.RestartErr != nil {
		return k.RestartErr
	}
	k.Reps = max(k.Reps, 1)
	k.Ready = k.ReadyAfterRestart
	return nil
}
func (k *Kube) RestartApp(context.Context) error { return k.restart("restart") }
func (k *Kube) Rebuild(context.Context) error    { return k.restart("rebuild") }
func (k *Kube) Teardown(context.Context) error   { return k.restart("teardown") }
func (k *Kube) ScaleApp(_ context.Context, n int32) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.rec("scale")
	if k.ScaleErr != nil {
		return k.ScaleErr
	}
	k.Reps = n
	k.Ready = n > 0 && k.ReadyAfterRestart
	return nil
}
func (k *Kube) AppReady(context.Context) (bool, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.Ready, nil
}
func (k *Kube) WarmPolicy(context.Context) (core.Policy, error) { return k.Policy, nil }

// DB es un DatabaseCleaner falso. Rows es el estado real; Clean lo deja en 0 salvo que StickyRows lo impida
// (simula un limpiador que "termina bien" pero deja la DB sucia).
type DB struct {
	mu        sync.Mutex
	Rows      int
	CleanErr  error
	Sticky    int
	CleanCall int
	Ver       string // versión del baseline; "" = desconocida
	VerErr    error
}

func (d *DB) Version(context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.Ver, d.VerErr
}

func (d *DB) Clean(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.CleanCall++
	if d.CleanErr != nil {
		return d.CleanErr
	}
	d.Rows = d.Sticky
	return nil
}
func (d *DB) Diff(context.Context) (int, error) { d.mu.Lock(); defer d.mu.Unlock(); return d.Rows, nil }

// Cache es un CacheFlusher falso con el mismo patrón.
type Cache struct {
	mu       sync.Mutex
	Keys     int64
	FlushErr error
	Sticky   int64
}

func (c *Cache) Flush(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.FlushErr != nil {
		return c.FlushErr
	}
	c.Keys = c.Sticky
	return nil
}
func (c *Cache) DBSize(context.Context) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Keys, nil
}

// State es un StateStore en memoria.
type State struct {
	mu     sync.Mutex
	Snap   core.Snapshot
	Exists bool
	PutErr error
	// FailPutN: si >0, falla el Put número N (1-based) y los siguientes.
	FailPutN int
	puts     int
}

func (s *State) Get(context.Context) (core.Snapshot, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Snap, s.Exists, nil
}
func (s *State) Put(_ context.Context, w core.WarmState, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.puts++
	if s.PutErr != nil || (s.FailPutN > 0 && s.puts >= s.FailPutN) {
		return errPut(s.PutErr)
	}
	s.Snap, s.Exists = core.Snapshot{WarmState: w, UpdatedAt: at}, true
	return nil
}

// Sessions es un SessionStore en memoria.
type Sessions struct {
	mu sync.Mutex
	M  map[string]core.Session
}

func (s *Sessions) Save(_ context.Context, se core.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.M == nil {
		s.M = map[string]core.Session{}
	}
	s.M[se.RunID] = se
	return nil
}
func (s *Sessions) List(context.Context) ([]core.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []core.Session
	for _, v := range s.M {
		out = append(out, v)
	}
	return out, nil
}

// Events recoge los eventos publicados.
type Events struct {
	mu  sync.Mutex
	Got []core.Event
	Err error
}

func (e *Events) Publish(_ context.Context, ev core.Event) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.Err != nil {
		return e.Err
	}
	e.Got = append(e.Got, ev)
	return nil
}
func (e *Events) Types() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var t []string
	for _, ev := range e.Got {
		t = append(t, ev.Type)
	}
	return t
}

// Alert cuenta las cuarentenas.
type Alert struct {
	mu      sync.Mutex
	Count   int
	Reasons []string
}

func (a *Alert) Quarantine(_ context.Context, _, _, reason string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Count++
	a.Reasons = append(a.Reasons, reason)
}

// ErrBoom es un error genérico de prueba.
var ErrBoom = errors.New("boom")

// Bundle agrupa un Service completo con fakes.
type Bundle struct {
	Svc      *core.Service
	Kube     *Kube
	DB       *DB
	Cache    *Cache
	State    *State
	Sessions *Sessions
	Events   *Events
	Alert    *Alert
	Clock    *Clock
}

// NewBundle arma un servicio con un warm ready y limpio.
func NewBundle() *Bundle {
	b := &Bundle{Kube: NewKube(), DB: &DB{Ver: "b1"}, Cache: &Cache{}, State: &State{}, Sessions: &Sessions{},
		Events: &Events{}, Alert: &Alert{}, Clock: NewClock(time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC))}
	b.State.Snap = core.Snapshot{WarmState: core.WarmState{WarmID: "warm-1", State: core.StateDirty, BaselineVersion: "b1"}, UpdatedAt: b.Clock.Now()}
	b.State.Exists = true
	b.Svc = &core.Service{Kube: b.Kube, DB: b.DB, Cache: b.Cache, State: b.State, Sessions: b.Sessions,
		Events: b.Events, Alert: b.Alert, Clock: b.Clock,
		Cfg: core.Config{DefaultWarmID: "warm-1", BaselineVersion: "b1", ReadyTimeout: 10 * time.Second,
			ReadyInterval: time.Second, Grace: 24 * time.Hour}}
	return b
}

func errPut(e error) error {
	if e != nil {
		return e
	}
	return ErrBoom
}
