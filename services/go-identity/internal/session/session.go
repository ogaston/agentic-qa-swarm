// Package session mantiene sesiones en memoria con expiración absoluta e inactiva.
// Solo se guarda el SHA-256 del token; el token en claro nunca se retiene.
package session

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"sync"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-identity/principal"
)

// Clock es el reloj inyectable.
type Clock func() time.Time

// Config son los límites del almacén.
type Config struct {
	AbsoluteTTL time.Duration
	IdleTTL     time.Duration
	MaxPerUser  int
	Clock       Clock     // nil usa time.Now
	Rand        io.Reader // nil usa crypto/rand
}

type entry struct {
	who      principal.Principal
	created  time.Time
	lastSeen time.Time
}

// Store es el almacén de sesiones.
type Store struct {
	cfg     Config
	mu      sync.Mutex
	byHash  map[[sha256.Size]byte]*entry
	perUser map[string][][sha256.Size]byte // más antigua primero
}

// New crea un almacén. Los valores no positivos toman los valores por defecto.
func New(cfg Config) *Store {
	if cfg.AbsoluteTTL <= 0 {
		cfg.AbsoluteTTL = 30 * time.Minute
	}
	if cfg.IdleTTL <= 0 {
		cfg.IdleTTL = 15 * time.Minute
	}
	if cfg.MaxPerUser <= 0 {
		cfg.MaxPerUser = 5
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.Rand == nil {
		cfg.Rand = rand.Reader
	}
	return &Store{cfg: cfg, byHash: map[[sha256.Size]byte]*entry{}, perUser: map[string][][sha256.Size]byte{}}
}

// Create abre una sesión y devuelve el token (32 bytes base64url) y su expiración absoluta.
func (s *Store) Create(who principal.Principal) (string, time.Time, error) {
	var raw [32]byte
	if _, err := io.ReadFull(s.cfg.Rand, raw[:]); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	h := sha256.Sum256([]byte(token))
	now := s.cfg.Clock()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeLocked(who.ID, now)
	list := s.perUser[who.ID]
	for len(list) >= s.cfg.MaxPerUser {
		delete(s.byHash, list[0])
		list = list[1:]
	}
	s.byHash[h] = &entry{who: who, created: now, lastSeen: now}
	s.perUser[who.ID] = append(list, h)
	return token, now.Add(s.cfg.AbsoluteTTL), nil
}

// Validate devuelve el principal si la sesión es vigente y renueva su inactividad.
func (s *Store) Validate(token string) (principal.Principal, bool) {
	h := sha256.Sum256([]byte(token))
	now := s.cfg.Clock()
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.byHash[h]
	if !ok {
		return principal.Principal{}, false
	}
	if s.expired(e, now) {
		s.removeLocked(e.who.ID, h)
		return principal.Principal{}, false
	}
	e.lastSeen = now
	return e.who, true
}

// Revoke revoca la sesión si es vigente; false si no existía, ya estaba revocada o expiró.
func (s *Store) Revoke(token string) bool {
	h := sha256.Sum256([]byte(token))
	now := s.cfg.Clock()
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.byHash[h]
	if !ok {
		return false
	}
	live := !s.expired(e, now)
	s.removeLocked(e.who.ID, h)
	return live
}

func (s *Store) expired(e *entry, now time.Time) bool {
	return !now.Before(e.created.Add(s.cfg.AbsoluteTTL)) || !now.Before(e.lastSeen.Add(s.cfg.IdleTTL))
}

func (s *Store) removeLocked(user string, h [sha256.Size]byte) {
	delete(s.byHash, h)
	list := s.perUser[user]
	for i, x := range list {
		if x == h {
			list = append(list[:i:i], list[i+1:]...)
			break
		}
	}
	if len(list) == 0 {
		delete(s.perUser, user)
	} else {
		s.perUser[user] = list
	}
}

func (s *Store) purgeLocked(user string, now time.Time) {
	for _, h := range append([][sha256.Size]byte(nil), s.perUser[user]...) {
		if e, ok := s.byHash[h]; ok && s.expired(e, now) {
			s.removeLocked(user, h)
		}
	}
}
