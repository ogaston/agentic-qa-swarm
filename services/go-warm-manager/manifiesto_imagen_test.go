package warmmanager_test

import (
	"bufio"
	"os"
	"strings"
	"testing"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
)

// valoresDeEnv devuelve el `value:` de cada `- name: <name>` del manifiesto (leído del archivo real, no copiado).
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

// U11-T03 F-01: el valor LITERAL de control-plane.yaml debe arrancar el parser real de go-warm-manager y
// dejar pasar RUN_ARTIFACT_REF del mismo manifiesto, y rechazar un repositorio hermano (-evil).
func TestManifiestoPrefijosImagenWarmSeCargan(t *testing.T) {
	const manifest = "../../deploy/flux/base/control-plane.yaml"
	vals := valoresDeEnv(t, manifest, "WARM_ALLOWED_IMAGE_PREFIXES")
	if len(vals) != 2 {
		t.Fatalf("WARM_ALLOWED_IMAGE_PREFIXES en el manifiesto: %d valores, quiero 2", len(vals))
	}
	refs := valoresDeEnv(t, manifest, "RUN_ARTIFACT_REF")
	if len(refs) != 1 {
		t.Fatalf("RUN_ARTIFACT_REF en el manifiesto: %d valores", len(refs))
	}
	for _, v := range vals {
		prefs, err := wm.ParseImagePrefixes(v)
		if err != nil {
			t.Fatalf("el valor literal del manifiesto no arranca go-warm-manager: %v", err)
		}
		if err := wm.ValidateArtifact(wm.Artifact{Kind: "published-image", Ref: refs[0]}, []string{"ghcr.io"}, prefs); err != nil {
			t.Fatalf("RUN_ARTIFACT_REF %q rechazado con el manifiesto: %v", refs[0], err)
		}
		if err := wm.ValidateArtifact(wm.Artifact{Kind: "published-image", Ref: "ghcr.io/ogaston/agentic-qa-swarm/target-app-evil:1"}, []string{"ghcr.io"}, prefs); err == nil {
			t.Fatal("…/target-app-evil:1 debe rechazarse con el manifiesto")
		}
	}
}
