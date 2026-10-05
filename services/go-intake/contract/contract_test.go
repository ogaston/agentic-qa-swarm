package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

const contracts = "../../../contracts"

func readJSON(t *testing.T, path string) any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("%s: JSON invalido: %v", path, err)
	}
	return v
}

func compile(t *testing.T, name string, doc any) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()
	if err := c.AddResource(name, doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile(name)
	if err != nil {
		t.Fatalf("compilando %s: %v", name, err)
	}
	return s
}

// openAPISchema extrae components.schemas.<name> de control-plane.yaml.
func openAPISchema(t *testing.T, root map[string]any, name string) *jsonschema.Schema {
	t.Helper()
	comps, _ := root["components"].(map[string]any)
	schemas, _ := comps["schemas"].(map[string]any)
	doc, ok := schemas[name]
	if !ok {
		t.Fatalf("components.schemas.%s no existe", name)
	}
	return compile(t, "mem:///"+name+".json", doc)
}

func TestRESTExamplesAgainstOpenAPI(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(contracts, "openapi/control-plane.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := yaml.Unmarshal(b, &root); err != nil {
		t.Fatal(err)
	}
	cache := map[string]*jsonschema.Schema{}
	schemaFor := func(file string) *jsonschema.Schema {
		n := strings.SplitN(filepath.Base(file), ".", 2)[0]
		if cache[n] == nil {
			cache[n] = openAPISchema(t, root, n)
		}
		return cache[n]
	}
	for _, tc := range []struct {
		dir   string
		valid bool
	}{{"valid", true}, {"invalid", false}} {
		files, err := filepath.Glob(filepath.Join(contracts, "openapi/examples", tc.dir, "*.json"))
		if err != nil {
			t.Fatal(err)
		}
		if len(files) < 5 {
			t.Fatalf("%s: %d ejemplos, se esperan al menos 5", tc.dir, len(files))
		}
		for _, f := range files {
			t.Run(tc.dir+"/"+filepath.Base(f), func(t *testing.T) {
				err := schemaFor(f).Validate(readJSON(t, f))
				if tc.valid && err != nil {
					t.Fatalf("debia ser valido: %v", err)
				}
				if !tc.valid && err == nil {
					t.Fatal("debia ser rechazado")
				}
			})
		}
	}
}

func TestNotifyCreatedExampleValid(t *testing.T) {
	s := compile(t, "mem:///notify.created.json", readJSON(t, filepath.Join(contracts, "events/notify.created.schema.json")))
	if err := s.Validate(readJSON(t, filepath.Join(contracts, "events/examples/valid/notify.created.json"))); err != nil {
		t.Fatal(err)
	}
}
