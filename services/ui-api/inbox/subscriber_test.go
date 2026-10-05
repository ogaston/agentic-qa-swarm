package inbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileSubscriberDiscardsInvalidAndTailsIncrementally(t *testing.T) {
	good := readFile(t, filepath.Join(events, "examples/valid/notify.created.json"))
	bad := readFile(t, filepath.Join(events, "examples/invalid/notify.created.missing-sha.json"))
	oneLine := func(b []byte) string {
		s, err := compact(b)
		if err != nil {
			t.Fatal(err)
		}
		return s + "\n"
	}
	p := filepath.Join(t.TempDir(), "events.jsonl")
	fs := &FileSubscriber{Path: p}
	var got []NotifyCreated
	h := func(e NotifyCreated) { got = append(got, e) }

	if n, err := fs.Drain(h); err != nil || n != 0 {
		t.Fatalf("archivo inexistente: n=%d err=%v", n, err)
	}
	partial := oneLine(good)
	if err := os.WriteFile(p, []byte(oneLine(bad)+partial[:20]), 0o640); err != nil {
		t.Fatal(err)
	}
	if n, _ := fs.Drain(h); n != 0 || fs.Discarded() != 1 {
		t.Fatalf("n=%d discarded=%d", n, fs.Discarded())
	}
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	_, _ = f.WriteString(partial[20:])
	f.Close()
	if n, _ := fs.Drain(h); n != 1 || len(got) != 1 || got[0].Data.NotificationID != "n-1" {
		t.Fatalf("tras completar la linea: n=%d got=%+v", n, got)
	}
	if n, _ := fs.Drain(h); n != 0 {
		t.Fatalf("no debe reentregar: %d", n)
	}
	// Reinicio del suscriptor: reentrega, pero el Store deduplica por event_id.
	st := open(t, t.TempDir())
	fs2 := &FileSubscriber{Path: p}
	for i := 0; i < 2; i++ {
		fs2.offset = 0
		_, _ = fs2.Drain(func(e NotifyCreated) { st.Apply(e) })
	}
	if len(st.List("")) != 1 {
		t.Fatal("la proyeccion debe ser idempotente")
	}
}
