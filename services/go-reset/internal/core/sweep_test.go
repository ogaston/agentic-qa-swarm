package core_test

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/core"
	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/fakes"
	"pgregory.net/rapid"
)

// Barrido de propiedad (versión reducida del barrido del revisor de U2-T06, ronda 3).
//
//	4 estados del warm (+ ilegible) x 5 operaciones x bits de fallo x Put fallando x kill x Publish.
//
// Oráculo:
//  1. En el instante de cada Publish el mundo real está limpio (0 filas, 0 claves, app Ready con >= 1
//     réplica) y el almacén es coherente (ready, verificado, con baseline_version conocida).
//  2. En NINGÚN instante (antes de cada llamada a un puerto, y al terminar o tras un kill) el almacén dice
//     ready con 0 réplicas, ni ready con el mundo sucio, ni ready sin reset_verified, ni verified en dirty/cuarentena.
//  3. Una operación que termina sin error termina en ready+evento o en cuarentena; nunca a medias.
//  4. Tras retirar los fallos, un Reset siempre llega a ready verificado (nada queda atascado).

func init() {
	if f := flag.Lookup("rapid.checks"); f != nil && f.Value.String() == f.DefValue {
		_ = f.Value.Set("2000")
	}
}

var sweepSeeds = []string{core.StateReady, core.StateDirty, core.StateQuarantine, core.StateIdle, "ilegible"}
var sweepOps = []string{"Reset", "Rebuild", "Teardown", "Housekeeping", "IdleCheck"}

type scenario struct {
	seed, op                                                         string
	dirtyWorld                                                       bool
	restartFail, neverReady, cleanErr, dbSticky, flushErr            bool
	redisSticky, versionErr, versionEmpty, repsErr, scaleErr, rdyErr bool
	putMode                                                          int // 0 nunca, 1 solo la llamada putAt, 2 desde putAt en adelante
	putAt                                                            int
	publishFail                                                      bool
	killAt                                                           int // -1 nunca
	abandoned                                                        bool
}

type plainScenario scenario

func (s scenario) String() string { return fmt.Sprintf("%+v", plainScenario(s)) }

type world struct {
	sc      scenario
	viol    []string
	reps    int32
	ready   bool
	rows    int
	keys    int64
	st      core.Snapshot
	exists  bool
	puts    int
	calls   int
	events  int
	faults  bool // los fallos están activos
	clk     *fakes.Clock
	sess    map[string]core.Session
	killing bool
}

var errBoomW = errors.New("boom")
var errKillW = errors.New("kill")

func (w *world) bad(f string, a ...any) { w.viol = append(w.viol, fmt.Sprintf(f, a...)) }

// check evalúa las invariantes del instante actual.
func (w *world) check(when string) {
	if !w.exists || w.st.Unreadable {
		return
	}
	s := w.st
	if s.State == core.StateReady {
		if w.reps == 0 {
			w.bad("%s: ready con 0 réplicas", when)
		}
		if w.rows != 0 || w.keys != 0 {
			w.bad("%s: ready con el mundo sucio (filas=%d claves=%d)", when, w.rows, w.keys)
		}
		if !s.ResetVerified {
			w.bad("%s: ready sin reset_verified", when)
		}
	}
	if s.ResetVerified && s.State != core.StateReady && s.State != core.StateIdle {
		w.bad("%s: reset_verified=true en %s", when, s.State)
	}
}

func (w *world) enter(name string) {
	w.check("antes de " + name)
	if w.faults && w.calls == w.sc.killAt {
		w.calls++
		w.killing = true
		panic(errKillW)
	}
	w.calls++
}

