// Package guard implementa el anti-brute-force: fallos consecutivos por usuario y por IP,
// con bloqueo de 30 s que se duplica en cada reincidencia hasta 15 min.
package guard

import (
	"sync"
	"time"
)

const (
	// BaseLock es el primer bloqueo.
	BaseLock = 30 * time.Second
	// MaxLock es el tope del bloqueo creciente.
	MaxLock = 15 * time.Minute
	// MaxEntries acota cada tabla (usuarios e IPs).
	MaxEntries = 10000
)

type entry struct {
	fails int
	level int // bloqueos ya impuestos
	until time.Time
	last  time.Time
}

type table struct {
	m map[string]*entry
}

// Guard cuenta intentos. Allow reserva el intento de forma atómica (cuenta el fallo por
// adelantado), de modo que intentos concurrentes no superan el límite sin bloquear.
type Guard struct {
	mu    sync.Mutex
	max   int
	clock func() time.Time
	users table
	ips   table
}

// Ticket identifica un intento admitido, para liberarlo o darlo por acertado.
type Ticket struct {
	user, ip         string
	userLock, ipLock bool
}

// New crea un Guard. max <= 0 toma 5; clock nil usa time.Now.
func New(max int, clock func() time.Time) *Guard {
	if max <= 0 {
		max = 5
	}
	if clock == nil {
		clock = time.Now
	}
	return &Guard{max: max, clock: clock, users: table{m: map[string]*entry{}}, ips: table{m: map[string]*entry{}}}
}

func lockFor(level int) time.Duration {
	d := BaseLock
	for i := 0; i < level && d < MaxLock; i++ {
		d *= 2
	}
	if d > MaxLock {
		d = MaxLock
	}
	return d
}

// Allow admite o rechaza un intento. Si lo rechaza devuelve el tiempo restante de bloqueo.
func (g *Guard) Allow(user, ip string) (Ticket, time.Duration, bool) {
	now := g.clock()
	g.mu.Lock()
	defer g.mu.Unlock()
	var retry time.Duration
	for _, p := range []struct {
		t   *table
		key string
	}{{&g.users, user}, {&g.ips, ip}} {
		if e, ok := p.t.m[p.key]; ok && now.Before(e.until) {
			if r := e.until.Sub(now); r > retry {
				retry = r
			}
		}
	}
	if retry > 0 {
		return Ticket{}, retry, false
	}
	tk := Ticket{user: user, ip: ip}
	tk.userLock = g.count(&g.users, user, now)
	tk.ipLock = g.count(&g.ips, ip, now)
	return tk, 0, true
}

func (g *Guard) count(t *table, key string, now time.Time) bool {
	e, ok := t.m[key]
	if !ok {
		if len(t.m) >= MaxEntries {
			evict(t, now)
		}
		e = &entry{}
		t.m[key] = e
	}
	e.fails++
	e.last = now
	if e.fails >= g.max {
		e.until = now.Add(lockFor(e.level))
		e.level++
		return true
	}
	return false
}

// evict descarta la entrada no bloqueada más antigua (o la más antigua si todas están bloqueadas).
func evict(t *table, now time.Time) {
	var oldKey, oldAny string
	var oldT, oldAnyT time.Time
	haveFree, haveAny := false, false
	for k, e := range t.m {
		if !haveAny || e.last.Before(oldAnyT) {
			oldAny, oldAnyT, haveAny = k, e.last, true
		}
		if !now.Before(e.until) && (!haveFree || e.last.Before(oldT)) {
			oldKey, oldT, haveFree = k, e.last, true
		}
	}
	if haveFree {
		delete(t.m, oldKey)
	} else if haveAny {
		delete(t.m, oldAny)
	}
}

// Success reinicia los contadores del usuario y de la IP.
func (g *Guard) Success(tk Ticket) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.users.m, tk.user)
	delete(g.ips.m, tk.ip)
}

// Release devuelve el intento sin contarlo como fallo (contraseña correcta pendiente de OTP).
func (g *Guard) Release(tk Ticket) {
	g.mu.Lock()
	defer g.mu.Unlock()
	release(g.users.m[tk.user], tk.userLock)
	release(g.ips.m[tk.ip], tk.ipLock)
}

func release(e *entry, locked bool) {
	if e == nil {
		return
	}
	if e.fails > 0 {
		e.fails--
	}
	if locked {
		e.until = time.Time{}
		if e.level > 0 {
			e.level--
		}
	}
}

// Len devuelve el tamaño de las tablas de usuarios e IPs (para pruebas).
func (g *Guard) Len() (users, ips int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.users.m), len(g.ips.m)
}
