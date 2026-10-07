package runctl

import (
	"errors"
	"testing"
)

// Publish fallando no marca la corrida como publicada: se reintenta en cada tick y se entrega al sanar.
func TestPublishFailureIsRetriedUntilDelivered(t *testing.T) {
	st := &schedStore{MemStore: NewMemStore()}
	pub := &FakePublisher{Err: errors.New("outbox lleno")}
	k, err := New(Config{Gate: AllowAll(), Store: st, Publisher: pub, Warm: &FakeWarm{Fact: True}, Alerter: &FakeAlerter{}, Phases: &FakePhases{}})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.MemStore.Save(Run{ID: "r", State: Done, TraceID: "t", Evidence: []string{"s3://x"}})
	ticks(k, 5)
	if r, _ := st.Get("r"); r.DonePublish || len(pub.Events) != 0 {
		t.Fatalf("publicada con el outbox caído: %+v %v", r, pub.Events)
	}
	pub.mu.Lock()
	pub.Err = nil
	pub.mu.Unlock()
	ticks(k, 5)
	if r, _ := st.Get("r"); !r.DonePublish || len(pub.Events) != 1 {
		t.Fatalf("al sanar debía publicarse una vez: %+v %v", r, pub.Events)
	}
}
