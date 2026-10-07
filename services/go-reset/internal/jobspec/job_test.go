package jobspec_test

import (
	"strings"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/jobspec"
)

func TestJobHasNoCredentialsAndIsHardened(t *testing.T) {
	j, err := jobspec.Build("r-1", "")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := jobspec.YAML(j)
	y := string(b)
	for _, bad := range []string{"envFrom", "secretKeyRef", "hostNetwork", "latest", "privileged: true"} {
		if strings.Contains(y, bad) {
			t.Fatalf("contiene %s", bad)
		}
	}
	for _, want := range []string{"automountServiceAccountToken: false", "serviceAccountName: aqs-reset-job", "runAsNonRoot: true",
		"readOnlyRootFilesystem: true", "namespace: aqs-test", "name: reset-r-1", "limits:", "requests:"} {
		if !strings.Contains(y, want) {
			t.Fatalf("falta %s", want)
		}
	}
}

func TestJobRejectsBadInput(t *testing.T) {
	for _, run := range []string{"", "R_1", "../x", strings.Repeat("a", 50)} {
		if _, err := jobspec.Build(run, ""); err == nil {
			t.Fatalf("run %q aceptado", run)
		}
	}
	for _, img := range []string{"x/y", "x/y:latest", "x/y:"} {
		if _, err := jobspec.Build("r-1", img); err == nil {
			t.Fatalf("imagen %q aceptada", img)
		}
	}
}
