package warmmanager_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/internal/fakes"
)

var okArt = wm.Artifact{Kind: "published-image", Ref: "ghcr.io/ogaston/demo:1.2.3"}

type rig struct {
	svc   *wm.Service
	state *fakes.MemState
	probe *fakes.FakeProbe
	rt    *fakes.FakeRuntime
	dep   *fakes.FakeDeployer
	pub   *fakes.MemPublisher
	al    *fakes.FakeAlerter
	objs  *fakes.MemObjects
	web   *fakes.FakeProber
}

func newRig(st wm.WarmState) *rig {
	r := &rig{state: &fakes.MemState{S: st}, probe: &fakes.FakeProbe{}, rt: &fakes.FakeRuntime{}, dep: &fakes.FakeDeployer{},
		pub: &fakes.MemPublisher{}, al: &fakes.FakeAlerter{}, objs: &fakes.MemObjects{},
		web: &fakes.FakeProber{Base: "http://warm-app.aqs-test.svc", Routes: map[string]fakes.FakeResponse{}}}
	r.svc = &wm.Service{Cfg: wm.Config{AllowedRegistries: wm.DefaultAllowedRegistries, PollInterval: time.Second, DeployTimeout: time.Minute}, State: r.state, Probe: r.probe, Runtime: r.rt,
		Deployer: r.dep, Surface: r.web, Objects: r.objs, Alerts: r.al, Pub: r.pub,
		Clock: &fakes.FakeClock{T: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}}
	return r
}

func (r *rig) types() []string {
	var out []string
	for _, e := range r.pub.Events {
		out = append(out, e.Type)
	}
	return out
}

func TestEnsureWarmReady(t *testing.T) {
	ctx := context.Background()
	t.Run("listo publica warm.ready valido", func(t *testing.T) {
		r := newRig(ws("ready", true))
		w, ok, err := r.svc.EnsureWarmReady(ctx, "t1")
		if err != nil || !ok || w.State != "ready" || len(r.pub.Events) != 1 {
			t.Fatalf("%+v %v %v %v", w, ok, err, r.pub.Events)
		}
		if err := validate(t, "events/warm.ready.schema.json", r.pub.Events[0]); err != nil {
			t.Fatal(err)
		}
	})
	for _, c := range []struct {
		name string
		st   wm.WarmState
	}{{"dirty", ws("dirty", false)}, {"cuarentena", ws("cuarentena", false)}, {"sin reset_verified", ws("ready", false)}} {
		t.Run(c.name+" no esta listo y no hace reset", func(t *testing.T) {
			r := newRig(c.st)
			w, ok, err := r.svc.EnsureWarmReady(ctx, "t")
			if err != nil || ok || w != c.st || len(r.pub.Events) != 0 || r.rt.Calls != 0 {
				t.Fatalf("%+v %v %v", w, ok, err)
			}
			if got, _ := r.state.Get(ctx); got != c.st {
				t.Fatal("el estado cambio")
			}
		})
	}
	t.Run("sonda caida", func(t *testing.T) {
		r := newRig(ws("ready", true))
		r.probe.Set(errors.New("redis caido"))
		_, ok, err := r.svc.EnsureWarmReady(ctx, "t")
		if err != nil || ok || len(r.pub.Events) != 0 {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
	})
	t.Run("idle-escalado escala y vuelve a ready", func(t *testing.T) {
		r := newRig(ws("idle-escalado", true))
		n := 0
		r.probe.Set(errors.New("arrancando"))
		// la sonda se pone verde tras 3 consultas
		r.svc.Probe = probeFunc(func() error {
			n++
			if n < 3 {
				return errors.New("no ready")
			}
			return nil
		})
		w, ok, err := r.svc.EnsureWarmReady(ctx, "t")
		if err != nil || !ok || w.State != "ready" || r.rt.Calls != 1 {
			t.Fatalf("%+v %v %v calls=%d", w, ok, err, r.rt.Calls)
		}
		if got, _ := r.state.Get(ctx); got.State != "ready" {
			t.Fatal("no se persistio ready")
		}
	})
	t.Run("idle-escalado con timeout", func(t *testing.T) {
		r := newRig(ws("idle-escalado", true))
		r.probe.Set(errors.New("nunca"))
		r.svc.Cfg.WarmReadyTimeout = 10 * time.Second
		_, ok, err := r.svc.EnsureWarmReady(ctx, "t")
		if !errors.Is(err, wm.ErrWarmTimeout) || ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
		if got, _ := r.state.Get(ctx); got.State != "idle-escalado" {
			t.Fatal("el estado no debia cambiar")
		}
	})
}

type probeFunc func() error

func (f probeFunc) Check(context.Context) error { return f() }

// neverUntil hace que el rollout solo complete tras el k-esimo parche: los intentos anteriores
// llegan al timeout (el reloj falso avanza con cada Sleep, asi que el timeout es instantaneo).
func neverUntil(r *rig, k int) {
	r.dep.Rollout = func(int) (bool, error) { return r.dep.Patches() > k, nil }
}

func TestDeployRetry(t *testing.T) {
	for _, c := range []struct {
		name     string
		fail     int
		jobs     int
		lastType string
	}{{"sin reintentos", 0, 1, "deploy.done"}, {"un reintento", 1, 2, "deploy.done"}, {"dos reintentos", 2, 3, "deploy.done"}, {"tercera falla", 3, 3, "deploy.failed"}} {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(ws("ready", true))
			neverUntil(r, c.fail)
			err := r.svc.Deploy(context.Background(), "r-1", okArt, "t")
			if (c.lastType == "deploy.done") != (err == nil) {
				t.Fatalf("err=%v", err)
			}
			if r.dep.Patches() != c.jobs {
				t.Fatalf("parches=%d want %d", r.dep.Patches(), c.jobs)
			}
			last := r.pub.Events[len(r.pub.Events)-1]
			if last.Type != c.lastType {
				t.Fatalf("evento %s", last.Type)
			}
			if err := validate(t, "events/deploy.schema.json", last); err != nil {
				t.Fatal(err)
			}
			wantHandoff := 0
			if c.lastType == "deploy.failed" {
				wantHandoff = 1
			}
			if len(r.al.Calls) != wantHandoff {
				t.Fatalf("handoffs=%d", len(r.al.Calls))
			}
			if got, _ := r.state.Get(context.Background()); got.State != "dirty" || got.ResetVerified {
				t.Fatalf("el warm debia quedar dirty: %+v", got)
			}
		})
	}
}

