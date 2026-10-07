package plan

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func strictDecode(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	return d.Decode(v)
}

// Los validadores a mano coinciden con el veredicto de los ejemplos válidos/inválidos del contrato.
func TestValidatorsAgreeWithContractExamples(t *testing.T) {
	n := 0
	for _, kind := range []string{"valid", "invalid"} {
		files, _ := filepath.Glob(filepath.Join("..", "..", "..", "contracts", "plans", "examples", kind, "*.json"))
		for _, f := range files {
			base := filepath.Base(f)
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var verr error
			switch {
			case strings.HasPrefix(base, "flow-plan."):
				var v FlowPlan
				if verr = strictDecode(b, &v); verr == nil {
					verr = ValidateFlowPlan(v)
				}
			case strings.HasPrefix(base, "surface-artifact."):
				var v SurfaceArtifact
				if verr = strictDecode(b, &v); verr == nil {
					verr = ValidateSurface(v)
				}
			case strings.HasPrefix(base, "warm-state."):
				var v WarmState
				if verr = strictDecode(b, &v); verr == nil {
					verr = ValidateWarmState(v)
				}
			default:
				continue
			}
			n++
			if (kind == "valid") != (verr == nil) {
				t.Errorf("%s/%s: veredicto %v", kind, base, verr)
			}
		}
	}
	if n < 12 {
		t.Fatalf("solo %d ejemplos recorridos", n)
	}
}
