package adapters

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

// faultFile envuelve un archivo real e inyecta fallos: Write parcial (partial bytes y error),
// Sync que falla (la línea queda en disco) y Truncate que falla.
type faultFile struct {
	f         *os.File
	partial   int // >0: el próximo Write escribe solo estos bytes y devuelve error
	syncFails int // cuántos Sync siguientes fallan
	truncFail bool
}

var errDisk = errors.New("disco lleno (inyectado)")

func (f *faultFile) Write(p []byte) (int, error) {
	if f.partial > 0 {
		n := min(f.partial, len(p))
		f.partial = 0
		_, _ = f.f.Write(p[:n])
		return n, errDisk
	}
	return f.f.Write(p)
}
func (f *faultFile) Sync() error {
	if f.syncFails > 0 {
		f.syncFails--
		return errDisk
	}
	return f.f.Sync()
}
func (f *faultFile) Truncate(n int64) error {
	if f.truncFail {
		return errDisk
	}
	return f.f.Truncate(n)
}
func (f *faultFile) Close() error { return f.f.Close() }

func openFaulty(t testing.TB, dir string) (*JournalStore, *faultFile) {
	t.Helper()
	ff := &faultFile{}
	s, err := OpenJournalOpts(dir, JournalOptions{open: func(p string) (appendFile, error) {
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
		ff.f = f
		return ff, err
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, ff
}

func mustReopen(t testing.TB, dir string) *JournalStore {
	t.Helper()
	s, err := OpenJournal(dir)
	if err != nil {
		t.Fatalf("el diario no se reabre: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestJournalPartialWriteThenNextSaveRecovers(t *testing.T) {
	dir := t.TempDir()
	s, ff := openFaulty(t, dir)
	if err := s.Save(runctl.Run{ID: "r", State: runctl.Confirmed}); err != nil {
		t.Fatal(err)
	}
	ff.partial = 17
	if err := s.Save(runctl.Run{ID: "r", State: runctl.WarmReady}); err == nil {
		t.Fatal("el Save con escritura parcial debía fallar")
	}
	if r, _ := s.Get("r"); r.State != runctl.Confirmed {
		t.Fatalf("la memoria avanzó con un Save fallido: %+v", r)
	}
	if err := s.Healthy(); err != nil {
		t.Fatalf("restaurado, el diario debe estar sano: %v", err)
	}
	if err := s.Save(runctl.Run{ID: "r", State: runctl.Deploying}); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	r, _ := mustReopen(t, dir).Get("r")
	if r.State != runctl.Deploying {
		t.Fatalf("estado reabierto %s, el último Save exitoso era deploying", r.State)
	}
}

// Tras reabrir un diario con líneas, un Write parcial restaura al tamaño real del archivo (no a 0).
func TestJournalPartialWriteAfterReopenKeepsPriorLines(t *testing.T) {
	dir := t.TempDir()
	s0 := mustReopen(t, dir)
	_ = s0.Save(runctl.Run{ID: "r", State: runctl.Confirmed})
	_ = s0.Close()
	before, _ := os.ReadFile(filepath.Join(dir, JournalFile))
	s, ff := openFaulty(t, dir)
	ff.partial = 11
	if err := s.Save(runctl.Run{ID: "r", State: runctl.WarmReady}); err == nil {
		t.Fatal("debía fallar")
	}
	if after, _ := os.ReadFile(filepath.Join(dir, JournalFile)); !bytes.Equal(after, before) {
		t.Fatalf("el fallo alteró las líneas previas: %q", after)
	}
}

func TestJournalPartialWriteTruncateFailsMarksBroken(t *testing.T) {
	dir := t.TempDir()
	s, ff := openFaulty(t, dir)
	_ = s.Save(runctl.Run{ID: "r", State: runctl.Confirmed})
	ff.partial, ff.truncFail = 9, true
	if err := s.Save(runctl.Run{ID: "r", State: runctl.WarmReady}); err == nil {
		t.Fatal("debía fallar")
	}
	ff.truncFail = false
	if err := s.Save(runctl.Run{ID: "r", State: runctl.Deploying}); !errors.Is(err, ErrJournalBroken) {
		t.Fatalf("un diario que no pudo restaurarse debe rechazar todo Save: %v", err)
	}
	if err := s.Healthy(); !errors.Is(err, ErrJournalBroken) {
		t.Fatalf("Healthy debe reportar el diario roto: %v", err)
	}
	_ = s.Close()
	// la cola huérfana no tiene \n: el siguiente arranque la descarta y conserva lo anterior.
	r, _ := mustReopen(t, dir).Get("r")
	if r.State != runctl.Confirmed {
		t.Fatalf("%+v", r)
	}
}

func TestJournalSyncFailureKeepsSeqAligned(t *testing.T) {
	dir := t.TempDir()
	s, ff := openFaulty(t, dir)
	_ = s.Save(runctl.Run{ID: "r", State: runctl.Confirmed})
	ff.syncFails = 1 // la línea llega a disco pero el Sync falla
	if err := s.Save(runctl.Run{ID: "r", State: runctl.WarmReady}); err == nil {
		t.Fatal("debía fallar")
	}
	if err := s.Save(runctl.Run{ID: "r", State: runctl.Deploying}); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	b, _ := os.ReadFile(filepath.Join(dir, JournalFile))
	if n := bytes.Count(b, []byte("\n")); n != 2 || bytes.Contains(b, []byte(`"warm_ready"`)) {
		t.Fatalf("el archivo debe tener 2 líneas y no la versión fallida:\n%s", b)
	}
	r, _ := mustReopen(t, dir).Get("r")
	if r.State != runctl.Deploying {
		t.Fatalf("%+v", r)
	}
}

func TestJournalSyncFailureTwiceMarksBroken(t *testing.T) {
	dir := t.TempDir()
	s, ff := openFaulty(t, dir)
	_ = s.Save(runctl.Run{ID: "r", State: runctl.Confirmed})
	ff.syncFails = 2 // también falla el Sync posterior al truncado: no se puede garantizar
	if err := s.Save(runctl.Run{ID: "r", State: runctl.WarmReady}); err == nil {
		t.Fatal("debía fallar")
	}
	if err := s.Save(runctl.Run{ID: "r", State: runctl.Deploying}); !errors.Is(err, ErrJournalBroken) {
		t.Fatalf("%v", err)
	}
}

func TestJournalTornTailDiscardedWithLogAndMetric(t *testing.T) {
	dir := t.TempDir()
	s, _ := openFaulty(t, dir)
	_ = s.Save(runctl.Run{ID: "r", State: runctl.Confirmed})
	_ = s.Save(runctl.Run{ID: "r", State: runctl.WarmReady})
	_ = s.Close()
	p := filepath.Join(dir, JournalFile)
	good, _ := os.ReadFile(p)
	tail := `{"seq":3,"prev":"ab`
	_ = os.WriteFile(p, append(append([]byte(nil), good...), tail...), 0o600) // kill -9 a mitad de write
	var logs bytes.Buffer
	s2, err := OpenJournalOpts(dir, JournalOptions{Log: slog.New(slog.NewTextHandler(&logs, nil))})
	if err != nil {
		t.Fatalf("una cola sin \\n no debe impedir el arranque: %v", err)
	}
	defer s2.Close()
	if s2.TailDiscarded() != len(tail) || !strings.Contains(logs.String(), "bytes_descartados=19") {
		t.Fatalf("descartados=%d log=%q", s2.TailDiscarded(), logs.String())
	}
	if after, _ := os.ReadFile(p); !bytes.Equal(after, good) {
		t.Fatalf("la cola debe truncarse del archivo: %q", after)
	}
	if err := s2.Save(runctl.Run{ID: "r", State: runctl.Deploying}); err != nil {
		t.Fatal(err)
	}
	_ = s2.Close()
	if r, _ := mustReopen(t, dir).Get("r"); r.State != runctl.Deploying {
		t.Fatal(r)
	}
}

func TestJournalTornTailOnlyFileOpensEmpty(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, JournalFile), []byte(`{"seq":1`), 0o600)
	s := mustReopen(t, dir)
	if len(s.List()) != 0 || s.TailDiscarded() != 8 {
		t.Fatal(s.List(), s.TailDiscarded())
	}
}

func TestJournalTornTailWithNewlineStillRefuses(t *testing.T) {
	for name, content := range map[string]string{
		"ilegible con \\n": "{\"seq\":1\n",
		"no encadena":      `{"seq":2,"prev":"","run":{"id":"r","state":"confirmed"}}` + "\n",
	} {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, JournalFile), []byte(content), 0o600)
		if _, err := OpenJournal(dir); err == nil {
			t.Errorf("%s: una línea completa que no encadena es corrupción real y debe impedir el arranque", name)
		}
	}
}

// Propiedad: con fallos aleatorios de Write/Sync/Truncate, reabrir nunca falla y el estado
// reabierto es el del último Save exitoso (o el diario quedó roto y no aceptó más).
func TestJournalPartialWriteSyncFailureProperty(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		dir := t.TempDir()
		ff := &faultFile{}
		s, err := OpenJournalOpts(dir, JournalOptions{open: func(p string) (appendFile, error) {
			f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
			ff.f = f
			return ff, err
		}})
		if err != nil {
			rt.Fatal(err)
		}
		want := map[string]runctl.State{}
		var maybeID string
		var maybe runctl.State
		states := []runctl.State{runctl.Confirmed, runctl.WarmReady, runctl.Deploying, runctl.Inferring}
		for i := range rapid.IntRange(1, 25).Draw(rt, "n") {
			id := rapid.SampledFrom([]string{"a", "b"}).Draw(rt, "id")
			st := rapid.SampledFrom(states).Draw(rt, "st")
			ff.partial = rapid.SampledFrom([]int{0, 0, 0, 1, 7, 60}).Draw(rt, "partial")
			ff.syncFails = rapid.SampledFrom([]int{0, 0, 0, 1, 2}).Draw(rt, "sync")
			ff.truncFail = rapid.SampledFrom([]bool{false, false, false, true}).Draw(rt, "trunc")
			if err := s.Save(runctl.Run{ID: id, State: st}); err == nil {
				want[id] = st
			} else if s.broken != nil { // roto: la línea en vuelo pudo llegar a disco; no se aceptan más Save
				maybeID, maybe = id, st
				if err := s.Save(runctl.Run{ID: "z", State: st}); !errors.Is(err, ErrJournalBroken) {
					rt.Fatalf("un diario roto aceptó un Save: %v", err)
				}
				break
			}
			_ = i
		}
		_ = s.Close()
		s2, err := OpenJournal(dir)
		if err != nil {
			rt.Fatalf("reabrir falló: %v", err)
		}
		defer s2.Close()
		for _, r := range s2.List() {
			if r.State != want[r.ID] && !(r.ID == maybeID && r.State == maybe) {
				rt.Fatalf("%s: reabierto %s, último Save exitoso %q", r.ID, r.State, want[r.ID])
			}
		}
		for id, st := range want {
			if r, _ := s2.Get(id); r.State != st && !(id == maybeID && r.State == maybe) {
				rt.Fatalf("%s: %s != %s", id, r.State, st)
			}
		}
	})
}
