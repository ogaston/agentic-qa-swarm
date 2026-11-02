package provision_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/provision"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/warmtarget"
)

func TestWorkloadTemplateParityWithWarmYAML(t *testing.T) {
	// Leer deploy/flux/base/warm.yaml
	yamlPath := filepath.Join("..", "..", "..", "deploy", "flux", "base", "warm.yaml")
	rawYAML, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("leyendo warm.yaml en %s: %v", yamlPath, err)
	}
	content := string(rawYAML)

	target := warmtarget.TargetForRepo("acme/payment-svc")
	res, err := provision.WorkloadTemplate(target)
	if err != nil {
		t.Fatalf("WorkloadTemplate: %v", err)
	}

	// 1. Namespace y Labels
	if res.Namespace.Name != target.Namespace {
		t.Errorf("Namespace = %s, esperado %s", res.Namespace.Name, target.Namespace)
	}
	if res.Namespace.Labels["aqs.io/plane"] != "warm" {
		t.Errorf("label aqs.io/plane = %s, esperado warm", res.Namespace.Labels["aqs.io/plane"])
	}
	// Pod Security Admission: misma postura que aqs-test (deploy/flux/base/namespaces.yaml).
	for k, want := range map[string]string{
		"pod-security.kubernetes.io/enforce": "baseline",
		"pod-security.kubernetes.io/warn":    "restricted",
		"pod-security.kubernetes.io/audit":   "restricted",
	} {
		if got := res.Namespace.Labels[k]; got != want {
			t.Errorf("label %s = %q, esperado %q", k, got, want)
		}
	}
	if res.Namespace.Annotations["aqs.io/repo"] != target.Repo {
		t.Errorf("label aqs.io/repo = %s, esperado %s", res.Namespace.Annotations["aqs.io/repo"], target.Repo)
	}

	// 2. warm-app Deployment
	appContainer := res.AppDeployment.Spec.Template.Spec.Containers[0]
	// El placeholder no sirve HTTP ni baseline; solo la imagen del evento puede
	// pasar las sondas y el reset verificado.
	if appContainer.Image != "registry.k8s.io/pause:3.10" {
		t.Errorf("app imagen inicial = %s", appContainer.Image)
	}
	if len(appContainer.Ports) != 2 || appContainer.Ports[0].ContainerPort != 80 || appContainer.Ports[1].ContainerPort != 9090 {
		t.Errorf("app puertos inesperados: %+v", appContainer.Ports)
	}
	if *res.AppDeployment.Spec.Template.Spec.SecurityContext.FSGroup != 65532 {
		t.Errorf("app fsGroup inesperado")
	}

	// 3. warm-db StatefulSet
	if !strings.Contains(content, "postgres:16.4") {
		t.Fatalf("warm.yaml no contiene postgres:16.4")
	}
	dbContainer := res.DBStatefulSet.Spec.Template.Spec.Containers[0]
	if dbContainer.Image != "postgres:16.4" {
		t.Errorf("db imagen = %s, esperado postgres:16.4", dbContainer.Image)
	}
	if dbContainer.Ports[0].ContainerPort != 5432 {
		t.Errorf("db puerto = %d, esperado 5432", dbContainer.Ports[0].ContainerPort)
	}
	if len(res.DBStatefulSet.Spec.VolumeClaimTemplates) != 1 {
		t.Fatalf("db volumeClaimTemplates esperado 1")
	}

	// 4. warm-redis Deployment
	if !strings.Contains(content, "redis:7.4.1") {
		t.Fatalf("warm.yaml no contiene redis:7.4.1")
	}
	redisContainer := res.RedisDeployment.Spec.Template.Spec.Containers[0]
	if redisContainer.Image != "redis:7.4.1" {
		t.Errorf("redis imagen = %s, esperado redis:7.4.1", redisContainer.Image)
	}
	if redisContainer.Ports[0].ContainerPort != 6379 {
		t.Errorf("redis puerto = %d, esperado 6379", redisContainer.Ports[0].ContainerPort)
	}

	// 5. ConfigMap warm-policy
	if res.PolicyConfigMap.Data["idleScaleDownAfter"] != "30m" || res.PolicyConfigMap.Data["minReplicasIdle"] != "0" {
		t.Errorf("warm-policy data inesperada: %+v", res.PolicyConfigMap.Data)
	}

	// 6. ResourceQuota
	hard := res.ResourceQuota.Spec.Hard
	if hard.Pods().String() != "15" {
		t.Errorf("quota pods = %s, esperado 15", hard.Pods().String())
	}
	if q, ok := hard["limits.cpu"]; !ok || q.String() != "6" {
		t.Errorf("quota limits.cpu = %s, esperado 6", q.String())
	}
	if q, ok := hard["limits.memory"]; !ok || q.String() != "6Gi" {
		t.Errorf("quota limits.memory = %s, esperado 6Gi", q.String())
	}

	// 7. LimitRange
	lrItem := res.LimitRange.Spec.Limits[0]
	if lrItem.Default.Cpu().String() != "250m" || lrItem.Default.Memory().String() != "256Mi" {
		t.Errorf("limitRange default inesperado: %+v", lrItem.Default)
	}
	if lrItem.DefaultRequest.Cpu().String() != "50m" || lrItem.DefaultRequest.Memory().String() != "64Mi" {
		t.Errorf("limitRange defaultRequest inesperado: %+v", lrItem.DefaultRequest)
	}

	// 8. Secrets aleatorios
	if len(res.DBCredentials.StringData["password"]) != 32 {
		t.Errorf("db password longitud inesperada: %d", len(res.DBCredentials.StringData["password"]))
	}
	if len(res.BaselineToken.StringData["token"]) != 32 {
		t.Errorf("baseline token longitud inesperada: %d", len(res.BaselineToken.StringData["token"]))
	}

	// 9. NetworkPolicies (4 políticas del template)
	if len(res.NetworkPolicies) != 4 {
		t.Fatalf("networkPolicies count = %d, esperado 4", len(res.NetworkPolicies))
	}
	npNames := make(map[string]bool)
	for _, np := range res.NetworkPolicies {
		npNames[np.Name] = true
	}
	for _, expected := range []string{"default-deny", "allow-same-namespace", "allow-dns", "allow-from-control-plane"} {
		if !npNames[expected] {
			t.Errorf("falta networkPolicy %s", expected)
		}
	}
}
