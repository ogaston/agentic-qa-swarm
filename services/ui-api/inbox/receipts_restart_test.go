package inbox

import (
	"fmt"
	"testing"
	"time"
)

func restartNotif(t *testing.T, s *Store, id string) {
	t.Helper()
	ev := NotifyCreated{EventID: "ev-" + id, Type: "notify.created", Version: 1, OccurredAt: time.Now(), TraceID: "t"}
	ev.Data.NotificationID, ev.Data.GithubEvent, ev.Data.Repo, ev.Data.SHA = id, "commit", "acme/shop", "a"
	ev.Data.Artifact = &Artifact{Kind: "build-from-repo", Ref: "acme/shop@a"}
	s.Apply(ev)
}

// Tras reabrir el store sobre el mismo directorio, Receipts debe devolver los recibos del archivo,
// el más reciente primero (orden de llegada del archivo).
func TestReceiptsTrasReinicioConservanOrden(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("n-%d", i)
		restartNotif(t, s, id)
		if _, err := s.Confirm(id, "u1", []string{"f"}); err != nil {
			t.Fatal(err)
		}
	}
	all := func(Receipt) bool { return true }
	before := s.Receipts(20, all)

	s2, err := OpenStore(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	after := s2.Receipts(20, all)
	if len(after) != 3 {
		t.Fatalf("tras reinicio: %d recibos, quiero 3", len(after))
	}
	for i := range before {
		if after[i].NotificationID != before[i].NotificationID || after[i].RunID != before[i].RunID {
			t.Fatalf("orden tras reinicio %d: %s, quiero %s", i, after[i].NotificationID, before[i].NotificationID)
		}
	}
}

// Un registro con un recibo duplicado por notification_id conserva solo la primera aparición.
func TestReceiptsTrasReinicioIgnoraDuplicados(t *testing.T) {
	dir := t.TempDir()
	s, _ := OpenStore(dir, nil)
	restartNotif(t, s, "n-1")
	if _, err := s.Confirm("n-1", "u1", []string{"f"}); err != nil {
		t.Fatal(err)
	}
	line := fmt.Sprintf(`{"run_id":"run-x","notification_id":"n-1","confirmed_by":"u2","confirmed_at":"2026-01-01T00:00:00Z","flows":["f"]}` + "\n")
	if err := s.appendRaw([]byte(line)); err != nil {
		t.Fatal(err)
	}
	s2, _ := OpenStore(dir, nil)
	got := s2.Receipts(20, func(Receipt) bool { return true })
	if len(got) != 1 {
		t.Fatalf("duplicado: %d recibos, quiero 1", len(got))
	}
}
