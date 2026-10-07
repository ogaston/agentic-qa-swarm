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

// ---- Deploy en proceso sobre el clientset falso con un rollout simulado ----

// cluster simula el Deployment warm-app: cada parche sube la generacion y el rollout completa segun
// done(parches, sondeos desde el ultimo parche). Mientras no completa, el status sigue en la generacion
// anterior y el pod viejo (imagen vieja) sigue Ready: justo lo que AppReady/RolloutComplete no deben aceptar.
type cluster struct {
	cs      *fake.Clientset
	mu      sync.Mutex
	patches []string // cuerpos de los parches de imagen
	polls   int
	done    func(patches, polls int) bool
}

const oldImage = "nginx:1.27.2"

func newCluster(done func(patches, polls int) bool) *cluster {
	one := int32(1)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "warm-app", Namespace: "aqs-test", Generation: 1},
		Spec: appsv1.DeploymentSpec{Replicas: &one, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "warm-app", Image: oldImage, Env: []corev1.EnvVar{{Name: "TZ", Value: "UTC"}}}}}}},
	}
	c := &cluster{cs: fake.NewClientset(dep), done: done}
	gvr := appsv1.SchemeGroupVersion.WithResource("deployments")
	c.cs.PrependReactor("patch", "deployments", func(a k8stesting.Action) (bool, runtime.Object, error) {
		c.mu.Lock()
		c.patches = append(c.patches, string(a.(k8stesting.PatchAction).GetPatch()))
		c.polls = 0
		c.mu.Unlock()
		return false, nil, nil // lo aplica el tracker (strategic merge patch real)
	})
	c.cs.PrependReactor("get", "deployments", func(a k8stesting.Action) (bool, runtime.Object, error) {
		obj, err := c.cs.Tracker().Get(gvr, "aqs-test", "warm-app")
		if err != nil {
			return true, nil, err
		}
		d := obj.(*appsv1.Deployment).DeepCopy()
		c.mu.Lock()
		defer c.mu.Unlock()
		c.polls++
		d.Generation = int64(1 + len(c.patches))
		if len(c.patches) == 0 || c.done(len(c.patches), c.polls) {
			d.Status = appsv1.DeploymentStatus{ObservedGeneration: d.Generation, UpdatedReplicas: 1, AvailableReplicas: 1, Replicas: 1}
		} else {
			d.Status = appsv1.DeploymentStatus{ObservedGeneration: d.Generation - 1, UpdatedReplicas: 0, AvailableReplicas: 1, Replicas: 1}
		}
		return true, d, nil
	})
	c.cs.PrependReactor("list", "pods", func(a k8stesting.Action) (bool, runtime.Object, error) {
		obj, _ := c.cs.Tracker().Get(gvr, "aqs-test", "warm-app")
		img := obj.(*appsv1.Deployment).Spec.Template.Spec.Containers[0].Image
		c.mu.Lock()
		defer c.mu.Unlock()
		if len(c.patches) > 0 && !c.done(len(c.patches), c.polls) {
			img = oldImage // el pod viejo sigue vivo y Ready
		}
		return true, &corev1.PodList{Items: []corev1.Pod{readyPod(img)}}, nil
	})
	return c
}

func readyPod(img string) corev1.Pod {
	return corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "warm-app-x", Namespace: "aqs-test", Labels: map[string]string{"app.kubernetes.io/name": "warm-app"}},
		Spec:   corev1.PodSpec{Containers: []corev1.Container{{Name: "warm-app", Image: img}}},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}}
}

func (c *cluster) patchCount() int { c.mu.Lock(); defer c.mu.Unlock(); return len(c.patches) }

// verbs cuenta las acciones del clientset por recurso y verbo.
func (c *cluster) actions(resource, verb string) int {
	n := 0
	for _, a := range c.cs.Actions() {
		if a.GetResource().Resource == resource && (verb == "" || a.GetVerb() == verb) {
			n++
		}
	}
	return n
}