func TestDeployExhaustedNeverPatchesFourthTime(t *testing.T) {
	r := newRig(ws("ready", true))
	neverUntil(r, 100)
	err := r.svc.Deploy(context.Background(), "r-1", okArt, "t")
	if err == nil || r.dep.Patches() != 3 || len(r.al.Calls) != 1 {
		t.Fatalf("err=%v parches=%d handoffs=%d", err, r.dep.Patches(), len(r.al.Calls))
	}
	if !strings.Contains(r.pub.Events[len(r.pub.Events)-1].Data["reason"].(string), "timeout") {
		t.Fatal("reason vacio")
	}
	// el warm queda dirty: otro deploy no parchea
	if err := r.svc.Deploy(context.Background(), "r-2", okArt, "t"); !errors.Is(err, wm.ErrNotReady) || r.dep.Patches() != 3 {
		t.Fatalf("no debia parchear mas: %v", err)
	}
}

// Un rollout que NUNCA completa llega al timeout: deploy.failed + 1 handoff, jamas deploy.done.
func TestDeployRolloutTimeoutNeverSucceeds(t *testing.T) {
	r := newRig(ws("ready", true))
	r.dep.Rollout = func(int) (bool, error) { return false, nil }
	err := r.svc.Deploy(context.Background(), "r-1", okArt, "t")
	if err == nil || r.dep.Patches() != 3 || len(r.al.Calls) != 1 {
		t.Fatalf("err=%v parches=%d handoffs=%d", err, r.dep.Patches(), len(r.al.Calls))
	}
	for _, ty := range r.types() {
		if ty == "deploy.done" {
			t.Fatalf("un timeout fue exito: %v", r.types())
		}
	}
	if n := len(r.types()); n != 1 || r.types()[0] != "deploy.failed" {
		t.Fatalf("eventos: %v", r.types())
	}
	if d, _, _ := r.svc.DeployState(context.Background(), "r-1"); d.State != "failed" || d.Attempts != 3 || !strings.Contains(d.Reason, "timeout") {
		t.Fatalf("%+v", d)
	}
}

