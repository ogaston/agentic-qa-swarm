package main

import (
	"bufio"
	"os"
	"strings"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/adapters"
)

// valoresDeEnv devuelve el `value:` de cada `- name: <name>` del manifiesto real.
func valoresDeEnv(t *testing.T, path, name string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	prevName := false
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if prevName && strings.HasPrefix(l, "value:") {
			out = append(out, strings.TrimSpace(strings.TrimPrefix(l, "value:")))
		}
		prevName = l == "- name: "+name
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// U11-T03 F-01: el valor LITERAL del manifiesto pasa loadConfig (parser real del controlador); el
// RUN_ARTIFACT_REF del mismo manifiesto queda aceptado y …/target-app-evil:1 rechazado.
func TestManifiestoPrefijosImagenControladorSeCargan(t *testing.T) {
	const manifest = "../../../../deploy/flux/base/control-plane.yaml"
	vals := valoresDeEnv(t, manifest, "WARM_ALLOWED_IMAGE_PREFIXES")
	if len(vals) != 2 {
		t.Fatalf("WARM_ALLOWED_IMAGE_PREFIXES en el manifiesto: %d valores, quiero 2", len(vals))
	}
	refs := valoresDeEnv(t, manifest, "RUN_ARTIFACT_REF")
	if len(refs) != 1 {
		t.Fatalf("RUN_ARTIFACT_REF en el manifiesto: %d valores", len(refs))
	}
	for _, v := range vals {
		env := realEnv()
		env["RUN_ARTIFACT_REF"] = refs[0]
		env["WARM_ALLOWED_IMAGE_PREFIXES"] = v
		c, err := loadConfig(envOf(env))
		if err != nil {
			t.Fatalf("el valor literal del manifiesto no arranca el controlador: %v", err)
		}
		if err := adapters.ValidateImage("published-image", refs[0], c.imagePrefixes); err != nil {
			t.Fatalf("RUN_ARTIFACT_REF %q rechazado con el manifiesto: %v", refs[0], err)
		}
		if err := adapters.ValidateImage("published-image", "ghcr.io/ogaston/agentic-qa-swarm/target-app-evil:1", c.imagePrefixes); err == nil {
			t.Fatal("…/target-app-evil:1 debe rechazarse con el manifiesto")
		}
	}
}
