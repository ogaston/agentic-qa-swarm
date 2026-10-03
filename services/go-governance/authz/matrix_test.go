package authz

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type matrixRow struct {
	Name           string    `json:"name"`
	Input          GateInput `json:"input"`
	Expect         string    `json:"expect"`
	ReasonContains string    `json:"reason_contains"`
}

func loadMatrix(t *testing.T, path string) []matrixRow {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rows []matrixRow
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

// checkShape valida la forma de la matriz y devuelve los errores.
func checkShape(rows []matrixRow) []string {
	var errs []string
	if len(rows) < 14 {
		errs = append(errs, "menos de 14 filas")
	}
	for _, r := range rows {
		if r.Name == "" {
			errs = append(errs, "fila sin name")
		}
		if r.Expect != "allow" && r.Expect != "deny" {
			errs = append(errs, r.Name+": expect inválido")
		}
		if r.Expect == "deny" && r.ReasonContains == "" {
			errs = append(errs, r.Name+": deny sin reason_contains")
		}
		if r.Expect == "allow" {
			in := r.Input
			for _, f := range []Fact{in.Confirmed, in.ResetVerified, in.EnsayoPassed, in.WorkflowAllowed} {
				if f == Unknown {
					errs = append(errs, r.Name+": allow con hecho unknown")
				}
			}
		}
	}
	return errs
}

func TestMatrixShape(t *testing.T) {
	if errs := checkShape(loadMatrix(t, "../testdata/authorize_matrix.json")); len(errs) > 0 {
		t.Fatal(errs)
	}
}

func TestMatrixShapeNegative(t *testing.T) {
	rows := []matrixRow{{Name: "mala", Expect: "allow", Input: GateInput{Confirmed: True, ResetVerified: Unknown}}}
	if errs := checkShape(rows); len(errs) == 0 {
		t.Fatal("la forma debía fallar con allow + unknown")
	}
}

// TestFakeWiringReplay programa el fake con la verdad de cada fila: solo valida el
// cableado del fake (es circular). La fuerza real está en TestMatrixReal.
func TestFakeWiringReplay(t *testing.T) {
	for _, r := range loadMatrix(t, "../testdata/authorize_matrix.json") {
		t.Run(r.Name, func(t *testing.T) {
			f := NewFakeEvaluator()
			if r.Expect == "allow" {
				f.Allow(r.Input.From, r.Input.To)
			} else {
				f.Deny(r.Input.From, r.Input.To, r.ReasonContains)
			}
			d, err := f.AuthorizeTransition(context.Background(), r.Input)
			if err != nil {
				t.Fatal(err)
			}
			if d.Allow != (r.Expect == "allow") {
				t.Fatalf("allow=%v esperado %s (%s)", d.Allow, r.Expect, d.Reason)
			}
			if !d.Allow && !strings.Contains(strings.ToLower(d.Reason), strings.ToLower(r.ReasonContains)) {
				t.Fatalf("reason %q no contiene %q", d.Reason, r.ReasonContains)
			}
		})
	}
}

// TestMatrixReal reproduce cada fila contra el evaluador REAL, sin programarlo
// con el veredicto de la fila (C-54).
func TestMatrixReal(t *testing.T) {
	ev := &RuleEvaluator{TestNamespace: "aqs-test"}
	for _, r := range loadMatrix(t, "../testdata/authorize_matrix.json") {
		t.Run(r.Name, func(t *testing.T) {
			d, err := ev.AuthorizeTransition(context.Background(), r.Input)
			if err != nil {
				t.Fatal(err)
			}
			if d.Allow != (r.Expect == "allow") {
				t.Fatalf("allow=%v esperado %s (%s)", d.Allow, r.Expect, d.Reason)
			}
			if !d.Allow {
				if d.Reason == "" {
					t.Fatal("deny sin razón")
				}
				if !strings.Contains(strings.ToLower(d.Reason), strings.ToLower(r.ReasonContains)) {
					t.Fatalf("reason %q no contiene %q", d.Reason, r.ReasonContains)
				}
			}
		})
	}
}
