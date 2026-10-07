package adapters

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

func js(w http.ResponseWriter, code int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = fmt.Fprint(w, body)
}

const warmReady = `{"warm_id":"w","state":"ready","reset_verified":true,"baseline_version":"v1"}`
const warmDirty = `{"warm_id":"w","state":"dirty","reset_verified":false,"baseline_version":"v1"}`

func warmClient(t *testing.T, h http.Handler) *WarmClient {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := NewWarmClient(srv.URL, "tok")
	if err != nil {
		t.Fatal(err)
	}
	c.s.client.Timeout = 150 * time.Millisecond
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	c.MaxWait = time.Minute
	return c
}

type httpCase struct {
	name    string
	h       http.HandlerFunc
	wantErr bool
}

func slow(w http.ResponseWriter, _ *http.Request) {
	time.Sleep(400 * time.Millisecond)
	js(w, 200, warmReady)
}

func redirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "http://127.0.0.1:1/x", http.StatusFound)
}

// Cada adaptador falla cerrado ante 409, 5xx, 401, cuerpo inválido, campo extra, redirección y timeout.
func TestWarmEnsureFailsClosed(t *testing.T) {
	for _, c := range []httpCase{
		{"200 listo", func(w http.ResponseWriter, _ *http.Request) { js(w, 200, warmReady) }, false},
		{"200 pero dirty", func(w http.ResponseWriter, _ *http.Request) { js(w, 200, warmDirty) }, true},
		{"ready pero reset_verified=false", func(w http.ResponseWriter, _ *http.Request) {
			js(w, 200, `{"warm_id":"w","state":"ready","reset_verified":false,"baseline_version":"v1"}`)
		}, true},
		{"dirty con reset_verified=true", func(w http.ResponseWriter, _ *http.Request) {
			js(w, 200, `{"warm_id":"w","state":"dirty","reset_verified":true,"baseline_version":"v1"}`)
		}, true},
		{"409", func(w http.ResponseWriter, _ *http.Request) { js(w, 409, warmDirty) }, true},
		{"409 con cuerpo ready", func(w http.ResponseWriter, _ *http.Request) { js(w, 409, warmReady) }, true},
		{"500", func(w http.ResponseWriter, _ *http.Request) { js(w, 500, `{}`) }, true},
		{"401", func(w http.ResponseWriter, _ *http.Request) { js(w, 401, `{}`) }, true},
		{"cuerpo roto", func(w http.ResponseWriter, _ *http.Request) { js(w, 200, `{`) }, true},
		{"campo extra", func(w http.ResponseWriter, _ *http.Request) {
			js(w, 200, `{"warm_id":"w","state":"ready","reset_verified":true,"baseline_version":"v","x":1}`)
		}, true},
		{"sin reset_verified", func(w http.ResponseWriter, _ *http.Request) {
			js(w, 200, `{"warm_id":"w","state":"ready","baseline_version":"v"}`)
		}, true},
		{"redirección", redirect, true},
		{"timeout", slow, true},
	} {
		w := warmClient(t, c.h)
		if err := w.Ensure(t.Context()); (err != nil) != c.wantErr {
			t.Errorf("%s: err=%v", c.name, err)
		}
	}
}

func TestWarmStateAndResetVerified(t *testing.T) {
	w := warmClient(t, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			js(rw, 401, `{}`)
			return
		}
		js(rw, 200, warmDirty)
	}))
	if f, err := w.ResetVerified(t.Context()); err != nil || f != runctl.False {
		t.Fatal(f, err)
	}
	w2 := warmClient(t, http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) { js(rw, 500, `{}`) }))
	if f, err := w2.ResetVerified(t.Context()); err == nil || f != runctl.Unknown {
		t.Fatalf("un error nunca es un hecho: %v %v", f, err)
	}
}

func TestWarmSurfaceFailsClosed(t *testing.T) {
	ok := `{"run_id":"r-1","base_url":"http://warm","endpoints":[{"method":"GET","path":"/a"}],"source":"probe"}`
	for _, c := range []httpCase{
		{"200", func(w http.ResponseWriter, _ *http.Request) { js(w, 200, ok) }, false},
		{"409", func(w http.ResponseWriter, _ *http.Request) { js(w, 409, `{"error":"deploy_not_done"}`) }, true},
		{"502", func(w http.ResponseWriter, _ *http.Request) { js(w, 502, `{}`) }, true},
		{"cuerpo roto", func(w http.ResponseWriter, _ *http.Request) { js(w, 200, `[]`) }, true},
		{"esquema inválido", func(w http.ResponseWriter, _ *http.Request) {
			js(w, 200, `{"run_id":"r-1","base_url":"ftp://x","endpoints":[],"source":"probe"}`)
		}, true},
		{"0 endpoints", func(w http.ResponseWriter, _ *http.Request) {
			js(w, 200, `{"run_id":"r-1","base_url":"http://w","endpoints":[],"source":"probe"}`)
		}, true},
		{"otra corrida", func(w http.ResponseWriter, _ *http.Request) {
			js(w, 200, strings.Replace(ok, "r-1", "r-9", 1))
		}, true},
		{"redirección", redirect, true},
		{"timeout", slow, true},
	} {
		w := warmClient(t, c.h)
		if _, err := w.Surface(t.Context(), "r-1"); (err != nil) != c.wantErr {
			t.Errorf("%s: err=%v", c.name, err)
		}
	}
	if _, err := warmClient(t, http.NotFoundHandler()).Surface(t.Context(), "../x"); err == nil {
		t.Error("run_id inválido aceptado")
	}
}

