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
	r.jobs.Outcome = failN(100)
	_ = r.svc.Deploy(ctx, "r-1", okArt, "t")
	if _, err := r.svc.InferSurface(ctx, "r-1", "t"); !errors.Is(err, wm.ErrDeployNotDone) { // deploy fallido
		t.Fatalf("deploy fallido: %v", err)
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

// ---- Dos instancias del servicio sobre un mismo almacen (reemplaza la justificacion de «defensa en profundidad») ----

func TestTwoServicesSharingStoreCreateOneJob(t *testing.T) {
	a := newRig(ws("ready", true))
	b := newRig(ws("ready", true))
	shared := slowState{a.state}
	a.svc.State, b.svc.State = shared, shared
	b.svc.Jobs = a.jobs // mismo clúster
	var wg sync.WaitGroup
	for i, r := range []*rig{a, b} {
		wg.Add(1)
		go func(i int, r *rig) {
			defer wg.Done()
			_ = r.svc.Deploy(context.Background(), "r-"+string(rune('a'+i)), okArt, "t")
		}(i, r)
	}
	wg.Wait()
	if a.jobs.Count() != 1 {
		t.Fatalf("Jobs=%d: el CAS debe proteger entre instancias", a.jobs.Count())
	}
}

// ---- Estado derivado de Jobs y deploys huerfanos (F-03) ----

func createJob(t *testing.T, j *fakes.FakeJobs, run string, n int, art wm.Artifact) {
	t.Helper()
	m, err := wm.BuildDeployJob(jcfg, run, n, art)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Create(context.Background(), m); err != nil {
		t.Fatal(err)
	}
}

func TestDeployStateDerivedFromJobsAfterRestart(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		name  string
		outs  []wm.JobPhase
		state string
		att   int
	}{{"en vuelo", []wm.JobPhase{wm.JobFailed, wm.JobPending}, "pending", 2}, {"terminado", []wm.JobPhase{wm.JobFailed, wm.JobSucceeded}, "done", 2}, {"fallido", []wm.JobPhase{wm.JobFailed, wm.JobFailed, wm.JobFailed}, "failed", 3}} {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(ws("dirty", false))
			i := 0
			r.jobs.Outcome = func(int) wm.JobPhase { p := c.outs[i]; i++; return p }
			for n := range c.outs {
				createJob(t, r.jobs, "r-1", n, okArt)
			}
			st, ok, err := r.svc.DeployState(ctx, "r-1") // memoria vacia: instancia recien arrancada
			if err != nil || !ok || st.State != c.state || st.Attempts != c.att {
				t.Fatalf("%+v ok=%v err=%v", st, ok, err)
			}
			if _, ok, _ := r.svc.DeployState(ctx, "otro"); ok {
				t.Fatal("run desconocido debe ser 404")
			}
		})
	}
	r := newRig(ws("dirty", false))
	r.jobs.ListErr = errors.New("apiserver caido")
	if _, _, err := r.svc.DeployState(ctx, "r-1"); err == nil {
		t.Fatal("el error de lectura no debe confundirse con 'no existe'")
	}
}

func TestResolveOrphans(t *testing.T) {
	ctx := context.Background()
	r := newRig(ws("dirty", false))
	phases := map[string]wm.JobPhase{"r-pend": wm.JobPending, "r-done": wm.JobSucceeded, "r-fail": wm.JobFailed, "r-known": wm.JobPending}
	cur := ""
	r.jobs.Outcome = func(int) wm.JobPhase { return phases[cur] }
	for run := range phases {
		cur = run
		createJob(t, r.jobs, run, 0, okArt)
	}
	r.svc.Cfg.Job = jcfg
	r.svc.TrackKnownForTest("r-known")
	if err := r.svc.ResolveOrphans(ctx, "tr"); err != nil {
		t.Fatal(err)
	}
	byRun := map[string]wm.Event{}
	for _, e := range r.pub.Events {
		byRun[e.Data["run_id"].(string)] = e
		if err := validate(t, "events/deploy.schema.json", e); err != nil {
			t.Fatal(err)
		}
	}
	if byRun["r-pend"].Type != "deploy.failed" || byRun["r-done"].Type != "deploy.done" || byRun["r-fail"].Type != "deploy.failed" || len(byRun) != 3 {
		t.Fatalf("eventos: %v", r.types())
	}
	if len(r.al.Calls) != 1 || !strings.Contains(r.al.Calls[0], "r-pend") {
		t.Fatalf("handoff solo para el deploy en vuelo huerfano: %v", r.al.Calls)
	}
	if d, _, _ := r.svc.DeployState(ctx, "r-pend"); d.State != "failed" || d.Reason == "" {
		t.Fatalf("%+v", d)
	}
	// idempotencia: el event_id de deploy.done coincide con el que habria emitido el deploy original
	if byRun["r-done"].EventID != wm.EventID("deploy.done/r-done") {
		t.Fatal("event_id no determinista")
	}
	r.jobs.ListErr = errors.New("sin acceso")
	if err := r.svc.ResolveOrphans(ctx, "tr"); err == nil {
		t.Fatal("error de lectura tragado")
	}
}

var _ = time.Second
