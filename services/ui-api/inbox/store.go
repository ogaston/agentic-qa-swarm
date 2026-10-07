package inbox

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Errores de Confirm; la capa HTTP los traduce a 404 y 409.
var (
	ErrNotFound         = errors.New("notificacion inexistente")
	ErrAlreadyConfirmed = errors.New("notificacion ya confirmada")
)

// ConfirmationsFile es el registro solo-agregar dentro de UIAPI_DATA_DIR.
const ConfirmationsFile = "confirmations.jsonl"

// record es la linea persistida de una confirmacion.
type record struct {
	Receipt
	Flows   []string `json:"flows"`
	TraceID string   `json:"trace_id,omitempty"`
}

type entry struct {
	n   Notification
	at  time.Time
	seq int
}

// Store es la proyeccion del inbox. El estado confirmed sale del registro
// de confirmaciones, no de memoria volatil.
type Store struct {
	mu      sync.Mutex
	now     func() time.Time
	path    string
	items   map[string]*entry
	events  map[string]struct{}
	receipt map[string]Receipt
	flows   map[string][]string // flujos confirmados por notification_id
	traces  map[string]string   // trace_id de la peticion que confirmo
	pubDone map[string]struct{} // notification_id con run.confirmed ya publicado
	pubPath string
	seq     int
	// Skipped cuenta lineas de recibos ilegibles ignoradas al abrir.
	Skipped int
}

// OpenStore abre (o crea) el registro en dir y reconstruye las confirmaciones.
// Una linea truncada o corrupta se ignora y no impide arrancar. now puede ser
// nil (reloj real).
func OpenStore(dir string, now func() time.Time) (*Store, error) {
	if now == nil {
		now = time.Now
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	s := &Store{
		now: now, path: filepath.Join(dir, ConfirmationsFile),
		items: map[string]*entry{}, events: map[string]struct{}{}, receipt: map[string]Receipt{},
		flows: map[string][]string{}, traces: map[string]string{}, pubDone: map[string]struct{}{},
		pubPath: filepath.Join(dir, PublishedFile),
	}
	b, err := os.ReadFile(s.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for sc.Scan() {
		var r record
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.NotificationID == "" || r.RunID == "" || r.ConfirmedBy == "" {
			s.Skipped++
			continue
		}
		if _, dup := s.receipt[r.NotificationID]; !dup {
			s.receipt[r.NotificationID] = r.Receipt
			s.flows[r.NotificationID], s.traces[r.NotificationID] = r.Flows, r.TraceID
		}
	}
	if len(b) > 0 && b[len(b)-1] != '\n' {
		// Linea final truncada: el siguiente registro debe empezar en linea propia.
		if err := s.appendRaw([]byte("\n")); err != nil {
			return nil, err
		}
	}
	if err := s.loadPublished(); err != nil {
		return nil, err
	}
	return s, nil
}

// Apply incorpora un notify.created ya validado. Es idempotente por event_id
// y por notification_id (la primera gana). Devuelve true si creo una entrada.
func (s *Store) Apply(ev NotifyCreated) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, seen := s.events[ev.EventID]; seen {
		return false
	}
	s.events[ev.EventID] = struct{}{}
	id := ev.Data.NotificationID
	if _, ok := s.items[id]; ok {
		return false
	}
	a := *ev.Data.Artifact
	s.seq++
	s.items[id] = &entry{
		n: Notification{
			ID: id, GithubEvent: ev.Data.GithubEvent, Repo: ev.Data.Repo, SHA: ev.Data.SHA,
			Artifact: &a, State: StatePending,
		},
		at: ev.OccurredAt, seq: s.seq,
	}
	return true
}

func (s *Store) view(e *entry) Notification {
	n := e.n
	if _, ok := s.receipt[n.ID]; ok {
		n.State = StateConfirmed
	}
	return n
}

// List devuelve las notificaciones, la mas reciente primero. state vacio = todas.
func (s *Store) List(state State) []Notification {
	s.mu.Lock()
	defer s.mu.Unlock()
	es := make([]*entry, 0, len(s.items))
	for _, e := range s.items {
		es = append(es, e)
	}
	sort.Slice(es, func(i, j int) bool {
		if !es[i].at.Equal(es[j].at) {
			return es[i].at.After(es[j].at)
		}
		return es[i].seq > es[j].seq
	})
	out := make([]Notification, 0, len(es))
	for _, e := range es {
		if n := s.view(e); state == "" || n.State == state {
			out = append(out, n)
		}
	}
	return out
}

// Counts devuelve cuantas notificaciones hay por estado (las tres claves siempre presentes).
func (s *Store) Counts() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]int{string(StatePending): 0, string(StateConfirmed): 0, string(StateRejected): 0}
	for _, e := range s.items {
		out[string(s.view(e).State)]++
	}
	return out
}

// Confirm registra la confirmacion de id por principalID: persiste (fsync) y
// solo despues cambia el estado. La segunda confirmacion devuelve
// ErrAlreadyConfirmed sin crear otro recibo.
func (s *Store) Confirm(id, principalID string, flows []string) (Receipt, error) {
	return s.ConfirmTraced(id, principalID, flows, "")
}

// ConfirmTraced es Confirm y ademas persiste el trace_id de la peticion, para que run.confirmed
// republicado conserve la correlacion.
func (s *Store) ConfirmTraced(id, principalID string, flows []string, traceID string) (Receipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return Receipt{}, ErrNotFound
	}
	if _, done := s.receipt[id]; done {
		return Receipt{}, ErrAlreadyConfirmed
	}
	runID, err := newRunID()
	if err != nil {
		return Receipt{}, err
	}
	r := Receipt{RunID: runID, NotificationID: id, ConfirmedBy: principalID, ConfirmedAt: s.now().UTC().Truncate(time.Second)}
	line, err := json.Marshal(record{Receipt: r, Flows: flows, TraceID: traceID})
	if err != nil {
		return Receipt{}, err
	}
	if err := s.appendRaw(append(line, '\n')); err != nil {
		return Receipt{}, fmt.Errorf("persistiendo confirmacion: %w", err)
	}
	s.receipt[id] = r
	s.flows[id], s.traces[id] = append([]string(nil), flows...), traceID
	return r, nil
}

func (s *Store) appendRaw(b []byte) error {
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
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

// newRunID genera "run-" + 32 hex (128 bits aleatorios).
func newRunID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "run-" + hex.EncodeToString(b[:]), nil
}
