package passhash_test

import (
	"bytes"
	"testing"

	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/gen"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/passhash"
)

// argon2id con m=19 MiB cuesta decenas de ms por hash: la propiedad se acota a
// 20 comprobaciones (-rapid.checks se respeta si es menor).
func TestPBT_Hash(t *testing.T) {
	gen.AtMostChecks(t, 20)
	rapid.Check(t, func(t *rapid.T) {
		p := rapid.StringN(0, 40, 128).Draw(t, "password")
		other := rapid.StringN(0, 40, 128).Filter(func(s string) bool { return s != p }).Draw(t, "other")
		salt := rapid.SliceOfN(rapid.Byte(), 16, 16).Draw(t, "salt") // sal inyectada: reproducible con el seed
		h, err := passhash.Hash(p, bytes.NewReader(salt))
		if err != nil {
			t.Fatal(err)
		}
		if err := passhash.Validate(h); err != nil {
			t.Fatalf("hash propio inválido: %v", err)
		}
		if ok, err := passhash.Verify(p, h); err != nil || !ok {
			t.Fatalf("Verify(p, Hash(p)) = %v, %v", ok, err)
		}
		if ok, err := passhash.Verify(other, h); err != nil || ok {
			t.Fatalf("Verify(p', Hash(p)) = %v, %v para p'=%q p=%q", ok, err, other, p)
		}
	})
}
