package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const goodSession = `{"principal_id":"u-1","role":"user","session_id":"s-1","expires_at":"2030-01-01T00:00:00Z"}`

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

type counts map[string]int

func (c counts) IdentityCall(r string) { c[r]++ }

func newV(t *testing.T, h http.HandlerFunc) (*HTTPTokenVerifier, *int32, *fakeClock, counts) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	v, err := NewHTTPTokenVerifier(srv.URL, clk.now)
	if err != nil {
		t.Fatal(err)
	}
	c := counts{}
	v.SetObserver(c)
	return v, &calls, clk, c
}

func TestHTTPVerifierValid200(t *testing.T) {
	var gotAuth, gotPath string
	v, _, _, c := newV(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		_, _ = w.Write([]byte(goodSession))
	})
	pr, err := v.Verify(context.Background(), "tok")
	if err != nil || pr != (Principal{ID: "u-1", Role: RoleUser}) {
		t.Fatalf("pr=%+v err=%v", pr, err)
	}
	if gotAuth != "Bearer tok" || gotPath != "/auth/session" || c[ResultOK] != 1 {
		t.Fatalf("auth=%q path=%q c=%v", gotAuth, gotPath, c)
	}
}

func TestHTTPVerifier401IsUnauthenticated(t *testing.T) {
	v, _, _, c := newV(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(401) })
	if _, err := v.Verify(context.Background(), "x"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("err=%v", err)
	}
	if c[ResultUnauthorized] != 1 || v.CircuitOpen() {
		t.Fatalf("c=%v", c)
	}
}

func TestHTTPVerifierInvalidResponsesAreUnavailable(t *testing.T) {
	for name, body := range map[string]string{
		"cuerpo roto":     `{no es json`,
		"rol desconocido": `{"principal_id":"u","role":"root","session_id":"s","expires_at":"2030-01-01T00:00:00Z"}`,
		"sin principal":   `{"role":"user","session_id":"s","expires_at":"2030-01-01T00:00:00Z"}`,
		"fecha invalida":  `{"principal_id":"u","role":"user","session_id":"s","expires_at":"manana"}`,
		"tipo erroneo":    `{"principal_id":5,"role":"user","session_id":"s","expires_at":"2030-01-01T00:00:00Z"}`,
	} {
		t.Run(name, func(t *testing.T) {
			v, _, _, _ := newV(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) })
			if pr, err := v.Verify(context.Background(), "x"); !errors.Is(err, ErrUnavailable) || pr != (Principal{}) {
				t.Fatalf("pr=%+v err=%v", pr, err)
			}
		})
	}
}

func TestHTTPVerifierOtherStatusAndRedirectAreUnavailable(t *testing.T) {
	for _, code := range []int{500, 403, 302, 204} {
		v, _, _, _ := newV(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "http://127.0.0.1:1/")
			w.WriteHeader(code)
		})
		if _, err := v.Verify(context.Background(), "x"); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("status %d: err=%v", code, err)
		}
	}
}