func TestDeployRefusals(t *testing.T) {
	ctx := context.Background()
	r := newRig(ws("dirty", false))
	if err := r.svc.Deploy(ctx, "r-1", okArt, "t"); !errors.Is(err, wm.ErrNotReady) || r.dep.Patches() != 0 {
		t.Fatalf("dirty: %v", err)
	}
	r = newRig(ws("ready", false)) // ready pero sin reset verificado: tampoco se despliega
	if err := r.svc.Deploy(ctx, "r-1", okArt, "t"); !errors.Is(err, wm.ErrNotReady) || r.dep.Patches() != 0 {
		t.Fatalf("ready sin reset_verified: %v", err)
	}
	if got, _ := r.state.Get(ctx); got != ws("ready", false) {
		t.Fatal("el estado no debia cambiar")
	}
	r = newRig(ws("ready", true))
	if err := r.svc.Deploy(ctx, "r-1", wm.Artifact{Kind: "published-image", Ref: "docker.io/evil/x:1"}, "t"); !errors.Is(err, wm.ErrRegistryNotAllowed) || r.dep.Patches() != 0 {
		t.Fatalf("registro: %v", err)
	}
	if got, _ := r.state.Get(ctx); got.State != "ready" {
		t.Fatal("un artefacto rechazado no debe ensuciar el warm")
	}
}

const openapi = `{"openapi":"3.0.0","paths":{"/orders":{"get":{},"post":{}},"/orders/{id}":{"get":{},"parameters":[]}}}`

func mustDeploy(t *testing.T, r *rig) {
	t.Helper()
	if err := r.svc.Deploy(context.Background(), "r-1", okArt, "t"); err != nil {
		t.Fatal(err)
	}
}

