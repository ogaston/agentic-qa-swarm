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

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/authz"
)

func identity(t *testing.T, h http.HandlerFunc) *HTTPTokenVerifier {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	v, err := NewHTTPTokenVerifier(srv.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestHTTPVerifierMapsIdentityResponses(t *testing.T) {
	var calls atomic.Int32
	v := identity(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/auth/session" || r.Method != http.MethodGet {
			t.Errorf("petición inesperada %s %s", r.Method, r.URL.Path)
		}
		switch strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") {
		case "admin":
			w.Write([]byte(`{"principal_id":"p1","role":"admin","session_id":"s","expires_at":"2030-01-01T00:00:00Z"}`))
		case "malo":
			w.WriteHeader(401)
		case "rolextra":
			w.Write([]byte(`{"principal_id":"p1","role":"root"}`))
		case "vacio":
			w.Write([]byte(`{"role":"user"}`))
		case "json":
			w.Write([]byte(`{rota`))
		case "redir":
			http.Redirect(w, r, "/otra", 302)
		default:
			w.WriteHeader(500)
		}
	})
	ctx := context.Background()
	if p, err := v.Verify(ctx, "admin"); err != nil || p != (authz.Principal{ID: "p1", Role: authz.RoleAdmin}) {
		t.Fatalf("%+v %v", p, err)
	}
	if _, err := v.Verify(ctx, "malo"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("401 de identidad debe ser token inválido: %v", err)
	}
	for _, tok := range []string{"rolextra", "vacio", "json", "redir", "otro"} {
		if _, err := v.Verify(ctx, tok); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("%s: debe ser no disponible (nunca permitir): %v", tok, err)
		}
	}
	before := calls.Load()
	v.Verify(ctx, "admin")
	v.Verify(ctx, "admin")
	if calls.Load()-before != 2 {
		t.Fatal("sin caché: una llamada a identidad por verificación")
	}
}

func TestHTTPVerifierIdentityDown(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	v, _ := NewHTTPTokenVerifier(srv.URL, time.Second)
	srv.Close()
	if _, err := v.Verify(context.Background(), "x"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}

func TestNewHTTPTokenVerifierRejectsBadURL(t *testing.T) {
	for _, u := range []string{"", "identity", "ftp://x", "http://", "://"} {
		if _, err := NewHTTPTokenVerifier(u, time.Second); err == nil {
			t.Errorf("%q debía rechazarse", u)
		}
	}
}

func TestParseFakeTokens(t *testing.T) {
	f, err := ParseFakeTokens("a=a1:admin, b=u1:user")
	if err != nil || len(f.Tokens) != 2 {
		t.Fatal(f, err)
	}
	if p, _ := f.Verify(context.Background(), "b"); p.Role != authz.RoleUser {
		t.Fatal(p)
	}
	if _, err := f.Verify(context.Background(), "zz"); !errors.Is(err, ErrInvalidToken) {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "a", "a=b", "a=b:root", "=b:user", "a=:user"} {
		if _, err := ParseFakeTokens(bad); err == nil {
			t.Errorf("%q debía fallar", bad)
		}
	}
}

func TestServiceToken(t *testing.T) {
	if _, err := NewServiceToken(strings.Repeat("a", 31)); err == nil {
		t.Fatal("31 caracteres no bastan")
	}
	tok, err := NewServiceToken(strings.Repeat("a", 32))
	if err != nil {
		t.Fatal(err)
	}
	if !tok.Matches(strings.Repeat("a", 32)) || tok.Matches(strings.Repeat("a", 31)) || tok.Matches("") || tok.Matches(strings.Repeat("a", 33)) {
		t.Fatal("comparación")
	}
}

func TestBearerToken(t *testing.T) {
	for h, want := range map[string]string{"Bearer abc": "abc", "bearer abc": "abc", "Bearer  abc ": "abc", "": "", "Bearer": "", "Bearer ": "", "Basic abc": ""} {
		hd := http.Header{}
		if h != "" {
			hd.Set("Authorization", h)
		}
		if got, ok := BearerToken(hd); got != want || ok != (want != "") {
			t.Errorf("%q -> %q %v", h, got, ok)
		}
	}
}
