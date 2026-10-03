// Package audit implementa el log de auditoría append-only con cadena de hashes.
//
// Formato: JSONL; cada línea es una Entry con prev_hash y
// hash = hex(sha256(prev_hash || canónico(entrada sin hash))). No existe ninguna
// función que borre o reescriba entradas. Un fallo de escritura envenena el log:
// todas las escrituras siguientes fallan (fail-closed) hasta reiniciar y verificar.
package audit

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// GenesisHash es el prev_hash de la primera entrada.
var GenesisHash = strings.Repeat("0", 64)

// Entry es una entrada del log.
type Entry struct {
	At       time.Time         `json:"at"`
	Actor    string            `json:"actor"`
	Action   string            `json:"action"`
	RunID    string            `json:"run_id,omitempty"`
	Detail   map[string]string `json:"detail,omitempty"`
	PrevHash string            `json:"prev_hash"`
	Hash     string            `json:"hash"`
}

// canonical serializa la entrada sin Hash (el orden de campos lo fija la struct
// y las claves de Detail las ordena encoding/json).
func (e Entry) canonical() ([]byte, error) {
	e.Hash = ""
	type noHash struct {
		At       string            `json:"at"`
		Actor    string            `json:"actor"`
		Action   string            `json:"action"`
		RunID    string            `json:"run_id,omitempty"`
		Detail   map[string]string `json:"detail,omitempty"`
		PrevHash string            `json:"prev_hash"`
	}
	return json.Marshal(noHash{e.At.UTC().Format(time.RFC3339Nano), e.Actor, e.Action, e.RunID, e.Detail, e.PrevHash})
}

// ComputeHash calcula el hash de la entrada a partir de su PrevHash.
func ComputeHash(e Entry) (string, error) {
	c, err := e.canonical()
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(e.PrevHash))
	h.Write(c)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ErrBroken indica una cadena rota o un log envenenado.
var ErrBroken = errors.New("cadena de auditoría rota")

// VerifyError señala la primera línea (1-based) donde la cadena falla.
type VerifyError struct {
	Line   int
	Reason string
}

func (e *VerifyError) Error() string { return fmt.Sprintf("línea %d: %s", e.Line, e.Reason) }
func (e *VerifyError) Unwrap() error { return ErrBroken }

const maxLine = 1 << 20

// Verify recorre la cadena y devuelve las entradas, o un *VerifyError.
func Verify(r io.Reader) ([]Entry, error) {
	br := bufio.NewReaderSize(r, 64<<10)
	var entries []Entry
	prev := GenesisHash
	for n := 1; ; n++ {
		line, err := br.ReadBytes('\n')
		if len(line) > maxLine {
			return nil, &VerifyError{n, "línea demasiado larga"}
		}
		if err != nil && err != io.EOF {
			return nil, err
		}
		if len(line) == 0 && err == io.EOF {
			return entries, nil
		}
		if err == io.EOF {
			return nil, &VerifyError{n, "línea truncada (sin salto de línea final)"}
		}
		var e Entry
		dec := json.NewDecoder(strings.NewReader(string(line)))
		dec.DisallowUnknownFields()
		if jerr := dec.Decode(&e); jerr != nil {
			return nil, &VerifyError{n, "JSON inválido: " + jerr.Error()}
		}
		if e.PrevHash != prev {
			return nil, &VerifyError{n, "prev_hash no encadena con la entrada anterior (entrada borrada, reordenada o alterada)"}
		}
		want, herr := ComputeHash(e)
		if herr != nil || want != e.Hash {
			return nil, &VerifyError{n, "hash no coincide (entrada alterada)"}
		}
		prev = e.Hash
		entries = append(entries, e)
	}
}

// VerifyFile abre y verifica un archivo.
func VerifyFile(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Verify(f)
}

// logFile es el archivo del log (os.File en producción; inyectable en pruebas).
type logFile interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
	Name() string
}

// Log es el log abierto en modo append. Seguro ante concurrencia.
type Log struct {
	mu      sync.Mutex
	f       logFile
	now     func() time.Time
	last    string
	entries []Entry
	allowed map[string]int // "día|workflow" -> gate.allow hacia running
	poison  error
}

// Open abre (o crea) el archivo, verifica la cadena existente y continúa sobre ella.
// Si la cadena está rota o la última línea truncada, devuelve error: no se arranca.
func Open(path string, now func() time.Time) (*Log, error) {
	if now == nil {
		now = time.Now
	}
	l := &Log{now: now, last: GenesisHash, allowed: map[string]int{}}
	if rf, err := os.Open(path); err == nil {
		entries, verr := Verify(rf)
		rf.Close()
		if verr != nil {
			return nil, fmt.Errorf("auditoría %s: %w", path, verr)
		}
		for _, e := range entries {
			l.index(e)
		}
		l.entries = entries
		if n := len(entries); n > 0 {
			l.last = entries[n-1].Hash
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	l.f = f
	return l, nil
}

func dayKey(t time.Time, wf string) string { return t.UTC().Format("2006-01-02") + "|" + wf }

func (l *Log) index(e Entry) {
	if e.Action == "gate.allow" && e.Detail["to"] == "running" && e.Detail["workflow"] != "" {
		l.allowed[dayKey(e.At, e.Detail["workflow"])]++
	}
}

// Append añade una entrada (At, PrevHash y Hash los fija el log) y hace fsync.
// Devuelve la entrada escrita. Si falla, el log queda envenenado.
func (l *Log) Append(e Entry) (Entry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.poison != nil {
		return Entry{}, l.poison
	}
	e.At = l.now().UTC()
	e.PrevHash = l.last
	h, err := ComputeHash(e)
	if err != nil {
		return Entry{}, err
	}
	e.Hash = h
	b, err := json.Marshal(e)
	if err != nil {
		return Entry{}, err
	}
	b = append(b, '\n')
	if _, err := l.f.Write(b); err != nil {
		l.poison = fmt.Errorf("%w: escritura fallida: %v", ErrBroken, err)
		return Entry{}, l.poison
	}
	if err := l.f.Sync(); err != nil {
		l.poison = fmt.Errorf("%w: fsync fallido: %v", ErrBroken, err)
		return Entry{}, l.poison
	}
	l.last = h
	l.entries = append(l.entries, e)
	l.index(e)
	return e, nil
}

// Query devuelve, en orden cronológico, las últimas limit entradas (de la corrida run si no es vacía).
func (l *Log) Query(run string, limit int) []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []Entry
	for i := len(l.entries) - 1; i >= 0 && len(out) < limit; i-- {
		if run == "" || l.entries[i].RunID == run {
			out = append(out, l.entries[i])
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// RunsAllowed cuenta las decisiones gate.allow hacia running del workflow en el día UTC de day.
func (l *Log) RunsAllowed(workflow string, day time.Time) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.allowed[dayKey(day, workflow)]
}

// Path devuelve la ruta del archivo del log.
func (l *Log) Path() string { return l.f.Name() }

// Close cierra el archivo.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f.Close()
}
