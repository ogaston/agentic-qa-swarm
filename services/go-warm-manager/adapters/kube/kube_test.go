package kube_test

import (
	"context"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/adapters/kube"
)

var seed = wm.WarmState{WarmID: "warm-1", State: "ready", ResetVerified: true, BaselineVersion: "b1"}

// jobOutcome hace que el clientset falso marque cada Job creado como Failed o Complete.
func jobOutcome(cs *fake.Clientset, fail func(n int) bool) {
	n := 0
	cs.PrependReactor("create", "jobs", func(a k8stesting.Action) (bool, runtime.Object, error) {
		job := a.(k8stesting.CreateAction).GetObject().(*batchv1.Job)
		t := batchv1.JobComplete
		if fail(n) {
			t = batchv1.JobFailed
		}
		n++
		job.Status.Conditions = []batchv1.JobCondition{{Type: t, Status: corev1.ConditionTrue, Reason: "Simulado", Message: "x"}}
		return false, nil, nil
	})
}

func service(cs *fake.Clientset) (*wm.Service, *wm.MemPublisher, *wm.FakeAlerter) {
	pub, al := &wm.MemPublisher{}, &wm.FakeAlerter{}
	return &wm.Service{
		Cfg:   wm.Config{Job: wm.JobConfig{AllowedRegistries: wm.DefaultAllowedRegistries}, PollInterval: time.Second},
		State: &kube.StateStore{C: cs, Seed: seed}, Probe: &wm.FakeProbe{}, Runtime: &kube.Runtime{C: cs}, Jobs: &kube.Jobs{C: cs},
		Surface: &wm.FakeProber{}, Objects: &wm.MemObjects{}, Alerts: al, Pub: pub, Clock: &wm.FakeClock{T: time.Unix(0, 0)},
	}, pub, al
}

var art = wm.Artifact{Kind: "published-image", Ref: "ghcr.io/ogaston/demo:1.2.3"}

func listDeployJobs(t *testing.T, cs *fake.Clientset) []string {
	t.Helper()
	l, err := cs.BatchV1().Jobs("aqs-test").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, j := range l.Items {
		if strings.HasPrefix(j.Name, "deploy-r-1-") {
			names = append(names, j.Name)
		}
	}
	return names
}

func TestDeployExhaustedFailClosedOnFakeClientset(t *testing.T) {
	cs := fake.NewClientset()
	jobOutcome(cs, func(int) bool { return true })
	svc, pub, al := service(cs)
	if err := svc.Deploy(context.Background(), "r-1", art, "t"); err == nil {
		t.Fatal("debia fallar")
	}
	if names := listDeployJobs(t, cs); len(names) != 3 {
		t.Fatalf("Jobs leidos del clientset: %v", names)
	}
	last := pub.Events[len(pub.Events)-1]
	if last.Type != "deploy.failed" || last.Data["reason"] == "" || len(al.Calls) != 1 {
		t.Fatalf("evento=%+v handoffs=%d", last, len(al.Calls))
	}
	// un deploy posterior no crea un cuarto Job: el warm quedo dirty
	if err := svc.Deploy(context.Background(), "r-1", art, "t"); err == nil || len(listDeployJobs(t, cs)) != 3 {
		t.Fatalf("no debia crear mas Jobs: %v", err)
	}
}

func TestDeployRetryRecoversOnFakeClientset(t *testing.T) {
	cs := fake.NewClientset()
	jobOutcome(cs, func(n int) bool { return n < 2 })
	svc, pub, al := service(cs)
	if err := svc.Deploy(context.Background(), "r-1", art, "t"); err != nil {
		t.Fatal(err)
	}
	if len(listDeployJobs(t, cs)) != 3 || pub.Events[len(pub.Events)-1].Type != "deploy.done" || len(al.Calls) != 0 {
		t.Fatal("se esperaban 3 Jobs y deploy.done sin handoff")
	}
}