func TestHTTPVerifierTimeoutIsUnavailable(t *testing.T) {
	v, _, _, _ := newV(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	v.timeout = 50 * time.Millisecond
	start := time.Now()
	if _, err := v.Verify(context.Background(), "x"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err=%v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("el timeout no corto la llamada")
	}
	if IdentityTimeout != 2*time.Second {
		t.Fatal("el timeout por defecto debe ser 2 s")
	}
}

func TestHTTPVerifierConnectionRefusedAndTokenNotLeaked(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	v, err := NewHTTPTokenVerifier(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = v.Verify(context.Background(), "tok-SECRETO")
	if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "SECRETO") {
		t.Fatalf("err=%v", err)
	}
}

func TestHTTPVerifierCircuitOpensClosesAndHalfOpens(t *testing.T) {
	var healthy atomic.Bool
	v, calls, clk, c := newV(t, func(w http.ResponseWriter, _ *http.Request) {
		if !healthy.Load() {
			w.WriteHeader(500)
			return
		}
		_, _ = w.Write([]byte(goodSession))
	})
	ctx := context.Background()
	for i := 0; i < CircuitThreshold; i++ {
		if v.CircuitOpen() {
			t.Fatalf("abierto antes de tiempo (i=%d)", i)
		}
		_, _ = v.Verify(ctx, "x")
	}
	if !v.CircuitOpen() {
		t.Fatal("debe abrir tras 5 fallos consecutivos")
	}
	before := atomic.LoadInt32(calls)
	if _, err := v.Verify(ctx, "x"); !errors.Is(err, ErrUnavailable) || atomic.LoadInt32(calls) != before {
		t.Fatalf("abierto no debe llamar a identidad (err=%v)", err)
	}
	if c[ResultCircuitOpen] != 1 {
		t.Fatalf("c=%v", c)
	}
	// Sonda medio-abierta que falla: reabre otros 10 s.
	clk.t = clk.t.Add(CircuitOpenDuration + time.Second)
	_, _ = v.Verify(ctx, "x")
	if !v.CircuitOpen() || atomic.LoadInt32(calls) != before+1 {
		t.Fatal("la sonda fallida debe reabrir el circuito con una sola llamada")
	}
	if _, err := v.Verify(ctx, "x"); !errors.Is(err, ErrUnavailable) || atomic.LoadInt32(calls) != before+1 {
		t.Fatal("tras reabrir no se llama")
	}
	// Sonda que funciona: cierra.
	healthy.Store(true)
	clk.t = clk.t.Add(CircuitOpenDuration + time.Second)
	if _, err := v.Verify(ctx, "x"); err != nil || v.CircuitOpen() {
		t.Fatalf("la sonda sana debe cerrar el circuito (err=%v)", err)
	}
}

func TestHTTPVerifierSuccessResetsFailureCount(t *testing.T) {
	var fail atomic.Bool
	v, _, _, _ := newV(t, func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(502)
			return
		}
		_, _ = w.Write([]byte(goodSession))
	})
	ctx := context.Background()
	for round := 0; round < 3; round++ {
		fail.Store(true)
		for i := 0; i < CircuitThreshold-1; i++ {
			_, _ = v.Verify(ctx, "x")
		}
		fail.Store(false)
		if _, err := v.Verify(ctx, "x"); err != nil {
			t.Fatal(err)
		}
	}
	if v.CircuitOpen() {
		t.Fatal("los fallos no son consecutivos: no debe abrir")
	}
}

func TestNewHTTPTokenVerifierRejectsBadURLs(t *testing.T) {
	for _, u := range []string{"", "ftp://x", "file:///etc/passwd", "http://", "127.0.0.1:8200", "http://u:p@h"} {
		if _, err := NewHTTPTokenVerifier(u, nil); err == nil {
			t.Errorf("%q debe rechazarse", u)
		}
	}
	if _, err := NewHTTPTokenVerifier("https://identity.local:8443/", nil); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPVerifierParentCancelDoesNotCountAsIdentityFailure(t *testing.T) {
	v, _, clk, c := newV(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(goodSession)) })
	for i := 0; i < CircuitThreshold*2; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := v.Verify(ctx, "x"); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("err=%v", err)
		}
	}
	if v.CircuitOpen() || c[ResultError] != 0 {
		t.Fatalf("los abortos del llamante no abren el circuito (abierto=%v c=%v)", v.CircuitOpen(), c)
	}
	if _, err := v.Verify(context.Background(), "x"); err != nil {
		t.Fatalf("identidad sana debe seguir sirviendo: %v", err)
	}
	// Una sonda abortada libera el slot sin reabrir.
	v.mu.Lock()
	v.openUntil = clk.t.Add(-time.Second)
	v.fails = CircuitThreshold
	v.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = v.Verify(ctx, "x")
	v.mu.Lock()
	probing := v.probing
	v.mu.Unlock()
	if probing {
		t.Fatal("la sonda abortada debe liberar probing")
	}
	if _, err := v.Verify(context.Background(), "x"); err != nil || v.CircuitOpen() {
		t.Fatalf("la siguiente sonda debe poder cerrar el circuito: %v", err)
	}
}
