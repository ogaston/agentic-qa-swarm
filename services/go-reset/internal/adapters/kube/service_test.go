package kube_test

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/core"
	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/fakes"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// controllerClock simula el controlador de Deployments: cada Sleep, el rollout avanza hasta
// el estado deseado (pods == réplicas, todos Ready, status al día). kubernetes/fake no lo hace solo.
type controllerClock struct {
	*fakes.Clock
	cs     *fake.Clientset
	slow   int // Sleeps que el controlador tarda en reaccionar
	sleeps int // Sleeps totales (sondeos fallidos de AppReady)
}

func (c *controllerClock) Sleep(ctx context.Context, d time.Duration) error {
	c.sleeps++
	if c.slow > 0 {
		c.slow--
		return c.Clock.Sleep(ctx, d)
	}
	dep, _ := c.cs.AppsV1().Deployments("aqs-test").Get(ctx, "warm-app", metav1.GetOptions{})
	want := int32(1)
	if dep.Spec.Replicas != nil {
		want = *dep.Spec.Replicas
	}
	pods := c.cs.CoreV1().Pods("aqs-test")
	pl, _ := pods.List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=warm-app"})
	have := int32(len(pl.Items))
	for i := have; i > want; i-- {
		_ = pods.Delete(ctx, pl.Items[i-1].Name, metav1.DeleteOptions{})
	}
	for i := have; i < want; i++ {
		_, _ = pods.Create(ctx, pod(fmt.Sprintf("p-%d-%d", c.Now().UnixNano(), i), true, corev1.PodRunning), metav1.CreateOptions{})
	}
	dep.Status = appsv1.DeploymentStatus{ObservedGeneration: dep.Generation, Replicas: want, UpdatedReplicas: want, AvailableReplicas: want}
	_, _ = c.cs.AppsV1().Deployments("aqs-test").UpdateStatus(ctx, dep, metav1.UpdateOptions{})
	return c.Clock.Sleep(ctx, d)
}

// stack arma Service + kube.Client real sobre kubernetes/fake con el warm en el estado y réplicas dados.
func stack(t *testing.T, state string, reps int32) (*core.Service, *fakes.Bundle, *fake.Clientset) {
	t.Helper()
	cs, k := seed()
	d, _ := cs.AppsV1().Deployments("aqs-test").Get(ctx, "warm-app", metav1.GetOptions{})
	d.Spec.Replicas = &reps
	_, _ = cs.AppsV1().Deployments("aqs-test").Update(ctx, d, metav1.UpdateOptions{})
	b := fakes.NewBundle()
	clk := &controllerClock{Clock: b.Clock, cs: cs}
	for i := int32(0); i < reps; i++ { // pods viejos, Ready
		_, _ = cs.CoreV1().Pods("aqs-test").Create(ctx, pod(fmt.Sprintf("old-%d", i), true, corev1.PodRunning), metav1.CreateOptions{})
	}
	b.State.Snap.State = state
	b.State.Snap.ResetVerified = state == core.StateReady
	b.Svc.Kube, b.Svc.Clock = k, clk
	return b.Svc, b, cs
}