func TestSurface(t *testing.T) {
	ctx := context.Background()
	t.Run("con OpenAPI", func(t *testing.T) {
		r := newRig(ws("ready", true))
		mustDeploy(t, r)
		r.web.Routes["/openapi.json"] = fakes.FakeResponse{Status: 200, Body: openapi}
		sa, err := r.svc.InferSurface(ctx, "r-1", "t")
		if err != nil || sa.Source != "openapi" || len(sa.Endpoints) != 3 {
			t.Fatalf("%+v %v", sa, err)
		}
		if err := validate(t, "plans/surface-artifact.schema.json", sa); err != nil {
			t.Fatal(err)
		}
		ev := r.pub.Events[len(r.pub.Events)-1]
		if ev.Type != "surface.ready" || ev.Data["endpoint_count"] != 3 {
			t.Fatalf("%+v", ev)
		}
		got, err := r.objs.Get(ctx, ev.Data["surface_uri"].(string))
		if err != nil || !strings.Contains(string(got), `"/orders"`) {
			t.Fatalf("no se guardo: %v", err)
		}
		if err := validate(t, "events/surface.ready.schema.json", ev); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("sin OpenAPI sondea", func(t *testing.T) {
		r := newRig(ws("ready", true))
		mustDeploy(t, r)
		r.web.Routes["/"] = fakes.FakeResponse{Status: 200}
		r.web.Routes["/health"] = fakes.FakeResponse{Status: 204}
		r.web.Routes["/api"] = fakes.FakeResponse{Status: 500}
		sa, err := r.svc.InferSurface(ctx, "r-1", "t")
		if err != nil || sa.Source != "probe" || len(sa.Endpoints) != 2 {
			t.Fatalf("%+v %v", sa, err)
		}
		if err := validate(t, "plans/surface-artifact.schema.json", sa); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("OpenAPI roto y sin rutas: error", func(t *testing.T) {
		r := newRig(ws("ready", true))
		mustDeploy(t, r)
		r.web.Routes["/openapi.json"] = fakes.FakeResponse{Status: 200, Body: "<html>"}
		_, err := r.svc.InferSurface(ctx, "r-1", "t")
		if !errors.Is(err, wm.ErrNoSurface) || len(r.pub.Events) != 1 { // solo deploy.done
			t.Fatalf("0 endpoints debe ser error y no publicar surface.ready: %v %v", err, r.types())
		}
	})
}

func TestEventsDeterministicIDAndSchema(t *testing.T) {
	if wm.EventID("a") != wm.EventID("a") || wm.EventID("a") == wm.EventID("b") {
		t.Fatal("EventID no determinista")
	}
	r := newRig(ws("ready", true))
	if err := r.svc.Deploy(context.Background(), "r-1", okArt, "trace-1"); err != nil {
		t.Fatal(err)
	}
	if err := validate(t, "events/deploy.schema.json", r.pub.Events[0]); err != nil {
		t.Fatal(err)
	}
}

// TestEventsDump vuelca a disco los cuatro eventos producidos por el servicio para validarlos con ajv (CA-4).
// Solo actua si WARM_DUMP_EVENTS_DIR esta definido; no es una prueba de comportamiento.
func TestEventsDump(t *testing.T) {
	dir := os.Getenv("WARM_DUMP_EVENTS_DIR")
	if dir == "" {
		t.Skip("WARM_DUMP_EVENTS_DIR no definido")
	}
	ctx := context.Background()
	r := newRig(ws("ready", true))
	r.web.Routes["/openapi.json"] = fakes.FakeResponse{Status: 200, Body: openapi}
	if _, _, err := r.svc.EnsureWarmReady(ctx, "4bf92f3577b34da6a3ce929d0e0e4736"); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Deploy(ctx, "r-1", okArt, "4bf92f3577b34da6a3ce929d0e0e4736"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.InferSurface(ctx, "r-1", "4bf92f3577b34da6a3ce929d0e0e4736"); err != nil {
		t.Fatal(err)
	}
	f := newRig(ws("ready", true))
	neverUntil(f, 100)
	_ = f.svc.Deploy(ctx, "r-2", okArt, "4bf92f3577b34da6a3ce929d0e0e4736")
	all := append(r.pub.Events, f.pub.Events...)
	for _, e := range all {
		raw, _ := json.Marshal(e)
		if err := os.WriteFile(filepath.Join(dir, e.Type+".json"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// slowState alarga el Get para ampliar la ventana de carrera entre leer y escribir.
type slowState struct{ *fakes.MemState }

func (s slowState) Get(ctx context.Context) (wm.WarmState, error) {
	w, err := s.MemState.Get(ctx)
	time.Sleep(20 * time.Millisecond)
	return w, err
}

func TestDeployConcurrentTakesWarmOnce(t *testing.T) {
	r := newRig(ws("ready", true))
	r.svc.State = slowState{r.state}
	var wg sync.WaitGroup
	errs := make([]error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = r.svc.Deploy(context.Background(), fmt.Sprintf("r-%d", i), okArt, "t")
		}(i)
	}
	wg.Wait()
	ok := 0
	for _, e := range errs {
		if e == nil {
			ok++
		} else if !errors.Is(e, wm.ErrNotReady) {
			t.Fatalf("los perdedores deben fallar con ErrNotReady: %v", e)
		}
	}
	if r.dep.Patches() != 1 || ok != 1 {
		t.Fatalf("parches=%d ganadores=%d", r.dep.Patches(), ok)
	}
}

// casLoser simula que otro escritor cambia el estado entre el Get y el CAS.
type casLoser struct{ *fakes.MemState }

func (c casLoser) CompareAndSwap(context.Context, wm.WarmState, wm.WarmState) error {
	return wm.ErrStateConflict
}

func TestDeployCASConflictIsNotReadyAndNoPatch(t *testing.T) {
	r := newRig(ws("ready", true))
	r.svc.State = casLoser{r.state}
	err := r.svc.Deploy(context.Background(), "r-1", okArt, "t")
	if !errors.Is(err, wm.ErrNotReady) || r.dep.Patches() != 0 {
		t.Fatalf("err=%v parches=%d", err, r.dep.Patches())
	}
	if d, _, _ := r.svc.DeployState(context.Background(), "r-1"); d.State != "failed" || d.Reason == "" {
		t.Fatalf("estado visible: %+v", d)
	}
}

func TestDeployRolloutReadErrorDoesNotConsumeRetry(t *testing.T) {
	r := newRig(ws("ready", true))
	r.dep.Rollout = func(call int) (bool, error) {
		if call == 1 {
			return false, errors.New("apiserver 500")
		}
		return true, nil
	}
	if err := r.svc.Deploy(context.Background(), "r-1", okArt, "t"); err != nil {
		t.Fatal(err)
	}
	if r.dep.Patches() != 1 || r.types()[len(r.types())-1] != "deploy.done" {
		t.Fatalf("parches=%d eventos=%v", r.dep.Patches(), r.types())
	}
}

func TestDeployRolloutReadErrorPersistentFailsClosed(t *testing.T) {
	r := newRig(ws("ready", true))
	r.dep.Rollout = func(int) (bool, error) { return false, errors.New("apiserver caido") }
	err := r.svc.Deploy(context.Background(), "r-1", okArt, "t")
	last := r.pub.Events[len(r.pub.Events)-1]
	if err == nil || r.dep.Patches() != 3 || last.Type != "deploy.failed" || len(r.al.Calls) != 1 {
		t.Fatalf("err=%v parches=%d ev=%s handoffs=%d", err, r.dep.Patches(), last.Type, len(r.al.Calls))
	}
	if !strings.Contains(last.Data["reason"].(string), "no se pudo leer el rollout") {
		t.Fatalf("reason=%v", last.Data["reason"])
	}
	if err := validate(t, "events/deploy.schema.json", last); err != nil {
		t.Fatal(err)
	}
}

// Un parche rechazado cuenta como intento fallido: se reintenta (el parche es idempotente) y agotados falla cerrado.
func TestDeployPatchErrorRetriesThenFailsClosed(t *testing.T) {
	r := newRig(ws("ready", true))
	r.dep.SetErr = func(int) error { return errors.New("forbidden") }
	err := r.svc.Deploy(context.Background(), "r-1", okArt, "t")
	if err == nil || r.dep.Patches() != 0 || len(r.al.Calls) != 1 || r.types()[len(r.types())-1] != "deploy.failed" {
		t.Fatalf("err=%v parches=%d handoffs=%d", err, r.dep.Patches(), len(r.al.Calls))
	}
	d, _, _ := r.svc.DeployState(context.Background(), "r-1")
	if d.State != "failed" || d.Attempts != 3 || !strings.Contains(d.Reason, "forbidden") {
		t.Fatalf("%+v", d)
	}
	r = newRig(ws("ready", true)) // un parche transitorio se recupera en el reintento
	r.dep.SetErr = func(call int) error {
		if call == 1 {
			return errors.New("conflicto")
		}
		return nil
	}
	if err := r.svc.Deploy(context.Background(), "r-1", okArt, "t"); err != nil || r.dep.Patches() != 1 || len(r.al.Calls) != 0 {
		t.Fatalf("err=%v parches=%d", err, r.dep.Patches())
	}
}

func TestDeployWarmNotReadyEmitsFailedWithoutHandoff(t *testing.T) {
	r := newRig(ws("dirty", false))
	err := r.svc.Deploy(context.Background(), "r-1", okArt, "t")
	if !errors.Is(err, wm.ErrNotReady) || len(r.al.Calls) != 0 || r.dep.Patches() != 0 {
		t.Fatalf("err=%v", err)
	}
	last := r.pub.Events[len(r.pub.Events)-1]
	if last.Type != "deploy.failed" {
		t.Fatalf("evento %s", last.Type)
	}
	if err := validate(t, "events/deploy.schema.json", last); err != nil {
		t.Fatal(err)
	}
	if d, _, _ := r.svc.DeployState(context.Background(), "r-1"); d.State != "failed" || d.Reason == "" {
		t.Fatalf("%+v", d)
	}
}

type failingState struct {
	*fakes.MemState
	getErr, casErr error
}

func (f failingState) Get(ctx context.Context) (wm.WarmState, error) {
	if f.getErr != nil {
		return wm.WarmState{}, f.getErr
	}
	return f.MemState.Get(ctx)
}
func (f failingState) CompareAndSwap(ctx context.Context, a, b wm.WarmState) error {
	if f.casErr != nil {
		return f.casErr
	}
	return f.MemState.CompareAndSwap(ctx, a, b)
}

type failingPub struct {
	*fakes.MemPublisher
	failType string
}

func (p failingPub) Publish(ctx context.Context, e wm.Event) error {
	if e.Type == p.failType {
		return errors.New("outbox lleno")
	}
	return p.MemPublisher.Publish(ctx, e)
}

type failingAlerter struct{}

func (failingAlerter) Handoff(context.Context, string, string, string) error {
	return errors.New("pager caido")
}

func logged(r *rig) *bytes.Buffer {
	b := &bytes.Buffer{}
	r.svc.Log = slog.New(slog.NewJSONHandler(b, nil))
	return b
}

func TestDeployPutDirtyFailureIsVisibleAndLogged(t *testing.T) {
	r := newRig(ws("ready", true))
	lg := logged(r)
	r.svc.State = failingState{MemState: r.state, casErr: errors.New("etcd caido")}
	if err := r.svc.Deploy(context.Background(), "r-1", okArt, "trace-x"); err == nil {
		t.Fatal("debia fallar")
	}
	d, _, _ := r.svc.DeployState(context.Background(), "r-1")
	if d.State != "failed" || !strings.Contains(d.Reason, "etcd caido") || r.dep.Patches() != 0 {
		t.Fatalf("%+v parches=%d", d, r.dep.Patches())
	}
	if !strings.Contains(lg.String(), `"run_id":"r-1"`) || !strings.Contains(lg.String(), `"trace_id":"trace-x"`) {
		t.Fatalf("log sin run_id/trace_id: %s", lg)
	}
}

func TestDeployStateReadFailureEmitsNoInvalidEvent(t *testing.T) {
	r := newRig(ws("ready", true))
	r.svc.State = failingState{MemState: r.state, getErr: errors.New("sin acceso")}
	if err := r.svc.Deploy(context.Background(), "r-1", okArt, "t"); err == nil || len(r.pub.Events) != 0 {
		t.Fatalf("err=%v eventos=%v", err, r.types())
	}
	if d, _, _ := r.svc.DeployState(context.Background(), "r-1"); d.State != "failed" {
		t.Fatalf("%+v", d)
	}
}

func TestDeployPublishDoneFailureMarksFailedAndLogs(t *testing.T) {
	r := newRig(ws("ready", true))
	lg := logged(r)
	r.svc.Pub = failingPub{r.pub, "deploy.done"}
	if err := r.svc.Deploy(context.Background(), "r-1", okArt, "t9"); err == nil {
		t.Fatal("debia devolver el error")
	}
	if d, _, _ := r.svc.DeployState(context.Background(), "r-1"); d.State != "failed" || !strings.Contains(d.Reason, "deploy.done") {
		t.Fatalf("%+v", d)
	}
	if !strings.Contains(lg.String(), "outbox lleno") || !strings.Contains(lg.String(), `"trace_id":"t9"`) {
		t.Fatalf("log: %s", lg)
	}
}

func TestDeployHandoffAndPublishFailedAreLoggedAndReturned(t *testing.T) {
	r := newRig(ws("ready", true))
	lg := logged(r)
	neverUntil(r, 100)
	r.svc.Alerts = failingAlerter{}
	r.svc.Pub = failingPub{r.pub, "deploy.failed"}
	err := r.svc.Deploy(context.Background(), "r-1", okArt, "t7")
	if err == nil || !strings.Contains(err.Error(), "pager caido") || !strings.Contains(err.Error(), "outbox lleno") {
		t.Fatalf("errores perdidos: %v", err)
	}
	if d, _, _ := r.svc.DeployState(context.Background(), "r-1"); d.State != "failed" {
		t.Fatalf("%+v", d)
	}
	for _, want := range []string{"pager caido", "outbox lleno", `"trace_id":"t7"`} {
		if !strings.Contains(lg.String(), want) {
			t.Fatalf("falta %q en el log: %s", want, lg)
		}
	}
}

// StartDeploy repetido con el mismo run_id (deploy aun pending) devuelve el estado existente sin tocar
// el warm: ni un segundo parche ni un cambio del WarmState.
func TestStartDeployIdempotentReturnsExistingStateWithoutTouchingWarm(t *testing.T) {
	ctx := context.Background()
	r := newRig(ws("ready", true))
	gate := make(chan struct{})
	r.dep.Gate = gate
	first, _, err := r.svc.StartDeploy(ctx, "r-1", okArt, "t")
	if err != nil || first.State != "pending" {
		t.Fatalf("%+v %v", first, err)
	}
	for i := 0; i < 500 && r.dep.Patches() == 0; i++ { // el parche ya se aplico y el rollout espera la compuerta
		time.Sleep(2 * time.Millisecond)
	}
	if r.dep.Patches() != 1 {
		t.Fatalf("parches=%d", r.dep.Patches())
	}
	warmBefore, _ := r.state.Get(ctx)
	for i := 0; i < 3; i++ {
		again, w, err := r.svc.StartDeploy(ctx, "r-1", okArt, "otro-trace")
		if err != nil || again.RunID != "r-1" || again.State != "pending" || w != (wm.WarmState{}) {
			t.Fatalf("repetido: %+v %+v %v", again, w, err)
		}
	}
	if warmAfter, _ := r.state.Get(ctx); warmAfter != warmBefore || r.dep.Patches() != 1 || len(r.pub.Snapshot()) != 0 {
		t.Fatalf("el repetido toco el warm o parcheo: %+v parches=%d", warmAfter, r.dep.Patches())
	}
	close(gate)
	var d wm.DeployStatus
	for i := 0; i < 500; i++ {
		if d, _, _ = r.svc.DeployState(ctx, "r-1"); d.State == "done" {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if d.State != "done" || r.dep.Patches() != 1 {
		t.Fatalf("%+v parches=%d", d, r.dep.Patches())
	}
	// ya terminado, un repetido tampoco toca nada (el warm sigue dirty)
	if again, _, err := r.svc.StartDeploy(ctx, "r-1", okArt, "t"); err != nil || again.State != "done" || r.dep.Patches() != 1 {
		t.Fatalf("%+v %v", again, err)
	}
}

// build-from-repo no esta soportado: se rechaza sin tomar el warm ni parchear (fail-closed).
func TestBuildFromRepoRejectedTouchesNothing(t *testing.T) {
	ctx := context.Background()
	r := newRig(ws("ready", true))
	a := wm.Artifact{Kind: "build-from-repo", Ref: "ogaston/demo@" + strings.Repeat("a", 40)}
	if _, _, err := r.svc.StartDeploy(ctx, "r-1", a, "t"); !errors.Is(err, wm.ErrBuildNotSupported) {
		t.Fatalf("StartDeploy: %v", err)
	}
	if err := r.svc.Deploy(ctx, "r-1", a, "t"); !errors.Is(err, wm.ErrBuildNotSupported) {
		t.Fatalf("Deploy: %v", err)
	}
	if got, _ := r.state.Get(ctx); got != ws("ready", true) || r.dep.Patches() != 0 || len(r.pub.Snapshot()) != 0 {
		t.Fatalf("toco algo: %+v parches=%d", got, r.dep.Patches())
	}
	if _, ok, _ := r.svc.DeployState(ctx, "r-1"); ok {
		t.Fatal("no debia crearse estado")
	}
}

func TestStartDeployWarmNotReadyCreatesNoState(t *testing.T) {
	r := newRig(ws("dirty", false))
	_, w, err := r.svc.StartDeploy(context.Background(), "r-1", okArt, "t")
	if !errors.Is(err, wm.ErrNotReady) || w != ws("dirty", false) {
		t.Fatalf("w=%+v err=%v", w, err)
	}
	if _, ok, _ := r.svc.DeployState(context.Background(), "r-1"); ok || r.dep.Patches() != 0 {
		t.Fatal("no debia haber estado ni parches")
	}
}

func TestEnsureIdleTimeoutIsConfigurableAndBounded(t *testing.T) {
	r := newRig(ws("idle-escalado", true))
	r.probe.Set(errors.New("nunca"))
	r.svc.Cfg.WarmReadyTimeout = 3 * time.Second
	r.svc.Cfg.PollInterval = time.Second
	_, _, err := r.svc.EnsureWarmReady(context.Background(), "t")
	if !errors.Is(err, wm.ErrWarmTimeout) {
		t.Fatal(err)
	}
}
