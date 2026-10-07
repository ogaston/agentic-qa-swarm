package warmmanager_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/internal/fakes"
)

// ---- Superficie fail-closed (F-01 ronda 2) ----

func deployedRig(t *testing.T) *rig {
	r := newRig(ws("ready", true))
	mustDeploy(t, r)
	return r
}

func TestSurfaceUnreachableFailsClosedAndLogs(t *testing.T) {
	r := deployedRig(t)
	lg := logged(r)
	r.web.Err = errors.New("connection refused")
	n := len(r.pub.Events)
	_, err := r.svc.InferSurface(context.Background(), "r-1", "tr-9")
	if !errors.Is(err, wm.ErrSurfaceUnreachable) || len(r.pub.Events) != n || len(r.objs.M) != 0 {
		t.Fatalf("err=%v eventos=%v objetos=%d", err, r.types(), len(r.objs.M))
	}
	for _, want := range []string{"connection refused", `"run_id":"r-1"`, `"trace_id":"tr-9"`, `"path":"/openapi.json"`} {
		if !strings.Contains(lg.String(), want) {
			t.Fatalf("falta %q en el log: %s", want, lg)
		}
	}
}

func TestSurfaceNoEndpointsIsErrorNeverSurfaceReady(t *testing.T) {
	r := deployedRig(t) // la app responde 404 a todo
	n := len(r.pub.Events)
	if _, err := r.svc.InferSurface(context.Background(), "r-1", "t"); !errors.Is(err, wm.ErrNoSurface) || len(r.pub.Events) != n {
		t.Fatalf("err=%v", err)
	}
}

