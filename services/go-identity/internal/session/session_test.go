package session

import (
	"bytes"
	"crypto/sha256"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-identity/principal"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

var who = principal.Principal{ID: "marta", Role: principal.RoleUser}

func newStore(max int) (*Store, *fakeClock) {
	c := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	return New(Config{AbsoluteTTL: 30 * time.Minute, IdleTTL: 15 * time.Minute, MaxPerUser: max, Clock: c.now}), c
}

func TestSessionCreateAndValidate(t *testing.T) {
	s, c := newStore(5)
	tok, exp, err := s.Create(who)
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) != 43 {
		t.Fatalf("token de %d caracteres, esperado 43", len(tok))
	}
	if !exp.Equal(c.now().Add(30 * time.Minute)) {
		t.Fatalf("expiración %v", exp)
	}
	if p, ok := s.Validate(tok); !ok || p != who {
		t.Fatalf("sesión debía ser vigente: %v %v", p, ok)
	}
}

func TestSessionExpiresAtAbsoluteTTL(t *testing.T) {
	s, c := newStore(5)
	tok, _, _ := s.Create(who)
	for i := 0; i < 5; i++ { // usos frecuentes no evitan la expiración absoluta
		c.add(5 * time.Minute)
		if _, ok := s.Validate(tok); !ok {
			t.Fatalf("a %d min seguía vigente", (i+1)*5)
		}
	}
	c.add(5 * time.Minute) // 30 min
	if _, ok := s.Validate(tok); ok {
		t.Fatal("a los 30 min debía expirar por TTL absoluto")
	}
}

func TestSessionIdleExpires(t *testing.T) {
	s, c := newStore(5)
	tok, _, _ := s.Create(who)
	c.add(15 * time.Minute)
	if _, ok := s.Validate(tok); ok {
		t.Fatal("a 15 min sin uso debía expirar por inactividad")
	}
}

func TestSessionIdleRenewedByUseButNotAbsolute(t *testing.T) {
	s, c := newStore(5)
	tok, _, _ := s.Create(who)
	c.add(14 * time.Minute)
	if _, ok := s.Validate(tok); !ok {
		t.Fatal("a 14 min seguía vigente")
	}
	c.add(14 * time.Minute) // 28 min en total, 14 desde el último uso
	if _, ok := s.Validate(tok); !ok {
		t.Fatal("el uso reciente debía renovar la inactividad")
	}
	c.add(14 * time.Minute) // 42 min > absoluto
	if _, ok := s.Validate(tok); ok {
		t.Fatal("el uso no renueva la expiración absoluta")
	}
}

func TestSessionRevokedByLogout(t *testing.T) {
	s, _ := newStore(5)
	tok, _, _ := s.Create(who)
	if !s.Revoke(tok) {
		t.Fatal("la primera revocación debía tener éxito")
	}
	if _, ok := s.Validate(tok); ok {
		t.Fatal("sesión revocada debía ser inválida de inmediato")
	}
	if s.Revoke(tok) {
		t.Fatal("revocar dos veces debía fallar")
	}
}

func TestSessionRevokeExpiredIsNotLive(t *testing.T) {
	s, c := newStore(5)
	tok, _, _ := s.Create(who)
	c.add(time.Hour)
	if s.Revoke(tok) {
		t.Fatal("una sesión expirada no cuenta como revocada con éxito")
	}
}

func TestSessionLimitDropsOldest(t *testing.T) {
	s, c := newStore(3)
	var toks []string
	for i := 0; i < 4; i++ {
		tok, _, _ := s.Create(who)
		toks = append(toks, tok)
		c.add(time.Second)
	}
	if _, ok := s.Validate(toks[0]); ok {
		t.Fatal("la más antigua debía descartarse")
	}
	for _, tok := range toks[1:] {
		if _, ok := s.Validate(tok); !ok {
			t.Fatal("las tres recientes debían seguir vigentes")
		}
	}
	other, _, _ := s.Create(principal.Principal{ID: "otro", Role: principal.RoleUser})
	if _, ok := s.Validate(toks[1]); !ok {
		t.Fatal("el límite es por usuario")
	}
	_ = other
}

func TestSessionStoresOnlyTokenHash(t *testing.T) {
	s, _ := newStore(5)
	tok, _, _ := s.Create(who)
	h := sha256.Sum256([]byte(tok))
	if _, ok := s.byHash[h]; !ok || len(s.byHash) != 1 {
		t.Fatal("el almacén debe indexar por SHA-256 del token")
	}
	if got := s.perUser[who.ID]; len(got) != 1 || got[0] != h {
		t.Fatal("el índice por usuario debe guardar el hash")
	}
}

func TestSessionTokenUsesInjectedRandom(t *testing.T) {
	c := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	s := New(Config{Clock: c.now, Rand: bytes.NewReader(bytes.Repeat([]byte{0}, 32))})
	tok, _, err := s.Create(who)
	if err != nil || tok != strings.Repeat("A", 43) {
		t.Fatalf("%q %v", tok, err)
	}
	if _, _, err := s.Create(who); err == nil {
		t.Fatal("sin entropía debía fallar")
	}
}

func TestSessionConcurrentLogoutAndUse(t *testing.T) {
	s, _ := newStore(5)
	tok, _, _ := s.Create(who)
	var wg sync.WaitGroup
	revoked := make(chan bool, 50)
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); s.Validate(tok) }()
		go func() { defer wg.Done(); revoked <- s.Revoke(tok) }()
	}
	wg.Wait()
	close(revoked)
	n := 0
	for r := range revoked {
		if r {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("exactamente una revocación debía tener éxito, hubo %d", n)
	}
}
