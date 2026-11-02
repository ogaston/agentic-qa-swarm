package provision_test

import (
	"context"
	"encoding/json"
	"testing"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/provision"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/warmtarget"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestWorkloadTemplateResourceSelections(t *testing.T) {
	for _, tc := range []struct {
		name      string
		resources []string
		pg, redis bool
	}{
		{name: "stateless", resources: []string{}},
		{name: "postgresql", resources: []string{warmtarget.ResourcePostgreSQL}, pg: true},
		{name: "redis", resources: []string{warmtarget.ResourceRedis}, redis: true},
		{name: "both", resources: []string{warmtarget.ResourcePostgreSQL, warmtarget.ResourceRedis}, pg: true, redis: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := warmtarget.TargetForRepo("acme/shop")
			target.Resources = tc.resources
			res, err := provision.WorkloadTemplate(target)
			if err != nil {
				t.Fatal(err)
			}
			if (res.DBStatefulSet != nil) != tc.pg || (res.DBService != nil) != tc.pg || (res.DBCredentials != nil) != tc.pg {
				t.Fatalf("PostgreSQL selection=%v objects: db=%v service=%v secret=%v", tc.pg, res.DBStatefulSet != nil, res.DBService != nil, res.DBCredentials != nil)
			}
			if (res.RedisDeployment != nil) != tc.redis || (res.RedisService != nil) != tc.redis {
				t.Fatalf("Redis selection=%v objects: deployment=%v service=%v", tc.redis, res.RedisDeployment != nil, res.RedisService != nil)
			}
			for _, env := range res.AppDeployment.Spec.Template.Spec.Containers[0].Env {
				if !tc.pg && (env.Name == "DATABASE_URL" || env.Name == "PGPASSWORD") {
					t.Fatalf("stateless app retains %s", env.Name)
				}
			}
		})
	}
}

func TestReconcileSelectedResourcesAndInfrastructureReadiness(t *testing.T) {
	for _, tc := range []struct {
		name      string
		resources []string
		pg, redis bool
	}{
		{name: "stateless", resources: []string{}},
		{name: "postgresql", resources: []string{warmtarget.ResourcePostgreSQL}, pg: true},
		{name: "redis", resources: []string{warmtarget.ResourceRedis}, redis: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			cs := fake.NewClientset()
			rec := &provision.Reconciler{Kube: cs}
			target := warmtarget.TargetForRepo("acme/shop")
			target.Resources = tc.resources
			if _, err := rec.Reconcile(ctx, []warmtarget.WarmTarget{target}); err != nil {
				t.Fatal(err)
			}
			_, err := cs.AppsV1().StatefulSets(target.Namespace).Get(ctx, "warm-db", metav1.GetOptions{})
			if tc.pg != (err == nil) {
				t.Fatalf("PostgreSQL expected=%v err=%v", tc.pg, err)
			}
			_, err = cs.AppsV1().Deployments(target.Namespace).Get(ctx, "warm-redis", metav1.GetOptions{})
			if tc.redis != (err == nil) {
				t.Fatalf("Redis expected=%v err=%v", tc.redis, err)
			}
			if !tc.pg && !apierrors.IsNotFound(err) && !tc.redis {
				t.Fatalf("unexpected Redis error: %v", err)
			}
			assertInfrastructureReady(t, cs, target.Namespace, !tc.pg && !tc.redis)

			if tc.pg {
				sts, _ := cs.AppsV1().StatefulSets(target.Namespace).Get(ctx, "warm-db", metav1.GetOptions{})
				sts.Status.ReadyReplicas = 1
				_, _ = cs.AppsV1().StatefulSets(target.Namespace).UpdateStatus(ctx, sts, metav1.UpdateOptions{})
			}
			if tc.redis {
				dep, _ := cs.AppsV1().Deployments(target.Namespace).Get(ctx, "warm-redis", metav1.GetOptions{})
				dep.Status.ReadyReplicas = 1
				_, _ = cs.AppsV1().Deployments(target.Namespace).UpdateStatus(ctx, dep, metav1.UpdateOptions{})
			}
			if _, err := rec.Reconcile(ctx, []warmtarget.WarmTarget{target}); err != nil {
				t.Fatal(err)
			}
			assertInfrastructureReady(t, cs, target.Namespace, true)
		})
	}
}

func assertInfrastructureReady(t *testing.T, cs *fake.Clientset, namespace string, want bool) {
	t.Helper()
	cm, err := cs.CoreV1().ConfigMaps(namespace).Get(context.Background(), "warm-state", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var state wm.WarmState
	if err := json.Unmarshal([]byte(cm.Data["state"]), &state); err != nil {
		t.Fatal(err)
	}
	if state.InfrastructureReady != want || state.ResetVerified || state.State != wm.StateProvisioning {
		t.Fatalf("infrastructure_ready=%v reset=%v state=%s", state.InfrastructureReady, state.ResetVerified, state.State)
	}
}