func TestSurfaceRequiresFinishedDeployAndTakenWarm(t *testing.T) {
	ctx := context.Background()
	r := newRig(ws("ready", true))
	r.web.Routes["/"] = fakes.FakeResponse{Status: 200}
	if _, err := r.svc.InferSurface(ctx, "r-1", "t"); !errors.Is(err, wm.ErrDeployNotDone) { // nunca desplegado
		t.Fatalf("sin deploy: %v", err)
	}
	r = newRig(ws("ready", true))
	r.web.Routes["/"] = fakes.FakeResponse{Status: 200}
	neverUntil(r, 100)
	_ = r.svc.Deploy(ctx, "r-1", okArt, "t")
	if _, err := r.svc.InferSurface(ctx, "r-1", "t"); !errors.Is(err, wm.ErrDeployNotDone) { // deploy fallido
		t.Fatalf("deploy fallido: %v", err)
	}
	r = newRig(ws("ready", true)) // deploy en vuelo (pending): la superficie se rechaza con ErrDeployNotDone
	r.web.Routes["/"] = fakes.FakeResponse{Status: 200}
	gate := make(chan struct{})
	r.dep.Gate = gate
	if _, _, err := r.svc.StartDeploy(ctx, "r-1", okArt, "t"); err != nil {
		t.Fatal(err)
	}
	if d, ok, _ := r.svc.DeployState(ctx, "r-1"); !ok || d.State != "pending" {
		t.Fatalf("%+v", d)
	}
	if _, err := r.svc.InferSurface(ctx, "r-1", "t"); !errors.Is(err, wm.ErrDeployNotDone) {
		t.Fatalf("deploy pending: %v", err)
	}
	if len(r.objs.M) != 0 || len(r.pub.Snapshot()) != 0 {
		t.Fatal("con el deploy pending no se guarda ni publica nada")
	}
	close(gate)
	for i := 0; i < 500; i++ {
		if d, _, _ := r.svc.DeployState(ctx, "r-1"); d.State == "done" {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	for _, st := range []string{"cuarentena", "idle-escalado", "ready"} { // el warm ya no esta tomado
		r = newRig(ws("ready", true))
		r.web.Routes["/"] = fakes.FakeResponse{Status: 200}
		mustDeploy(t, r)
		r.state.S = ws(st, st == "ready")
		if _, err := r.svc.InferSurface(ctx, "r-1", "t"); !errors.Is(err, wm.ErrNotReady) {
			t.Fatalf("warm %s: %v", st, err)
		}
	}
}

func TestSurfaceInvalidBaseURLIsRejectedBeforeStoringOrPublishing(t *testing.T) {
	r := deployedRig(t)
	r.web.Base = "warm-app:8080"
	r.web.Routes["/"] = fakes.FakeResponse{Status: 200}
	n := len(r.pub.Events)
	if _, err := r.svc.InferSurface(context.Background(), "r-1", "t"); err == nil || len(r.objs.M) != 0 || len(r.pub.Events) != n {
		t.Fatalf("err=%v objetos=%d", err, len(r.objs.M))
	}
}

// Paridad: la validacion en tiempo de ejecucion coincide con el esquema real.
func TestValidateSurfaceMatchesSchema(t *testing.T) {
	ok := wm.SurfaceArtifact{RunID: "r", BaseURL: "http://warm-app.aqs-test.svc", Endpoints: []wm.Endpoint{{Method: "GET", Path: "/a"}}, Source: "probe"}
	mut := func(f func(*wm.SurfaceArtifact)) wm.SurfaceArtifact {
		c := ok
		c.Endpoints = append([]wm.Endpoint(nil), ok.Endpoints...)
		f(&c)
		return c
	}
	cases := map[string]wm.SurfaceArtifact{
		"valido":              ok,
		"https":               mut(func(s *wm.SurfaceArtifact) { s.BaseURL = "https://x.y:8443" }),
		"run vacio":           mut(func(s *wm.SurfaceArtifact) { s.RunID = "" }),
		"base sin esquema":    mut(func(s *wm.SurfaceArtifact) { s.BaseURL = "warm-app:8080" }),
		"base ftp":            mut(func(s *wm.SurfaceArtifact) { s.BaseURL = "ftp://x" }),
		"metodo minuscula":    mut(func(s *wm.SurfaceArtifact) { s.Endpoints[0].Method = "get" }),
		"metodo desconocido":  mut(func(s *wm.SurfaceArtifact) { s.Endpoints[0].Method = "TRACE" }),
		"path sin barra":      mut(func(s *wm.SurfaceArtifact) { s.Endpoints[0].Path = "a" }),
		"source desconocido":  mut(func(s *wm.SurfaceArtifact) { s.Source = "otro" }),
		"endpoints ausentes":  mut(func(s *wm.SurfaceArtifact) { s.Endpoints = nil }),
		"endpoints vacios ok": mut(func(s *wm.SurfaceArtifact) { s.Endpoints = []wm.Endpoint{} }),
	}
	for n, sa := range cases {
		t.Run(n, func(t *testing.T) {
			got, want := wm.ValidateSurface(sa) == nil, validate(t, "plans/surface-artifact.schema.json", sa) == nil
			if got != want {
				t.Fatalf("runtime=%v esquema=%v", got, want)
			}
		})
	}
}

func TestSurfaceStoreAndPublishErrorsPropagate(t *testing.T) {
	ctx := context.Background()
	r := deployedRig(t)
	r.web.Routes["/"] = fakes.FakeResponse{Status: 200}
	lg := logged(r)
	r.objs.PutErr = errors.New("bucket lleno")
	n := len(r.pub.Events)
	if _, err := r.svc.InferSurface(ctx, "r-1", "t1"); err == nil || len(r.pub.Events) != n { // P3
		t.Fatalf("Put: %v", err)
	}
	r.objs.PutErr = nil
	r.svc.Pub = failingPub{r.pub, "surface.ready"}
	if _, err := r.svc.InferSurface(ctx, "r-1", "t1"); err == nil { // P4
		t.Fatal("Publish de surface.ready tragado")
	}
	for _, w := range []string{"bucket lleno", "outbox lleno", `"trace_id":"t1"`} {
		if !strings.Contains(lg.String(), w) {
			t.Fatalf("falta %q: %s", w, lg)
		}
	}
}

// ---- ensure: errores no tragados y concurrencia (F-02 / F-04 P1-P2 / MD) ----

func TestEnsureErrorsPropagate(t *testing.T) {
	ctx := context.Background()
	r := newRig(ws("ready", true))
	r.svc.Pub = failingPub{r.pub, "warm.ready"}
	lg := logged(r)
	if _, ok, err := r.svc.EnsureWarmReady(ctx, "t"); err == nil || ok { // P1
		t.Fatalf("Publish tragado: ok=%v err=%v", ok, err)
	}
	if !strings.Contains(lg.String(), "outbox lleno") {
		t.Fatal("sin log")
	}
	r = newRig(ws("idle-escalado", true))
	r.rt.Err = errors.New("apiserver no escala")
	if _, ok, err := r.svc.EnsureWarmReady(ctx, "t"); err == nil || ok { // P2
		t.Fatalf("ScaleUp tragado: ok=%v err=%v", ok, err)
	}
	r = newRig(ws("idle-escalado", true)) // CAS con error que no es conflicto: no se traga
	r.svc.State = failingState{MemState: r.state, casErr: errors.New("etcd caido")}
	if _, ok, err := r.svc.EnsureWarmReady(ctx, "t"); err == nil || ok {
		t.Fatalf("error de CAS tragado: ok=%v err=%v", ok, err)
	}
}

func TestEnsureProbeDownIsLogged(t *testing.T) {
	r := newRig(ws("ready", true))
	lg := logged(r)
	r.probe.Set(errors.New("redis no Ready"))
	if _, ok, err := r.svc.EnsureWarmReady(context.Background(), "tz"); ok || err != nil {
		t.Fatal(ok, err)
	}
	if !strings.Contains(lg.String(), "redis no Ready") || !strings.Contains(lg.String(), `"trace_id":"tz"`) {
		t.Fatalf("sin rastro de la sonda caida: %s", lg)
	}
}

// conflictThenReady simula que otra instancia despierta al warm entre nuestro Get y nuestro CAS.
type conflictThenReady struct{ *fakes.MemState }

func (c conflictThenReady) CompareAndSwap(context.Context, wm.WarmState, wm.WarmState) error {
	c.MemState.S = ws("ready", true)
	return wm.ErrStateConflict
}

func TestEnsureIdleCASConflictRereadsAndContinues(t *testing.T) {
	r := newRig(ws("idle-escalado", true))
	r.svc.State = conflictThenReady{r.state}
	w, ok, err := r.svc.EnsureWarmReady(context.Background(), "t")
	if err != nil || !ok || w.State != "ready" {
		t.Fatalf("el conflicto no debe salir como error si el warm quedo ready: %+v ok=%v err=%v", w, ok, err)
	}
}

func TestEnsureIdleConcurrentSingleScaleUp(t *testing.T) {
	r := newRig(ws("idle-escalado", true))
	var up sync.Mutex
	scaled := false
	r.rt.OnUp = func() { up.Lock(); scaled = true; up.Unlock() }
	r.svc.Probe = probeFunc(func() error {
		up.Lock()
		defer up.Unlock()
		if !scaled {
			return errors.New("escalado")
		}
		return nil
	})
	r.svc.State = slowState{r.state}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok, err := r.svc.EnsureWarmReady(context.Background(), "t"); err != nil || !ok {
				t.Errorf("ok=%v err=%v", ok, err)
			}
		}()
	}
	wg.Wait()
	if r.rt.Calls != 1 {
		t.Fatalf("ScaleUp=%d", r.rt.Calls)
	}
}

