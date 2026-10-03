package policy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Version es una versión guardada de una política.
type Version struct {
	Name    string          `json:"name"`
	Version int             `json:"version"`
	Value   json.RawMessage `json:"value"`
	At      time.Time       `json:"at"`
	Actor   string          `json:"actor"`
}

// Store es el almacén de políticas versionadas (solo agregar).
type Store interface {
	// Get devuelve la última versión; found=false si nunca se escribió.
	Get(ctx context.Context, name string) (v Version, found bool, err error)
	// Plan dice qué haría Put con value: la versión resultante y si crearía una nueva.
	Plan(ctx context.Context, name string, value json.RawMessage) (version int, isNew bool, err error)
	// Put guarda una versión nueva, salvo que sea idéntica a la última.
	Put(ctx context.Context, name string, value json.RawMessage, actor string) (version int, isNew bool, err error)
}

// FileStore guarda una línea JSON por versión en un archivo abierto con O_APPEND.
type FileStore struct {
	mu     sync.Mutex
	f      *os.File
	now    func() time.Time
	latest map[string]Version
	poison error
}

// OpenFileStore abre (o crea) el almacén. Una línea corrupta impide abrirlo.
func OpenFileStore(path string, now func() time.Time) (*FileStore, error) {
	if now == nil {
		now = time.Now
	}
	s := &FileStore{now: now, latest: map[string]Version{}}
	if rf, err := os.Open(path); err == nil {
		defer rf.Close()
		br := bufio.NewReaderSize(rf, 64<<10)
		for n := 1; ; n++ {
			line, err := br.ReadBytes('\n')
			if err == io.EOF && len(line) == 0 {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("políticas %s línea %d: %w", path, n, errors.New("línea truncada o ilegible"))
			}
			var v Version
			if jerr := json.Unmarshal(bytes.TrimSpace(line), &v); jerr != nil || !Known(v.Name) || v.Version < 1 {
				return nil, fmt.Errorf("políticas %s línea %d: entrada inválida", path, n)
			}
			if prev, ok := s.latest[v.Name]; ok && v.Version != prev.Version+1 || !ok && v.Version != 1 {
				return nil, fmt.Errorf("políticas %s línea %d: versión no consecutiva", path, n)
			}
			s.latest[v.Name] = v
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	s.f = f
	return s, nil
}

// Get implementa Store.
func (s *FileStore) Get(ctx context.Context, name string) (Version, bool, error) {
	if err := ctx.Err(); err != nil {
		return Version{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.latest[name]
	return v, ok, nil
}

func (s *FileStore) plan(name string, value json.RawMessage) (int, bool) {
	if v, ok := s.latest[name]; ok {
		if bytes.Equal(v.Value, value) {
			return v.Version, false
		}
		return v.Version + 1, true
	}
	return 1, true
}

// Plan implementa Store.
func (s *FileStore) Plan(ctx context.Context, name string, value json.RawMessage) (int, bool, error) {
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, isNew := s.plan(name, value)
	return v, isNew, nil
}

// Put implementa Store. El valor ya debe estar normalizado (Validate).
func (s *FileStore) Put(ctx context.Context, name string, value json.RawMessage, actor string) (int, bool, error) {
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.poison != nil {
		return 0, false, s.poison
	}
	ver, isNew := s.plan(name, value)
	if !isNew {
		return ver, false, nil
	}
	v := Version{Name: name, Version: ver, Value: value, At: s.now().UTC(), Actor: actor}
	b, err := json.Marshal(v)
	if err != nil {
		return 0, false, err
	}
	if _, err := s.f.Write(append(b, '\n')); err != nil {
		s.poison = fmt.Errorf("almacén de políticas envenenado: %w", err)
		return 0, false, s.poison
	}
	if err := s.f.Sync(); err != nil {
		s.poison = fmt.Errorf("almacén de políticas envenenado: %w", err)
		return 0, false, s.poison
	}
	s.latest[name] = v
	return ver, true, nil
}

// Close cierra el archivo.
func (s *FileStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.f.Close()
}
