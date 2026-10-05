package intake

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// JSONLStore persiste Records en <dir>/notifications.jsonl: solo se agrega,
// con fsync por escritura; gana la ultima linea de cada ID. Una linea rota
// (p. ej. truncada por un corte) se ignora y se registra. Los archivos se crean
// en la primera escritura.
type JSONLStore struct {
	mu       sync.Mutex
	path     string
	byID     map[string]Record
	byDeliv  map[string]string
	needsEOL bool
}

var _ NotificationStore = (*JSONLStore)(nil)

// OpenJSONLStore crea dir si falta y carga lo ya escrito.
func OpenJSONLStore(dir string) (*JSONLStore, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	s := &JSONLStore{
		path:    filepath.Join(dir, "notifications.jsonl"),
		byID:    map[string]Record{},
		byDeliv: map[string]string{},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *JSONLStore) load() error {
	f, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	n := 0
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			n++
			s.needsEOL = line[len(line)-1] != '\n'
			var rec Record
			if jerr := json.Unmarshal(bytes.TrimSpace(line), &rec); jerr != nil || rec.ID == "" || rec.DeliveryID == "" {
				log.Printf("store: linea %d ilegible, se ignora", n)
			} else {
				s.apply(rec)
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (s *JSONLStore) apply(rec Record) {
	s.byID[rec.ID] = rec
	s.byDeliv[rec.DeliveryID] = rec.ID
}

// GetByDelivery implementa NotificationStore.
func (s *JSONLStore) GetByDelivery(deliveryID string) (Record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byDeliv[deliveryID]
	if !ok {
		return Record{}, false
	}
	return s.byID[id], true
}

// Put implementa NotificationStore: agrega una linea y hace fsync antes de
// actualizar el indice en memoria.
func (s *JSONLStore) Put(rec Record) error {
	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	if s.needsEOL { // la ultima linea quedo truncada: no pegar la nueva a ella
		line = append([]byte{'\n'}, line...)
	}
	if _, err := f.Write(line); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	s.needsEOL = false
	s.apply(rec)
	return nil
}
