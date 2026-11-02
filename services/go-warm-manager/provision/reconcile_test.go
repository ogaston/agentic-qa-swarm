package provision_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/provision"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/warmtarget"
)

func TestReconcilerCreateAndIdempotency(t *testing.T) {
	ctx := context.Background()
	cs := fake.NewClientset()
	rec := &provision.Reconciler{Kube: cs}

	t1 := warmtarget.TargetForRepo("acme/service-a")
	t2 := warmtarget.TargetForRepo("acme/service-b")

	// 1. Primera corrida: crea recursos para ambos targets
	res1, err := rec.Reconcile(ctx, []warmtarget.WarmTarget{t1, t2})
	if err != nil {
		t.Fatalf("primera reconciliacion fallo: %v", err)
	}
	if len(res1.CreatedNamespaces) != 2 {
		t.Fatalf("esperado 2 namespaces creados, got: %d", len(res1.CreatedNamespaces))
	}
	if len(res1.DeletedNamespaces) != 0 {
		t.Fatalf("esperado 0 namespaces eliminados, got: %d", len(res1.DeletedNamespaces))
	}

	// Verificar que los recursos de t1 existen en client-go
	if _, err := cs.CoreV1().Namespaces().Get(ctx, t1.Namespace, metav1.GetOptions{}); err != nil {
		t.Fatalf("namespace t1 no encontrado: %v", err)
	}
	if _, err := cs.AppsV1().Deployments(t1.Namespace).Get(ctx, "warm-app", metav1.GetOptions{}); err != nil {
		t.Fatalf("warm-app en t1 no encontrado: %v", err)
	}
	if _, err := cs.AppsV1().StatefulSets(t1.Namespace).Get(ctx, "warm-db", metav1.GetOptions{}); err != nil {
		t.Fatalf("warm-db en t1 no encontrado: %v", err)
	}
	if _, err := cs.AppsV1().Deployments(t1.Namespace).Get(ctx, "warm-redis", metav1.GetOptions{}); err != nil {
		t.Fatalf("warm-redis en t1 no encontrado: %v", err)
	}
	if _, err := cs.CoreV1().ConfigMaps(t1.Namespace).Get(ctx, "warm-policy", metav1.GetOptions{}); err != nil {
		t.Fatalf("warm-policy en t1 no encontrado: %v", err)
	}
	if _, err := cs.CoreV1().Secrets(t1.Namespace).Get(ctx, "warm-db-credentials", metav1.GetOptions{}); err != nil {
		t.Fatalf("warm-db-credentials en t1 no encontrado: %v", err)
	}
	if _, err := cs.CoreV1().Secrets(t1.Namespace).Get(ctx, "target-app-baseline-token", metav1.GetOptions{}); err != nil {
		t.Fatalf("target-app-baseline-token en t1 no encontrado: %v", err)
	}
	if _, err := cs.NetworkingV1().NetworkPolicies(t1.Namespace).Get(ctx, "default-deny", metav1.GetOptions{}); err != nil {
		t.Fatalf("default-deny en t1 no encontrado: %v", err)
	}

	// 2. Segunda corrida con los mismos targets: idempotencia
	res2, err := rec.Reconcile(ctx, []warmtarget.WarmTarget{t1, t2})
	if err != nil {
		t.Fatalf("segunda reconciliacion fallo: %v", err)
	}
	if len(res2.CreatedNamespaces) != 0 {
		t.Fatalf("idempotencia: esperado 0 namespaces creados, got: %d", len(res2.CreatedNamespaces))
	}
	if len(res2.DeletedNamespaces) != 0 {
		t.Fatalf("idempotencia: esperado 0 namespaces eliminados, got: %d", len(res2.DeletedNamespaces))
	}
}

func TestReconcilerDeleteRemovedRepo(t *testing.T) {
	ctx := context.Background()
	cs := fake.NewClientset()
	rec := &provision.Reconciler{Kube: cs}

	t1 := warmtarget.TargetForRepo("acme/service-a")
	t2 := warmtarget.TargetForRepo("acme/service-b")

	// Crear ambos
	_, err := rec.Reconcile(ctx, []warmtarget.WarmTarget{t1, t2})
	if err != nil {
		t.Fatalf("setup fallo: %v", err)
	}

	// Reconciliar con t2 removido
	res, err := rec.Reconcile(ctx, []warmtarget.WarmTarget{t1})
	if err != nil {
		t.Fatalf("reconcile con t2 removido fallo: %v", err)
	}
	if len(res.DeletedNamespaces) != 1 || res.DeletedNamespaces[0] != t2.Namespace {
		t.Fatalf("esperado namespace %s eliminado, got: %+v", t2.Namespace, res.DeletedNamespaces)
	}

	// t1 debe seguir existiendo
	if _, err := cs.CoreV1().Namespaces().Get(ctx, t1.Namespace, metav1.GetOptions{}); err != nil {
		t.Fatalf("t1 debia conservarse: %v", err)
	}
	// t2 debe estar eliminado
	if _, err := cs.CoreV1().Namespaces().Get(ctx, t2.Namespace, metav1.GetOptions{}); err == nil {
		t.Fatalf("t2 debia haberse eliminado")
	}
}

func TestReconcilerNeverDeletesLegacyNamespace(t *testing.T) {
	ctx := context.Background()
	// Pre-crear namespace legacy aqs-test con labels plane=warm
	cs := fake.NewClientset(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "aqs-test",
			Labels: map[string]string{
				"aqs.io/plane": "warm",
			},
		},
	})
	rec := &provision.Reconciler{Kube: cs}

	t1 := warmtarget.TargetForRepo("acme/service-a")

	// Reconciliar lista que no incluye aqs-test
	res, err := rec.Reconcile(ctx, []warmtarget.WarmTarget{t1})
	if err != nil {
		t.Fatalf("reconcile fallo: %v", err)
	}

	for _, del := range res.DeletedNamespaces {
		if del == "aqs-test" {
			t.Fatalf("aqs-test nunca debe ser eliminado")
		}
	}

	// aqs-test debe seguir existiendo
	if _, err := cs.CoreV1().Namespaces().Get(ctx, "aqs-test", metav1.GetOptions{}); err != nil {
		t.Fatalf("aqs-test debe existir: %v", err)
	}
}
