package adapters

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

// JournalFile es el nombre del diario dentro de RUN_DATA_DIR.
const JournalFile = "runs.jsonl"

type journalLine struct {
	Seq  int        `json:"seq"`
	Prev string     `json:"prev"` // sha256 hex de la línea anterior ("" en la primera)
	Run  runctl.Run `json:"run"`
}

// JournalStore es un RunStore con diario JSONL append-only: cada línea lleva seq y el hash de la
// anterior; se reproduce al abrir y una cadena rota impide arrancar. Un Save que falla deja el
// archivo como estaba (o el diario queda roto y rechaza todo Save posterior): nunca bytes huérfanos.
type JournalStore struct {
	mu        sync.Mutex
	f         appendFile
	size      int64 // bytes válidos en disco (todas las líneas completas)
	seq       int
	last      string
	runs      map[string]runctl.Run
	broken    error
	discarded int
}

var _ runctl.RunStore = (*JournalStore)(nil)

// ErrJournalBroken indica que un Save fallido no pudo devolver el archivo a su estado previo.
var ErrJournalBroken = errors.New("diario roto: no se pudo restaurar tras un fallo de escritura")

// JournalOptions son los parámetros opcionales de OpenJournalOpts.
type JournalOptions struct {
	Log  *slog.Logger // registra la cola descartada; nil = silencio
	open openAppendFunc
}

func lineHash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// OpenJournal abre (o crea) dir/runs.jsonl y reproduce el diario verificando la cadena.
func OpenJournal(dir string) (*JournalStore, error) { return OpenJournalOpts(dir, JournalOptions{}) }

// OpenJournalOpts es OpenJournal con opciones. Una cola sin '\n' final (escritura interrumpida que
// nunca tuvo un Save exitoso detrás) se descarta y se registra; una línea con '\n' que no encadena
// sigue siendo error de arranque.
func OpenJournalOpts(dir string, o JournalOptions) (*JournalStore, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	open := o.open
	if open == nil {
		open = osOpenAppend
	}
	path := filepath.Join(dir, JournalFile)
	s := &JournalStore{runs: map[string]runctl.Run{}}
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	valid := b[:bytes.LastIndexByte(b, '\n')+1]
	s.discarded = len(b) - len(valid)
	if len(valid) > 0 {
		for i, ln := range bytes.Split(bytes.TrimSuffix(valid, []byte("\n")), []byte("\n")) {
			var jl journalLine
			dec := json.NewDecoder(bytes.NewReader(ln))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&jl); err != nil {
				return nil, fmt.Errorf("diario: línea %d ilegible: %w", i+1, err)
			}
			if jl.Seq != i+1 || jl.Prev != s.last || jl.Run.ID == "" || !jl.Run.State.Valid() {
				return nil, fmt.Errorf("diario: la cadena se rompe en la línea %d", i+1)
			}
			s.seq, s.last = jl.Seq, lineHash(ln)
			s.runs[jl.Run.ID] = jl.Run
		}
	}
	if s.discarded > 0 {
		if err := os.Truncate(path, int64(len(valid))); err != nil {
			return nil, fmt.Errorf("diario: no se pudo descartar la cola incompleta: %w", err)
		}
		if o.Log != nil {
			o.Log.Warn("diario: cola sin salto de línea descartada", "bytes_descartados", s.discarded, "bytes_validos", len(valid))
		}
	}
	f, err := open(path)
	if err != nil {
		return nil, err
	}
	s.f, s.size = f, int64(len(valid))
	return s, nil
}

// TailDiscarded devuelve los bytes de cola incompleta descartados al abrir (0 si no hubo).
func (s *JournalStore) TailDiscarded() int { return s.discarded }

// Close cierra el archivo.
func (s *JournalStore) Close() error { return s.f.Close() }

// Get implementa RunStore.
func (s *JournalStore) Get(id string) (runctl.Run, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	return cloneRun(r), ok
}

// List implementa RunStore.
func (s *JournalStore) List() []runctl.Run {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]runctl.Run, 0, len(s.runs))
	for _, r := range s.runs {
		out = append(out, cloneRun(r))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Healthy comprueba que el diario sigue abierto y escribible.
func (s *JournalStore) Healthy() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.broken != nil {
		return s.broken
	}
	return s.f.Sync()
}

func cloneRun(r runctl.Run) runctl.Run {
	b, _ := json.Marshal(r)
	var c runctl.Run
	_ = json.Unmarshal(b, &c)
	return c
}

// Save añade una línea (fsync) y solo entonces actualiza la memoria. Ante un fallo de Write o Sync
// trunca el archivo al tamaño previo y no avanza seq/last; si no puede, el diario queda roto.
func (s *JournalStore) Save(r runctl.Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.broken != nil {
		return s.broken
	}
	line, err := json.Marshal(journalLine{Seq: s.seq + 1, Prev: s.last, Run: r})
	if err != nil {
		return err
	}
	data := append(append([]byte(nil), line...), '\n')
	if err, intact := appendDurable(s.f, s.size, data); err != nil {
		if !intact {
			s.broken = fmt.Errorf("%w: %v", ErrJournalBroken, err)
		}
		return err
	}
	s.size += int64(len(data))
	s.seq, s.last = s.seq+1, lineHash(line)
	s.runs[r.ID] = cloneRun(r)
	return nil
}
