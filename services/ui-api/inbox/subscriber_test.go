package inbox

import (
	"os"
	"path/filepath"
	"strings"
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

func TestFileSubscriberResetsOffsetOnTruncate(t *testing.T) {
	good := readFile(t, filepath.Join(events, "examples/valid/notify.created.json"))
	line, _ := compact(good)
	p := filepath.Join(t.TempDir(), "events.jsonl")
	fs := &FileSubscriber{Path: p}
	count := 0
	h := func(NotifyCreated) { count++ }
	if err := os.WriteFile(p, []byte(line+"\n"+line+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	fs.Drain(h)
	// El archivo se rota/trunca a algo mas corto: debe releerse desde el inicio.
	if err := os.WriteFile(p, []byte(line+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	fs.Drain(h)
	if count != 3 {
		t.Fatalf("entregas = %d, esperado 3 (2 + 1 tras truncar)", count)
	}
}

func TestFileSubscriberSkipsOversizedLine(t *testing.T) {
	good := readFile(t, filepath.Join(events, "examples/valid/notify.created.json"))
	line, _ := compact(good)
	p := filepath.Join(t.TempDir(), "events.jsonl")
	// Evento valido segun el esquema, pero con trace_id gigante: solo el tope lo rechaza.
	huge := strings.Replace(line, "4bf92f3577b34da6a3ce929d0e0e4736", strings.Repeat("x", MaxLineBytes+10), 1)
	if _, err := ParseNotifyCreated([]byte(huge)); err != nil {
		t.Fatalf("la linea grande debe ser valida salvo por el tamano: %v", err)
	}
	if err := os.WriteFile(p, []byte(huge+"\n"+line+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	fs := &FileSubscriber{Path: p}
	n, err := fs.Drain(func(NotifyCreated) {})
	if err != nil || n != 1 || fs.Discarded() != 1 {
		t.Fatalf("n=%d err=%v discarded=%d", n, err, fs.Discarded())
	}
}
