package runner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
)

// ErrNotFound: el objeto no existe en el almacén.
var ErrNotFound = errors.New("objeto inexistente")

// Evidence es el puerto hacia el almacén durable (MinIO). Las claves son relativas al bucket.
type Evidence interface {
	Put(ctx context.Context, key string, data []byte) (uri string, err error)
	Get(ctx context.Context, key string) ([]byte, error) // ErrNotFound si no existe
	URI(key string) string
}

// ErrHashMismatch: lo leído de vuelta no coincide con lo escrito.
var ErrHashMismatch = errors.New("la evidencia leída de vuelta no coincide por hash")

// PutVerified escribe, lee de vuelta y compara por hash (SHA-256). Solo devuelve la URI si coincide:
// una escritura fallida nunca declara evidencia que no existe.
func PutVerified(ctx context.Context, ev Evidence, key string, data []byte) (string, error) {
	uri, err := ev.Put(ctx, key, data)
	if err != nil {
		return "", fmt.Errorf("escribiendo evidencia: %w", err)
	}
	back, err := ev.Get(ctx, key)
	if err != nil {
		return "", fmt.Errorf("leyendo de vuelta la evidencia: %w", err)
	}
	if sha256.Sum256(back) != sha256.Sum256(data) {
		return "", ErrHashMismatch
	}
	return uri, nil
}

// MemEvidence es un Evidence en memoria para pruebas.
type MemEvidence struct {
	mu      sync.Mutex
	Objs    map[string][]byte
	FailPut func(key string) error  // opcional: fuerza el fallo de una escritura
	Corrupt func(key string) []byte // opcional: devuelve otro contenido al leer
	GetErr  func(key string) error  // opcional: fuerza un error de lectura distinto de ErrNotFound
	Bucket  string
}

// Put implementa Evidence.
func (m *MemEvidence) Put(_ context.Context, key string, data []byte) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.FailPut != nil {
		if err := m.FailPut(key); err != nil {
			return "", err
		}
	}
	if m.Objs == nil {
		m.Objs = map[string][]byte{}
	}
	m.Objs[key] = bytes.Clone(data)
	return m.URI(key), nil
}

// Get implementa Evidence.
func (m *MemEvidence) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.GetErr != nil {
		if err := m.GetErr(key); err != nil {
			return nil, err
		}
	}
	if m.Corrupt != nil {
		if b := m.Corrupt(key); b != nil {
			return b, nil
		}
	}
	b, ok := m.Objs[key]
	if !ok {
		return nil, ErrNotFound
	}
	return bytes.Clone(b), nil
}

// URI implementa Evidence.
func (m *MemEvidence) URI(key string) string {
	b := m.Bucket
	if b == "" {
		b = "evidence"
	}
	return "s3://" + b + "/" + key
}

func sha256Hex(b []byte) string { h := sha256.Sum256(b); return fmt.Sprintf("%x", h[:]) }
