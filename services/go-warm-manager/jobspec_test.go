package warmmanager_test

import (
	"encoding/json"
	"strings"
	"testing"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
)

var jcfg = wm.JobConfig{AllowedRegistries: wm.DefaultAllowedRegistries}
var okArt = wm.Artifact{Kind: "published-image", Ref: "ghcr.io/ogaston/demo:1.2.3"}

func pod(t *testing.T, m wm.Manifest) map[string]any {
	t.Helper()
	return m["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)
}

func TestBuildDeployJobSecurity(t *testing.T) {
	m, err := wm.BuildDeployJob(jcfg, "r-1", 0, okArt)
	if err != nil {
		t.Fatal(err)
	}
	p := pod(t, m)
	c := p["containers"].([]any)[0].(map[string]any)
	sc := c["securityContext"].(map[string]any)
	psc := p["securityContext"].(map[string]any)
	switch {
	case m["metadata"].(map[string]any)["name"] != "deploy-r-1-0",
		m["metadata"].(map[string]any)["namespace"] != "aqs-test",
		p["automountServiceAccountToken"] != false,
		p["serviceAccountName"] == "" || p["serviceAccountName"] == nil,
		p["hostNetwork"] != false,
		psc["runAsNonRoot"] != true,
		sc["readOnlyRootFilesystem"] != true,
		sc["allowPrivilegeEscalation"] != false:
		t.Fatalf("Job inseguro: %v", m)
	}
	if _, ok := c["envFrom"]; ok {
		t.Fatal("envFrom presente")
	}
	raw, _ := json.Marshal(m)
	s := string(raw)
	for _, bad := range []string{"secretKeyRef", "secretRef", "\"secret\"", ":latest"} {
		if strings.Contains(s, bad) {
			t.Fatalf("contiene %s", bad)
		}
	}
	res := c["resources"].(map[string]any)
	for _, k := range []string{"requests", "limits"} {
		r := res[k].(map[string]any)
		if r["cpu"] == nil || r["memory"] == nil {
			t.Fatalf("resources.%s incompleto", k)
		}
	}
	img := c["image"].(string)
	if !strings.Contains(img, ":") || strings.HasSuffix(img, ":latest") {
		t.Fatalf("imagen sin tag fijado: %s", img)
	}
}

func TestBuildDeployJobRejects(t *testing.T) {
	if _, err := wm.BuildDeployJob(jcfg, "r-1", 0, wm.Artifact{Kind: "published-image", Ref: "docker.io/evil/x:1"}); err == nil {
		t.Fatal("registro ajeno aceptado")
	}
	if _, err := wm.BuildDeployJob(jcfg, "r-1", 0, wm.Artifact{Kind: "published-image", Ref: "ghcr.io/a/b:latest"}); err == nil {
		t.Fatal("latest aceptado")
	}
	if _, err := wm.BuildDeployJob(wm.JobConfig{AllowedRegistries: []string{"ghcr.io"}, DeployerImage: "ghcr.io/x/y:latest"}, "r-1", 0, okArt); err == nil {
		t.Fatal("deployer latest aceptado")
	}
	if _, err := wm.BuildDeployJob(jcfg, "", 0, okArt); err == nil {
		t.Fatal("run vacio aceptado")
	}
}
