package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/ui-api/inbox"
	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/auth"
)

type errVerifier struct{ err error }

func (v errVerifier) Verify(context.Context, string) (auth.Principal, error) {
	return auth.Principal{}, v.err
}

func TestErrUnavailableIs503WithRetryAfterAndOtherErrorsAre401(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code int
	}{
		{auth.ErrUnavailable, 503},
		{fmt.Errorf("envuelto: %w", auth.ErrUnavailable), 503},
		{auth.ErrUnauthenticated, 401},
		{errors.New("otro error"), 401},
	} {
		e := newEnv(t, errVerifier{tc.err}, nil)
		for _, req := range [][2]string{{"GET", "/notifications"}, {"POST", "/notifications/n-1/confirm"}} {
			w := e.do(req[0], req[1], "tok", `{"flows":["f"]}`)
			if w.Code != tc.code {
				t.Fatalf("%v %s: %d", tc.err, req[1], w.Code)
			}
			if tc.code == 503 {
				var b map[string]string
				_ = json.Unmarshal(w.Body.Bytes(), &b)
				if b["code"] != "identity_unavailable" || w.Header().Get("Retry-After") == "" || strings.Contains(w.Body.String(), "tok") {
					t.Fatalf("503 mal formado: %v %v", w.Header(), w.Body)
				}
			}
		}
	}
	if n := len(newEnv(t, errVerifier{auth.ErrUnavailable}, nil).store.List(inbox.StateConfirmed)); n != 0 {
		t.Fatal("ningun estado cambia con identidad caida")
	}
}

type flakyPub struct {
	mu    sync.Mutex
	fails int
	got   []inbox.RunConfirmed
}

func (p *flakyPub) Publish(_ context.Context, ev inbox.RunConfirmed) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fails > 0 {
		p.fails--
		return errors.New("transporte caido")
	}
	p.got = append(p.got, ev)
	return nil
}

func withPublisher(p inbox.EventPublisher) func(*Config) {
	return func(c *Config) { c.Publisher = inbox.NewPublisher(c.Store, p, nil) }
}

func TestConfirmPublishesRunConfirmedOnce(t *testing.T) {
	p := &flakyPub{}
	e := newEnv(t, nil, withPublisher(p))
	w := e.do("POST", "/notifications/n-1/confirm", secretTok, `{"flows":["checkout","refund"]}`, "traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	if w.Code != 201 || len(p.got) != 1 {
		t.Fatalf("%d eventos=%d", w.Code, len(p.got))
	}
	ev := p.got[0]
	var rc inbox.Receipt
	_ = json.Unmarshal(w.Body.Bytes(), &rc)
	if ev.Type != "run.confirmed" || ev.Data.RunID != rc.RunID || ev.Data.ConfirmedBy != "u1" ||
		strings.Join(ev.Data.Flows, ",") != "checkout,refund" || !ev.OccurredAt.Equal(rc.ConfirmedAt) || ev.EventID != inbox.EventID(rc.RunID) {
		t.Fatalf("evento %+v recibo %+v", ev, rc)
	}
	if w2 := e.do("POST", "/notifications/n-1/confirm", secretTok, `{"flows":["f"]}`); w2.Code != 409 || len(p.got) != 1 {
		t.Fatalf("segunda confirmacion: %d eventos=%d", w2.Code, len(p.got))
	}
}

func TestPublishPendingKeepsReceiptWhenPublishFails(t *testing.T) {
	p := &flakyPub{fails: 1}
	e := newEnv(t, nil, withPublisher(p))
	w := e.do("POST", "/notifications/n-1/confirm", secretTok, `{"flows":["f"]}`)
	if w.Code != 201 {
		t.Fatalf("publicar falla pero la respuesta debe ser 201, fue %d", w.Code)
	}
	if e.store.PublishPendingCount() != 1 || len(e.store.List(inbox.StateConfirmed)) != 1 || len(p.got) != 0 {
		t.Fatal("el recibo existe y queda publish_pending")
	}
}

func TestConcurrentConfirmPublishesSingleEvent(t *testing.T) {
	p := &flakyPub{}
	e := newEnv(t, nil, withPublisher(p))
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.do(http.MethodPost, "/notifications/n-1/confirm", secretTok, `{"flows":["f"]}`)
		}()
	}
	wg.Wait()
	if len(p.got) != 1 {
		t.Fatalf("eventos=%d, quiero 1", len(p.got))
	}
}