// Puertos.
func (w *world) Replicas(context.Context) (int32, error) {
	w.enter("Replicas")
	if w.faults && w.sc.repsErr {
		return 0, errBoomW
	}
	return w.reps, nil
}
func (w *world) restart(name string) error {
	w.enter(name)
	if w.faults && w.sc.restartFail {
		return errBoomW
	}
	if w.reps < 1 {
		w.reps = 1
	}
	w.ready = !(w.faults && w.sc.neverReady)
	return nil
}
func (w *world) RestartApp(context.Context) error { return w.restart("RestartApp") }
func (w *world) Rebuild(context.Context) error    { return w.restart("Rebuild") }
func (w *world) Teardown(context.Context) error   { return w.restart("Teardown") }
func (w *world) ScaleApp(_ context.Context, n int32) error {
	w.enter("ScaleApp")
	if w.faults && w.sc.scaleErr {
		return errBoomW
	}
	w.reps, w.ready = n, n > 0
	w.check("después de ScaleApp")
	return nil
}
func (w *world) AppReady(context.Context) (bool, error) {
	w.enter("AppReady")
	if w.faults && w.sc.rdyErr {
		return false, errBoomW
	}
	return w.ready && w.reps > 0, nil
}
func (w *world) WarmPolicy(context.Context) (core.Policy, error) {
	return core.Policy{IdleScaleDownAfter: 30 * time.Minute}, nil
}
func (w *world) Clean(context.Context) error {
	w.enter("Clean")
	if w.faults && w.sc.cleanErr {
		return errBoomW
	}
	w.rows = 0
	if w.faults && w.sc.dbSticky {
		w.rows = 3
	}
	return nil
}
func (w *world) Diff(context.Context) (int, error) { w.enter("Diff"); return w.rows, nil }
func (w *world) Version(context.Context) (string, error) {
	w.enter("Version")
	if w.faults && w.sc.versionErr {
		return "", errBoomW
	}
	if w.faults && w.sc.versionEmpty {
		return "", nil
	}
	return "b1", nil
}
func (w *world) Flush(context.Context) error {
	w.enter("Flush")
	if w.faults && w.sc.flushErr {
		return errBoomW
	}
	w.keys = 0
	if w.faults && w.sc.redisSticky {
		w.keys = 5
	}
	return nil
}
func (w *world) DBSize(context.Context) (int64, error) { w.enter("DBSize"); return w.keys, nil }
func (w *world) Get(context.Context) (core.Snapshot, bool, error) {
	return w.st, w.exists, nil
}
func (w *world) Put(_ context.Context, s core.WarmState, at time.Time) error {
	w.enter("Put " + s.State)
	n := w.puts
	w.puts++
	if w.faults && ((w.sc.putMode == 1 && n == w.sc.putAt) || (w.sc.putMode == 2 && n >= w.sc.putAt)) {
		return errBoomW
	}
	w.st, w.exists = core.Snapshot{WarmState: s, UpdatedAt: at}, true
	w.check("después de Put " + s.State)
	return nil
}
func (w *world) Save(_ context.Context, s core.Session) error { w.sess[s.RunID] = s; return nil }
func (w *world) List(context.Context) ([]core.Session, error) {
	var out []core.Session
	for _, s := range w.sess {
		out = append(out, s)
	}
	return out, nil
}
func (w *world) Publish(_ context.Context, e core.Event) error {
	w.enter("Publish")
	if w.rows != 0 || w.keys != 0 || w.reps < 1 || !w.ready {
		w.bad("Publish %s con el mundo sucio: filas=%d claves=%d réplicas=%d ready=%v", e.Type, w.rows, w.keys, w.reps, w.ready)
	}
	if !w.exists || w.st.State != core.StateReady || !w.st.ResetVerified || w.st.BaselineVersion == "" || w.st.BaselineVersion == "desconocida" {
		w.bad("Publish %s con el almacén incoherente: %+v", e.Type, w.st.WarmState)
	}
	if w.faults && w.sc.publishFail {
		return errBoomW
	}
	w.events++
	return nil
}
func (w *world) Quarantine(context.Context, string, string, string) {}

type nopAlert struct{}

func (nopAlert) Quarantine(context.Context, string, string, string) {}

func (w *world) Now() time.Time                                 { return w.clk.Now() }
func (w *world) Sleep(c context.Context, d time.Duration) error { return w.clk.Sleep(c, d) }

func runScenario(sc scenario) []string {
	clk := fakes.NewClock(time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC))
	w := &world{sc: sc, reps: 1, ready: true, clk: clk, sess: map[string]core.Session{}}
	switch sc.seed {
	case core.StateIdle:
		w.reps, w.ready = 0, false
	}
	if sc.seed != core.StateReady && sc.dirtyWorld {
		w.rows, w.keys = 4, 6
	}
	base := core.WarmState{WarmID: "warm-1", State: sc.seed, BaselineVersion: "b1"}
	switch sc.seed {
	case core.StateReady:
		base.ResetVerified = true
	case core.StateIdle:
		base.ResetVerified = true
	case "ilegible":
		base = core.WarmState{State: core.StateDirty}
	}
	w.st, w.exists = core.Snapshot{WarmState: base, UpdatedAt: clk.Now(), Unreadable: sc.seed == "ilegible"}, true
	if sc.abandoned {
		w.sess["r-old"] = core.Session{RunID: "r-old", Status: core.SessionActive, LastActivity: clk.Now().Add(-48 * time.Hour)}
	}
	clk.Advance(time.Hour)
	svc := &core.Service{Kube: w, DB: w, Cache: w, State: w, Sessions: w, Events: w, Alert: nopAlert{}, Clock: w,
		Cfg: core.Config{DefaultWarmID: "warm-1", BaselineVersion: "b1", ReadyTimeout: 10 * time.Second,
			ReadyInterval: time.Second, Grace: 24 * time.Hour}}
	w.faults = true
	ctx := context.Background()
	var err error
	var rep core.HousekeepingReport
	var scaled bool
	func() {
		defer func() {
			if x := recover(); x != nil {
				if x != errKillW {
					panic(x)
				}
			}
		}()
		switch sc.op {
		case "Reset":
			_, err = svc.Reset(ctx, "r-1", "")
		case "Rebuild":
			_, err = svc.Rebuild(ctx, "rebuild", "")
		case "Teardown":
			_, err = svc.Rebuild(ctx, "teardown", "")
		case "Housekeeping":
			rep, err = svc.Housekeeping(ctx)
		case "IdleCheck":
			scaled, err = svc.IdleCheck(ctx)
		}
	}()
	w.check("al terminar")
	if !w.killing && err == nil && w.exists && !w.st.Unreadable {
		switch sc.op {
		case "Reset", "Rebuild", "Teardown":
			okReady := w.st.State == core.StateReady && w.events == 1
			if !okReady && w.st.State != core.StateQuarantine {
				w.bad("%s terminó sin error y sin cuarentena: estado=%+v eventos=%d", sc.op, w.st.WarmState, w.events)
			}
		case "Housekeeping":
			if rep.ResetRun && w.st.State != core.StateReady && w.st.State != core.StateQuarantine {
				w.bad("Housekeeping dejó el warm en %s", w.st.State)
			}
		case "IdleCheck":
			if scaled && (w.reps != 0 || w.st.State != core.StateIdle) {
				w.bad("IdleCheck escaló pero réplicas=%d estado=%s", w.reps, w.st.State)
			}
		}
	}
	// Recuperación: sin fallos, un Reset siempre llega a ready verificado.
	w.faults, w.killing = false, false
	w.viol = append([]string(nil), w.viol...)
	if res, rerr := svc.Reset(ctx, "r-rec", ""); rerr != nil || !res.Verified || w.st.State != core.StateReady || !w.st.ResetVerified {
		w.bad("recuperación: err=%v res=%+v estado=%+v", rerr, res.State, w.st.WarmState)
	}
	return w.viol
}

