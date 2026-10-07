package adapters

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

var doneEv = runctl.OutEvent{Type: "run.done", RunID: "r-1", TraceID: "t", EvidenceURIs: []string{"s3://x"}}

func faultyOutbox(p string, ff *faultFile) *Outbox {
	return &Outbox{Path: p, open: func(path string) (appendFile, error) {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
		ff.f = f
		return ff, err
	}}
}

// Una línea parcial (kill -9 a mitad de write) NO cuenta como publicada: se vuelve a escribir completa.
func TestOutboxPartialLineIsNotPublished(t *testing.T) {
	p := filepath.Join(t.TempDir(), "o.jsonl")
	id := OutEventID("run.done", "r-1")
	_ = os.WriteFile(p, []byte(`{"event_id":"`+id+`","type":"run.d`), 0o600)
	if err := (&Outbox{Path: p}).Publish(t.Context(), doneEv); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !published(b, id) || !strings.HasSuffix(string(b), "\n") {
		t.Fatalf("falta la línea completa: %q", b)
	}
}

func TestOutboxWriteFailureLeavesNoOrphanAndRetryPublishes(t *testing.T) {
	p := filepath.Join(t.TempDir(), "o.jsonl")
	ff := &faultFile{partial: 70} // el id del evento ya cabe en los primeros bytes
	o := faultyOutbox(p, ff)
	if err := o.Publish(t.Context(), doneEv); err == nil {
		t.Fatal("debía fallar")
	}
	if b, _ := os.ReadFile(p); len(b) != 0 {
		t.Fatalf("bytes huérfanos tras el fallo: %q", b)
	}
	if err := o.Publish(t.Context(), doneEv); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if bytes.Count(b, []byte("\n")) != 1 || !published(b, OutEventID("run.done", "r-1")) {
		t.Fatalf("%q", b)
	}
}

// Sync fallido: la línea se retira; el reintento escribe una sola.
func TestOutboxSyncFailureThenRetryWritesOneLine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "o.jsonl")
	ff := &faultFile{syncFails: 1}
	o := faultyOutbox(p, ff)
	if err := o.Publish(t.Context(), doneEv); err == nil {
		t.Fatal("debía fallar")
	}
	if err := o.Publish(t.Context(), doneEv); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); bytes.Count(b, []byte("\n")) != 1 {
		t.Fatalf("%q", b)
	}
}

// Un Publish fallido no borra los run.done anteriores (restaura a su tamaño previo, no a 0).
func TestOutboxFailureKeepsPriorLines(t *testing.T) {
	p := filepath.Join(t.TempDir(), "o.jsonl")
	if err := (&Outbox{Path: p}).Publish(t.Context(), runctl.OutEvent{Type: "run.done", RunID: "r-0", TraceID: "t"}); err != nil {
		t.Fatal(err)
	}
	prior, _ := os.ReadFile(p)
	o := faultyOutbox(p, &faultFile{partial: 40})
	if err := o.Publish(t.Context(), doneEv); err == nil {
		t.Fatal("debía fallar")
	}
	if b, _ := os.ReadFile(p); !bytes.Equal(b, prior) {
		t.Fatalf("el fallo alteró las líneas anteriores: %q", b)
	}
}

// El objeto completo sin '\n' final (kill -9 justo antes del salto) solo recibe el '\n': una línea.
func TestOutboxCompleteJSONWithoutNewlineIsNotDuplicated(t *testing.T) {
	p := filepath.Join(t.TempDir(), "o.jsonl")
	o := &Outbox{Path: p}
	if err := o.Publish(t.Context(), doneEv); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	_ = os.WriteFile(p, bytes.TrimSuffix(b, []byte("\n")), 0o600)
	if err := o.Publish(t.Context(), doneEv); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if !bytes.Equal(got, b) {
		t.Fatalf("esperaba la misma línea con su salto:\n%q\n%q", got, b)
	}
}

// Una cola completa de OTRO evento no es este evento: se aísla y se escribe el nuestro.
func TestOutboxForeignCompleteTailIsIsolated(t *testing.T) {
	p := filepath.Join(t.TempDir(), "o.jsonl")
	_ = os.WriteFile(p, []byte(`{"event_id":"otro"}`), 0o600)
	if err := (&Outbox{Path: p}).Publish(t.Context(), doneEv); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if bytes.Count(b, []byte("\n")) != 2 || !published(b, OutEventID("run.done", "r-1")) {
		t.Fatalf("%q", b)
	}
}

// Doble fallo (Sync y truncado fallan): la línea queda completa en disco sin ser durable; el siguiente
// Publish no la da por publicada hasta sincronizar con éxito, y después ya no vuelve a sincronizar.
func TestOutboxDoubleFailureIsNotReportedPublishedUntilSynced(t *testing.T) {
	p := filepath.Join(t.TempDir(), "o.jsonl")
	ff := &faultFile{syncFails: 1, truncFail: true}
	o := faultyOutbox(p, ff)
	if err := o.Publish(t.Context(), doneEv); err == nil {
		t.Fatal("debía fallar")
	}
	if b, _ := os.ReadFile(p); !published(b, OutEventID("run.done", "r-1")) {
		t.Fatalf("el escenario exige la línea completa en disco: %q", b)
	}
	ff.truncFail, ff.syncFails = false, 1
	if err := o.Publish(t.Context(), doneEv); err == nil {
		t.Fatal("con el Sync aún fallando no puede darse por publicada")
	}
	if err := o.Publish(t.Context(), doneEv); err != nil {
		t.Fatal(err)
	}
	ff.syncFails = 5 // ya sincronizada: un Publish repetido no vuelve a tocar el disco
	if err := o.Publish(t.Context(), doneEv); err != nil {
		t.Fatalf("reintento innecesario de Sync: %v", err)
	}
	if b, _ := os.ReadFile(p); bytes.Count(b, []byte("\n")) != 1 {
		t.Fatalf("%q", b)
	}
}

// Tras una publicación exitosa un repetido es un no-op: no toca el disco (no sincroniza).
func TestOutboxRepeatAfterSuccessDoesNotTouchDisk(t *testing.T) {
	ff := &faultFile{}
	o := faultyOutbox(filepath.Join(t.TempDir(), "o.jsonl"), ff)
	if err := o.Publish(t.Context(), doneEv); err != nil {
		t.Fatal(err)
	}
	ff.syncFails = 3
	if err := o.Publish(t.Context(), doneEv); err != nil {
		t.Fatalf("un repetido no debe sincronizar: %v", err)
	}
}
