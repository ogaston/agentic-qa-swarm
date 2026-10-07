package api_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/internal/api"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/internal/fakes"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/internal/obs"
)

const tok = "s3cret-token"

func srv(st wm.WarmState) (*httptest.Server, *fakes.FakeJobs) {
	m := obs.NewMetrics()
	jobs := &fakes.FakeJobs{}
	svc := &wm.Service{Cfg: wm.Config{Job: wm.JobConfig{AllowedRegistries: wm.DefaultAllowedRegistries}, PollInterval: time.Millisecond},
		State: &fakes.MemState{S: st}, Probe: &fakes.FakeProbe{}, Runtime: &fakes.FakeRuntime{}, Jobs: jobs,
		Surface: &fakes.FakeProber{Base: "http://x", Routes: map[string]fakes.FakeResponse{"/openapi.json": {Status: 200, Body: `{"paths":{"/a":{"get":{}}}}`}}},
		Objects: &fakes.MemObjects{}, Alerts: &fakes.FakeAlerter{}, Pub: &fakes.MemPublisher{}, Clock: wm.RealClock{}, Observer: m}
	s := &api.Server{Svc: svc, Token: tok, Log: slog.New(slog.NewJSONHandler(io.Discard, nil)), Registry: m.Reg}
	return httptest.NewServer(s.Handler()), jobs
}

