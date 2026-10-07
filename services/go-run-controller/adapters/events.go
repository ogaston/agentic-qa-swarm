package adapters

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // UUIDv5 (RFC 4122) exige SHA-1; no es un uso criptográfico.
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sync"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

// FileSource lee el JSONL de eventos (RUN_EVENTS_FILE; marcador de transición C-45). Solo entrega
// líneas completas; ignora tipos que no consume y descarta las inválidas.
type FileSource struct {
	Path string

	mu        sync.Mutex
	offset    int64
	discarded int
}

var _ runctl.EventSource = (*FileSource)(nil)

// Discarded cuenta las líneas inválidas descartadas.
func (f *FileSource) Discarded() int { f.mu.Lock(); defer f.mu.Unlock(); return f.discarded }

type rawEvent struct {
	EventID string `json:"event_id"`
	Type    string `json:"type"`
	Version int    `json:"version"`
	TraceID string `json:"trace_id"`
	Data    struct {
		RunID       string   `json:"run_id"`
		ConfirmedBy string   `json:"confirmed_by"`
		Flows       []string `json:"flows"`
	} `json:"data"`
}

// ParseEvent valida una línea; ok=false si el tipo no es de este servicio.
func ParseEvent(line []byte) (ev runctl.Event, ok bool, err error) {
	var r rawEvent
	if err = json.Unmarshal(line, &r); err != nil {
		return ev, false, err
	}
	switch r.Type {
	case runctl.EvRunConfirmed, runctl.EvRehearsalPassed, runctl.EvRehearsalFailed:
	default:
		return ev, false, nil
	}
	if r.EventID == "" || r.Version != 1 || r.TraceID == "" || r.Data.RunID == "" {
		return ev, true, errors.New("evento sin event_id, version 1, trace_id o run_id")
	}
	if r.Type == runctl.EvRunConfirmed && (r.Data.ConfirmedBy == "" || len(r.Data.Flows) == 0) {
		return ev, true, errors.New("run.confirmed sin confirmed_by o flows")
	}
	return runctl.Event{Type: r.Type, EventID: r.EventID, TraceID: r.TraceID, RunID: r.Data.RunID,
		ConfirmedBy: r.Data.ConfirmedBy, Flows: r.Data.Flows}, true, nil
}

// Poll implementa EventSource.
func (f *FileSource) Poll(context.Context) ([]runctl.Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	file, err := os.Open(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if st, err := file.Stat(); err == nil && st.Size() < f.offset {
		f.offset = 0
	}
	if _, err := file.Seek(f.offset, io.SeekStart); err != nil {
		return nil, err
	}
	var out []runctl.Event
	r := bufio.NewReader(file)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			break // línea parcial: se reintenta
		}
		f.offset += int64(len(line))
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		ev, ok, perr := ParseEvent(line)
		if perr != nil {
			f.discarded++
			continue
		}
		if ok {
			out = append(out, ev)
		}
	}
	return out, nil
}

var outNamespace = [16]byte{0x5c, 0x2e, 0x91, 0x0a, 0x37, 0x6b, 0x4d, 0x12, 0x9a, 0x4e, 0x80, 0x1f, 0x6d, 0x33, 0xb7, 0x08}

// OutEventID es el UUIDv5 determinista de (tipo, run_id).
func OutEventID(typ, runID string) string {
	h := sha1.New() //nolint:gosec
	h.Write(outNamespace[:])
	h.Write([]byte(typ + "\x00" + runID))
	b := h.Sum(nil)[:16]
	b[6] = (b[6] & 0x0f) | 0x50
	b[8] = (b[8] & 0x3f) | 0x80
	x := hex.EncodeToString(b)
	return x[0:8] + "-" + x[8:12] + "-" + x[12:16] + "-" + x[16:20] + "-" + x[20:]
}

// Outbox escribe eventos del controlador (run.done) en RUN_OUTBOX_FILE, idempotente por event_id.
type Outbox struct {
	Path string
	Now  func() time.Time
	mu   sync.Mutex
}

var _ runctl.EventPublisher = (*Outbox)(nil)

// Publish implementa EventPublisher.
func (o *Outbox) Publish(_ context.Context, ev runctl.OutEvent) error {
	if ev.Type != "run.done" {
		return errors.New("el outbox solo publica run.done")
	}
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	trace := ev.TraceID
	if trace == "" {
		trace = hex.EncodeToString([]byte(ev.RunID))
	}
	uris := ev.EvidenceURIs
	if uris == nil {
		uris = []string{}
	}
	id := OutEventID(ev.Type, ev.RunID)
	line, err := json.Marshal(map[string]any{"event_id": id, "type": ev.Type, "version": 1,
		"occurred_at": now().UTC().Format(time.RFC3339), "trace_id": trace,
		"data": map[string]any{"run_id": ev.RunID, "evidence_uris": uris}})
	if err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	b, err := os.ReadFile(o.Path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if bytes.Contains(b, []byte(`"event_id":"`+id+`"`)) {
		return nil
	}
	if len(b) > 0 && b[len(b)-1] != '\n' {
		line = append([]byte("\n"), line...)
	}
	f, err := os.OpenFile(o.Path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return f.Sync()
}
