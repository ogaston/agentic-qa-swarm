package users

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/passhash"
)

func gen(t *testing.T) (hash, secret string) {
	t.Helper()
	pw := make([]byte, 12)
	_, _ = rand.Read(pw)
	h, err := passhash.Hash(string(pw), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := make([]byte, 20)
	_, _ = rand.Read(s)
	return h, base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(s)
}

func enc(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type u = map[string]string

func TestParseOK(t *testing.T) {
	h, s := gen(t)
	got, err := Parse(enc(t, []u{{"username": "Marta", "password_hash": h, "role": "user"}, {"username": "julian", "password_hash": h, "role": "admin", "mfa_secret": s}}))
	if err != nil {
		t.Fatal(err)
	}
	if got["marta"] == nil || got["julian"].MFASecret == nil || len(got) != 2 {
		t.Fatalf("usuarios mal cargados: %+v", got)
	}
}

func TestParseRejects(t *testing.T) {
	h, s := gen(t)
	short := strings.Repeat("A", 16)
	cases := map[string][]byte{
		"vacío":                []byte(`[]`),
		"no es JSON":           []byte(`{`),
		"hash no argon2id":     enc(t, []u{{"username": "a", "password_hash": "plano", "role": "user"}}),
		"rol inválido":         enc(t, []u{{"username": "a", "password_hash": h, "role": "root"}}),
		"admin sin mfa":        enc(t, []u{{"username": "a", "password_hash": h, "role": "admin"}}),
		"mfa corto":            enc(t, []u{{"username": "a", "password_hash": h, "role": "admin", "mfa_secret": short}}),
		"duplicado":            enc(t, []u{{"username": "a", "password_hash": h, "role": "user"}, {"username": "A", "password_hash": h, "role": "user"}}),
		"username vacío":       enc(t, []u{{"username": "", "password_hash": h, "role": "user"}}),
		"campo desconocido":    []byte(`[{"username":"a","role":"user","password":"x"}]`),
		"mfa de user inválido": enc(t, []u{{"username": "a", "password_hash": h, "role": "user", "mfa_secret": "!!"}}),
	}
	_ = s
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(b); err == nil {
				t.Fatal("debía rechazarse")
			}
		})
	}
}

func TestLoadFile(t *testing.T) {
	if _, err := LoadFile(""); err == nil {
		t.Fatal("sin ruta debía fallar")
	}
	if _, err := LoadFile(filepath.Join(t.TempDir(), "no-existe.json")); err == nil {
		t.Fatal("archivo ausente debía fallar")
	}
	h, _ := gen(t)
	p := filepath.Join(t.TempDir(), "u.json")
	if err := os.WriteFile(p, enc(t, []u{{"username": "a", "password_hash": h, "role": "user"}}), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadFile(p); err != nil || len(got) != 1 {
		t.Fatalf("%v %v", got, err)
	}
}