// F-07: el rebuild de un warm idle (0 réplicas) debe subir a >=1 réplica.
func TestKubeRebuildScalesUpIdleWarm(t *testing.T) {
	_, k := seed()
	if err := k.ScaleApp(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if err := k.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := k.Replicas(ctx); n < 1 {
		t.Fatalf("tras Rebuild de un warm idle: replicas=%d", n)
	}
}

// Barrido de clase: toda ruta que reinicia o reconstruye el warm, con el warm en cada estado.
func TestRebuildPathsFromEveryWarmState(t *testing.T) {
	states := map[string]struct {
		state string
		reps  int32
	}{
		"ready": {core.StateReady, 1}, "dirty": {core.StateDirty, 1},
		"cuarentena": {core.StateQuarantine, 1}, "idle-escalado 0 réplicas": {core.StateIdle, 0},
	}
	paths := map[string]func(*core.Service, *fakes.Bundle) (core.Result, error){
		"Reset":    func(s *core.Service, _ *fakes.Bundle) (core.Result, error) { return s.Reset(ctx, "r-1", "") },
		"Rebuild":  func(s *core.Service, _ *fakes.Bundle) (core.Result, error) { return s.Rebuild(ctx, "rebuild", "") },
		"Teardown": func(s *core.Service, _ *fakes.Bundle) (core.Result, error) { return s.Rebuild(ctx, "teardown", "") },
		"Housekeeping->Reset": func(s *core.Service, b *fakes.Bundle) (core.Result, error) {
			_ = b.Sessions.Save(ctx, core.Session{RunID: "r-9", Status: core.SessionActive, LastActivity: b.Clock.Now().Add(-48 * time.Hour)})
			rep, err := s.Housekeeping(ctx)
			return core.Result{Verified: rep.ResetRun && rep.WarmState == core.StateReady}, err
		},
	}
	for pn, run := range paths {
		for sn, st := range states {
			t.Run(pn+"/"+sn, func(t *testing.T) {
				svc, b, _ := stack(t, st.state, st.reps)
				res, err := run(svc, b)
				if err != nil || !res.Verified {
					t.Fatalf("res=%+v err=%v alertas=%v", res, err, b.Alert.Reasons)
				}
				got, _, _ := b.State.Get(ctx)
				if got.State != core.StateReady || !got.ResetVerified {
					t.Fatalf("estado %+v", got.WarmState)
				}
				if pn == "Rebuild" || pn == "Teardown" {
					if ts := b.Events.Types(); len(ts) != 1 || ts[0] != "teardown.verified" {
						t.Fatalf("eventos %v", ts)
					}
				}
			})
		}
	}
}

// IdleCheck solo entra a idle; la salida de idle es de go-warm-manager (ensureWarmReady) o de un reset (arriba).
func TestIdleCheckThenRebuildCycle(t *testing.T) {
	svc, b, _ := stack(t, core.StateReady, 1)
	b.State.Snap.UpdatedAt = b.Clock.Now()
	b.Clock.Advance(time.Hour)
	if ok, err := svc.IdleCheck(ctx); !ok || err != nil {
		t.Fatalf("%v %v", ok, err)
	}
	res, err := svc.Rebuild(ctx, "rebuild", "")
	if err != nil || !res.Verified || len(b.Events.Types()) != 1 {
		t.Fatalf("rebuild desde idle: %+v %v", res, err)
	}
}

// F-08: guardas de AppReady.
func TestKubeAppReadyGuards(t *testing.T) {
	now := metav1.Now()
	mk := func(mut func(*corev1.Pod)) *corev1.Pod { p := pod("a", true, corev1.PodRunning); mut(p); return p }
	cases := map[string]struct {
		pod  *corev1.Pod
		reps int32
	}{
		"pod Ready pero terminando": {mk(func(p *corev1.Pod) { p.DeletionTimestamp = &now; p.Finalizers = []string{"x"} }), 1},
		"pod Ready no Running":      {mk(func(p *corev1.Pod) { p.Status.Phase = corev1.PodPending }), 1},
		"replicas=0 y 0 pods":       {nil, 0},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			cs, k := seed()
			d, _ := cs.AppsV1().Deployments("aqs-test").Get(ctx, "warm-app", metav1.GetOptions{})
			d.Spec.Replicas = &c.reps
			d.Status = appsv1.DeploymentStatus{UpdatedReplicas: c.reps, AvailableReplicas: c.reps}
			_, _ = cs.AppsV1().Deployments("aqs-test").Update(ctx, d, metav1.UpdateOptions{})
			if c.pod != nil {
				_, _ = cs.CoreV1().Pods("aqs-test").Create(ctx, c.pod, metav1.CreateOptions{})
			}
			if ok, err := k.AppReady(ctx); err != nil || ok {
				t.Fatalf("AppReady=%v err=%v, debía ser false", ok, err)
			}
		})
	}
}

// F-12 (c): transición. Tras el restart el pod viejo sigue Ready y el status del Deployment sigue viejo
// durante `slow` sondeos; Verify debe exigir >= slow sondeos fallidos antes de dar app-ready.
func slowRolloutStack(t *testing.T, slow int) (*core.Service, *fakes.Bundle, *controllerClock) {
	t.Helper()
	svc, b, cs := stack(t, core.StateReady, 1)
	// El Deployment tenía su rollout completo antes del restart.
	d, _ := cs.AppsV1().Deployments("aqs-test").Get(ctx, "warm-app", metav1.GetOptions{})
	d.Generation = 1
	d.Status = appsv1.DeploymentStatus{ObservedGeneration: 1, Replicas: 1, UpdatedReplicas: 1, AvailableReplicas: 1}
	_, _ = cs.AppsV1().Deployments("aqs-test").Update(ctx, d, metav1.UpdateOptions{})
	// Como el API server real: cada cambio de spec/template sube generation; el controlador va por detrás.
	cs.PrependReactor("patch", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		cur, _ := cs.Tracker().Get(appsv1.SchemeGroupVersion.WithResource("deployments"), "aqs-test", "warm-app")
		dd := cur.(*appsv1.Deployment).DeepCopy()
		dd.Generation++
		_ = cs.Tracker().Update(appsv1.SchemeGroupVersion.WithResource("deployments"), dd, "aqs-test")
		return false, nil, nil
	})
	clk := svc.Clock.(*controllerClock)
	clk.slow = slow
	return svc, b, clk
}

func TestResetSlowRolloutRequiresPollingBeforeAppReady(t *testing.T) {
	for _, slow := range []int{1, 3, 8} {
		t.Run(fmt.Sprint("slow", slow), func(t *testing.T) {
			svc, b, clk := slowRolloutStack(t, slow)
			res, err := svc.Reset(ctx, "r-1", "")
			if err != nil || !res.Verified {
				t.Fatalf("res=%+v err=%v", res, err)
			}
			if clk.sleeps < slow {
				t.Fatalf("app-ready tras %d sondeos fallidos; el controlador tardó %d: dio Ready con el rollout sin completar", clk.sleeps, slow)
			}
			if got := b.Events.Types(); len(got) != 1 {
				t.Fatalf("eventos %v", got)
			}
		})
	}
}

// Si el rollout no termina dentro de ReadyTimeout: cuarentena y ningún evento.
func TestResetSlowRolloutBeyondTimeoutQuarantines(t *testing.T) {
	svc, b, _ := slowRolloutStack(t, 1000)
	res, err := svc.Reset(ctx, "r-1", "")
	if err != nil || res.Verified || len(b.Events.Types()) != 0 {
		t.Fatalf("res=%+v err=%v eventos=%v", res, err, b.Events.Types())
	}
	if st, _, _ := b.State.Get(ctx); st.State != core.StateQuarantine {
		t.Fatalf("estado %+v", st.WarmState)
	}
}
