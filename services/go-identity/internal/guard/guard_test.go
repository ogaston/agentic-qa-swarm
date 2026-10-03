package guard

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

type clk struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clk) now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clk) add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

func newClk() *clk { return &clk{t: time.Unix(1_700_000_000, 0)} }

func burn(g *Guard, user, ip string, n int) {
	for i := 0; i < n; i++ {
		g.Allow(user, ip)
	}
}

func TestLocksAfterMaxFailures(t *testing.T) {
	c := newClk()
	g := New(5, c.now)
	for i := 0; i < 5; i++ {
		if _, _, ok := g.Allow("marta", "1.1.1.1"); !ok {
			t.Fatalf("intento %d debía admitirse", i+1)
		}
	}
	_, retry, ok := g.Allow("marta", "1.1.1.1")
	if ok || retry != 30*time.Second {
		t.Fatalf("el sexto debía bloquearse 30 s: ok=%v retry=%v", ok, retry)
	}
}

func TestBackoffDoublesUpToCap(t *testing.T) {
	c := newClk()
	g := New(2, c.now)
	want := []time.Duration{30 * time.Second, 60 * time.Second, 120 * time.Second, 240 * time.Second, 480 * time.Second, 900 * time.Second, 900 * time.Second}
	for i, w := range want {
		burn(g, "u", "ip", 2) // el segundo fallo impone el bloqueo (el primero tras expirar relanza)
		_, retry, ok := g.Allow("u", "ip")
		if ok || retry != w {
			t.Fatalf("bloqueo %d: ok=%v retry=%v, esperado %v", i, ok, retry, w)
		}
		c.add(w)
	}
}

func TestSuccessResetsCounter(t *testing.T) {
	c := newClk()
	g := New(5, c.now)
	var tk Ticket
	for i := 0; i < 4; i++ {
		tk, _, _ = g.Allow("u", "ip")
	}
	g.Success(tk)
	for i := 0; i < 4; i++ {
		if _, _, ok := g.Allow("u", "otra-ip"); !ok {
			t.Fatal("tras un acierto el contador del usuario reinicia")
		}
	}
}

func TestReleaseDoesNotCount(t *testing.T) {
	c := newClk()
	g := New(3, c.now)
	for i := 0; i < 20; i++ {
		tk, _, ok := g.Allow("u", "ip")
		if !ok {
			t.Fatal("liberar un intento no debe acercar al bloqueo")
		}
		g.Release(tk)
	}
}

func TestReleaseUndoesLockItTriggered(t *testing.T) {
	c := newClk()
	g := New(2, c.now)
	g.Allow("u", "ip")
	tk, _, _ := g.Allow("u", "ip") // dispara el bloqueo
	g.Release(tk)
	if _, _, ok := g.Allow("u", "ip"); !ok {
		t.Fatal("el bloqueo provocado por el intento liberado debía deshacerse")
	}
}

func TestPerIPCounterIndependentOfUsername(t *testing.T) {
	c := newClk()
	g := New(3, c.now)
	for i := 0; i < 3; i++ {
		g.Allow(fmt.Sprintf("u%d", i), "9.9.9.9")
	}
	if _, _, ok := g.Allow("otro", "9.9.9.9"); ok {
		t.Fatal("la IP con 3 fallos debía bloquearse aunque cambie el usuario")
	}
	if _, _, ok := g.Allow("otro", "8.8.8.8"); !ok {
		t.Fatal("otra IP no debía verse afectada")
	}
}

func TestTablesAreBounded(t *testing.T) {
	c := newClk()
	g := New(5, c.now)
	for i := 0; i < MaxEntries+500; i++ {
		c.add(time.Millisecond)
		g.Allow(fmt.Sprintf("nadie-%d", i), fmt.Sprintf("ip-%d", i))
	}
	u, ip := g.Len()
	if u > MaxEntries || ip > MaxEntries {
		t.Fatalf("tablas sin acotar: %d %d", u, ip)
	}
}

func TestConcurrentAttemptsNeverExceedLimit(t *testing.T) {
	g := New(5, nil)
	var admitted int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, ok := g.Allow("u", "ip"); ok {
				mu.Lock()
				admitted++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if admitted != 5 {
		t.Fatalf("se admitieron %d intentos concurrentes, el límite es 5", admitted)
	}
}

func TestSuccessDoesNotResetIPCounter(t *testing.T) {
	c := newClk()
	g := New(5, c.now)
	// Password spraying: un fallo contra cada víctima, intercalado con un acierto propio.
	for i := 0; i < 4; i++ {
		g.Allow(fmt.Sprintf("victima-%d", i), "6.6.6.6")
		tk, _, ok := g.Allow("atacante", "6.6.6.6")
		if !ok {
			t.Fatalf("iteración %d: el acierto propio aún debía admitirse", i)
		}
		g.Success(tk)
	}
	g.Allow("victima-4", "6.6.6.6") // quinto fallo acumulado
	if _, _, ok := g.Allow("victima-9", "6.6.6.6"); ok {
		t.Fatal("5 fallos acumulados por IP debían bloquear pese a los aciertos intercalados")
	}
}

func TestSuccessStillResetsUserCounter(t *testing.T) {
	c := newClk()
	g := New(5, c.now)
	var tk Ticket
	for i := 0; i < 4; i++ {
		tk, _, _ = g.Allow("marta", "ip-a")
	}
	g.Success(tk)
	for i := 0; i < 4; i++ {
		if _, _, ok := g.Allow("marta", "ip-b"); !ok {
			t.Fatal("el acierto reinicia el contador del usuario")
		}
	}
}

func TestIPFailuresDecayAfterWindow(t *testing.T) {
	c := newClk()
	g := New(5, c.now)
	for i := 0; i < 4; i++ {
		g.Allow(fmt.Sprintf("u%d", i), "7.7.7.7")
	}
	c.add(IPWindow + time.Second)
	for i := 0; i < 4; i++ {
		if _, _, ok := g.Allow(fmt.Sprintf("v%d", i), "7.7.7.7"); !ok {
			t.Fatal("tras la ventana los fallos por IP debían haber expirado")
		}
	}
}

func TestTicketLockedOnlyWhenThisAttemptImposesLock(t *testing.T) {
	g := New(3, nil)
	for i := 1; i <= 2; i++ {
		tk, _, ok := g.Allow("u", "1.1.1.1")
		if !ok || tk.Locked() {
			t.Fatalf("intento %d no debía imponer bloqueo", i)
		}
	}
	tk, _, ok := g.Allow("u", "1.1.1.1")
	if !ok || !tk.Locked() {
		t.Fatal("el intento que alcanza el máximo impone el bloqueo")
	}
	if _, _, ok := g.Allow("u", "1.1.1.1"); ok {
		t.Fatal("el siguiente intento debe rechazarse")
	}
}
