package core_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/core"
	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/fakes"
	"pgregory.net/rapid"
)

var ctx = context.Background()

func state(t *testing.T, b *fakes.Bundle) core.WarmState {
	t.Helper()
	s, ok, err := b.State.Get(ctx)
	if err != nil || !ok {
		t.Fatalf("estado: %v %v", ok, err)
	}
	return s.WarmState
}

func TestResetHappyPathReadsBack(t *testing.T) {
	b := fakes.NewBundle()
	b.DB.Rows, b.Cache.Keys = 7, 9
	res, err := b.Svc.Reset(ctx, "r-1", "")
	if err != nil || !res.Verified {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	st := state(t, b)
	if st.State != core.StateReady || !st.ResetVerified {
		t.Fatalf("estado %+v", st)
	}
	if got := b.Events.Types(); len(got) != 1 || got[0] != "reset.verified" {
		t.Fatalf("eventos %v", got)
	}
	if len(res.Checks) < 3 {
		t.Fatalf("checks %v", res.Checks)
	}
	if b.Kube.Calls[0] != "restart" || b.DB.CleanCall != 1 {
		t.Fatalf("orden %v", b.Kube.Calls)
	}
}

func TestResetFailsRestartStep(t *testing.T) {
	b := fakes.NewBundle()
	b.Kube.RestartErr = fakes.ErrBoom
	res, _ := b.Svc.Reset(ctx, "r-1", "")
	assertQuarantined(t, b, res)
}

func TestResetFailsDBStep(t *testing.T) {
	b := fakes.NewBundle()
	b.DB.CleanErr = fakes.ErrBoom
	res, _ := b.Svc.Reset(ctx, "r-1", "")
	assertQuarantined(t, b, res)
}

func TestResetFailsCacheStep(t *testing.T) {
	b := fakes.NewBundle()
	b.Cache.FlushErr = fakes.ErrBoom
	res, _ := b.Svc.Reset(ctx, "r-1", "")
	assertQuarantined(t, b, res)
}

func assertQuarantined(t *testing.T, b *fakes.Bundle, res core.Result) {
	t.Helper()
	if res.Verified {
		t.Fatal("no debía verificar")
	}
	st := state(t, b)
	if st.State != core.StateQuarantine || st.ResetVerified {
		t.Fatalf("estado %+v", st)
	}
	if n := len(b.Events.Got); n != 0 {
		t.Fatalf("hay %d eventos: %v", n, b.Events.Types())
	}
	if b.Alert.Count != 1 {
		t.Fatalf("alertas=%d", b.Alert.Count)
	}
}

func TestResetRetriesExactlyOnce(t *testing.T) {
	b := fakes.NewBundle()
	b.DB.CleanErr = fakes.ErrBoom
	res, _ := b.Svc.Reset(ctx, "r-1", "")
	if res.Attempts != 2 || b.DB.CleanCall != 2 {
		t.Fatalf("attempts=%d clean=%d", res.Attempts, b.DB.CleanCall)
	}
}

func TestResetNoEventWithoutVerification(t *testing.T) {
	b := fakes.NewBundle()
	b.DB.Sticky = 3
	res, _ := b.Svc.Reset(ctx, "r-1", "")
	assertQuarantined(t, b, res)
}

func TestResetQuarantineLeavesOnlyWithVerifiedReset(t *testing.T) {
	b := fakes.NewBundle()
	b.DB.CleanErr = fakes.ErrBoom
	_, _ = b.Svc.Reset(ctx, "r-1", "")
	b.DB.CleanErr = nil
	if st := state(t, b); st.State != core.StateQuarantine {
		t.Fatal(st)
	}
	res, _ := b.Svc.Reset(ctx, "r-1", "")
	if !res.Verified || state(t, b).State != core.StateReady {
		t.Fatalf("%+v", res)
	}
}

func TestResetRecoversFromIdleScaledWarm(t *testing.T) {
	b := fakes.NewBundle()
	b.Kube.Reps, b.Kube.Ready = 0, false
	res, _ := b.Svc.Reset(ctx, "r-1", "")
	if !res.Verified || b.Kube.Reps != 1 {
		t.Fatalf("%+v reps=%d", res, b.Kube.Reps)
	}
}

func TestResetRejectsEmptyRun(t *testing.T) {
	if _, err := fakes.NewBundle().Svc.Reset(ctx, "", ""); err == nil {
		t.Fatal("run_id vacío aceptado")
	}
}

func TestResetStatePutErrorNoEvent(t *testing.T) {
	b := fakes.NewBundle()
	b.State.PutErr = fakes.ErrBoom
	if _, err := b.Svc.Reset(ctx, "r-1", ""); err == nil {
		t.Fatal("esperaba error")
	}
	if len(b.Events.Got) != 0 {
		t.Fatal("evento sin estado persistido")
	}
}

// Verificación: los pasos devuelven éxito pero el estado real queda sucio.
func TestVerifyDirtyDB(t *testing.T) {
	b := fakes.NewBundle()
	b.DB.Sticky, b.DB.Rows = 2, 2
	if _, err := b.Svc.Verify(ctx); err == nil {
		t.Fatal("DB sucia pasó la verificación")
	}
	res, _ := b.Svc.Reset(ctx, "r-1", "")
	assertQuarantined(t, b, res)
}

func TestVerifyDirtyCache(t *testing.T) {
	b := fakes.NewBundle()
	b.Cache.Sticky = 5
	res, _ := b.Svc.Reset(ctx, "r-1", "")
	assertQuarantined(t, b, res)
}

func TestVerifyPodNotReady(t *testing.T) {
	b := fakes.NewBundle()
	b.Kube.ReadyAfterRestart = false
	res, _ := b.Svc.Reset(ctx, "r-1", "")
	assertQuarantined(t, b, res)
}

func TestVerifyAllClean(t *testing.T) {
	b := fakes.NewBundle()
	checks, err := b.Svc.Verify(ctx)
	if err != nil || len(checks) != 3 {
		t.Fatalf("%v %v", checks, err)
	}
}

func TestIdleCheckScalesAfterThreshold(t *testing.T) {
	b := fakes.NewBundle()
	_, _ = b.Svc.Reset(ctx, "r-1", "")
	b.Clock.Advance(29 * time.Minute)
	if ok, _ := b.Svc.IdleCheck(ctx); ok {
		t.Fatal("escaló antes de tiempo")
	}
	b.Clock.Advance(2 * time.Minute)
	ok, err := b.Svc.IdleCheck(ctx)
	if err != nil || !ok || b.Kube.Reps != 0 || state(t, b).State != core.StateIdle {
		t.Fatalf("ok=%v err=%v reps=%d", ok, err, b.Kube.Reps)
	}
}

func TestIdleCheckNeverScalesDirty(t *testing.T) {
	b := fakes.NewBundle() // arranca dirty
	b.Clock.Advance(5 * time.Hour)
	if ok, _ := b.Svc.IdleCheck(ctx); ok || b.Kube.Reps != 1 {
		t.Fatal("escaló un warm dirty")
	}
}

func TestIdleCheckNeverScalesWithActiveRun(t *testing.T) {
	b := fakes.NewBundle()
	_, _ = b.Svc.Reset(ctx, "r-1", "")
	_ = b.Sessions.Save(ctx, core.Session{RunID: "r-2", Status: core.SessionActive, LastActivity: b.Clock.Now()})
	b.Clock.Advance(5 * time.Hour)
	if ok, _ := b.Svc.IdleCheck(ctx); ok || b.Kube.Reps != 1 {
		t.Fatal("escaló con corrida activa")
	}
}

func TestIdleCheckNeverScalesQuarantine(t *testing.T) {
	b := fakes.NewBundle()
	b.DB.CleanErr = fakes.ErrBoom
	_, _ = b.Svc.Reset(ctx, "r-1", "")
	b.Clock.Advance(5 * time.Hour)
	if ok, _ := b.Svc.IdleCheck(ctx); ok {
		t.Fatal("escaló cuarentena")
	}
}

func TestHousekeepingBeforeGraceClosesNothing(t *testing.T) {
	b := fakes.NewBundle()
	_ = b.Sessions.Save(ctx, core.Session{RunID: "r-1", Status: core.SessionActive, LastActivity: b.Clock.Now()})
	b.Clock.Advance(23*time.Hour + 59*time.Minute)
	rep, err := b.Svc.Housekeeping(ctx)
	if err != nil || len(rep.Closed) != 0 || rep.ResetRun {
		t.Fatalf("%+v %v", rep, err)
	}
	if b.DB.CleanCall != 0 {
		t.Fatal("reseteó una corrida vigente")
	}
}

func TestHousekeepingAfterGraceClosesAndResets(t *testing.T) {
	b := fakes.NewBundle()
	plan := json.RawMessage(`{"flows":["f1"]}`)
	_ = b.Sessions.Save(ctx, core.Session{RunID: "r-1", Status: core.SessionActive, LastActivity: b.Clock.Now(),
		Plan: plan, State: "executing"})
	b.Clock.Advance(24*time.Hour + time.Minute)
	rep, err := b.Svc.Housekeeping(ctx)
	if err != nil || len(rep.Closed) != 1 {
		t.Fatalf("%+v %v", rep, err)
	}
	if st := state(t, b); st.State == core.StateDirty || st.State != core.StateReady {
		t.Fatalf("estado %+v", st)
	}
}

func TestIncompleteSessionPersistedAndReread(t *testing.T) {
	b := fakes.NewBundle()
	plan := json.RawMessage(`{"flows":["f1"]}`)
	_ = b.Sessions.Save(ctx, core.Session{RunID: "r-1", Status: core.SessionActive, LastActivity: b.Clock.Now(),
		Plan: plan, State: "executing"})
	b.Clock.Advance(25 * time.Hour)
	if _, err := b.Svc.Housekeeping(ctx); err != nil {
		t.Fatal(err)
	}
	l, _ := b.Sessions.List(ctx)
	if len(l) != 1 || l[0].Status != core.SessionIncomplete || string(l[0].Plan) != string(plan) ||
		l[0].State != "executing" || l[0].WarmState != core.StateReady {
		t.Fatalf("%+v", l)
	}
}

func TestHousekeepingQuarantineWhenResetFails(t *testing.T) {
	b := fakes.NewBundle()
	b.DB.Sticky = 1
	_ = b.Sessions.Save(ctx, core.Session{RunID: "r-1", Status: core.SessionActive, LastActivity: b.Clock.Now()})
	b.Clock.Advance(25 * time.Hour)
	rep, _ := b.Svc.Housekeeping(ctx)
	if state(t, b).State != core.StateQuarantine || len(rep.Closed) != 1 {
		t.Fatalf("%+v", rep)
	}
	l, _ := b.Sessions.List(ctx)
	if l[0].Status != core.SessionIncomplete || l[0].WarmState != core.StateQuarantine {
		t.Fatalf("%+v", l[0])
	}
}

func TestHousekeepingNeverLeavesWarmDirty(t *testing.T) {
	b := fakes.NewBundle() // dirty, sin sesiones activas (corrida terminada)
	if _, err := b.Svc.Housekeeping(ctx); err != nil {
		t.Fatal(err)
	}
	if state(t, b).State == core.StateDirty {
		t.Fatal("quedó dirty")
	}
}

func TestHousekeepingExactGraceBoundaryDoesNotClose(t *testing.T) {
	b := fakes.NewBundle()
	_ = b.Sessions.Save(ctx, core.Session{RunID: "r-1", Status: core.SessionActive, LastActivity: b.Clock.Now()})
	b.Clock.Advance(24 * time.Hour)
	if rep, _ := b.Svc.Housekeeping(ctx); len(rep.Closed) != 0 {
		t.Fatal("cerró en el borde exacto")
	}
}

func TestHousekeepingFreshRunProtectsWarm(t *testing.T) {
	b := fakes.NewBundle()
	_ = b.Sessions.Save(ctx, core.Session{RunID: "old", Status: core.SessionActive, LastActivity: b.Clock.Now().Add(-48 * time.Hour)})
	_ = b.Sessions.Save(ctx, core.Session{RunID: "new", Status: core.SessionActive, LastActivity: b.Clock.Now()})
	rep, _ := b.Svc.Housekeeping(ctx)
	if rep.ResetRun || len(rep.Closed) != 1 || rep.Closed[0] != "old" || b.DB.CleanCall != 0 {
		t.Fatalf("%+v", rep)
	}
}

func TestRebuildPublishesTeardownVerifiedWithMode(t *testing.T) {
	b := fakes.NewBundle()
	res, err := b.Svc.Rebuild(ctx, "rebuild", "")
	if err != nil || !res.Verified {
		t.Fatalf("%+v %v", res, err)
	}
	ev := b.Events.Got[0]
	if ev.Type != "teardown.verified" || ev.Data["mode"] != "rebuild" || ev.Data["verified"] != true {
		t.Fatalf("%+v", ev)
	}
	if b.Kube.Calls[0] != "rebuild" {
		t.Fatal(b.Kube.Calls)
	}
}

func TestTeardownPublishesTeardownVerifiedWithMode(t *testing.T) {
	b := fakes.NewBundle()
	res, _ := b.Svc.Rebuild(ctx, "teardown", "")
	if !res.Verified || b.Events.Got[0].Data["mode"] != "teardown" || b.Kube.Calls[0] != "teardown" {
		t.Fatalf("%+v", b.Events.Got)
	}
}

func TestRebuildFailedVerificationQuarantinesWithoutEvent(t *testing.T) {
	b := fakes.NewBundle()
	b.Cache.Sticky = 4
	res, _ := b.Svc.Rebuild(ctx, "rebuild", "")
	assertQuarantined(t, b, res)
}

func TestTeardownFailedVerificationQuarantinesWithoutEvent(t *testing.T) {
	b := fakes.NewBundle()
	b.Kube.ReadyAfterRestart = false
	res, _ := b.Svc.Rebuild(ctx, "teardown", "")
	assertQuarantined(t, b, res)
}

func TestRebuildUnknownMode(t *testing.T) {
	if _, err := fakes.NewBundle().Svc.Rebuild(ctx, "x", ""); err == nil {
		t.Fatal("modo inválido aceptado")
	}
}

func TestEventIDDeterministicUUIDv5(t *testing.T) {
	a, b := core.EventID("k"), core.EventID("k")
	if a != b || a == core.EventID("j") || a[14] != '5' {
		t.Fatal(a)
	}
}

// Propiedad: reset.verified se publica sii todas las comprobaciones leyeron el estado limpio;
// ready nunca con reset_verified=false; siempre termina.
func TestResetPropertyEventIffClean(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		b := fakes.NewBundle()
		if rapid.Bool().Draw(t, "restartErr") {
			b.Kube.RestartErr = fakes.ErrBoom
		}
		if rapid.Bool().Draw(t, "dbErr") {
			b.DB.CleanErr = fakes.ErrBoom
		}
		if rapid.Bool().Draw(t, "cacheErr") {
			b.Cache.FlushErr = fakes.ErrBoom
		}
		b.DB.Sticky = rapid.IntRange(0, 2).Draw(t, "dirtyRows")
		b.Cache.Sticky = int64(rapid.IntRange(0, 2).Draw(t, "dirtyKeys"))
		b.Kube.ReadyAfterRestart = rapid.Bool().Draw(t, "ready")
		b.DB.Rows, b.Cache.Keys = 5, 5
		clean := b.Kube.RestartErr == nil && b.DB.CleanErr == nil && b.Cache.FlushErr == nil &&
			b.DB.Sticky == 0 && b.Cache.Sticky == 0 && b.Kube.ReadyAfterRestart
		res, err := b.Svc.Reset(ctx, "r-p", "")
		if err != nil {
			t.Fatal(err)
		}
		published := len(b.Events.Got) == 1
		if published != clean || res.Verified != clean {
			t.Fatalf("clean=%v published=%v verified=%v", clean, published, res.Verified)
		}
		s, _, _ := b.State.Get(ctx)
		if s.State == core.StateReady && !s.ResetVerified {
			t.Fatal("ready sin reset_verified")
		}
		if (s.State == core.StateReady) != clean {
			t.Fatalf("estado %v clean=%v", s.State, clean)
		}
	})
}
