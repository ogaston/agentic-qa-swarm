package inbox

import (
	"testing"
	"time"
)

func TestCountsByState(t *testing.T) {
	s := open(t, t.TempDir())
	want := func(p, c, r int) {
		t.Helper()
		got := s.Counts()
		if got["pending"] != p || got["confirmed"] != c || got["rejected"] != r || len(got) != 3 {
			t.Fatalf("Counts = %v, esperado pending=%d confirmed=%d rejected=%d", got, p, c, r)
		}
	}
	want(0, 0, 0) // las tres claves existen aunque no haya nada
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.Apply(ev("a", t0))
	s.Apply(ev("b", t0.Add(time.Hour)))
	want(2, 0, 0)
	if _, err := s.Confirm("a", "u1", []string{"f"}); err != nil {
		t.Fatal(err)
	}
	want(1, 1, 0)
}
