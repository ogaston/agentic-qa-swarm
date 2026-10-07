// Package sessions es el SessionStore JSONL (última línea por run_id gana).
package sessions

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/core"
)

type Store struct {
	mu   sync.Mutex
	path string
}

// New usa RESET_DATA_DIR/sessions.jsonl.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{path: filepath.Join(dir, "sessions.jsonl")}, nil
}

func (s *Store) Save(_ context.Context, se core.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.Marshal(se)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func (s *Store) List(context.Context) ([]core.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	last := map[string]core.Session{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	for sc.Scan() {
		var se core.Session
		if json.Unmarshal(sc.Bytes(), &se) == nil && se.RunID != "" {
			last[se.RunID] = se
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	out := make([]core.Session, 0, len(last))
	for _, v := range last {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RunID < out[j].RunID })
	return out, nil
}
