package plan

import (
	"bytes"
	"encoding/json"
	"fmt"
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
			case strings.HasPrefix(base, "report."):
				var v Report
				if verr = strictDecode(b, &v); verr == nil {
					verr = ValidateReport(v)
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
	if n < 12+4 {
		t.Fatalf("solo %d ejemplos recorridos", n)
	}
}

// U8-T08b: body no objeto e invariant_probe mal ubicado o no booleano son rechazados por el decodificador
// estricto o por ValidateFlowPlan; un body objeto y un invariant_probe booleano se aceptan.
func TestValidateFlowPlanBodyEInvariantProbe(t *testing.T) {
	const base = `{"run_id":"r-1","workflow":"w","flows":[{"flow_id":"f1","name":"n","invariant":"i",%s,"steps":[{"method":"POST","path":"/orders","expect_status":409%s}]}]}`
	cases := []struct {
		name string
		json string
		ok   bool
	}{
		{"body objeto", fmt.Sprintf(base, `"invariant_probe":true`, `,"body":{"qty":5}`), true},
		{"sin campos nuevos", fmt.Sprintf(base, `"invariant_probe":false`, ``), true},
		{"body array", fmt.Sprintf(base, `"invariant_probe":true`, `,"body":[5]`), false},
		{"body null", fmt.Sprintf(base, `"invariant_probe":true`, `,"body":null`), false},
		{"body escalar", fmt.Sprintf(base, `"invariant_probe":true`, `,"body":"x"`), false},
		{"invariant_probe no booleano", fmt.Sprintf(base, `"invariant_probe":"si"`, ``), false},
		{"invariant_probe en el paso", fmt.Sprintf(base, ``, `,"invariant_probe":true`), false},
		{"body en el flujo", `{"run_id":"r-1","workflow":"w","flows":[{"flow_id":"f1","name":"n","invariant":"i","body":{"a":1},"steps":[{"method":"GET","path":"/x","expect_status":200}]}]}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var v FlowPlan
			err := strictDecode([]byte(tc.json), &v)
			if err == nil {
				err = ValidateFlowPlan(v)
			}
			if (err == nil) != tc.ok {
				t.Fatalf("err=%v, quería ok=%v", err, tc.ok)
			}
		})
	}
}
