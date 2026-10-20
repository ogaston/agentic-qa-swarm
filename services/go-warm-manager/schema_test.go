package warmmanager_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// validate valida v (serializado) contra contracts/<rel> con el esquema real, con formatos.
func validate(t testing.TB, rel string, v any) error {
	t.Helper()
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	sch, err := c.Compile(filepath.Join("..", "..", "contracts", rel))
	if err != nil {
		t.Fatalf("esquema %s: %v", rel, err)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	return sch.Validate(inst)
}

// CA-1: el contrato acepta request_body opcional en el endpoint y rechaza lo malformado.
func TestSurfaceRequestBodyEjemplos(t *testing.T) {
	cases := map[string]bool{
		"valid/surface-artifact.request-body.json":               true,
		"invalid/surface-artifact.request-body-no-objeto.json":   false,
		"invalid/surface-artifact.request-body-campo-extra.json": false,
	}
	for rel, want := range cases {
		t.Run(rel, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("..", "..", "contracts", "plans", "examples", rel))
			if err != nil {
				t.Fatal(err)
			}
			var v any
			if err := json.Unmarshal(raw, &v); err != nil {
				t.Fatal(err)
			}
			if got := validate(t, "plans/surface-artifact.schema.json", v) == nil; got != want {
				t.Fatalf("esquema acepta=%v, se esperaba %v", got, want)
			}
		})
	}
}