// deployServer simula go-warm-manager: gets son los estados que devuelve GET /deploys/{run}; los 404 iniciales = no existe.
func deployServer(post int, getBodies ...string) (http.Handler, *atomic.Int32, *atomic.Int32) {
	var gets, posts atomic.Int32
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/warm/ensure":
			js(w, 200, warmReady)
		case r.Method == "POST" && r.URL.Path == "/deploys":
			posts.Add(1)
			js(w, post, `{"run_id":"r-1","state":"pending","attempts":1}`)
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/deploys/"):
			i := int(gets.Add(1)) - 1
			if i >= len(getBodies) {
				i = len(getBodies) - 1
			}
			if getBodies[i] == "404" {
				js(w, 404, `{"error":"not_found"}`)
				return
			}
			js(w, 200, getBodies[i])
		default:
			js(w, 500, `{}`)
		}
	}), &gets, &posts
}

func st(state string) string { return `{"run_id":"r-1","state":"` + state + `","attempts":1}` }

func TestWarmDeployFailsClosedAndWaitsForDone(t *testing.T) {
	for _, c := range []struct {
		name    string
		post    int
		gets    []string
		wantErr bool
	}{
		{"nuevo y done", 202, []string{"404", st("pending"), st("done")}, false},
		{"failed", 202, []string{"404", st("pending"), `{"run_id":"r-1","state":"failed","attempts":3,"reason":"rollout"}`}, true},
		{"post 409", 409, []string{"404"}, true},
		{"post 422", 422, []string{"404"}, true},
		{"post 5xx", 503, []string{"404"}, true},
		{"estado inválido", 202, []string{"404", `{"run_id":"r-1","state":"raro","attempts":1}`}, true},
		{"estado de otra corrida", 202, []string{"404", `{"run_id":"r-2","state":"done","attempts":1}`}, true},
		{"se pierde el estado", 202, []string{"404", st("pending"), "404"}, true},
		{"ya existía y done (reintento tras crash)", 202, []string{st("done")}, false},
	} {
		h, _, _ := deployServer(c.post, c.gets...)
		w := warmClient(t, h)
		if err := w.Deploy(t.Context(), "r-1", "published-image", "ghcr.io/x/y:1"); (err != nil) != c.wantErr {
			t.Errorf("%s: err=%v", c.name, err)
		}
	}
}

func TestWarmDeployIsIdempotentAndBoundedInTime(t *testing.T) {
	h, _, posts := deployServer(202, st("done"))
	w := warmClient(t, h)
	for i := 0; i < 3; i++ {
		if err := w.Deploy(t.Context(), "r-1", "published-image", "ghcr.io/x/y:1"); err != nil {
			t.Fatal(err)
		}
	}
	if posts.Load() != 0 {
		t.Fatalf("un deploy existente se adopta, no se vuelve a crear: %d POST", posts.Load())
	}
	h, _, _ = deployServer(202, "404", st("pending"))
	w = warmClient(t, h)
	w.MaxWait = 0
	if err := w.Deploy(t.Context(), "r-1", "published-image", "ghcr.io/x/y:1"); err == nil {
		t.Fatal("pending eterno no puede ser éxito")
	}
}

func TestResetFailsClosed(t *testing.T) {
	okBody := `{"state":"ready","reset_verified":true,"checks":[],"attempts":1}`
	for _, c := range []httpCase{
		{"200 verificado", func(w http.ResponseWriter, _ *http.Request) { js(w, 200, okBody) }, false},
		{"409", func(w http.ResponseWriter, _ *http.Request) {
			js(w, 409, `{"state":"dirty","reset_verified":false,"checks":[],"attempts":3}`)
		}, true},
		{"500", func(w http.ResponseWriter, _ *http.Request) { js(w, 500, `{"error":"internal"}`) }, true},
		{"401", func(w http.ResponseWriter, _ *http.Request) { js(w, 401, `{}`) }, true},
		{"200 sin verificar", func(w http.ResponseWriter, _ *http.Request) {
			js(w, 200, `{"state":"ready","reset_verified":false,"checks":[],"attempts":1}`)
		}, true},
		{"200 sin reset_verified", func(w http.ResponseWriter, _ *http.Request) { js(w, 200, `{"state":"ready"}`) }, true},
		{"200 dirty", func(w http.ResponseWriter, _ *http.Request) {
			js(w, 200, `{"state":"dirty","reset_verified":true,"checks":[],"attempts":1}`)
		}, true},
		{"cuerpo roto", func(w http.ResponseWriter, _ *http.Request) { js(w, 200, `nope`) }, true},
		{"redirección", redirect, true},
		{"timeout", slow, true},
	} {
		srv := httptest.NewServer(c.h)
		r, err := NewResetClient(srv.URL, "tok")
		if err != nil {
			t.Fatal(err)
		}
		r.s.client.Timeout = 150 * time.Millisecond
		if err := r.Reset(t.Context(), "r-1"); (err != nil) != c.wantErr {
			t.Errorf("%s: err=%v", c.name, err)
		}
		srv.Close()
	}
}

