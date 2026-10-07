package api_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/internal/api"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/internal/obs"
)

const tok = "s3cret-token"

func srv(st wm.WarmState) (*httptest.Server, *wm.FakeJobs) {
	m := obs.NewMetrics()
	jobs := &wm.FakeJobs{}
	svc := &wm.Service{Cfg: wm.Config{Job: wm.JobConfig{AllowedRegistries: wm.DefaultAllowedRegistries}, PollInterval: time.Millisecond},
		State: &wm.MemState{S: st}, Probe: &wm.FakeProbe{}, Runtime: &wm.FakeRuntime{}, Jobs: jobs,
		Surface: &wm.FakeProber{Base: "http://x", Routes: map[string]wm.FakeResponse{"/openapi.json": {Status: 200, Body: `{"paths":{"/a":{"get":{}}}}`}}},
		Objects: &wm.MemObjects{}, Alerts: &wm.FakeAlerter{}, Pub: &wm.MemPublisher{}, Clock: wm.RealClock{}, Observer: m}
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