func drawScenario(t *rapid.T) scenario {
	b := func(n string) bool { return rapid.Bool().Draw(t, n) }
	sc := scenario{
		seed: rapid.SampledFrom(sweepSeeds).Draw(t, "seed"), op: rapid.SampledFrom(sweepOps).Draw(t, "op"),
		dirtyWorld: b("dirtyWorld"), restartFail: b("restartFail"), neverReady: b("neverReady"), cleanErr: b("cleanErr"),
		dbSticky: b("dbSticky"), flushErr: b("flushErr"), redisSticky: b("redisSticky"), versionErr: b("versionErr"),
		versionEmpty: b("versionEmpty"), repsErr: b("repsErr"), scaleErr: b("scaleErr"), rdyErr: b("rdyErr"),
		putMode: rapid.IntRange(0, 2).Draw(t, "putMode"), putAt: rapid.IntRange(0, 3).Draw(t, "putAt"),
		publishFail: b("publishFail"), killAt: rapid.IntRange(-1, 40).Draw(t, "killAt"), abandoned: b("abandoned"),
	}
	return sc
}

// Con la propiedad en verde, el barrido no encuentra ninguna violación de las 4 invariantes.
func TestSweepPropertyRapid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sc := drawScenario(t)
		if v := runScenario(sc); len(v) > 0 {
			t.Fatalf("%s\n%s", sc, strings.Join(v, "\n"))
		}
	})
}

// Enumeración determinista (reproducible sin semilla): estado x operación x Put x kill x Publish x un fallo.
func TestSweepPropertyEnumerated(t *testing.T) {
	setters := []func(*scenario){
		func(*scenario) {}, func(s *scenario) { s.restartFail = true }, func(s *scenario) { s.neverReady = true },
		func(s *scenario) { s.cleanErr = true }, func(s *scenario) { s.dbSticky = true }, func(s *scenario) { s.flushErr = true },
		func(s *scenario) { s.redisSticky = true }, func(s *scenario) { s.versionErr = true }, func(s *scenario) { s.versionEmpty = true },
		func(s *scenario) { s.repsErr = true }, func(s *scenario) { s.scaleErr = true }, func(s *scenario) { s.rdyErr = true },
	}
	puts := [][2]int{{0, 0}, {1, 0}, {1, 1}, {1, 2}, {1, 3}, {2, 0}, {2, 1}, {2, 2}, {2, 3}}
	n := 0
	for _, seed := range sweepSeeds {
		for _, op := range sweepOps {
			for _, pm := range puts {
				for _, pf := range []bool{false, true} {
					for kill := -1; kill <= 14; kill++ {
						for _, set := range setters {
							for _, ab := range []bool{false, true} {
								sc := scenario{seed: seed, op: op, dirtyWorld: true, putMode: pm[0], putAt: pm[1], publishFail: pf, killAt: kill, abandoned: ab}
								set(&sc)
								n++
								if v := runScenario(sc); len(v) > 0 {
									t.Fatalf("%s\n%s", sc, strings.Join(v, "\n"))
								}
							}
						}
					}
				}
			}
		}
	}
	t.Logf("%d escenarios sin violaciones", n)
}
