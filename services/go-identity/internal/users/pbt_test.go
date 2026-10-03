package users_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/gen"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/users"
)

type fileEntry struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	Role         string `json:"role"`
	MFASecret    string `json:"mfa_secret,omitempty"`
}

// serialize escribe el formato de IDENTITY_USERS_FILE.
func serialize(t *rapid.T, us []gen.User) []byte {
	list := make([]fileEntry, 0, len(us))
	for _, u := range us {
		list = append(list, fileEntry{u.Username, u.Hash, u.Role, u.MFAText})
	}
	b, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPBT_RoundTrip_UsersFile(t *testing.T) {
	gen.AtLeastChecks(t, 300)
	rapid.Check(t, func(t *rapid.T) {
		us := gen.ValidUsers().Draw(t, "users")
		raw := serialize(t, us)
		got, err := users.Parse(raw)
		if err != nil {
			t.Fatalf("archivo válido rechazado: %v\n%s", err, raw)
		}
		if len(got) != len(us) {
			t.Fatalf("%d usuarios, esperados %d", len(got), len(us))
		}
		for _, u := range us { // el orden no importa: la carga normaliza a un mapa por users.Key
			g, ok := got[users.Key(u.Username)]
			if !ok || g.Name != u.Username || g.PasswordHash != u.Hash || string(g.Role) != u.Role || !bytes.Equal(g.MFASecret, u.MFASecret) {
				t.Fatalf("usuario %q no sobrevive al round-trip: %+v", u.Username, g)
			}
		}
	})
}

func TestPBT_UsersFile_RejectsInvalid(t *testing.T) {
	gen.AtLeastChecks(t, 300)
	rapid.Check(t, func(t *rapid.T) {
		in := gen.InvalidUsers().Draw(t, "invalid")
		raw := serialize(t, in.Users)
		if got, err := users.Parse(raw); err == nil {
			t.Fatalf("archivo inválido (%s) aceptado con %d usuarios:\n%s", in.Why, len(got), raw)
		}
	})
}

// Ejemplos fijos: uno válido y uno inválido por regla.
func TestPBT_Examples_UsersFile(t *testing.T) {
	hash := "$argon2id$v=19$m=19456,t=2,p=1$c2FsdHNhbHQ$YWFhYWFhYWFhYWFhYWFhYQ"
	secret := "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"
	mk := func(name, role, h, mfa string) []byte {
		b, _ := json.Marshal([]fileEntry{{name, h, role, mfa}})
		return b
	}
	if _, err := users.Parse(mk("Ana", "admin", hash, secret)); err != nil {
		t.Errorf("válido: %v", err)
	}
	for name, raw := range map[string][]byte{
		"hash bcrypt":    mk("a", "user", "$2a$12$xxxxxxxxxxxxxxxxxxxxxx", ""),
		"rol Admin":      mk("a", "Admin", hash, secret),
		"admin sin mfa":  mk("a", "admin", hash, ""),
		"mfa corto":      mk("a", "user", hash, "JBSWY3DP"),
		"username vacío": mk("", "user", hash, ""),
		"duplicado":      []byte(`[{"username":"Ana","password_hash":"` + hash + `","role":"user"},{"username":"ANA","password_hash":"` + hash + `","role":"user"}]`),
	} {
		if _, err := users.Parse(raw); err == nil {
			t.Errorf("%s: aceptado", name)
		}
	}
}
