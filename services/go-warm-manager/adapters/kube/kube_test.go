package kube_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/adapters/kube"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/internal/fakes"
)

// La semilla (estado si falta el ConfigMap) es siempre no-listo: nunca ready+reset_verified.
var seed = wm.WarmState{WarmID: "warm-1", State: "dirty", ResetVerified: false, BaselineVersion: "b1"}
var ready = wm.WarmState{WarmID: "warm-1", State: "ready", ResetVerified: true, BaselineVersion: "b1"}

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

func service(cs *fake.Clientset) (*wm.Service, *fakes.MemPublisher, *fakes.FakeAlerter) {
	pub, al := &fakes.MemPublisher{}, &fakes.FakeAlerter{}
	st := &kube.StateStore{C: cs, Seed: seed}
	if err := st.Put(context.Background(), ready); err != nil { // go-reset dejo el warm verificado
		panic(err)
	}
	return &wm.Service{
		Cfg:   wm.Config{Job: wm.JobConfig{AllowedRegistries: wm.DefaultAllowedRegistries}, PollInterval: time.Second},
		State: st, Probe: &fakes.FakeProbe{}, Runtime: &kube.Runtime{C: cs}, Jobs: &kube.Jobs{C: cs},
		Surface: &fakes.FakeProber{}, Objects: &fakes.MemObjects{}, Alerts: al, Pub: pub, Clock: &fakes.FakeClock{T: time.Unix(0, 0)},
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

func TestStateStoreSeedIsNotReadyAndRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := &kube.StateStore{C: fake.NewClientset(), Seed: seed}
	got, err := s.Get(ctx)
	if err != nil || got != seed || got.State == "ready" || got.ResetVerified {
		t.Fatalf("sin ConfigMap el warm no puede estar listo: %+v %v", got, err)
	}
	if err := s.Put(ctx, ready); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, seed); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, ready); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Get(ctx); err != nil || got != ready {
		t.Fatalf("lectura: %+v %v", got, err)
	}
	if err := s.Put(ctx, wm.WarmState{State: "raro"}); err == nil {
		t.Fatal("estado invalido aceptado")
	}
}

func TestStateStoreCompareAndSwap(t *testing.T) {
	ctx := context.Background()
	dirty := wm.WarmState{WarmID: "warm-1", State: "dirty", BaselineVersion: "b1"}
	s := &kube.StateStore{C: fake.NewClientset(), Seed: seed}
	// Sin ConfigMap: solo vale el CAS contra la semilla.
	if err := s.CompareAndSwap(ctx, ready, dirty); !errors.Is(err, wm.ErrStateConflict) {
		t.Fatalf("CAS contra un estado que no es la semilla debia ser conflicto: %v", err)
	}
	if err := s.Put(ctx, ready); err != nil {
		t.Fatal(err)
	}
	if err := s.CompareAndSwap(ctx, ready, dirty); err != nil {
		t.Fatal(err)
	}
	if err := s.CompareAndSwap(ctx, ready, dirty); !errors.Is(err, wm.ErrStateConflict) {
		t.Fatalf("segundo CAS con expect obsoleto debia ser conflicto: %v", err)
	}
	if got, _ := s.Get(ctx); got != dirty {
		t.Fatalf("el estado no debia cambiar: %+v", got)
	}
}

func TestStateStoreCompareAndSwapConflictFromAPIServer(t *testing.T) {
	cs := fake.NewClientset()
	s := &kube.StateStore{C: cs, Seed: seed}
	if err := s.Put(context.Background(), ready); err != nil {
		t.Fatal(err)
	}
	cs.PrependReactor("update", "configmaps", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "configmaps"}, "warm-state", errors.New("resourceVersion obsoleto"))
	})
	dirty := wm.WarmState{WarmID: "warm-1", State: "dirty", BaselineVersion: "b1"}
	if err := s.CompareAndSwap(context.Background(), ready, dirty); !errors.Is(err, wm.ErrStateConflict) {
		t.Fatalf("409 del apiserver debia mapearse a ErrStateConflict: %v", err)
	}
}

func TestDeployConcurrentOnKubeStoreCreatesOneJob(t *testing.T) {
	cs := fake.NewClientset()
	jobOutcome(cs, func(int) bool { return false })
	svc, _, _ := service(cs)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); _ = svc.Deploy(context.Background(), fmt.Sprintf("r-%d", i), art, "t") }(i)
	}
	wg.Wait()
	l, _ := cs.BatchV1().Jobs("aqs-test").List(context.Background(), metav1.ListOptions{})
	if len(l.Items) != 1 {
		t.Fatalf("Jobs=%d", len(l.Items))
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

// MA: el Update condicional debe llevar el resourceVersion del ConfigMap leido (protege entre replicas).
func TestStateStoreCompareAndSwapSendsResourceVersionOfTheRead(t *testing.T) {
	cs := fake.NewClientset(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "warm-state", Namespace: "aqs-test", ResourceVersion: "42"},
		Data: map[string]string{"state": `{"warm_id":"warm-1","state":"ready","reset_verified":true,"baseline_version":"b1"}`}})
	var sent string
	cs.PrependReactor("update", "configmaps", func(a k8stesting.Action) (bool, runtime.Object, error) {
		sent = a.(k8stesting.UpdateAction).GetObject().(*corev1.ConfigMap).ResourceVersion
		return false, nil, nil
	})
	s := &kube.StateStore{C: cs, Seed: seed}
	dirty := wm.WarmState{WarmID: "warm-1", State: "dirty", BaselineVersion: "b1"}
	if err := s.CompareAndSwap(context.Background(), ready, dirty); err != nil {
		t.Fatal(err)
	}
	if sent != "42" {
		t.Fatalf("el Update llevo resourceVersion %q, se esperaba el del Get (42)", sent)
	}
}

func TestJobsListAndDerivedStatusViaKube(t *testing.T) {
	ctx := context.Background()
	cs := fake.NewClientset()
	outs := []bool{true, false} // el primer Job falla y el segundo termina
	jobOutcome(cs, func(n int) bool { return outs[n] })
	svc, _, _ := service(cs)
	if err := svc.Deploy(ctx, "r-1", art, "t"); err != nil {
		t.Fatal(err)
	}
	fresh, _, _ := service(cs) // instancia nueva: memoria vacia
	st, ok, err := fresh.DeployState(ctx, "r-1")
	if err != nil || !ok || st.State != "done" || st.Attempts != 2 {
		t.Fatalf("%+v ok=%v err=%v", st, ok, err)
	}
	if _, ok, _ := fresh.DeployState(ctx, "r-2"); ok {
		t.Fatal("r-2 no existe")
	}
	views, err := (&kube.Jobs{C: cs}).List(ctx, "")
	if err != nil || len(views) != 2 || views[0].Artifact != art {
		t.Fatalf("%+v %v", views, err)
	}
}