func call(t *testing.T, method, url, token, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestAPIAuth401(t *testing.T) {
	s, _ := srv(wm.WarmState{WarmID: "w", State: "dirty", BaselineVersion: "b"})
	defer s.Close()
	for _, c := range [][2]string{{"GET", "/warm"}, {"POST", "/warm/ensure"}, {"POST", "/deploys"}, {"GET", "/deploys/r-1"}, {"POST", "/surface"}} {
		for _, tk := range []string{"", "mal"} {
			if code, _ := call(t, c[0], s.URL+c[1], tk, "{}"); code != 401 {
				t.Errorf("%v token=%q -> %d", c, tk, code)
			}
		}
	}
}

func TestAPIEnsure409AndWarm(t *testing.T) {
	s, _ := srv(wm.WarmState{WarmID: "w", State: "dirty", BaselineVersion: "b"})
	defer s.Close()
	code, body := call(t, "POST", s.URL+"/warm/ensure", tok, "")
	if code != 409 || !strings.Contains(body, `"state":"dirty"`) {
		t.Fatalf("%d %s", code, body)
	}
	if code, _ := call(t, "GET", s.URL+"/warm", tok, ""); code != 200 {
		t.Fatalf("GET /warm %d", code)
	}
	s2, _ := srv(wm.WarmState{WarmID: "w", State: "ready", ResetVerified: true, BaselineVersion: "b"})
	defer s2.Close()
	if code, _ := call(t, "POST", s2.URL+"/warm/ensure", tok, ""); code != 200 {
		t.Fatalf("ensure listo %d", code)
	}
}

func TestAPIDeploy400(t *testing.T) {
	s, jobs := srv(wm.WarmState{WarmID: "w", State: "ready", ResetVerified: true, BaselineVersion: "b"})
	defer s.Close()
	for _, b := range []string{`{"run_id":"","extra":1}`, `{"run_id":"","artifact":{"kind":"published-image","ref":"ghcr.io/a/b:1"}}`,
		`{"run_id":"r-1","artifact":{"kind":"published-image","ref":"ghcr.io/a/b:1"},"x":1}`, `{"run_id":"r-1","artifact":{"kind":"published-image","ref":"docker.io/a/b:1"}}`,
		`{"run_id":"r-1","artifact":{"kind":"published-image","ref":"ghcr.io/a/b:latest"}}`, `no json`, `{"run_id":"r-1"} {}`} {
		if code, _ := call(t, "POST", s.URL+"/deploys", tok, b); code != 400 {
			t.Errorf("%s -> %d", b, code)
		}
	}
	if len(jobs.Created) != 0 {
		t.Fatal("cuerpo invalido creo Jobs")
	}
}

func TestAPIDeployAcceptedAndStatus(t *testing.T) {
	s, _ := srv(wm.WarmState{WarmID: "w", State: "ready", ResetVerified: true, BaselineVersion: "b"})
	defer s.Close()
	if code, _ := call(t, "POST", s.URL+"/deploys", tok, `{"run_id":"r-1","artifact":{"kind":"published-image","ref":"ghcr.io/a/b:1"}}`); code != 202 {
		t.Fatalf("%d", code)
	}
	var body string
	for i := 0; i < 100; i++ {
		_, body = call(t, "GET", s.URL+"/deploys/r-1", tok, "")
		if strings.Contains(body, `"done"`) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(body, `"state":"done"`) || !strings.Contains(body, `"attempts":1`) {
		t.Fatalf("%s", body)
	}
	if code, _ := call(t, "GET", s.URL+"/deploys/otro", tok, ""); code != 404 {
		t.Fatalf("desconocido %d", code)
	}
}

func TestAPISurface(t *testing.T) {
	s, _ := srv(wm.WarmState{WarmID: "w", State: "ready", ResetVerified: true, BaselineVersion: "b"})
	defer s.Close()
	if code, _ := call(t, "POST", s.URL+"/surface", tok, `{"run_id":"r-1"}`); code != 409 {
		t.Fatalf("sin deploy terminado debia ser 409, fue %d", code)
	}
	call(t, "POST", s.URL+"/deploys", tok, `{"run_id":"r-1","artifact":{"kind":"published-image","ref":"ghcr.io/a/b:1"}}`)
	for i := 0; i < 100; i++ {
		if _, b := call(t, "GET", s.URL+"/deploys/r-1", tok, ""); strings.Contains(b, `"done"`) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if code, b := call(t, "POST", s.URL+"/surface", tok, `{"run_id":"r-1"}`); code != 200 || !strings.Contains(b, `"openapi"`) {
		t.Fatalf("%d %s", code, b)
	}
	if code, _ := call(t, "POST", s.URL+"/surface", tok, `{"run_id":""}`); code != 400 {
		t.Fatal("run vacio")
	}
}

func TestAPIMetricsHealthNoTokenLeak(t *testing.T) {
	s, _ := srv(wm.WarmState{WarmID: "w", State: "dirty", BaselineVersion: "b"})
	defer s.Close()
	call(t, "GET", s.URL+"/warm", tok, "")
	code, m := call(t, "GET", s.URL+"/metrics", "", "")
	if code != 200 || !strings.Contains(m, `aqs_warm_state{state="dirty"} 1`) || !strings.Contains(m, "aqs_deploy_attempts_total") || !strings.Contains(m, "aqs_handoff_total") || strings.Contains(m, tok) {
		t.Fatalf("%d %s", code, m)
	}
	for _, p := range []string{"/healthz", "/readyz"} {
		if code, _ := call(t, "GET", s.URL+p, "", ""); code != 200 {
			t.Errorf("%s %d", p, code)
		}
	}
}

func TestAPIWrongMethodWithoutTokenIs401(t *testing.T) {
	s, _ := srv(wm.WarmState{WarmID: "w", State: "dirty", BaselineVersion: "b"})
	defer s.Close()
	if code, _ := call(t, "GET", s.URL+"/warm/ensure", "", ""); code != 401 {
		t.Fatalf("%d", code)
	}
	if code, _ := call(t, "GET", s.URL+"/warm/ensure", tok, ""); code != 405 {
		t.Fatalf("con token y metodo malo debia ser 405, fue %d", code)
	}
}

func TestAPIEmptyTokenServerRejectsEverything(t *testing.T) {
	m := obs.NewMetrics()
	svc := &wm.Service{State: &fakes.MemState{S: wm.WarmState{WarmID: "w", State: "ready", ResetVerified: true, BaselineVersion: "b"}}}
	s := httptest.NewServer((&api.Server{Svc: svc, Token: "", Log: slog.New(slog.NewJSONHandler(io.Discard, nil)), Registry: m.Reg}).Handler())
	defer s.Close()
	for _, h := range []string{"", "Bearer ", "Bearer", "bearer "} {
		req, _ := http.NewRequest("GET", s.URL+"/warm", nil)
		if h != "" {
			req.Header.Set("Authorization", h)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Errorf("Authorization=%q -> %d", h, resp.StatusCode)
		}
	}
}

func TestAPIBearerSchemeIsCaseInsensitive(t *testing.T) {
	s, _ := srv(wm.WarmState{WarmID: "w", State: "dirty", BaselineVersion: "b"})
	defer s.Close()
	for _, scheme := range []string{"bearer", "BEARER", "Bearer"} {
		req, _ := http.NewRequest("GET", s.URL+"/warm", nil)
		req.Header.Set("Authorization", scheme+" "+tok)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("%s -> %d", scheme, resp.StatusCode)
		}
	}
	if code, _ := call(t, "GET", s.URL+"/warm", tok+"x", ""); code != 401 {
		t.Fatalf("token distinto %d", code)
	}
}

func TestAPIDeployWarmNotReadyIs409AndNoPending(t *testing.T) {
	s, jobs := srv(wm.WarmState{WarmID: "w", State: "dirty", BaselineVersion: "b"})
	defer s.Close()
	code, body := call(t, "POST", s.URL+"/deploys", tok, `{"run_id":"r-1","artifact":{"kind":"published-image","ref":"ghcr.io/a/b:1"}}`)
	if code != 409 || !strings.Contains(body, `"state":"dirty"`) {
		t.Fatalf("%d %s", code, body)
	}
	if code, _ := call(t, "GET", s.URL+"/deploys/r-1", tok, ""); code != 404 || jobs.Count() != 0 {
		t.Fatalf("no debia existir estado pending: %d jobs=%d", code, jobs.Count())
	}
}

func TestAPITraceIDIsSameInLogAndEvent(t *testing.T) {
	m := obs.NewMetrics()
	pub := &fakes.MemPublisher{}
	var logs fakes.SyncBuffer
	svc := &wm.Service{Cfg: wm.Config{Job: wm.JobConfig{AllowedRegistries: wm.DefaultAllowedRegistries}, PollInterval: time.Millisecond},
		State: &fakes.MemState{S: wm.WarmState{WarmID: "w", State: "ready", ResetVerified: true, BaselineVersion: "b"}},
		Probe: &fakes.FakeProbe{}, Jobs: &fakes.FakeJobs{}, Alerts: &fakes.FakeAlerter{}, Pub: pub, Clock: wm.RealClock{}}
	lg := slog.New(slog.NewJSONHandler(&logs, nil))
	svc.Log = lg
	s := httptest.NewServer((&api.Server{Svc: svc, Token: tok, Log: lg, Registry: m.Reg}).Handler())
	defer s.Close()
	if code, _ := call(t, "POST", s.URL+"/deploys", tok, `{"run_id":"r-1","artifact":{"kind":"published-image","ref":"ghcr.io/a/b:1"}}`); code != 202 { // sin X-Trace-Id
		t.Fatal(code)
	}
	for i := 0; i < 100 && len(pub.Snapshot()) == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	evs := pub.Snapshot()
	if len(evs) != 1 {
		t.Fatal("sin evento")
	}
	want := `"trace_id":"` + evs[0].TraceID + `"`
	if !strings.Contains(logs.String(), want) {
		t.Fatalf("el log no lleva el trace_id del evento (%s):\n%s", evs[0].TraceID, logs.String())
	}
}

// El WriteTimeout debe superar la espera maxima de ensure: un ensure desde idle-escalado que tarda
// mas que el timeout fijo antiguo debe poder responder 200 por un servidor HTTP real.
func TestAPIEnsureLongerThanFixedWriteTimeoutCompletesE2E(t *testing.T) {
	m := obs.NewMetrics()
	start := time.Now()
	probe := probeFunc(func() error {
		if time.Since(start) < 1500*time.Millisecond {
			return errors.New("arrancando")
		}
		return nil
	})
	svc := &wm.Service{Cfg: wm.Config{WarmReadyTimeout: 3 * time.Second, PollInterval: 50 * time.Millisecond},
		State: &fakes.MemState{S: wm.WarmState{WarmID: "w", State: "idle-escalado", ResetVerified: true, BaselineVersion: "b"}},
		Probe: probe, Runtime: &fakes.FakeRuntime{}, Pub: &fakes.MemPublisher{}, Clock: wm.RealClock{}}
	srv := &api.Server{Svc: svc, Token: tok, Log: slog.New(slog.NewJSONHandler(io.Discard, nil)), Registry: m.Reg, WriteSlack: 500 * time.Millisecond}
	hs := srv.HTTPServer("127.0.0.1:0")
	if hs.WriteTimeout <= svc.Cfg.WarmReadyTimeout {
		t.Fatalf("WriteTimeout %v <= espera maxima %v", hs.WriteTimeout, svc.Cfg.WarmReadyTimeout)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go hs.Serve(ln)
	defer hs.Close()
	code, body := call(t, "POST", "http://"+ln.Addr().String()+"/warm/ensure", tok, "")
	if code != 200 || !strings.Contains(body, `"state":"ready"`) {
		t.Fatalf("%d %s", code, body)
	}
	if time.Since(start) < 1500*time.Millisecond {
		t.Fatal("la prueba no ejercito una espera larga")
	}
}

func TestAPIDefaultWriteTimeoutExceedsDefaultReadyTimeout(t *testing.T) {
	s := &api.Server{Svc: &wm.Service{}}
	if hs := s.HTTPServer(":0"); hs.WriteTimeout <= wm.DefaultWarmReadyTimeout {
		t.Fatalf("%v", hs.WriteTimeout)
	}
}

type probeFunc func() error

func (f probeFunc) Check(context.Context) error { return f() }

func TestAPIDeployReadyWithoutResetVerifiedIs409(t *testing.T) {
	s, jobs := srv(wm.WarmState{WarmID: "w", State: "ready", ResetVerified: false, BaselineVersion: "b"})
	defer s.Close()
	code, body := call(t, "POST", s.URL+"/deploys", tok, `{"run_id":"r-1","artifact":{"kind":"published-image","ref":"ghcr.io/a/b:1"}}`)
	if code != 409 || !strings.Contains(body, `"reset_verified":false`) || jobs.Count() != 0 {
		t.Fatalf("%d %s jobs=%d", code, body, jobs.Count())
	}
}
