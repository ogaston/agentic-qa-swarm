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

// MaxLineBytes es el tope de tamano de una linea del outbox, contando el '\n':
// 1.048.575 bytes de contenido se aceptan; 1.048.576 de contenido se descartan.
const MaxLineBytes = 1 << 20

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
		line, consumed, tooBig, err := readLine(r)
		if err != nil {
			break // EOF: una linea parcial se reintenta en la proxima pasada
		}
		f.offset += int64(consumed)
		if tooBig {
			f.discarded++
			if f.Logger != nil {
				f.Logger.Printf("linea del outbox descartada: supera %d bytes", MaxLineBytes)
			}
			continue
		}
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

// readLine lee hasta '\n' y devuelve la linea y los bytes consumidos. Si la
// linea supera MaxLineBytes, la consume entera sin acumularla y devuelve
// tooBig. Una linea sin '\n' final devuelve error (se reintenta al completarse).
func readLine(r *bufio.Reader) (line []byte, consumed int, tooBig bool, err error) {
	for {
		chunk, e := r.ReadSlice('\n')
		consumed += len(chunk)
		if !tooBig {
			if consumed > MaxLineBytes {
				tooBig, line = true, nil
			} else {
				line = append(line, chunk...)
			}
		}
		switch {
		case e == nil:
			return line, consumed, tooBig, nil
		case errors.Is(e, bufio.ErrBufferFull):
			continue
		default:
			return nil, 0, false, e
		}
	}
}
