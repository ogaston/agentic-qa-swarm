// Package session mantiene sesiones en memoria con expiración absoluta e inactiva.
// Solo se guarda el SHA-256 del token; el token en claro nunca se retiene.
package session

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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

// Info describe una sesión vigente. ID es público y distinto del token: conocerlo no da acceso.
type Info struct {
	Principal principal.Principal
	ID        string
	ExpiresAt time.Time // expiración absoluta
}

type entry struct {
	id       string
	who      principal.Principal
	created  time.Time
	lastSeen time.Time
}

// Store es el almacén de sesiones.
type Store struct {
	cfg     Config
	mu      sync.Mutex
	byHash  map[[sha256.Size]byte]*entry
	byID    map[string][sha256.Size]byte
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
	return &Store{cfg: cfg, byHash: map[[sha256.Size]byte]*entry{}, byID: map[string][sha256.Size]byte{}, perUser: map[string][][sha256.Size]byte{}}
}

// sessionID deriva el identificador público (128 bits, hex) del token con separación de dominio.
func sessionID(token string) string {
	d := sha256.Sum256([]byte("session-id\x00" + token))
	return hex.EncodeToString(d[:16])
}

// Create abre una sesión y devuelve el token (32 bytes base64url) y su expiración absoluta.
func (s *Store) Create(who principal.Principal) (string, time.Time, error) {
	tok, info, err := s.CreateSession(who)
	return tok, info.ExpiresAt, err
}

// CreateSession es Create pero devuelve también el Info (con el ID público de la sesión).
func (s *Store) CreateSession(who principal.Principal) (string, Info, error) {
	var raw [32]byte
	if _, err := io.ReadFull(s.cfg.Rand, raw[:]); err != nil {
		return "", Info{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	h := sha256.Sum256([]byte(token))
	now := s.cfg.Clock()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeLocked(who.ID, now)
	list := s.perUser[who.ID]
	for len(list) >= s.cfg.MaxPerUser {
		if old, ok := s.byHash[list[0]]; ok {
			delete(s.byID, old.id)
		}
		delete(s.byHash, list[0])
		list = list[1:]
	}
	e := &entry{id: sessionID(token), who: who, created: now, lastSeen: now}
	s.byHash[h] = e
	s.byID[e.id] = h
	s.perUser[who.ID] = append(list, h)
	return token, s.infoLocked(e), nil
}

func (s *Store) infoLocked(e *entry) Info {
	return Info{Principal: e.who, ID: e.id, ExpiresAt: e.created.Add(s.cfg.AbsoluteTTL)}
}

// Validate devuelve el principal si la sesión es vigente y renueva su inactividad.
func (s *Store) Validate(token string) (principal.Principal, bool) {
	info, ok := s.Authenticate(token)
	return info.Principal, ok
}

// Authenticate es Validate pero devuelve el Info completo. Sin caché: se evalúa en cada llamada.
func (s *Store) Authenticate(token string) (Info, bool) {
	h := sha256.Sum256([]byte(token))
	now := s.cfg.Clock()
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.byHash[h]
	if !ok {
		return Info{}, false
	}
	if s.expired(e, now) {
		s.removeLocked(e.who.ID, h)
		return Info{}, false
	}
	e.lastSeen = now
	return s.infoLocked(e), true
}

// Lookup busca una sesión vigente por su ID público. No renueva la inactividad.
func (s *Store) Lookup(id string) (Info, bool) {
	now := s.cfg.Clock()
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.byID[id]
	if !ok {
		return Info{}, false
	}
	e := s.byHash[h]
	if e == nil || s.expired(e, now) {
		return Info{}, false
	}
	return s.infoLocked(e), true
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
	if e, ok := s.byHash[h]; ok {
		delete(s.byID, e.id)
	}
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
