package inbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const events = "../../../contracts/events"

func schema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(events, "notify.created.schema.json"))
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
	if err := c.AddResource("mem:///n.json", doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("mem:///n.json")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func readFile(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestParseAgreesWithSchema exige que el validador manual coincida con el
// esquema real en los ejemplos del contrato y en mutaciones del valido.
func TestParseAgreesWithSchema(t *testing.T) {
	s := schema(t)
	valid := readFile(t, filepath.Join(events, "examples/valid/notify.created.json"))
	cases := map[string][]byte{"valid": valid}
	invalid, _ := filepath.Glob(filepath.Join(events, "examples/invalid/notify.created.*.json"))
	if len(invalid) != 3 {
		t.Fatalf("se esperaban 3 ejemplos invalidos, hay %d", len(invalid))
	}
	for _, f := range invalid {
		cases[filepath.Base(f)] = readFile(t, f)
	}
	mut := func(name, old, repl string) {
		if !strings.Contains(string(valid), old) {
			t.Fatalf("mutacion %s: no encuentra %q", name, old)
		}
		cases["mut-"+name] = []byte(strings.Replace(string(valid), old, repl, 1))
	}
	mut("sha-corta", `aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`, `aaaa"`)
	mut("sha-mayuscula", `"sha": "aaaa`, `"sha": "AAAA`)
	mut("version", `"version": 1`, `"version": 2`)
	mut("uuid", `3f2b8c1e-6a4d-4e7b-9c10-5d2e8a7f1b34`, `no-es-uuid`)
	mut("fecha", `2026-01-15T10:00:00Z`, `ayer`)
	mut("kind", `"build-from-repo"`, `"otro"`)
	mut("extra", `"trace_id"`, `"x": 1, "trace_id"`)
	mut("extra-data", `"repo"`, `"x": 1, "repo"`)
	mut("trace-vacio", `4bf92f3577b34da6a3ce929d0e0e4736`, ``)
	mut("ref-vacio", `"ref": "acme/shop@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`, `"ref": ""`)
	for _, k := range []string{"event_id", "type", "version", "occurred_at", "trace_id", "data",
		"notification_id", "github_event", "repo", "sha", "artifact", "kind", "ref"} {
		mut("mayus-"+k, `"`+k+`":`, `"`+strings.ToUpper(k)+`":`)
		mut("capital-"+k, `"`+k+`":`, `"`+strings.ToUpper(k[:1])+k[1:]+`":`)
	}
	cases["no-json"] = []byte(`{`)
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			var v any
			schemaOK := json.Unmarshal(b, &v) == nil && s.Validate(v) == nil
			_, err := ParseNotifyCreated(b)
			if (err == nil) != schemaOK {
				t.Fatalf("esquema valido=%v pero ParseNotifyCreated err=%v", schemaOK, err)
			}
			if name == "valid" && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func compact(b []byte) (string, error) {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return "", err
	}
	o, err := json.Marshal(v)
	return string(o), err
}