func TestDeployFailedJobsPassSecurityShape(t *testing.T) {
	cs := fake.NewClientset()
	jobOutcome(cs, func(int) bool { return false })
	svc, _, _ := service(cs)
	if err := svc.Deploy(context.Background(), "r-1", art, "t"); err != nil {
		t.Fatal(err)
	}
	j, err := cs.BatchV1().Jobs("aqs-test").Get(context.Background(), "deploy-r-1-0", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p := j.Spec.Template.Spec
	if *p.AutomountServiceAccountToken || p.HostNetwork || p.ServiceAccountName == "" || *j.Spec.BackoffLimit != 0 {
		t.Fatalf("Job creado inseguro: %+v", p)
	}
}

func TestStateStoreRoundTripAndSeed(t *testing.T) {
	ctx := context.Background()
	s := &kube.StateStore{C: fake.NewClientset(), Seed: seed}
	if got, err := s.Get(ctx); err != nil || got != seed {
		t.Fatalf("semilla: %+v %v", got, err)
	}
	d := wm.WarmState{WarmID: "warm-1", State: "dirty", BaselineVersion: "b1"}
	if err := s.Put(ctx, d); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, seed); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, d); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Get(ctx); err != nil || got != d {
		t.Fatalf("lectura: %+v %v", got, err)
	}
	if err := s.Put(ctx, wm.WarmState{State: "raro"}); err == nil {
		t.Fatal("estado invalido aceptado")
	}
}

func TestHealthNeedsAllThreeComponentsReady(t *testing.T) {
	pod := func(name string, ready bool) *corev1.Pod {
		st := corev1.ConditionFalse
		if ready {
			st = corev1.ConditionTrue
		}
		return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "aqs-test", Labels: map[string]string{"app.kubernetes.io/name": name}},
			Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: st}}}}
	}
	ctx := context.Background()
	if err := (&kube.Health{C: fake.NewClientset(pod("warm-app", true), pod("warm-db", true), pod("warm-redis", true))}).Check(ctx); err != nil {
		t.Fatal(err)
	}
	if err := (&kube.Health{C: fake.NewClientset(pod("warm-app", true), pod("warm-db", true), pod("warm-redis", false))}).Check(ctx); err == nil {
		t.Fatal("Redis caido debia fallar")
	}
	if err := (&kube.Health{C: fake.NewClientset()}).Check(ctx); err == nil {
		t.Fatal("sin pods debia fallar")
	}
}

func TestRuntimeScaleUp(t *testing.T) {
	zero := int32(0)
	meta := func(n string) metav1.ObjectMeta { return metav1.ObjectMeta{Name: n, Namespace: "aqs-test"} }
	cs := fake.NewClientset(
		&appsv1.Deployment{ObjectMeta: meta("warm-app"), Spec: appsv1.DeploymentSpec{Replicas: &zero}},
		&appsv1.Deployment{ObjectMeta: meta("warm-redis"), Spec: appsv1.DeploymentSpec{Replicas: &zero}},
		&appsv1.StatefulSet{ObjectMeta: meta("warm-db"), Spec: appsv1.StatefulSetSpec{Replicas: &zero}})
	if err := (&kube.Runtime{C: cs}).ScaleUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	d, _ := cs.AppsV1().Deployments("aqs-test").Get(context.Background(), "warm-app", metav1.GetOptions{})
	s, _ := cs.AppsV1().StatefulSets("aqs-test").Get(context.Background(), "warm-db", metav1.GetOptions{})
	if *d.Spec.Replicas != 1 || *s.Spec.Replicas != 1 {
		t.Fatal("no escalo")
	}
}

func TestJobsRejectsForeignNamespace(t *testing.T) {
	m, _ := wm.BuildDeployJob(wm.JobConfig{AllowedRegistries: wm.DefaultAllowedRegistries}, "r-1", 0, art)
	m["metadata"].(map[string]any)["namespace"] = "aqs-prod"
	if err := (&kube.Jobs{C: fake.NewClientset()}).Create(context.Background(), m); err == nil {
		t.Fatal("namespace ajeno aceptado")
	}
}