func service(cs *fake.Clientset) (*wm.Service, *fakes.MemPublisher, *fakes.FakeAlerter) {
	pub, al := &fakes.MemPublisher{}, &fakes.FakeAlerter{}
	st := &kube.StateStore{C: cs, Seed: seed}
	if err := st.Put(context.Background(), ready); err != nil { // go-reset dejo el warm verificado
		panic(err)
	}
	return &wm.Service{
		Cfg:   wm.Config{AllowedRegistries: wm.DefaultAllowedRegistries, PollInterval: time.Second, DeployTimeout: time.Minute},
		State: st, Probe: &fakes.FakeProbe{}, Runtime: &kube.Runtime{C: cs}, Deployer: &kube.Deployer{C: cs},
		Surface: &fakes.FakeProber{}, Objects: &fakes.MemObjects{}, Alerts: al, Pub: pub, Clock: &fakes.FakeClock{T: time.Unix(0, 0)},
	}, pub, al
}

var art = wm.Artifact{Kind: "published-image", Ref: "ghcr.io/ogaston/demo:1.2.3"}

func never(int, int) bool { return false }

func TestDeployExhaustedFailClosedOnFakeClientset(t *testing.T) {
	c := newCluster(never)
	svc, pub, al := service(c.cs)
	if err := svc.Deploy(context.Background(), "r-1", art, "t"); err == nil {
		t.Fatal("debia fallar")
	}
	if c.patchCount() != 3 || c.actions("deployments", "patch") != 3 { // leido del clientset, no del servicio
		t.Fatalf("parches=%d", c.patchCount())
	}
	for _, p := range c.patches {
		if !strings.Contains(p, art.Ref) {
			t.Fatalf("el parche no lleva la imagen: %s", p)
		}
	}
	last := pub.Events[len(pub.Events)-1]
	if last.Type != "deploy.failed" || last.Data["reason"] == "" || len(al.Calls) != 1 || len(pub.Events) != 1 {
		t.Fatalf("evento=%+v eventos=%d handoffs=%d", last, len(pub.Events), len(al.Calls))
	}
	// un deploy posterior no parchea: el warm quedo dirty
	if err := svc.Deploy(context.Background(), "r-2", art, "t"); !errors.Is(err, wm.ErrNotReady) || c.patchCount() != 3 {
		t.Fatalf("no debia parchear mas: %v", err)
	}
}

func TestDeployRetryRecoversOnFakeClientset(t *testing.T) {
	c := newCluster(func(patches, polls int) bool { return patches >= 3 && polls >= 2 }) // los 2 primeros intentos no completan
	svc, pub, al := service(c.cs)
	if err := svc.Deploy(context.Background(), "r-1", art, "t"); err != nil {
		t.Fatal(err)
	}
	if c.patchCount() != 3 || pub.Events[len(pub.Events)-1].Type != "deploy.done" || len(al.Calls) != 0 {
		t.Fatalf("parches=%d eventos=%v", c.patchCount(), pub.Events)
	}
}

func TestDeployNeverCreatesJobsOnFakeClientset(t *testing.T) {
	c := newCluster(func(patches, polls int) bool { return polls >= 3 }) // completa tras unos sondeos (con retardo)
	svc, pub, _ := service(c.cs)
	if err := svc.Deploy(context.Background(), "r-1", art, "t"); err != nil {
		t.Fatal(err)
	}
	if n := c.actions("jobs", ""); n != 0 { // ni crear, ni listar, ni leer Jobs
		t.Fatalf("acciones sobre Jobs: %d", n)
	}
	if l, _ := c.cs.BatchV1().Jobs("aqs-test").List(context.Background(), metav1.ListOptions{}); len(l.Items) != 0 {
		t.Fatalf("Jobs: %d", len(l.Items))
	}
	if c.patchCount() != 1 || pub.Events[len(pub.Events)-1].Type != "deploy.done" {
		t.Fatalf("parches=%d", c.patchCount())
	}
	d, _ := c.cs.AppsV1().Deployments("aqs-test").Get(context.Background(), "warm-app", metav1.GetOptions{})
	ct := d.Spec.Template.Spec.Containers
	if len(ct) != 1 || ct[0].Image != art.Ref || len(ct[0].Env) != 1 { // solo cambia la imagen
		t.Fatalf("contenedores tras el parche: %+v", ct)
	}
}

