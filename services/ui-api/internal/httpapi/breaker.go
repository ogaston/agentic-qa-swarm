package httpapi

// Cortacircuitos de un upstream (mismo criterio que el del warm): abre tras threshold fallos de
// transporte o plazo; cualquier respuesta HTTP cuenta como «alcanzable». Tras openFor admite una sonda.

import (
	"sync"
	"time"
)

type breaker struct {
	threshold int
	openFor   time.Duration
	now       func() time.Time

	mu        sync.Mutex
	fails     int
	openUntil time.Time
	probing   bool
}

func newBreaker(threshold int, openFor time.Duration, now func() time.Time) *breaker {
	return &breaker{threshold: threshold, openFor: openFor, now: now}
}

// admit dice si se puede llamar. probe=true significa que esta llamada es la sonda tras el plazo.
func (b *breaker) admit() (probe, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.openUntil.IsZero() {
		return false, true
	}
	if b.now().Before(b.openUntil) || b.probing {
		return false, false
	}
	b.probing = true
	return true, true
}

// settle registra el resultado de una llamada admitida.
func (b *breaker) settle(probe, reachable bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if probe {
		b.probing = false
	}
	if reachable {
		b.fails, b.openUntil = 0, time.Time{}
		return
	}
	b.fails++
	if probe || b.fails >= b.threshold {
		b.openUntil = b.now().Add(b.openFor)
	}
}

// abort libera la sonda sin contar ni como éxito ni como fallo (p. ej. petición mal formada).
func (b *breaker) abort(probe bool) {
	if probe {
		b.mu.Lock()
		b.probing = false
		b.mu.Unlock()
	}
}
