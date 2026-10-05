package inbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func ev(id string, at time.Time) NotifyCreated {
	e := NotifyCreated{EventID: "e-" + id, Type: "notify.created", Version: 1, OccurredAt: at, TraceID: "t"}
	e.Data.NotificationID = id
	e.Data.GithubEvent = "commit"
	e.Data.Repo = "acme/shop"
	e.Data.SHA = strings.Repeat("a", 40)
	e.Data.Artifact = &Artifact{Kind: "build-from-repo", Ref: "acme/shop@" + e.Data.SHA}
	return e
}

func open(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := OpenStore(dir, func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestListOrderAndFilter(t *testing.T) {
	s := open(t, t.TempDir())
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.Apply(ev("a", t0))
	s.Apply(ev("b", t0.Add(time.Hour)))
	s.Apply(ev("c", t0.Add(2*time.Hour)))
	if _, err := s.Confirm("b", "u1", []string{"f"}); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, n := range s.List("") {
		ids = append(ids, n.ID+":"+string(n.State))
	}
	if got := strings.Join(ids, ","); got != "c:pending,b:confirmed,a:pending" {
		t.Fatalf("orden/estados: %s", got)
	}
	if got := s.List(StateConfirmed); len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("filtro confirmed: %+v", got)
	}
	if got := s.List(StateRejected); got == nil || len(got) != 0 {
		t.Fatalf("filtro rejected debe ser lista vacia no nil: %#v", got)
	}
}

func TestApplyIdempotent(t *testing.T) {
	s := open(t, t.TempDir())
	e := ev("a", time.Now())
	if !s.Apply(e) || s.Apply(e) {
		t.Fatal("el mismo event_id debe aplicarse una sola vez")
	}
	e2 := ev("a", time.Now())
	e2.EventID = "otro"
	if s.Apply(e2) || len(s.List("")) != 1 {
		t.Fatal("el mismo notification_id no debe duplicar la entrada")
	}
}

func TestConfirmPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	s.Apply(ev("a", time.Now()))
	r, err := s.Confirm("a", "u1", []string{"checkout"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(r.RunID, "run-") || len(r.RunID) != 36 || r.ConfirmedBy != "u1" {
		t.Fatalf("recibo: %+v", r)
	}
	if _, err := s.Confirm("a", "u2", []string{"x"}); err != ErrAlreadyConfirmed {
		t.Fatalf("segunda confirmacion: %v", err)
	}
	if _, err := s.Confirm("zzz", "u1", []string{"x"}); err != ErrNotFound {
		t.Fatalf("inexistente: %v", err)
	}
	s2 := open(t, dir)
	s2.Apply(ev("a", time.Now()))
	if got := s2.List(StateConfirmed); len(got) != 1 {
		t.Fatalf("tras reinicio: %+v", got)
	}
	b, _ := os.ReadFile(filepath.Join(dir, ConfirmationsFile))
	if n := strings.Count(string(b), "\n"); n != 1 {
		t.Fatalf("lineas en disco: %d", n)
	}
}

func TestTruncatedReceiptLineDoesNotBlockStart(t *testing.T) {
	dir := t.TempDir()
	good := `{"run_id":"run-1","notification_id":"a","confirmed_by":"u1","confirmed_at":"2026-01-01T00:00:00Z","flows":["f"]}`
	if err := os.WriteFile(filepath.Join(dir, ConfirmationsFile), []byte(good+"\n"+`{"run_id":"run-2","notif`), 0o640); err != nil {
		t.Fatal(err)
	}
	s := open(t, dir)
	if s.Skipped != 1 {
		t.Fatalf("Skipped=%d", s.Skipped)
	}
	s.Apply(ev("a", time.Now()))
	s.Apply(ev("b", time.Now()))
	if got := s.List(StateConfirmed); len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("confirmadas: %+v", got)
	}
	if _, err := s.Confirm("b", "u1", []string{"f"}); err != nil {
		t.Fatal(err)
	}
	s3 := open(t, dir) // el nuevo recibo quedo en linea propia
	if len(s3.receipt) != 2 {
		t.Fatalf("recibos tras reabrir: %d", len(s3.receipt))
	}
}

func TestConcurrentConfirmExactlyOne(t *testing.T) {
	s := open(t, t.TempDir())
	s.Apply(ev("a", time.Now()))
	var wg sync.WaitGroup
	res := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.Confirm("a", fmt.Sprintf("u%d", i), []string{"f"})
			res <- err
		}(i)
	}
	wg.Wait()
	close(res)
	ok, conflict := 0, 0
	for err := range res {
		switch err {
		case nil:
			ok++
		case ErrAlreadyConfirmed:
			conflict++
		default:
			t.Fatal(err)
		}
	}
	if ok != 1 || conflict != 19 {
		t.Fatalf("ok=%d conflict=%d", ok, conflict)
	}
}
