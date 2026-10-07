package adapters

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
// anterior; se reproduce al abrir y una cadena rota impide arrancar.
type JournalStore struct {
	mu   sync.Mutex
	f    *os.File
	seq  int
	last string
	runs map[string]runctl.Run
}

var _ runctl.RunStore = (*JournalStore)(nil)

func lineHash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// OpenJournal abre (o crea) dir/runs.jsonl y reproduce el diario verificando la cadena.
func OpenJournal(dir string) (*JournalStore, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, JournalFile)
	s := &JournalStore{runs: map[string]runctl.Run{}}
	if b, err := os.ReadFile(path); err == nil {
		if len(b) > 0 && b[len(b)-1] != '\n' {
			return nil, errors.New("diario: última línea incompleta")
		}
		for i, ln := range bytes.Split(bytes.TrimSuffix(b, []byte("\n")), []byte("\n")) {
			if len(b) == 0 {
				break
			}
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
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	s.f = f
	return s, nil
}

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
	return s.f.Sync()
}

func cloneRun(r runctl.Run) runctl.Run {
	b, _ := json.Marshal(r)
	var c runctl.Run
	_ = json.Unmarshal(b, &c)
	return c
}

// Save añade una línea (fsync) y solo entonces actualiza la memoria.
func (s *JournalStore) Save(r runctl.Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	line, err := json.Marshal(journalLine{Seq: s.seq + 1, Prev: s.last, Run: r})
	if err != nil {
		return err
	}
	if _, err := s.f.Write(append(append([]byte(nil), line...), '\n')); err != nil {
		return err
	}
	if err := s.f.Sync(); err != nil {
		return err
	}
	s.seq, s.last = s.seq+1, lineHash(line)
	s.runs[r.ID] = cloneRun(r)
	return nil
}
