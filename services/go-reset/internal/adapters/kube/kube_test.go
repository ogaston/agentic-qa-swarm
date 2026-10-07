package kube_test

import (
	"context"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/adapters/kube"
	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/core"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

var ctx = context.Background()

func seed() (*fake.Clientset, *kube.Client) {
	two := int32(2)
	cs := fake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "warm-app", Namespace: "aqs-test"}, Spec: appsv1.DeploymentSpec{Replicas: &two}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "warm-policy", Namespace: "aqs-test"},
			Data: map[string]string{"idleScaleDownAfter": "30m", "minReplicasIdle": "0"}},
	)
	c, _ := kube.New(cs, "aqs-test", func() time.Time { return time.Unix(0, 0) })
	return cs, c
}

func pod(name string, ready bool, phase corev1.PodPhase) *corev1.Pod {
	st := corev1.ConditionFalse
	if ready {
		st = corev1.ConditionTrue
	}
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "aqs-test", Labels: map[string]string{"app.kubernetes.io/name": "warm-app"}},
		Status: corev1.PodStatus{Phase: phase, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: st}}}}
}

func TestKubeRejectsForeignNamespace(t *testing.T) {
	if _, err := kube.New(fake.NewSimpleClientset(), "aqs-system", nil); err == nil {
		t.Fatal("aceptó aqs-system")
	}
}

func TestKubeAppReadyReadsPods(t *testing.T) {
	cs, c := seed()
	if ok, _ := c.AppReady(ctx); ok {
		t.Fatal("sin pods no es Ready")
	}
	_, _ = cs.CoreV1().Pods("aqs-test").Create(ctx, pod("a", true, corev1.PodRunning), metav1.CreateOptions{})
	if ok, _ := c.AppReady(ctx); !ok {
		t.Fatal("pod Ready no detectado")
	}
	_, _ = cs.CoreV1().Pods("aqs-test").Create(ctx, pod("b", false, corev1.PodRunning), metav1.CreateOptions{})
	if ok, _ := c.AppReady(ctx); ok {
		t.Fatal("un pod no Ready debe bloquear")
	}
}

func TestKubeScaleAndReplicas(t *testing.T) {
	_, c := seed()
	if n, _ := c.Replicas(ctx); n != 2 {
		t.Fatal(n)
	}
	if err := c.ScaleApp(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if n, _ := c.Replicas(ctx); n != 0 {
		t.Fatalf("n=%d", n)
	}
}

func TestKubeRestartAnnotatesTemplate(t *testing.T) {
	cs, c := seed()
	if err := c.RestartApp(ctx); err != nil {
		t.Fatal(err)
	}
	d, _ := cs.AppsV1().Deployments("aqs-test").Get(ctx, "warm-app", metav1.GetOptions{})
	if d.Spec.Template.Annotations["aqs/restarted-at"] == "" {
		t.Fatal("sin anotación de restart")
	}
}

func TestKubeRebuildAndTeardownDeletePods(t *testing.T) {
	for _, mode := range []string{"rebuild", "teardown"} {
		cs, c := seed()
		_, _ = cs.CoreV1().Pods("aqs-test").Create(ctx, pod("a", true, corev1.PodRunning), metav1.CreateOptions{})
		var err error
		if mode == "rebuild" {
			err = c.Rebuild(ctx)
		} else {
			err = c.Teardown(ctx)
		}
		if err != nil {
			t.Fatal(err)
		}
		l, _ := cs.CoreV1().Pods("aqs-test").List(ctx, metav1.ListOptions{})
		if len(l.Items) != 0 {
			t.Fatalf("%s: quedan pods", mode)
		}
		if n, _ := c.Replicas(ctx); mode == "teardown" && n != 1 {
			t.Fatalf("teardown no reprovisionó: %d", n)
		}
	}
}

func TestKubeWarmPolicy(t *testing.T) {
	_, c := seed()
	p, err := c.WarmPolicy(ctx)
	if err != nil || p.IdleScaleDownAfter != 30*time.Minute || p.MinReplicasIdle != 0 {
		t.Fatalf("%+v %v", p, err)
	}
}

func TestKubeStateStoreRoundTrip(t *testing.T) {
	_, c := seed()
	if _, ok, _ := c.Get(ctx); ok {
		t.Fatal("no debería existir")
	}
	w := core.WarmState{WarmID: "w", State: core.StateReady, ResetVerified: true, BaselineVersion: "b"}
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for i := 0; i < 2; i++ { // crea y luego actualiza
		if err := c.Put(ctx, w, at); err != nil {
			t.Fatal(err)
		}
	}
	got, ok, err := c.Get(ctx)
	if err != nil || !ok || got.WarmState != w || !got.UpdatedAt.Equal(at) {
		t.Fatalf("%+v %v", got, err)
	}
}
