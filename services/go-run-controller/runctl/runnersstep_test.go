package runctl

import (
	"context"
	"errors"
	"testing"
)

type outRunners struct {
	out   RunOutcome
	calls int
}

func (o *outRunners) Progress(context.Context, Run) (RunOutcome, error) { o.calls++; return o.out, nil }

func runningRun() Run {
	return Run{ID: "r-1", State: Running, EnsayoPassed: True, ConfirmedBy: "u", TraceID: "t", Flows: []string{"a", "b"}}
}

func newRunnersCtl(t *testing.T, run Run, rn RunnerResults, pub EventPublisher) (*Controller, *MemStore) {
	t.Helper()
	st := NewMemStore()
	_ = st.Save(run)
	k, err := New(Config{Gate: AllowAll(), Store: st, Publisher: pub, Warm: &FakeWarm{Fact: True}, Alerter: &FakeAlerter{},
		Phases: &FakePhases{}, Runners: rn})
	if err != nil {
		t.Fatal(err)
	}
	return k, st
}

// Con 2 flujos y uno sin evidencia hay URIs pero también FailReason: nunca run.done.
func TestRunDonePartialEvidence(t *testing.T) {
	pub := &FakePublisher{}
	rn := &outRunners{out: RunOutcome{Done: true, URIs: []string{"s3://e/runs/r-1/a/logs.txt", "s3://e/runs/r-1/a/result.json"}, FailReason: "flujo b: evidencia no escrita"}}
	k, st := newRunnersCtl(t, runningRun(), rn, pub)
	for i := 0; i < 10; i++ {
		k.DriveAll(context.Background())
	}
	if len(pub.Events) != 0 {
		t.Fatalf("run.done con evidencia parcial: %+v", pub.Events)
	}
	if r, _ := st.Get("r-1"); r.State == Running || r.FailReason == "" || r.DonePublish {
		t.Fatalf("%+v", r)
	}
}

// Un error al publicar run.done no se traga: no se marca publicado y se reintenta.
func TestRunDonePublishErrorRetried(t *testing.T) {
	pub := &FakePublisher{Err: errors.New("bus caído")}
	rn := &outRunners{out: RunOutcome{Done: true, URIs: []string{"s3://e/runs/r-1/a/result.json"}}}
	k, st := newRunnersCtl(t, runningRun(), rn, pub)
	for i := 0; i < 3; i++ {
		k.DriveAll(context.Background())
	}
	if r, _ := st.Get("r-1"); r.DonePublish || r.State != Running || len(pub.Events) != 0 {
		t.Fatalf("avanzó con el bus caído: %+v", r)
	}
	pub.Err = nil
	for i := 0; i < 10; i++ {
		k.DriveAll(context.Background())
	}
	if len(pub.Events) != 1 || pub.Events[0].Type != "run.done" {
		t.Fatalf("no se reintentó: %+v", pub.Events)
	}
}

// Tras un reinicio entre la publicación y la salida de running, run.done no se publica otra vez.
func TestRunDoneOncePerRun(t *testing.T) {
	run := runningRun()
	run.DonePublish = true
	run.Evidence = []string{"s3://e/runs/r-1/a/result.json"}
	pub := &FakePublisher{}
	rn := &outRunners{out: RunOutcome{Done: true, URIs: run.Evidence}}
	k, st := newRunnersCtl(t, run, rn, pub)
	for i := 0; i < 10; i++ {
		k.DriveAll(context.Background())
	}
	if len(pub.Events) != 0 || rn.calls != 0 {
		t.Fatalf("run.done repetido: eventos=%d, consultas a runners=%d", len(pub.Events), rn.calls)
	}
	if r, _ := st.Get("r-1"); r.State == Running {
		t.Fatalf("no salió de running: %+v", r)
	}
}
