package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestRunFailsClosedOnBadConfig(t *testing.T) {
	cases := map[string]map[string]string{
		"sin config":    {},
		"ns ajeno":      {"WARM_NAMESPACE": "aqs-prod", "WARM_SERVICE_TOKEN": "x", "WARM_OUTBOX_FILE": "/tmp/o"},
		"token vacio":   {"WARM_NAMESPACE": "aqs-test", "WARM_OUTBOX_FILE": "/tmp/o"},
		"sin outbox":    {"WARM_NAMESPACE": "aqs-test", "WARM_SERVICE_TOKEN": "x"},
		"s3 sin config": {"WARM_NAMESPACE": "aqs-test", "WARM_SERVICE_TOKEN": "x", "WARM_OUTBOX_FILE": "/tmp/o", "WARM_KUBE": "fake", "WARM_OBJECT_STORE": "s3"},
		"store raro":    {"WARM_NAMESPACE": "aqs-test", "WARM_SERVICE_TOKEN": "x", "WARM_OUTBOX_FILE": "/tmp/o", "WARM_KUBE": "fake", "WARM_OBJECT_STORE": "gcs"},
	}
	for n, e := range cases {
		t.Run(n, func(t *testing.T) {
			if err := run(env(e), nil); err == nil {
				t.Fatal("debia fallar")
			}
		})
	}
}

func TestSafeSeedIsNeverReady(t *testing.T) {
	s := safeSeed()
	if s.State == "ready" || s.ResetVerified || !s.Valid() {
		t.Fatalf("la semilla debe ser no-lista y valida: %+v", s)
	}
}

func fakeEnv(extra map[string]string) map[string]string {
	m := map[string]string{"WARM_NAMESPACE": "aqs-test", "WARM_SERVICE_TOKEN": "x", "WARM_OUTBOX_FILE": "/tmp/o", "WARM_KUBE": "fake", "WARM_ALLOW_FAKE_KUBE": "true"}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestFakeKubeFences(t *testing.T) {
	if err := checkFakeAllowed(env(fakeEnv(nil))); err != nil {
		t.Fatalf("con permiso y sin cluster debe valer: %v", err)
	}
	cases := map[string]map[string]string{
		"sin permiso":          {"WARM_ALLOW_FAKE_KUBE": ""},
		"permiso distinto":     {"WARM_ALLOW_FAKE_KUBE": "yes"},
		"dentro de un cluster": {"KUBERNETES_SERVICE_HOST": "10.0.0.1"},
		"env prod":             {"WARM_ENV": "prod"},
		"env Production":       {"WARM_ENV": "Production"},
		"env con espacios":     {"WARM_ENV": "  PROD "},
	}
	for n, e := range cases {
		t.Run(n, func(t *testing.T) {
			if err := checkFakeAllowed(env(fakeEnv(e))); err == nil {
				t.Fatal("debia rechazar")
			}
			if err := run(env(fakeEnv(e)), nil); err == nil {
				t.Fatal("run debia rechazar")
			}
		})
	}
	if err := checkFakeAllowed(env(fakeEnv(map[string]string{"WARM_ENV": "dev"}))); err != nil {
		t.Fatal(err)
	}
}

func TestDurationEnv(t *testing.T) {
	for _, bad := range []string{"0", "0s", "-5s", "abc", "10"} {
		if _, err := durationEnv(env(map[string]string{"K": bad}), "K", time.Second); err == nil {
			t.Errorf("%q aceptado", bad)
		}
	}
	if d, err := durationEnv(env(nil), "K", 7*time.Second); err != nil || d != 7*time.Second {
		t.Fatal(d, err)
	}
	if d, err := durationEnv(env(map[string]string{"K": "90s"}), "K", time.Second); err != nil || d != 90*time.Second {
		t.Fatal(d, err)
	}
}

func TestRunRejectsBadTimeouts(t *testing.T) {
	for _, k := range []string{"WARM_READY_TIMEOUT", "WARM_DEPLOY_TIMEOUT", "WARM_POLL_INTERVAL"} {
		for _, v := range []string{"0", "-1s", "x"} {
			if err := run(env(fakeEnv(map[string]string{k: v})), nil); err == nil || !strings.Contains(err.Error(), k) {
				t.Errorf("%s=%s: %v", k, v, err)
			}
		}
	}
}

func TestServeWaitsForInFlightRequestOnShutdown(t *testing.T) {
	started := make(chan struct{})
	var finished atomic.Bool
	hs := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		time.Sleep(700 * time.Millisecond)
		w.Write([]byte("respuesta completa"))
		finished.Store(true)
	})}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, hs, ln, 5*time.Second) }()
	got := make(chan string, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String())
		if err != nil {
			got <- "ERROR " + err.Error()
			return
		}
		b, _ := io.ReadAll(resp.Body)
		got <- string(b)
	}()
	<-started
	cancel() // SIGTERM con la peticion en vuelo
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
		if !finished.Load() { // si serve vuelve antes, run retorna y el proceso sale cortando la respuesta
			t.Fatal("serve volvio con la peticion en vuelo sin terminar")
		}
		select {
		case body := <-got:
			if body != "respuesta completa" {
				t.Fatalf("la respuesta se corto: %s", body)
			}
		case <-time.After(time.Second):
			t.Fatal("serve volvio antes de que la peticion en vuelo terminara")
		}
	case <-time.After(4 * time.Second):
		t.Fatal("serve no volvio")
	}
}

