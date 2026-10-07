package warmmanager_test

import (
	"bytes"
	"encoding/json"
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
