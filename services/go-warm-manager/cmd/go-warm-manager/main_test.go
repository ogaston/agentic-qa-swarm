package main

import (
	"bytes"
	"strings"
	"testing"
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
