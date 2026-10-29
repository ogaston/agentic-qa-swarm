package repos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Store es la interfaz del almacén de repositorios registrados.
type Store interface {
	List(ctx context.Context) ([]Repository, error)
	Get(ctx context.Context, repo string) (Repository, bool, error)
	Create(ctx context.Context, r Repository) error
	Update(ctx context.Context, r Repository) error
	Delete(ctx context.Context, repo string) (bool, error)
	Close() error
}

// FileStore implementa Store con persistencia atómica en archivo JSON.
type FileStore struct {
	mu      sync.RWMutex
	path    string
	entries map[string]Repository
}

var _ Store = (*FileStore)(nil)

// NewMemoryStore crea un almacén en memoria para pruebas.
func NewMemoryStore() *FileStore {
	return &FileStore{
		entries: make(map[string]Repository),
	}
}

// OpenFileStore abre o crea el almacén en el archivo especificado.
func OpenFileStore(path string) (*FileStore, error) {
	s := &FileStore{
		path:    path,
		entries: make(map[string]Repository),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *FileStore) load() error {
	if s.path == "" {
		return nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("leyendo almacén de repositorios %s: %w", s.path, err)
	}
	if len(data) == 0 {
		return nil
	}
	var list []Repository
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("parseando almacén de repositorios %s: %w", s.path, err)
	}
	for _, r := range list {
		s.entries[strings.ToLower(r.Repo)] = r
	}
	return nil
}

func (s *FileStore) persistLocked() error {
	if s.path == "" {
		return nil
	}
	list := make([]Repository, 0, len(s.entries))
	for _, r := range s.entries {
		list = append(list, r)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Repo < list[j].Repo
	})
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("serializando repositorios: %w", err)
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creando directorio %s: %w", dir, err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("escribiendo temporal %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("renombrando %s a %s: %w", tmp, s.path, err)
	}
	return nil
}

// List devuelve todos los repositorios registrados ordenados por nombre.
func (s *FileStore) List(ctx context.Context) ([]Repository, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Repository, 0, len(s.entries))
	for _, r := range s.entries {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Repo < out[j].Repo
	})
	return out, nil
}

// Get devuelve el repositorio por su nombre repo (insensible a mayúsculas).
func (s *FileStore) Get(ctx context.Context, repo string) (Repository, bool, error) {
	if err := ctx.Err(); err != nil {
		return Repository{}, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.entries[strings.ToLower(strings.TrimSpace(repo))]
	return r, ok, nil
}

// Create registra un nuevo repositorio. Devuelve ErrAlreadyExists si ya existe.
func (s *FileStore) Create(ctx context.Context, r Repository) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key := strings.ToLower(strings.TrimSpace(r.Repo))
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.entries[key]; exists {
		return ErrAlreadyExists
	}
	s.entries[key] = r
	if err := s.persistLocked(); err != nil {
		delete(s.entries, key)
		return err
	}
	return nil
}

// Update actualiza un repositorio existente. Devuelve ErrNotFound si no existe.
func (s *FileStore) Update(ctx context.Context, r Repository) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key := strings.ToLower(strings.TrimSpace(r.Repo))
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.entries[key]; !exists {
		return ErrNotFound
	}
	prev := s.entries[key]
	s.entries[key] = r
	if err := s.persistLocked(); err != nil {
		s.entries[key] = prev
		return err
	}
	return nil
}

// Delete elimina un repositorio. Devuelve (true, nil) si existía y fue borrado.
func (s *FileStore) Delete(ctx context.Context, repo string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	key := strings.ToLower(strings.TrimSpace(repo))
	s.mu.Lock()
	defer s.mu.Unlock()

	prev, exists := s.entries[key]
	if !exists {
		return false, nil
	}
	delete(s.entries, key)
	if err := s.persistLocked(); err != nil {
		s.entries[key] = prev
		return false, err
	}
	return true, nil
}

// Close implementa Store.
func (s *FileStore) Close() error {
	return nil
}