// Un rollout que no completa NUNCA es exito, ni aunque el status ya cuente replicas o haya un pod Ready.
func TestDeployRolloutTimeoutNeverSucceedsOnFakeClientset(t *testing.T) {
	c := newCluster(never)
	svc, pub, al := service(c.cs)
	err := svc.Deploy(context.Background(), "r-1", art, "t")
	d, _, _ := svc.DeployState(context.Background(), "r-1")
	if err == nil || d.State != "failed" || d.Attempts != 3 || !strings.Contains(d.Reason, "timeout") {
		t.Fatalf("err=%v %+v", err, d)
	}
	for _, e := range pub.Events {
		if e.Type == "deploy.done" {
			t.Fatal("un timeout fue exito")
		}
	}
	if len(pub.Events) != 1 || len(al.Calls) != 1 {
		t.Fatalf("eventos=%d handoffs=%d", len(pub.Events), len(al.Calls))
	}
}

func TestDeployConcurrentOnKubeStorePatchesOnce(t *testing.T) {
	c := newCluster(func(int, int) bool { return true })
	svc, _, _ := service(c.cs)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); _ = svc.Deploy(context.Background(), fmt.Sprintf("r-%d", i), art, "t") }(i)
	}
	wg.Wait()
	if c.patchCount() != 1 {
		t.Fatalf("parches=%d", c.patchCount())
	}
}

// Reinicio: el warm persistido quedo dirty (CAS en el ConfigMap) y el estado del deploy es de memoria.
func TestDeployRestartLeavesWarmDirtyAndForgetsDeploys(t *testing.T) {
	ctx := context.Background()
	c := newCluster(func(int, int) bool { return true })
	svc, _, _ := service(c.cs)
	if err := svc.Deploy(ctx, "r-1", art, "t"); err != nil {
		t.Fatal(err)
	}
	st := &kube.StateStore{C: c.cs, Seed: seed}
	if w, _ := st.Get(ctx); w.State != "dirty" || w.ResetVerified {
		t.Fatalf("el ConfigMap debia quedar dirty: %+v", w)
	}
	fresh := &wm.Service{Cfg: svc.Cfg, State: st, Probe: &fakes.FakeProbe{}, Deployer: &kube.Deployer{C: c.cs}, Alerts: &fakes.FakeAlerter{},
		Pub: &fakes.MemPublisher{}, Clock: &fakes.FakeClock{T: time.Unix(0, 0)}}
	if _, ok, _ := fresh.DeployState(ctx, "r-1"); ok {
		t.Fatal("tras reinicio el deploy no se conoce")
	}
	if err := fresh.Deploy(ctx, "r-2", art, "t"); !errors.Is(err, wm.ErrNotReady) || c.patchCount() != 1 {
		t.Fatalf("no debia parchear: %v parches=%d", err, c.patchCount())
	}
}

// SetImage: el parche por nombre inexistente AGREGARIA un contenedor; el adaptador lo impide.
func TestDeployerSetImageNeedsTheContainer(t *testing.T) {
	ctx := context.Background()
	one := int32(1)
	cs := fake.NewClientset(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "warm-app", Namespace: "aqs-test"},
		Spec: appsv1.DeploymentSpec{Replicas: &one, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "otro", Image: "x"}}}}}})
	if err := (&kube.Deployer{C: cs}).SetImage(ctx, art.Ref); err == nil {
		t.Fatal("sin contenedor warm-app debia fallar")
	}
	n := 0
	for _, a := range cs.Actions() {
		if a.GetVerb() == "patch" {
			n++
		}
	}
	if n != 0 {
		t.Fatal("no debia parchear")
	}
	if err := (&kube.Deployer{C: fake.NewClientset()}).SetImage(ctx, art.Ref); err == nil {
		t.Fatal("sin Deployment debia fallar")
	}
}

