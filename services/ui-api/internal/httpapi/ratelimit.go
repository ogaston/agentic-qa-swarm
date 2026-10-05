package httpapi

import (
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// limiter es un token bucket por IP, en memoria, con reloj inyectable.
type limiter struct {
	mu      sync.Mutex
	rps     float64
	burst   float64
	now     func() time.Time
	buckets map[string]*bucket
	lastGC  time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newLimiter(rps float64, burst int, now func() time.Time) *limiter {
	return &limiter{rps: rps, burst: float64(burst), now: now, buckets: map[string]*bucket{}}
}

// allow consume un token de ip; si no hay, devuelve los segundos a esperar.
func (l *limiter) allow(ip string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.gc(now)
	b, ok := l.buckets[ip]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[ip] = b
	}
	if el := now.Sub(b.last).Seconds(); el > 0 {
		b.tokens = math.Min(l.burst, b.tokens+el*l.rps)
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := int(math.Ceil((1 - b.tokens) / l.rps))
	if wait < 1 {
		wait = 1
	}
	return false, wait
}

// gc descarta buckets ya llenos e inactivos para acotar la memoria.
func (l *limiter) gc(now time.Time) {
	if now.Sub(l.lastGC) < time.Minute {
		return
	}
	l.lastGC = now
	for ip, b := range l.buckets {
		if b.tokens+now.Sub(b.last).Seconds()*l.rps >= l.burst {
			delete(l.buckets, ip)
		}
	}
}

// clientIP usa RemoteAddr; X-Forwarded-For (ultimo salto) solo con trustProxy.
func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if ip := strings.TrimSpace(parts[len(parts)-1]); ip != "" {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
