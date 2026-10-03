package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

func writeN(t *testing.T, l *Log, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := l.Append(Entry{Actor: "a", Action: "gate.deny", RunID: fmt.Sprintf("r%d", i),
			Detail: map[string]string{"k": "v", "z": "1"}}); err != nil {
			t.Fatal(err)
		}
	}
}

func newLog(t *testing.T, n int) (string, *Log) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "audit.jsonl")
	l, err := Open(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	writeN(t, l, n)
	return p, l
}

func TestChainIntactAfterConcurrentWrites(t *testing.T) {
	p, l := newLog(t, 0)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := l.Append(Entry{Actor: "a", Action: "gate.allow", RunID: fmt.Sprint(i)}); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	es, err := VerifyFile(p)
	if err != nil || len(es) != 50 {
		t.Fatalf("n=%d err=%v", len(es), err)
	}
	if es[0].PrevHash != GenesisHash {
		t.Fatal("la primera entrada debe encadenar con el génesis")
	}
}

func lines(t *testing.T, p string) []string {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

func rewrite(t *testing.T, p string, ls []string) {
	if err := os.WriteFile(p, []byte(strings.Join(ls, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTamperingDetected(t *testing.T) {
	cases := map[string]struct {
		mut  func([]string) []string
		line int
	}{
		"alterada":        {func(l []string) []string { l[1] = strings.Replace(l[1], "gate.deny", "gate.allow", 1); return l }, 2},
		"borrada":         {func(l []string) []string { return append(l[:1:1], l[2:]...) }, 2},
		"primera borrada": {func(l []string) []string { return l[1:] }, 1},
		"reordenada":      {func(l []string) []string { l[1], l[2] = l[2], l[1]; return l }, 2},
		"duplicada":       {func(l []string) []string { return append(l[:2:2], l[1:]...) }, 3},
		"campo extra":     {func(l []string) []string { l[0] = strings.Replace(l[0], `{"at"`, `{"x":1,"at"`, 1); return l }, 1},
		"basura":          {func(l []string) []string { l[2] = "no-json"; return l }, 3},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			p, l := newLog(t, 4)
			l.Close()
			rewrite(t, p, c.mut(lines(t, p)))
			_, err := VerifyFile(p)
			ve, ok := err.(*VerifyError)
			if !ok || ve.Line != c.line {
				t.Fatalf("esperaba VerifyError en línea %d, obtuve %v", c.line, err)
			}
		})
	}
}

func TestTruncatedLastLineDetectedAndOpenRefuses(t *testing.T) {
	p, l := newLog(t, 3)
	l.Close()
	b, _ := os.ReadFile(p)
	if err := os.WriteFile(p, b[:len(b)-10], 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyFile(p); err == nil {
		t.Fatal("la línea truncada debe detectarse")
	}
	if _, err := Open(p, nil); err == nil || !strings.Contains(err.Error(), "línea 3") {
		t.Fatalf("Open debe negarse a continuar sobre una cadena rota e indicar la línea: %v", err)
	}
}

func TestReopenContinuesChain(t *testing.T) {
	p, l := newLog(t, 3)
	l.Close()
	l2, err := Open(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	writeN(t, l2, 2)
	es, err := VerifyFile(p)
	if err != nil || len(es) != 5 {
		t.Fatalf("n=%d err=%v", len(es), err)
	}
}

func TestAppendFailurePoisonsLog(t *testing.T) {
	_, l := newLog(t, 1)
	l.f.Close() // simula disco caído
	if _, err := l.Append(Entry{Actor: "a", Action: "x"}); err == nil {
		t.Fatal("la escritura debía fallar")
	}
	if _, err := l.Append(Entry{Actor: "a", Action: "x"}); err == nil {
		t.Fatal("el log envenenado debe seguir fallando")
	}
}

func TestQueryAndRunsAllowed(t *testing.T) {
	day1 := time.Date(2026, 3, 1, 23, 59, 0, 0, time.UTC)
	cur := day1
	l, err := Open(filepath.Join(t.TempDir(), "a.jsonl"), func() time.Time { return cur })
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	allow := Entry{Actor: "s", Action: "gate.allow", RunID: "r", Detail: map[string]string{"to": "running", "workflow": "checkout"}}
	l.Append(allow)
	l.Append(allow)
	l.Append(Entry{Actor: "s", Action: "gate.deny", RunID: "r", Detail: map[string]string{"to": "running", "workflow": "checkout"}})
	l.Append(Entry{Actor: "s", Action: "gate.allow", RunID: "r", Detail: map[string]string{"to": "inferring", "workflow": "checkout"}})
	if n := l.RunsAllowed("checkout", day1); n != 2 {
		t.Fatalf("runs=%d", n)
	}
	cur = day1.Add(2 * time.Minute) // día UTC siguiente
	l.Append(allow)
	if l.RunsAllowed("checkout", day1) != 2 || l.RunsAllowed("checkout", cur) != 1 || l.RunsAllowed("otro", cur) != 0 {
		t.Fatal("el conteo debe separarse por día UTC y workflow")
	}
	if got := l.Query("", 3); len(got) != 3 || got[2].At != cur {
		t.Fatalf("Query orden/limit: %d", len(got))
	}
	if got := l.Query("nada", 10); len(got) != 0 {
		t.Fatal("run inexistente")
	}
	// el índice se reconstruye al reabrir
	l.Close()
	l2, _ := Open(l.f.Name(), fixedClock(cur))
	defer l2.Close()
	if l2.RunsAllowed("checkout", day1) != 2 {
		t.Fatal("el índice debe reconstruirse al reabrir")
	}
}

func TestNoDeleteAPI(t *testing.T) {
	// Los únicos métodos exportados de Log: ninguno borra ni reescribe.
	typ := reflect.TypeOf(&Log{})
	var got []string
	for i := 0; i < typ.NumMethod(); i++ {
		got = append(got, typ.Method(i).Name)
	}
	// LastAt y VerifyNow solo leen.
	if want := []string{"Append", "Close", "LastAt", "Path", "Query", "RunsAllowed", "VerifyNow"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("métodos de Log = %v, esperados %v", got, want)
	}
}

type countingFile struct {
	logFile
	syncs int
}

func (c *countingFile) Sync() error { c.syncs++; return c.logFile.Sync() }

// F-06: cada entrada debe hacer fsync (y después de escribirla).
func TestAppendSyncsEveryEntry(t *testing.T) {
	_, l := newLog(t, 0)
	cf := &countingFile{logFile: l.f}
	l.f = cf
	writeN(t, l, 5)
	if cf.syncs != 5 {
		t.Fatalf("Sync llamado %d veces, esperadas 5 (una por entrada)", cf.syncs)
	}
}

type failSyncFile struct{ logFile }

func (failSyncFile) Sync() error { return os.ErrInvalid }

func TestSyncFailureFailsAppendAndPoisons(t *testing.T) {
	_, l := newLog(t, 1)
	l.f = failSyncFile{l.f}
	if _, err := l.Append(Entry{Actor: "a", Action: "x"}); err == nil {
		t.Fatal("un fsync fallido debe fallar la entrada (no se permite sin persistir)")
	}
	if _, err := l.Append(Entry{Actor: "a", Action: "x"}); err == nil {
		t.Fatal("el log debe quedar envenenado")
	}
}
