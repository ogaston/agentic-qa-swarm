package main

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/audit"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/policy"
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

func TestRetentionMinimumIs90Days(t *testing.T) {
	for _, d := range []string{"0", "1", "30", "89", "-5", "abc", "90.5", " "} {
		if _, err := loadConfig(envOf(fakeEnv(map[string]string{"GOVERNANCE_AUDIT_RETENTION_DAYS": d}))); err == nil {
			t.Errorf("retención %q: el servicio no debe arrancar", d)
		}
	}
	for d, want := range map[string]int{"": 90, "90": 90, "365": 365} {
		c, err := loadConfig(envOf(fakeEnv(map[string]string{"GOVERNANCE_AUDIT_RETENTION_DAYS": d})))
		if err != nil || c.retentionDays != want {
			t.Errorf("retención %q: %d %v", d, c.retentionDays, err)
		}
	}
}

func TestVerifyIntervalConfig(t *testing.T) {
	c, err := loadConfig(envOf(fakeEnv(nil)))
	if err != nil || c.verifyInterval != DefaultVerifyInterval {
		t.Fatalf("por defecto 15 m: %v %v", c.verifyInterval, err)
	}
	for _, bad := range []string{"0", "-1m", "mucho"} {
		if _, err := loadConfig(envOf(fakeEnv(map[string]string{"GOVERNANCE_VERIFY_INTERVAL": bad}))); err == nil {
			t.Errorf("intervalo %q debía rechazarse", bad)
		}
	}
}

func TestReadyChecks(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	if err := dataDirCheck(dir)(ctx); err == nil {
		t.Error("sin audit.jsonl el directorio no cuenta como listo")
	}
	al, err := audit.Open(filepath.Join(dir, "audit.jsonl"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer al.Close()
	if err := dataDirCheck(dir)(ctx); err != nil {
		t.Errorf("directorio escribible: %v", err)
	}
	if err := dataDirCheck(filepath.Join(dir, "no-existe"))(ctx); err == nil {
		t.Error("directorio inexistente")
	}
	ps, err := policy.OpenFileStore(filepath.Join(dir, "policies.jsonl"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close()
	if err := policyCheck(ps)(ctx); err != nil {
		t.Errorf("políticas legibles: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "policies.jsonl"), []byte("basura{\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := policyCheck(ps)(ctx); err == nil {
		t.Error("policies.jsonl corrupto tras el arranque debía fallar el chequeo")
	}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			w.WriteHeader(404)
		}
	}))
	if err := identityCheck(up.URL)(ctx); err != nil {
		t.Errorf("identidad viva: %v", err)
	}
	up.Close()
	if err := identityCheck(up.URL)(ctx); err == nil || strings.Contains(err.Error(), "127.0.0.1") {
		t.Errorf("identidad caída debía fallar sin filtrar la dirección: %v", err)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer bad.Close()
	if err := identityCheck(bad.URL)(ctx); err == nil {
		t.Error("identidad con 500 no está lista")
	}
}