// ---- Dos instancias del servicio sobre un mismo almacen ----

func TestTwoServicesSharingStorePatchOnce(t *testing.T) {
	a := newRig(ws("ready", true))
	b := newRig(ws("ready", true))
	shared := slowState{a.state}
	a.svc.State, b.svc.State = shared, shared
	b.svc.Deployer = a.dep // mismo clúster
	var wg sync.WaitGroup
	for i, r := range []*rig{a, b} {
		wg.Add(1)
		go func(i int, r *rig) {
			defer wg.Done()
			_ = r.svc.Deploy(context.Background(), "r-"+string(rune('a'+i)), okArt, "t")
		}(i, r)
	}
	wg.Wait()
	if a.dep.Patches() != 1 {
		t.Fatalf("parches=%d: el CAS debe proteger entre instancias", a.dep.Patches())
	}
}

// ---- Reinicio: el estado del deploy es solo memoria y el warm persistido esta dirty ----

func TestDeployStateIsMemoryOnly(t *testing.T) {
	ctx := context.Background()
	r := newRig(ws("ready", true))
	mustDeploy(t, r)
	if d, ok, err := r.svc.DeployState(ctx, "r-1"); err != nil || !ok || d.State != "done" {
		t.Fatalf("%+v %v %v", d, ok, err)
	}
	// instancia nueva sobre el mismo warm persistido (dirty): no conoce el run y no despliega nada
	n := newRig(ws("dirty", false))
	n.state = r.state
	n.svc.State = r.state
	if _, ok, _ := n.svc.DeployState(ctx, "r-1"); ok {
		t.Fatal("tras un reinicio el deploy no se conoce")
	}
	if err := n.svc.Deploy(ctx, "r-2", okArt, "t"); !errors.Is(err, wm.ErrNotReady) || n.dep.Patches() != 0 {
		t.Fatalf("warm dirty: no se despliega: %v", err)
	}
}
