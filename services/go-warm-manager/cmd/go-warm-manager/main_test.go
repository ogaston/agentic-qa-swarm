package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestRenderDeployJobYAML(t *testing.T) {
	var b bytes.Buffer
	err := renderDeployJob([]string{"--run", "r-1", "--artifact-kind", "published-image", "--artifact-ref", "ghcr.io/ogaston/demo:1.2.3"}, env(nil), &b)
	if err != nil || !strings.Contains(b.String(), "kind: Job") || !strings.Contains(b.String(), "automountServiceAccountToken: false") {
		t.Fatalf("%v\n%s", err, b.String())
	}
}

func TestRenderDeployJobRejects(t *testing.T) {
	for _, ref := range []string{"ghcr.io/ogaston/demo:latest", "docker.io/evil/x:1"} {
		var b bytes.Buffer
		if err := renderDeployJob([]string{"--run", "r-1", "--artifact-kind", "published-image", "--artifact-ref", ref}, env(nil), &b); err == nil || b.Len() != 0 {
			t.Errorf("%s aceptado", ref)
		}
	}
}

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
	for _, k := range []string{"WARM_READY_TIMEOUT", "WARM_JOB_TIMEOUT", "WARM_POLL_INTERVAL"} {
		for _, v := range []string{"0", "-1s", "x"} {
			if err := run(env(fakeEnv(map[string]string{k: v})), nil); err == nil || !strings.Contains(err.Error(), k) {
				t.Errorf("%s=%s: %v", k, v, err)
			}
		}
	}
}
