package policy

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFileStoreHealthy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policies.jsonl")
	s, err := OpenFileStore(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Healthy(); err != nil {
		t.Fatalf("almacén vacío: %v", err)
	}
	if _, _, err := s.Put(context.Background(), Events, json.RawMessage(`{"enabled_events":["tag"]}`), "a1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Healthy(); err != nil {
		t.Fatalf("tras un Put: %v", err)
	}
}

func TestFileStoreUnhealthyWhenFileCorruptedMissingOrAltered(t *testing.T) {
	setup := func(t *testing.T) (*FileStore, string) {
		path := filepath.Join(t.TempDir(), "policies.jsonl")
		s, err := OpenFileStore(path, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		if _, _, err := s.Put(context.Background(), Events, json.RawMessage(`{"enabled_events":["tag"]}`), "a1"); err != nil {
			t.Fatal(err)
		}
		return s, path
	}
	for name, mutate := range map[string]func(path string) error{
		"corrupto": func(p string) error { return os.WriteFile(p, []byte("basura{\n"), 0o600) },
		"truncado": func(p string) error { return os.WriteFile(p, []byte(`{"name":"events"`), 0o600) },
		"borrado":  func(p string) error { return os.Remove(p) },
		"vaciado":  func(p string) error { return os.WriteFile(p, nil, 0o600) },
		"alterado": func(p string) error {
			return os.WriteFile(p, []byte(`{"name":"events","version":1,"value":{"enabled_events":[]},"at":"2026-01-01T00:00:00Z","actor":"x"}`+"\n"), 0o600)
		},
		"sin lectura": func(p string) error { return os.Chmod(p, 0) },
	} {
		t.Run(name, func(t *testing.T) {
			s, path := setup(t)
			if err := mutate(path); err != nil {
				t.Fatal(err)
			}
			if name == "sin lectura" && os.Geteuid() == 0 {
				t.Skip("root lee archivos sin permisos")
			}
			if err := s.Healthy(); err == nil {
				t.Fatal("Healthy debía fallar")
			}
		})
	}
}

func TestFileStoreUnhealthyAfterWriteFailurePoisons(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policies.jsonl")
	s, err := OpenFileStore(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Close() // las escrituras siguientes fallan
	if _, _, err := s.Put(context.Background(), Events, json.RawMessage(`{"enabled_events":["tag"]}`), "a1"); err == nil {
		t.Fatal("el Put sobre un archivo cerrado debía fallar")
	}
	if err := s.Healthy(); err == nil {
		t.Fatal("tras un fallo de escritura el almacén está envenenado")
	}
}
