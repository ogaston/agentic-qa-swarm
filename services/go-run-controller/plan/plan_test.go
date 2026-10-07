package plan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const plansDir = "../../../contracts/plans"

func compile(t *testing.T, dir, name string) *jsonschema.Schema {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name+".schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()
	if err := c.AddResource("mem:///"+name+".json", doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("mem:///" + name + ".json")
	if err != nil {
		t.Fatalf("compilando %s: %v", name, err)
	}
	return s
}

func readJSON(t *testing.T, p string) any {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("%s: %v", p, err)
	}
	return v
}

// examplesDir permite a la prueba negativa apuntar a un directorio temporal.
func checkExamples(t *testing.T, dir string, minValid, minInvalid int) {
	t.Helper()
	cache := map[string]*jsonschema.Schema{}
	counts := map[bool]int{}
	for _, tc := range []struct {
		sub   string
		valid bool
	}{{"valid", true}, {"invalid", false}} {
		files, err := filepath.Glob(filepath.Join(dir, "examples", tc.sub, "*.json"))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			n := strings.SplitN(filepath.Base(f), ".", 2)[0]
			if cache[n] == nil {
				cache[n] = compile(t, dir, n)
			}
			err := cache[n].Validate(readJSON(t, f))
			if tc.valid && err != nil {
				t.Errorf("%s debia ser valido: %v", f, err)
			}
			if !tc.valid && err == nil {
				t.Errorf("%s debia ser rechazado", f)
			}
			counts[tc.valid]++
		}
	}
	if counts[true] < minValid || counts[false] < minInvalid {
		t.Errorf("ejemplos insuficientes: %d validos, %d invalidos", counts[true], counts[false])
	}
}

func TestPlanExamplesAgainstSchemas(t *testing.T) {
	checkExamples(t, plansDir, 8, 12)
}

// Validate valida v contra el esquema <name> (util para los stubs via copia en prueba).
func TestExamplesDecodeIntoTypes(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(plansDir, "examples/valid/flow-plan.tres-flujos.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fp FlowPlan
	if err := json.Unmarshal(b, &fp); err != nil || len(fp.Flows) != 3 {
		t.Fatalf("decode: %v %+v", err, fp)
	}
}
