package httpapi_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/fakes"
	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/httpapi"
	"github.com/prometheus/client_golang/prometheus"
)

func srv(b *fakes.Bundle) http.Handler {
	return httpapi.New(httpapi.Config{Token: "tok", Service: b.Svc, Sessions: b.Sessions, Clock: b.Clock,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Gatherer: prometheus.NewRegistry()})
}

func do(h http.Handler, method, path, tok, body string) int {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if tok != "" {
		r.Header.Set("Authorization", "Bearer "+tok)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

func TestAPIAuthAndValidation(t *testing.T) {
	h := srv(fakes.NewBundle())
	cases := []struct {
		tok, body string
		want      int
	}{
		{"", `{"run_id":"r-1"}`, 401},
		{"mal", `{"run_id":"r-1"}`, 401},
		{"tok", `{"run_id":""}`, 400},
		{"tok", `{"run_id":"r-1","namespace":"aqs-system"}`, 400},
		{"tok", `{"run_id":"r-1"} {}`, 400},
		{"tok", `no-json`, 400},
		{"tok", `{"run_id":"../x"}`, 400},
		{"tok", `{"run_id":"r-1"}`, 200},
	}
	for _, c := range cases {
		if got := do(h, "POST", "/resets", c.tok, c.body); got != c.want {
			t.Errorf("%q %q: %d != %d", c.tok, c.body, got, c.want)
		}
	}
}

func TestAPIQuarantineIs409(t *testing.T) {
	b := fakes.NewBundle()
	b.DB.Sticky = 1
	if got := do(srv(b), "POST", "/resets", "tok", `{"run_id":"r-1"}`); got != 409 {
		t.Fatal(got)
	}
}

func TestAPISessionHeartbeat(t *testing.T) {
	b := fakes.NewBundle()
	h := srv(b)
	if got := do(h, "PUT", "/sessions", "tok", `{"run_id":"r-1","plan":{"x":1},"state":"executing"}`); got != 204 {
		t.Fatal(got)
	}
	if l, _ := b.Sessions.List(context.Background()); len(l) != 1 {
		t.Fatal(l)
	}
	if got := do(h, "PUT", "/sessions", "", `{}`); got != 401 {
		t.Fatal(got)
	}
}
