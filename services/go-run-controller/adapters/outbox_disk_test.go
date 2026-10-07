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
