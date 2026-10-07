package warmmanager

import (
	"crypto/sha1"
	"fmt"
	"time"
)

// Event es el sobre común de los eventos (contracts/events/*).
type Event struct {
	EventID    string         `json:"event_id"`
	Type       string         `json:"type"`
	Version    int            `json:"version"`
	OccurredAt string         `json:"occurred_at"`
	TraceID    string         `json:"trace_id"`
	Data       map[string]any `json:"data"`
}

var eventNS = [16]byte{0x7a, 0x1c, 0x52, 0x0e, 0x31, 0x9b, 0x4c, 0x6d, 0x8e, 0x44, 0x1f, 0x03, 0xa5, 0x77, 0x2b, 0x90}

// EventID es un UUIDv5 determinista de la clave dada (reintentos no duplican identidad).
func EventID(key string) string {
	h := sha1.New()
	h.Write(eventNS[:])
	h.Write([]byte(key))
	s := h.Sum(nil)
	var u [16]byte
	copy(u[:], s[:16])
	u[6] = u[6]&0x0f | 0x50
	u[8] = u[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}

func newEvent(typ, key, trace string, now time.Time, data map[string]any) Event {
	return Event{EventID: EventID(typ + "/" + key), Type: typ, Version: 1,
		OccurredAt: now.UTC().Format(time.RFC3339), TraceID: trace, Data: data}
}
