package principal

import (
	"encoding/json"
	"os"
	"testing"
)

func TestParseRole(t *testing.T) {
	for _, s := range []string{"user", "admin"} {
		if r, err := ParseRole(s); err != nil || string(r) != s {
			t.Fatalf("%q: %v", s, err)
		}
	}
	for _, s := range []string{"", "root", "ADMIN", "user "} {
		if _, err := ParseRole(s); err == nil {
			t.Fatalf("%q debía fallar", s)
		}
	}
}

func TestPrincipalFixtures(t *testing.T) {
	b, err := os.ReadFile("../testdata/principals.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Valid   []Principal `json:"valid"`
		Invalid []Principal `json:"invalid"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Valid) < 3 || len(f.Invalid) < 3 {
		t.Fatal("fixtures insuficientes")
	}
	for _, p := range f.Valid {
		if err := p.Validate(); err != nil {
			t.Fatalf("%+v: %v", p, err)
		}
	}
	for _, p := range f.Invalid {
		if err := p.Validate(); err == nil {
			t.Fatalf("%+v debía ser inválido", p)
		}
	}
}