func TestClientsRejectBadURLsAndEmptyTokens(t *testing.T) {
	for _, u := range []string{"ftp://x", "file:///etc/passwd", "http://u:p@x", "", "x", "http://"} {
		if _, err := NewWarmClient(u, "t"); err == nil {
			t.Errorf("warm acepta %q", u)
		}
		if _, err := NewResetClient(u, "t"); err == nil {
			t.Errorf("reset acepta %q", u)
		}
	}
	if _, err := NewWarmClient("http://x", ""); err == nil {
		t.Error("token vacío")
	}
	if _, err := NewResetClient("https://x", ""); err == nil {
		t.Error("token vacío")
	}
}

func TestClientsSendBearerAndStopRedirects(t *testing.T) {
	var got atomic.Value
	var hits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits.Add(1); js(w, 200, warmReady) }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Header.Get("Authorization"))
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	w, _ := NewWarmClient(srv.URL, "s3cret")
	if err := w.Ensure(t.Context()); err == nil {
		t.Fatal("la redirección no puede ser éxito")
	}
	if got.Load() != "Bearer s3cret" || hits.Load() != 0 {
		t.Fatalf("auth=%v, el destino de la redirección recibió %d peticiones", got.Load(), hits.Load())
	}
}

func TestServiceDownFailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	w, _ := NewWarmClient(url, "t")
	r, _ := NewResetClient(url, "t")
	if w.Ensure(t.Context()) == nil || r.Reset(t.Context(), "r-1") == nil {
		t.Fatal("servicio detenido debe fallar")
	}
	if _, err := w.ResetVerified(t.Context()); err == nil {
		t.Fatal("servicio detenido no es un hecho")
	}
}

func TestRealPhasesDispatchAndUnimplementedFailClosed(t *testing.T) {
	p := &RealPhases{}
	for _, ph := range []string{runctl.PhaseRun, runctl.PhaseReport, "otra"} {
		if _, err := p.Launch(t.Context(), ph, runctl.Run{ID: "r-1"}); err == nil {
			t.Errorf("%s no implementada y aceptada", ph)
		}
	}
	if _, err := p.Launch(t.Context(), runctl.PhaseRehearse, runctl.Run{ID: "r-1"}); err == nil {
		t.Error("ensayo sin lanzador aceptado")
	}
	if _, err := p.Launch(t.Context(), runctl.PhaseDeploy, runctl.Run{ID: "r-1"}); err == nil {
		t.Error("deploy sin fuente de artefacto aceptado")
	}
}

// El POST /deploys con run_id ajeno es error aunque el estado posterior diga done.
func TestWarmDeployPostRunIDMismatch(t *testing.T) {
	var gets atomic.Int32
	w := warmClient(t, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/warm/ensure":
			js(rw, 200, warmReady)
		case r.Method == "POST":
			js(rw, 202, `{"run_id":"otra","state":"done","attempts":1}`)
		default:
			if gets.Add(1) == 1 {
				js(rw, 404, `{"error":"not_found"}`)
				return
			}
			js(rw, 200, st("done"))
		}
	}))
	if err := w.Deploy(t.Context(), "r-1", "published-image", "ghcr.io/x/y:1"); err == nil {
		t.Fatal("run_id ajeno aceptado")
	}
}

func TestDefaultTimeoutsAndEnsureOwnDeadline(t *testing.T) {
	w, _ := NewWarmClient("http://x", "t")
	if w.s.timeout != 5*time.Second || w.EnsureTimeout != 130*time.Second {
		t.Fatalf("plazos %v %v", w.s.timeout, w.EnsureTimeout)
	}
	r, _ := NewResetClient("http://x", "t")
	if r.s.timeout != 5*time.Second {
		t.Fatalf("reset %v", r.s.timeout)
	}
	// ensure lento (300 ms) pasa con su plazo propio aunque el de fase sea menor; el resto de llamadas no.
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		js(rw, 200, warmReady)
	}))
	defer srv.Close()
	w, _ = NewWarmClient(srv.URL, "t")
	w.s.timeout, w.EnsureTimeout = 100*time.Millisecond, 3*time.Second
	if err := w.Ensure(t.Context()); err != nil {
		t.Fatalf("ensure debe esperar su plazo propio: %v", err)
	}
	if _, err := w.State(t.Context()); err == nil {
		t.Fatal("GET /warm debe respetar el plazo de fase")
	}
	w.EnsureTimeout = 100 * time.Millisecond
	if err := w.Ensure(t.Context()); err == nil {
		t.Fatal("ensure debe fallar al agotar su plazo")
	}
}
