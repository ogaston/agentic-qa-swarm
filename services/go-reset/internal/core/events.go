package core

import (
	"crypto/sha1" //nolint:gosec // UUIDv5 lo exige (RFC 4122), no es un uso de seguridad
	"fmt"
	"time"
)

var eventNS = [16]byte{0x6b, 0xa7, 0xb8, 0x10, 0x9d, 0xad, 0x11, 0xd1, 0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8}

// EventID es un UUIDv5 determinista de la clave.
func EventID(key string) string {
	h := sha1.New() //nolint:gosec
	h.Write(eventNS[:])
	h.Write([]byte(key))
	s := h.Sum(nil)[:16]
	s[6] = (s[6] & 0x0f) | 0x50
	s[8] = (s[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", s[0:4], s[4:6], s[6:8], s[8:10], s[10:16])
}

// ResetVerifiedEvent construye reset.verified (checks siempre ok:true).
func ResetVerifiedEvent(runID, warmID, trace string, checks []string, at time.Time) Event {
	cs := make([]map[string]any, 0, len(checks))
	for _, c := range checks {
		cs = append(cs, map[string]any{"name": c, "ok": true})
	}
	return Event{
		EventID: EventID("reset.verified|" + runID + "|" + at.UTC().Format(time.RFC3339Nano)),
		Type:    "reset.verified", Version: 1, OccurredAt: at.UTC(), TraceID: trace,
		Data: map[string]any{"run_id": runID, "warm_id": warmID, "checks": cs},
	}
}

// TeardownVerifiedEvent construye teardown.verified.
func TeardownVerifiedEvent(warmID, mode, trace string, at time.Time) Event {
	return Event{
		EventID: EventID("teardown.verified|" + warmID + "|" + mode + "|" + at.UTC().Format(time.RFC3339Nano)),
		Type:    "teardown.verified", Version: 1, OccurredAt: at.UTC(), TraceID: trace,
		Data: map[string]any{"warm_id": warmID, "mode": mode, "verified": true},
	}
}
