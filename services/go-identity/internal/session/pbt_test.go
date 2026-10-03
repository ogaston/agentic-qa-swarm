package session_test

import (
	"bytes"
	"encoding/hex"
	"regexp"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/gen"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/session"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/principal"
)

var (
	tokenRe = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	idRe    = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

func TestPBT_Token(t *testing.T) {
	gen.AtLeastChecks(t, 300)
	at := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(2, 8).Draw(t, "n")
		// n bloques aleatorios de 32 bytes, distintos entre sí (la fuente es inyectada: reproducible con el seed)
		blocks := rapid.SliceOfNDistinct(rapid.SliceOfN(rapid.Byte(), 32, 32), n, n, func(b []byte) string { return hex.EncodeToString(b) }).Draw(t, "blocks")
		s := session.New(session.Config{MaxPerUser: 100, Clock: func() time.Time { return at }, Rand: bytes.NewReader(bytes.Join(blocks, nil))})
		who := rapid.SampledFrom(gen.IDs).Draw(t, "user")
		tokens, ids := map[string]bool{}, map[string]bool{}
		for i := 0; i < n; i++ {
			p := principal.Principal{ID: who, Role: principal.RoleUser}
			if rapid.Bool().Draw(t, "other-user") {
				p.ID = rapid.SampledFrom(gen.IDs).Draw(t, "user2")
			}
			tok, info, err := s.CreateSession(p)
			if err != nil {
				t.Fatal(err)
			}
			if !tokenRe.MatchString(tok) || !idRe.MatchString(info.ID) {
				t.Fatalf("formato inválido: token %q id %q", tok, info.ID)
			}
			if tok == info.ID || tokens[tok] || ids[info.ID] {
				t.Fatalf("token o session_id repetidos: %q %q", tok, info.ID)
			}
			tokens[tok], ids[info.ID] = true, true
		}
	})
}

// Con la fuente real (crypto/rand) los tokens también cumplen el patrón y no se repiten.
func TestPBT_Examples_TokenCryptoRand(t *testing.T) {
	s := session.New(session.Config{MaxPerUser: 100})
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		tok, info, err := s.CreateSession(principal.Principal{ID: "u1", Role: principal.RoleUser})
		if err != nil || !tokenRe.MatchString(tok) || !idRe.MatchString(info.ID) || seen[tok] || seen[info.ID] {
			t.Fatalf("token %q id %q err %v", tok, info.ID, err)
		}
		seen[tok], seen[info.ID] = true, true
	}
}
