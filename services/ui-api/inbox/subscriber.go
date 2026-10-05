package inbox

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

// EventSubscriber entrega eventos notify.created a la proyeccion.
type EventSubscriber interface {
	// Run bloquea hasta que ctx termina, llamando a handle por cada evento valido.
	Run(ctx context.Context, handle func(NotifyCreated)) error
}

// FileSubscriber lee el outbox JSONL de go-intake (UIAPI_EVENTS_FILE) desde el
// inicio. Es transicion hasta que C-45 decida el transporte de eventos.
// Solo consume lineas completas (terminadas en \n); la idempotencia por
// event_id la aplica el Store.
type FileSubscriber struct {
	Path   string
	Every  time.Duration
	Logger *log.Logger

	mu        sync.Mutex
	offset    int64
	discarded int
}

var _ EventSubscriber = (*FileSubscriber)(nil)

// Discarded devuelve cuantas lineas invalidas se descartaron.
func (f *FileSubscriber) Discarded() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.discarded
}

// Drain hace una pasada sobre lo nuevo del archivo y devuelve cuantos eventos
// valido entrego. Un archivo inexistente no es error (aun no hay eventos).
func (f *FileSubscriber) Drain(handle func(NotifyCreated)) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	file, err := os.Open(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer file.Close()
	if st, err := file.Stat(); err == nil && st.Size() < f.offset {
		f.offset = 0 // el archivo se trunco o rotó: releer; el Store deduplica
	}
	if _, err := file.Seek(f.offset, io.SeekStart); err != nil {
		return 0, err
	}
	n := 0
	r := bufio.NewReaderSize(file, 64<<10)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			break // EOF: una linea parcial se reintenta en la proxima pasada
		}
		f.offset += int64(len(line))
		if len(line) <= 1 {
			continue
		}
		ev, perr := ParseNotifyCreated(line)
		if perr != nil {
			f.discarded++
			if f.Logger != nil {
				f.Logger.Printf("evento descartado: %v", perr)
			}
			continue
		}
		handle(ev)
		n++
	}
	return n, nil
}

// Run implementa EventSubscriber con sondeo periodico.
func (f *FileSubscriber) Run(ctx context.Context, handle func(NotifyCreated)) error {
	every := f.Every
	if every <= 0 {
		every = 500 * time.Millisecond
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			if _, err := f.Drain(handle); err != nil && f.Logger != nil {
				f.Logger.Printf("leyendo eventos: %v", err)
			}
		}
	}
}