func TestServeGraceExpiredIsAnError(t *testing.T) {
	hs := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(2 * time.Second) })}
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, hs, ln, 100*time.Millisecond) }()
	go http.Get("http://" + ln.Addr().String())
	time.Sleep(200 * time.Millisecond)
	cancel()
	if err := <-done; err == nil {
		t.Fatal("agotar la ventana debe ser un error visible")
	}
}

func TestValidateTimeouts(t *testing.T) {
	ok := func(r, j, p time.Duration) error { return validateTimeouts(r, j, p) }
	if err := ok(2*time.Minute, 11*time.Minute, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	for n, c := range map[string][3]time.Duration{
		"ready enorme":  {999999 * time.Hour, 11 * time.Minute, time.Second},
		"poll 1ns":      {time.Minute, 11 * time.Minute, time.Nanosecond},
		"deploy < poll": {time.Minute, time.Second, 2 * time.Second},
		"deploy enorme": {time.Minute, 2 * time.Hour, time.Second},
	} {
		if ok(c[0], c[1], c[2]) == nil {
			t.Errorf("%s aceptado", n)
		}
	}
}

func TestValidateAppURL(t *testing.T) {
	for _, good := range []string{"http://warm-app.aqs-test.svc", "https://x:8443"} {
		if validateAppURL(good) != nil {
			t.Errorf("%s rechazado", good)
		}
	}
	for _, bad := range []string{"warm-app:8080", "ftp://x", "http://", "http://u:p@x", "//x", ""} {
		if validateAppURL(bad) == nil {
			t.Errorf("%q aceptado", bad)
		}
	}
}

func TestFileStoreFencesAndExplicitStore(t *testing.T) {
	if err := checkFileStoreAllowed(env(map[string]string{"WARM_ALLOW_FILE_STORE": "true"})); err != nil {
		t.Fatal(err)
	}
	for n, e := range map[string]map[string]string{
		"sin permiso": {}, "cluster": {"WARM_ALLOW_FILE_STORE": "true", "KUBERNETES_SERVICE_HOST": "10.0.0.1"},
		"prod": {"WARM_ALLOW_FILE_STORE": "true", "WARM_ENV": " Production "},
	} {
		if checkFileStoreAllowed(env(e)) == nil {
			t.Errorf("%s aceptado", n)
		}
	}
	base := fakeEnv(nil)
	if err := run(env(base), nil); err == nil || !strings.Contains(err.Error(), "obligatorio") || !strings.Contains(err.Error(), "WARM_OBJECT_STORE") {
		t.Fatalf("sin WARM_OBJECT_STORE debe fallar: %v", err)
	}
	e := fakeEnv(map[string]string{"WARM_OBJECT_STORE": "file"})
	if err := run(env(e), nil); err == nil || !strings.Contains(err.Error(), "WARM_ALLOW_FILE_STORE") {
		t.Fatalf("file sin permiso: %v", err)
	}
	e = fakeEnv(map[string]string{"WARM_OBJECT_STORE": "file", "WARM_ALLOW_FILE_STORE": "true", "WARM_APP_URL": "warm-app:8080"})
	if err := run(env(e), nil); err == nil || !strings.Contains(err.Error(), "WARM_APP_URL") {
		t.Fatalf("WARM_APP_URL invalida: %v", err)
	}
}
