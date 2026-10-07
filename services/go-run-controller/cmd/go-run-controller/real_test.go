package main

import (
	"bytes"
	"strings"
	"testing"
)

func realEnv() map[string]string {
	m := good()
	m["RUN_PHASES"] = "real"
	delete(m, "RUN_ALLOW_FAKE_PHASES")
	m["WARM_URL"], m["RESET_URL"] = "http://warm:8080", "https://reset:8080"
	m["WARM_SERVICE_TOKEN"], m["RESET_SERVICE_TOKEN"] = "w", "r"
	m["RUN_ARTIFACT_REF"] = "ghcr.io/x/app:1.0"
	m["REHEARSAL_IMAGE"] = "ghcr.io/ogaston/aqs-rehearsal:0.1.0"
	m["REHEARSAL_TARGET_URL"] = "http://warm-app.aqs-test.svc:8080"
	return m
}

func TestLoadConfigRealOK(t *testing.T) {
	c, err := loadConfig(envOf(realEnv()))
	if err != nil || !c.real || c.rehearsalSA != "aqs-runner" {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestLoadConfigRealFailsClosed(t *testing.T) {
	for name, mut := range map[string]func(map[string]string){
		"sin WARM_URL":      func(m map[string]string) { delete(m, "WARM_URL") },
		"sin RESET_URL":     func(m map[string]string) { delete(m, "RESET_URL") },
		"WARM_URL ftp":      func(m map[string]string) { m["WARM_URL"] = "ftp://x" },
		"RESET_URL file":    func(m map[string]string) { m["RESET_URL"] = "file:///x" },
		"sin token warm":    func(m map[string]string) { delete(m, "WARM_SERVICE_TOKEN") },
		"sin token reset":   func(m map[string]string) { delete(m, "RESET_SERVICE_TOKEN") },
		"imagen latest":     func(m map[string]string) { m["REHEARSAL_IMAGE"] = "ghcr.io/x/y:latest" },
		"imagen sin tag":    func(m map[string]string) { m["REHEARSAL_IMAGE"] = "ghcr.io/x/y" },
		"sin imagen":        func(m map[string]string) { delete(m, "REHEARSAL_IMAGE") },
		"sin artefacto":     func(m map[string]string) { delete(m, "RUN_ARTIFACT_REF") },
		"destino no http":   func(m map[string]string) { m["REHEARSAL_TARGET_URL"] = "tcp://x" },
		"namespace staging": func(m map[string]string) { m["RUN_TEST_NAMESPACE"] = "staging" },
	} {
		m := realEnv()
		mut(m)
		if _, err := loadConfig(envOf(m)); err == nil {
			t.Errorf("%s: aceptado", name)
		}
	}
}

func TestLoadConfigFakeStillGuarded(t *testing.T) {
	m := good()
	m["RUN_PHASES"], m["RUN_ENV"], m["RUN_ALLOW_FAKE_PHASES"] = "fake", "prod", "true"
	if _, err := loadConfig(envOf(m)); err == nil {
		t.Fatal("fake con RUN_ENV=prod aceptado")
	}
	m["RUN_ENV"] = ""
	delete(m, "RUN_ALLOW_FAKE_PHASES")
	if _, err := loadConfig(envOf(m)); err == nil {
		t.Fatal("fake sin RUN_ALLOW_FAKE_PHASES aceptado")
	}
}

// Ninguna variable de entorno salta el ensayo: la configuración ni la lee.
func TestNoBypassConfigIgnoresSkipVariables(t *testing.T) {
	a, _ := loadConfig(envOf(realEnv()))
	m := realEnv()
	for _, k := range []string{"RUN_SKIP_REHEARSAL", "SKIP_REHEARSAL", "RUN_ENSAYO_PASSED"} {
		m[k] = "true"
	}
	b, err := loadConfig(envOf(m))
	if err != nil || a != b {
		t.Fatalf("la configuración cambió con variables de salto: %v", err)
	}
}

func TestRenderRehearsalJob(t *testing.T) {
	var out bytes.Buffer
	if err := renderRehearsalJob(&out, []string{"--run", "r-1", "--flow", "checkout"}, envOf(nil)); err != nil {
		t.Fatal(err)
	}
	y := out.String()
	for _, w := range []string{"kind: Job", "name: rehearsal-r-1-1", "namespace: aqs-test", "automountServiceAccountToken: false", "readOnlyRootFilesystem: true"} {
		if !strings.Contains(y, w) {
			t.Errorf("falta %q", w)
		}
	}
	for _, bad := range []string{"envFrom", "secretKeyRef", "LLM", "API_KEY"} {
		if strings.Contains(y, bad) {
			t.Errorf("contiene %q", bad)
		}
	}
	for _, args := range [][]string{nil, {"--run", "r-1"}, {"--run", "R_1", "--flow", "f"}, {"--run", "r", "--flow"}, {"--x", "1", "--flow", "f"}} {
		if err := renderRehearsalJob(&out, args, envOf(map[string]string{})); err == nil {
			t.Errorf("args %v aceptados", args)
		}
	}
	if err := renderRehearsalJob(&out, []string{"--run", "r-1", "--flow", "f"}, envOf(map[string]string{"REHEARSAL_IMAGE": "x:latest"})); err == nil {
		t.Error("imagen latest aceptada")
	}
}
