package core_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/core"
	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/fakes"
)

// F-11: IdleCheck nunca deja el almacén en ready con 0 réplicas, en ningún instante observable.

var errKill = errors.New("kill")

// idleRig envuelve Kube y State para observar el almacén y las réplicas antes y después de cada llamada
// que muta, y para "matar" el proceso (panic) en la llamada número killAt (0-based; -1 = nunca).
type idleRig struct {
	t      *testing.T
	b      *fakes.Bundle
	calls  int
	killAt int
	seen   []string
}

type watchKube struct {
	*fakes.Kube
	r *idleRig
}
type watchState struct {
	*fakes.State
	r *idleRig
}

func (r *idleRig) step(what string) {
	r.t.Helper()
	r.check("antes de " + what)
	if r.calls == r.killAt {
		r.calls++
		panic(errKill)
	}
	r.calls++
}

func (r *idleRig) check(when string) {
	r.t.Helper()
	reps, _ := r.b.Kube.Replicas(context.Background())
	st, ok, _ := r.b.State.Get(context.Background())
	r.seen = append(r.seen, fmt.Sprintf("%s: store=%s verified=%v reps=%d", when, st.State, st.ResetVerified, reps))
	if ok && st.State == core.StateReady && reps == 0 {
		r.t.Fatalf("(b') ready en el almacén pero 0 réplicas en el clúster %s\n%v", when, r.seen)
	}
}

func (w watchKube) ScaleApp(c context.Context, n int32) error {
	w.r.step("ScaleApp")
	err := w.Kube.ScaleApp(c, n)
	w.r.check("después de ScaleApp")
	return err
}

func (w watchState) Put(c context.Context, s core.WarmState, at time.Time) error {
	w.r.step("Put " + s.State)
	err := w.State.Put(c, s, at)
	w.r.check("después de Put " + s.State)
	return err
}

func newIdleRig(t *testing.T, killAt int) *idleRig {
	b := fakes.NewBundle()
	b.State.Snap = core.Snapshot{WarmState: core.WarmState{WarmID: "warm-1", State: core.StateReady, ResetVerified: true, BaselineVersion: "b1"}, UpdatedAt: b.Clock.Now()}
	b.Clock.Advance(time.Hour)
	r := &idleRig{t: t, b: b, killAt: killAt}
	b.Svc.Kube = watchKube{b.Kube, r}
	b.Svc.State = watchState{b.State, r}
	return r
}

func (r *idleRig) run() (ok bool, err error, killed bool) {
	defer func() {
		if x := recover(); x != nil {
			if x != errKill {
				panic(x)
			}
			killed = true
		}
	}()
	ok, err = r.b.Svc.IdleCheck(context.Background())
	return
}

func TestIdleCheckPutFailureNeverReadyWithZeroReplicas(t *testing.T) {
	for n := 0; n <= 3; n++ { // el Put falla desde la llamada n (0-based)
		t.Run(fmt.Sprintf("put%d", n), func(t *testing.T) {
			r := newIdleRig(t, -1)
			r.b.State.FailPutN = n + 1
			ok, err, _ := r.run()
			r.check("final")
			if ok && err != nil {
				t.Fatalf("ok y error a la vez")
			}
			if n == 0 && (ok || r.b.Kube.Reps != 1) {
				t.Fatalf("con el primer Put fallando no debe escalar: ok=%v reps=%d", ok, r.b.Kube.Reps)
			}
		})
	}
}

func TestIdleCheckNeverReadyWithZeroReplicasUnderKill(t *testing.T) {
	for k := 0; k <= 4; k++ { // kill en cada llamada a un puerto que muta
		t.Run(fmt.Sprintf("kill%d", k), func(t *testing.T) {
			r := newIdleRig(t, k)
			r.run()
			r.check("tras el kill")
		})
	}
}

func TestIdleCheckHappyPathOrder(t *testing.T) {
	r := newIdleRig(t, -1)
	ok, err, _ := r.run()
	if !ok || err != nil || r.b.Kube.Reps != 0 {
		t.Fatalf("ok=%v err=%v reps=%d", ok, err, r.b.Kube.Reps)
	}
	if st := state(t, r.b); st.State != core.StateIdle {
		t.Fatalf("%+v", st)
	}
}

// F-11: si el escalado falla tras escribir idle-escalado, se compensa.
func TestIdleCheckCompensatesWhenScaleFails(t *testing.T) {
	t.Run("sigue Ready con 1 réplica: vuelve a ready verificado", func(t *testing.T) {
		r := newIdleRig(t, -1)
		r.b.Kube.ScaleErr = fakes.ErrBoom
		ok, err, _ := r.run()
		r.check("final")
		st := state(t, r.b)
		if ok || err == nil || st.State != core.StateReady || !st.ResetVerified || r.b.Kube.Reps != 1 {
			t.Fatalf("ok=%v err=%v st=%+v reps=%d\n%v", ok, err, st, r.b.Kube.Reps, r.seen)
		}
	})
	t.Run("no queda Ready: queda dirty", func(t *testing.T) {
		r := newIdleRig(t, -1)
		r.b.Kube.ScaleErr = fakes.ErrBoom
		r.b.Kube.Ready = false
		ok, err, _ := r.run()
		r.check("final")
		st := state(t, r.b)
		if ok || err == nil || st.State != core.StateDirty || st.ResetVerified {
			t.Fatalf("ok=%v err=%v st=%+v\n%v", ok, err, st, r.seen)
		}
	})
	t.Run("la compensación también falla: queda idle-escalado, nunca ready con 0", func(t *testing.T) {
		r := newIdleRig(t, -1)
		r.b.Kube.ScaleErr = fakes.ErrBoom
		r.b.State.FailPutN = 2 // el Put idle-escalado pasa; la compensación falla
		_, err, _ := r.run()
		r.check("final")
		if err == nil || state(t, r.b).State != core.StateIdle {
			t.Fatalf("err=%v st=%+v", err, state(t, r.b))
		}
	})
}

// F-12 (b): idleScaleDownAfter <= 0 no escala (fail-closed), con la política en el puerto.
func TestWarmPolicyNonPositiveCoreDoesNotScale(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Hour} {
		t.Run(d.String(), func(t *testing.T) {
			r := newIdleRig(t, -1)
			r.b.Kube.Policy.IdleScaleDownAfter = d
			ok, err, _ := r.run()
			if ok || err == nil || r.b.Kube.Reps != 1 || state(t, r.b).State != core.StateReady {
				t.Fatalf("ok=%v err=%v reps=%d st=%+v", ok, err, r.b.Kube.Reps, state(t, r.b))
			}
		})
	}
}

// F-12 (g): con el Put de cuarentena fallando, el Alerter igualmente recibe el aviso.
func TestQuarantinePutFailsStillAlerts(t *testing.T) {
	b := fakes.NewBundle()
	b.DB.CleanErr = fakes.ErrBoom
	b.State.FailPutN = 2 // el Put dirty inicial pasa; el de cuarentena falla
	res, err := b.Svc.Reset(ctx, "r-1", "")
	if err == nil {
		t.Fatalf("debía devolver el error del Put: res=%+v", res)
	}
	if b.Alert.Count != 1 {
		t.Fatalf("avisos=%d", b.Alert.Count)
	}
	if st := state(t, b); st.State != core.StateDirty || st.ResetVerified {
		t.Fatalf("estado %+v", st)
	}
	if len(b.Events.Types()) != 0 {
		t.Fatal("evento publicado")
	}
}
