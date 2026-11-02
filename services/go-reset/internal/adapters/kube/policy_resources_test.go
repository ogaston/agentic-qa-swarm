package kube_test

import (
	"reflect"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWarmPolicyResourceSelection(t *testing.T) {
	cs, client := seed()
	cm, err := cs.CoreV1().ConfigMaps("aqs-test").Get(ctx, "warm-policy", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cm.Data["resources"] = `["redis"]`
	if _, err := cs.CoreV1().ConfigMaps("aqs-test").Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	policy, err := client.WarmPolicy(ctx)
	if err != nil || !reflect.DeepEqual(policy.Resources, []string{core.ResourceRedis}) {
		t.Fatalf("policy = %+v, %v", policy, err)
	}
	cm.Data["resources"] = `["mysql"]`
	_, _ = cs.CoreV1().ConfigMaps("aqs-test").Update(ctx, cm, metav1.UpdateOptions{})
	if _, err := client.WarmPolicy(ctx); err == nil {
		t.Fatal("unsupported resource accepted")
	}
}

func TestLegacyWarmPolicyDefaultsBothResources(t *testing.T) {
	_, client := seed()
	policy, err := client.WarmPolicy(ctx)
	if err != nil || !reflect.DeepEqual(policy.Resources, []string{core.ResourcePostgreSQL, core.ResourceRedis}) {
		t.Fatalf("legacy policy = %+v, %v", policy, err)
	}
}