// Cada criterio del rollout completo por separado: si uno falla, NO esta completo.
func TestRolloutCompleteCriteria(t *testing.T) {
	ctx := context.Background()
	two := int32(2)
	base := func() (*appsv1.Deployment, []corev1.Pod) {
		one := int32(1)
		d := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "warm-app", Namespace: "aqs-test", Generation: 2},
			Spec:   appsv1.DeploymentSpec{Replicas: &one, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "warm-app", Image: art.Ref}}}}},
			Status: appsv1.DeploymentStatus{ObservedGeneration: 2, UpdatedReplicas: 1, AvailableReplicas: 1}}
		return d, []corev1.Pod{readyPod(art.Ref)}
	}
	cases := []struct {
		name string
		mut  func(*appsv1.Deployment, *[]corev1.Pod)
		want bool
	}{
		{"completo", func(*appsv1.Deployment, *[]corev1.Pod) {}, true},
		{"observedGeneration vieja", func(d *appsv1.Deployment, _ *[]corev1.Pod) { d.Status.ObservedGeneration = 1 }, false},
		{"updatedReplicas distinto", func(d *appsv1.Deployment, _ *[]corev1.Pod) { d.Status.UpdatedReplicas = 0 }, false},
		{"availableReplicas distinto", func(d *appsv1.Deployment, _ *[]corev1.Pod) { d.Status.AvailableReplicas = 0 }, false},
		{"replicas 2 con 1 disponible", func(d *appsv1.Deployment, _ *[]corev1.Pod) { d.Spec.Replicas = &two; d.Status.UpdatedReplicas = 2 }, false},
		{"replicas 0", func(d *appsv1.Deployment, _ *[]corev1.Pod) {
			z := int32(0)
			d.Spec.Replicas = &z
			d.Status.UpdatedReplicas, d.Status.AvailableReplicas = 0, 0
		}, false},
		{"plantilla con otra imagen", func(d *appsv1.Deployment, _ *[]corev1.Pod) { d.Spec.Template.Spec.Containers[0].Image = oldImage }, false},
		{"pod extra (el viejo sigue)", func(_ *appsv1.Deployment, p *[]corev1.Pod) { *p = append(*p, readyPod(oldImage)) }, false},
		{"sin pods", func(_ *appsv1.Deployment, p *[]corev1.Pod) { *p = nil }, false},
		{"pod con imagen vieja", func(_ *appsv1.Deployment, p *[]corev1.Pod) { *p = []corev1.Pod{readyPod(oldImage)} }, false},
		{"pod no Ready", func(_ *appsv1.Deployment, p *[]corev1.Pod) {
			(*p)[0].Status.Conditions[0].Status = corev1.ConditionFalse
		}, false},
		{"pod sin condicion Ready", func(_ *appsv1.Deployment, p *[]corev1.Pod) { (*p)[0].Status.Conditions = nil }, false},
		{"pod no Running", func(_ *appsv1.Deployment, p *[]corev1.Pod) { (*p)[0].Status.Phase = corev1.PodPending }, false},
		{"pod terminando", func(_ *appsv1.Deployment, p *[]corev1.Pod) {
			now := metav1.Now()
			(*p)[0].DeletionTimestamp = &now
		}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, pods := base()
			c.mut(d, &pods)
			objs := []runtime.Object{d}
			for i := range pods {
				pods[i].Name, pods[i].Labels = fmt.Sprintf("p%d", i), map[string]string{"app.kubernetes.io/name": "warm-app"}
				objs = append(objs, &pods[i])
			}
			got, err := (&kube.Deployer{C: fake.NewClientset(objs...)}).RolloutComplete(ctx, art.Ref)
			if err != nil || got != c.want {
				t.Fatalf("got=%v err=%v want=%v", got, err, c.want)
			}
		})
	}
	if _, err := (&kube.Deployer{C: fake.NewClientset()}).RolloutComplete(ctx, art.Ref); err == nil {
		t.Fatal("sin Deployment el error debe propagarse, no tragarse como 'no completo'")
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
