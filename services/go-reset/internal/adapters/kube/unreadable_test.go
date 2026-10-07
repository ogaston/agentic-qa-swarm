package kube_test

import (
	"context"
	"errors"
	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/fakes"
	"k8s.io/apimachinery/pkg/runtime"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/adapters/kube"
	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/core"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// F-10: contenidos ilegibles del ConfigMap warm-state, con kube.Client real (Kube y StateStore) sobre kubernetes/fake.
var unreadableContents = map[string]map[string]string{
	"vacio":              {"state": ""},
	"ausente":            {"updated_at": "2026-10-06T12:00:00Z"},
	"no JSON":            {"state": "no-es-json{", "updated_at": "2026-10-06T12:00:00Z"},
	"no objeto":          {"state": "[1,2,3]", "updated_at": "2026-10-06T12:00:00Z"},
	"null":               {"state": "null"},
	"estado desconocido": {"state": `{"warm_id":"warm-1","state":"???","reset_verified":true}`},
}

// corruptStack deja el warm con un warm-state ilegible y kube.Client real como almacén.
func corruptStack(t *testing.T, data map[string]string) (*core.Service, *kube.Client, *fakes.Bundle) {
	t.Helper()
	svc, b, cs := stack(t, core.StateReady, 1)
	k := svc.Kube.(*kube.Client)
	svc.State = k
	_, err := cs.CoreV1().ConfigMaps("aqs-test").Create(ctx,
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "warm-state", Namespace: "aqs-test"}, Data: data}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return svc, k, b
}

func TestSweepCorruptWarmStateStuck(t *testing.T) {
	for cname, data := range unreadableContents {
		ops := map[string]func(*core.Service) (core.Result, error){
			"Reset":    func(s *core.Service) (core.Result, error) { return s.Reset(ctx, "r-1", "") },
			"Rebuild":  func(s *core.Service) (core.Result, error) { return s.Rebuild(ctx, "rebuild", "") },
			"Teardown": func(s *core.Service) (core.Result, error) { return s.Rebuild(ctx, "teardown", "") },
		}
		for oname, op := range ops {
			t.Run(cname+"/"+oname, func(t *testing.T) {
				svc, k, _ := corruptStack(t, data)
				res, err := op(svc)
				if err != nil || !res.Verified {
					t.Fatalf("res=%+v err=%v", res, err)
				}
				got, ok, err := k.Get(ctx)
				if err != nil || !ok || got.Unreadable || got.State != core.StateReady || !got.ResetVerified {
					t.Fatalf("estado final %+v ok=%v err=%v", got, ok, err)
				}
			})
		}
		t.Run(cname+"/Housekeeping", func(t *testing.T) {
			svc, _, b := corruptStack(t, data)
			_ = b.Sessions.Save(ctx, core.Session{RunID: "r-9", Status: core.SessionActive, LastActivity: b.Clock.Now().Add(-48 * time.Hour)})
			rep, err := svc.Housekeeping(ctx)
			if err != nil || rep.WarmState != core.StateReady {
				t.Fatalf("rep=%+v err=%v", rep, err)
			}
		})
	}
}

// Con estado ilegible, Housekeeping sin sesiones abandonadas también lo repara (desconocido = dirty).
func TestUnreadableStateHousekeepingRepairsWithoutSessions(t *testing.T) {
	svc, k, _ := corruptStack(t, map[string]string{"state": ""})
	rep, err := svc.Housekeeping(ctx)
	got, _, _ := k.Get(ctx)
	if err != nil || !rep.ResetRun || got.State != core.StateReady {
		t.Fatalf("rep=%+v err=%v got=%+v", rep, err, got)
	}
}

// IdleCheck con estado ilegible no escala (fail-closed), aunque haya pasado el umbral.
func TestUnreadableStateIdleCheckDoesNotScale(t *testing.T) {
	for cname, data := range unreadableContents {
		t.Run(cname, func(t *testing.T) {
			svc, k, b := corruptStack(t, data)
			b.Clock.Advance(48 * time.Hour)
			ok, err := svc.IdleCheck(ctx)
			if ok {
				t.Fatalf("escaló con estado ilegible (err=%v)", err)
			}
			if n, _ := k.Replicas(ctx); n != 1 {
				t.Fatalf("réplicas=%d", n)
			}
		})
	}
}

type countMetrics struct {
	core.NopMetrics
	unreadable int
}

func (m *countMetrics) StateUnreadable() { m.unreadable++ }

func TestUnreadableStateCountsMetric(t *testing.T) {
	svc, _, _ := corruptStack(t, map[string]string{"state": "x"})
	m := &countMetrics{}
	svc.Metrics = m
	if _, err := svc.Reset(ctx, "r-1", ""); err != nil {
		t.Fatal(err)
	}
	if m.unreadable < 1 {
		t.Fatal("aqs_warm_state_unreadable_total no se incrementó")
	}
}

// Un error transitorio de la API NO es ilegible: se devuelve como error y no se sobrescribe nada.
func TestUnreadableStateTransientAPIErrorIsNotUnreadable(t *testing.T) {
	cs := fake.NewSimpleClientset()
	cs.PrependReactor("get", "configmaps", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("etcd timeout")
	})
	k, _ := kube.New(cs, "aqs-test", nil)
	if _, ok, err := k.Get(ctx); err == nil || ok {
		t.Fatalf("ok=%v err=%v, debía ser error", ok, err)
	}
	svc, b, _ := stack(t, core.StateReady, 1)
	svc.State = &failingGetState{err: errors.New("etcd timeout")}
	if _, err := svc.Reset(ctx, "r-1", ""); err == nil {
		t.Fatal("Reset siguió adelante con la API fallando")
	}
	if len(b.Events.Types()) != 0 {
		t.Fatal("evento con la API fallando")
	}
}

type failingGetState struct{ err error }

func (f *failingGetState) Get(c context.Context) (core.Snapshot, bool, error) {
	return core.Snapshot{}, false, f.err
}
func (f *failingGetState) Put(context.Context, core.WarmState, time.Time) error {
	return errors.New("no debe escribir")
}

// F-12 (b): la política con idleScaleDownAfter <= 0 se rechaza y IdleCheck no escala un warm recién verificado.
func TestWarmPolicyNonPositiveKubeRejectedAndNoScale(t *testing.T) {
	for _, v := range []string{"0s", "-1h"} {
		t.Run(v, func(t *testing.T) {
			svc, b, cs := stack(t, core.StateReady, 1)
			k := svc.Kube.(*kube.Client)
			pol, _ := cs.CoreV1().ConfigMaps("aqs-test").Get(ctx, "warm-policy", metav1.GetOptions{})
			pol.Data["idleScaleDownAfter"] = v
			_, _ = cs.CoreV1().ConfigMaps("aqs-test").Update(ctx, pol, metav1.UpdateOptions{})
			if _, err := k.WarmPolicy(ctx); err == nil {
				t.Fatal("WarmPolicy aceptó un valor <= 0")
			}
			b.State.Snap.UpdatedAt = b.Clock.Now()
			b.Clock.Advance(time.Minute)
			if ok, _ := svc.IdleCheck(ctx); ok {
				t.Fatal("escaló")
			}
			if n, _ := k.Replicas(ctx); n != 1 {
				t.Fatalf("réplicas=%d", n)
			}
		})
	}
}
