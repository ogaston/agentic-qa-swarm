package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/audit"
)

const tok32 = "0123456789abcdef0123456789abcdef"

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func fakeEnv(extra map[string]string) map[string]string {
	m := map[string]string{"GOVERNANCE_AUTH": "fake", "GOVERNANCE_ALLOW_FAKE_AUTH": "true",
		"GOVERNANCE_FAKE_TOKENS": "a=a1:admin", "GOVERNANCE_SERVICE_TOKEN": tok32, "GOVERNANCE_DATA_DIR": "/x"}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestLoadConfigSecureStartup(t *testing.T) {
	if _, err := loadConfig(envOf(fakeEnv(nil))); err != nil {
		t.Fatalf("configuración válida rechazada: %v", err)
	}
	bad := map[string]map[string]string{
		"sin token de servicio":    {"GOVERNANCE_SERVICE_TOKEN": ""},
		"token corto":              {"GOVERNANCE_SERVICE_TOKEN": "corto"},
		"fake en prod":             {"GOVERNANCE_ENV": "prod"},
		"fake en PROD":             {"GOVERNANCE_ENV": " PROD "},
		"fake en production":       {"GOVERNANCE_ENV": "production"},
		"fake sin permiso":         {"GOVERNANCE_ALLOW_FAKE_AUTH": ""},
		"fake permiso false":       {"GOVERNANCE_ALLOW_FAKE_AUTH": "false"},
		"fake sin tokens":          {"GOVERNANCE_FAKE_TOKENS": ""},
		"token persona = servicio": {"GOVERNANCE_FAKE_TOKENS": tok32 + "=a1:admin"},
		"sin auth":                 {"GOVERNANCE_AUTH": ""},
		"auth desconocida":         {"GOVERNANCE_AUTH": "none"},
		"identity sin url":         {"GOVERNANCE_AUTH": "identity"},
		"sin data dir":             {"GOVERNANCE_DATA_DIR": ""},
		"namespace prod":           {"GOVERNANCE_TEST_NAMESPACE": "prod"},
		"namespace staging":        {"GOVERNANCE_TEST_NAMESPACE": "Staging"},
	}
	for name, extra := range bad {
		if _, err := loadConfig(envOf(fakeEnv(extra))); err == nil {
			t.Errorf("%s: debía no arrancar", name)
		}
	}
	c, err := loadConfig(envOf(fakeEnv(map[string]string{"GOVERNANCE_AUTH": "identity", "IDENTITY_URL": "http://identity:8080"})))
	if err != nil || c.testNS != "aqs-test" || c.addr != ":8080" {
		t.Fatalf("identity: %+v %v", c, err)
	}
}

func TestRunRefusesBrokenAuditChain(t *testing.T) {
	dir := t.TempDir()
	l, err := audit.Open(filepath.Join(dir, "audit.jsonl"), nil)
	if err != nil {
		t.Fatal(err)
	}
	l.Append(audit.Entry{Actor: "a", Action: "gate.deny"})
	l.Append(audit.Entry{Actor: "a", Action: "gate.deny"})
	l.Close()
	b, _ := os.ReadFile(filepath.Join(dir, "audit.jsonl"))
	os.WriteFile(filepath.Join(dir, "audit.jsonl"), b[:len(b)-5], 0o600)
	m := fakeEnv(map[string]string{"GOVERNANCE_DATA_DIR": dir, "LISTEN_ADDR": "127.0.0.1:0"})
	err = run(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), envOf(m))
	if err == nil || !strings.Contains(err.Error(), "línea 2") {
		t.Fatalf("el servicio no debe arrancar sobre una cadena rota y debe decir la línea: %v", err)
	}
}

func TestVerifyAuditExitCodes(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "audit.jsonl")
	l, _ := audit.Open(p, nil)
	for i := 0; i < 3; i++ {
		l.Append(audit.Entry{Actor: "a", Action: "gate.deny", RunID: "r"})
	}
	l.Close()
	var out, errw bytes.Buffer
	if rc := verifyAudit(p, &out, &errw); rc != 0 || !strings.Contains(out.String(), "3 entradas") {
		t.Fatalf("rc=%d %s %s", rc, out.String(), errw.String())
	}
	b, _ := os.ReadFile(p)
	os.WriteFile(p, []byte(strings.Replace(string(b), "gate.deny", "gate.allow", 1)), 0o600)
	out.Reset()
	errw.Reset()
	if rc := verifyAudit(p, &out, &errw); rc != 1 || !strings.Contains(errw.String(), "línea 1") {
		t.Fatalf("rc=%d %s", rc, errw.String())
	}
	if rc := verifyAudit(filepath.Join(dir, "nada"), &out, &errw); rc != 2 {
		t.Fatalf("archivo inexistente rc=%d", rc)
	}
}
