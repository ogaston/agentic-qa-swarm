package inbox

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"sort"
)

// PublishedFile registra (solo-agregar) las confirmaciones cuyo run.confirmed ya se publico.
const PublishedFile = "published.jsonl"

// Confirmed es un recibo con lo necesario para publicar run.confirmed.
type Confirmed struct {
	Receipt
	Flows   []string
	TraceID string
}

func (s *Store) loadPublished() error {
	b, err := os.ReadFile(s.pubPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		var m struct {
			NotificationID string `json:"notification_id"`
		}
		if json.Unmarshal(sc.Bytes(), &m) == nil && m.NotificationID != "" {
			s.pubDone[m.NotificationID] = struct{}{}
		}
	}
	if len(b) > 0 && b[len(b)-1] != '\n' {
		return appendFile(s.pubPath, []byte("\n"))
	}
	return nil
}

// Confirmation devuelve el recibo de id con sus flujos y trace_id.
func (s *Store) Confirmation(id string) (Confirmed, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.receipt[id]
	if !ok {
		return Confirmed{}, false
	}
	return Confirmed{Receipt: r, Flows: append([]string(nil), s.flows[id]...), TraceID: s.traces[id]}, true
}

// PublishPending devuelve las confirmaciones sin run.confirmed publicado, las mas antiguas primero.
func (s *Store) PublishPending() []Confirmed {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Confirmed
	for id, r := range s.receipt {
		if _, done := s.pubDone[id]; !done {
			out = append(out, Confirmed{Receipt: r, Flows: append([]string(nil), s.flows[id]...), TraceID: s.traces[id]})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].ConfirmedAt.Equal(out[j].ConfirmedAt) {
			return out[i].ConfirmedAt.Before(out[j].ConfirmedAt)
		}
		return out[i].NotificationID < out[j].NotificationID
	})
	return out
}

// PublishPendingCount es el numero de confirmaciones con publicacion pendiente.
func (s *Store) PublishPendingCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.receipt) - len(s.pubDone)
}

// IsPublished indica si el run.confirmed de id ya se publico.
func (s *Store) IsPublished(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.pubDone[id]
	return ok
}

// MarkPublished persiste (fsync) que el run.confirmed de id se publico. Idempotente.
func (s *Store) MarkPublished(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.receipt[id]; !ok {
		return ErrNotFound
	}
	if _, ok := s.pubDone[id]; ok {
		return nil
	}
	line, err := json.Marshal(map[string]string{"notification_id": id})
	if err != nil {
		return err
	}
	if err := appendFile(s.pubPath, append(line, '\n')); err != nil {
		return err
	}
	s.pubDone[id] = struct{}{}
	return nil
}

func appendFile(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
