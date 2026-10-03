package server

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/authz"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/audit"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/auth"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/obs"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/policy"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/service"
)

type obsFix struct {
	h     http.Handler
	path  string
	logs  *bytes.Buffer
	count func() int
}

func newObsFix(t *testing.T) *obsFix {
	t.Helper()
	dir := t.TempDir()
	buf := &bytes.Buffer{}
	log := obs.NewLogger(buf, ServiceName, slog.LevelDebug)
	reg := obs.NewRegistry()
	gov := obs.NewGov(reg, 90, log)
	path := filepath.Join(dir, "audit.jsonl")
	al, err := audit.Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { al.Close() })
	al.OnAppend = gov.AuditAppended
	ps, err := policy.OpenFileStore(filepath.Join(dir, "policies.jsonl"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ps.Close() })
	mon := obs.NewChainMonitor(al.VerifyNow, gov, 0, nil, log)
	_ = mon.Verify()
	tok, _ := auth.NewServiceToken(svcTok)
	v := auth.FakeTokenVerifier{Tokens: map[string]authz.Principal{"tok-admin": {ID: "a1", Role: authz.RoleAdmin}}}
	s := New(Config{Service: service.New(service.Config{Policies: ps, Audit: al, TestNamespace: "aqs-test", Observer: gov}),
		Verifier: v, Token: tok, Logger: log, Registry: reg, Ready: []obs.Check{{Name: "audit_chain", Fn: mon.Check}}})
	return &obsFix{h: s.Handler(), path: path, logs: buf, count: func() int { return len(al.Query("", 500)) }}
}

func (f *obsFix) get(path string) (int, string) {
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec.Code, rec.Body.String()
}

func (f *obsFix) gate(body string) {
	req := httptest.NewRequest("POST", "/gates/authorize", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+svcTok)
	f.h.ServeHTTP(httptest.NewRecorder(), req)
}

func TestReadyzAuditChainIntact(t *testing.T) {
	f := newObsFix(t)
	f.gate(okGate)
	code, body := f.get("/readyz")
	if code != 200 || !strings.Contains(body, `"ready"`) || !strings.Contains(body, `"audit_chain":"ok"`) {
		t.Fatalf("cadena íntegra: %d %s", code, body)
	}
	if _, m := f.get("/metrics"); !strings.Contains(m, "aqs_audit_chain_ok 1\n") {
		t.Fatalf("aqs_audit_chain_ok debía ser 1:\n%s", grep(m, "aqs_audit_chain_ok"))
	}
}

func TestReadyzAuditChainBroken(t *testing.T) {
	f := newObsFix(t)
	f.gate(okGate)
	f.gate(strings.Replace(okGate, `"r1"`, `"r2"`, 1))
	b, err := os.ReadFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.path, bytes.Replace(b, []byte(`"gate.allow"`), []byte(`"gate.deny"`), 1), 0o600); err != nil {
		t.Fatal(err)
	}
	code, body := f.get("/readyz")
	if code != 503 || !strings.Contains(body, `"unavailable"`) || !strings.Contains(body, `"audit_chain":"fail"`) {
		t.Fatalf("línea alterada: %d %s", code, body)
	}
	if _, m := f.get("/metrics"); !strings.Contains(m, "aqs_audit_chain_ok 0\n") {
		t.Fatalf("aqs_audit_chain_ok debía ser 0:\n%s", grep(m, "aqs_audit_chain_ok"))
	}
	if !strings.Contains(f.logs.String(), `"message":"audit: cadena rota"`) {
		t.Error("la cadena rota debe quedar en el log")
	}
}

func grep(s, sub string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, sub) {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

func TestOpsEndpointsAreNotAudited(t *testing.T) {
	f := newObsFix(t)
	for _, p := range []string{"/healthz", "/readyz", "/metrics"} {
		if c, _ := f.get(p); c != 200 {
			t.Errorf("%s sin token: %d", p, c)
		}
	}
	if f.count() != 0 {
		t.Fatal("los endpoints operativos no deben generar auditoría")
	}
}

func TestMetricsCountGateDecisionsAndAuditEntries(t *testing.T) {
	f := newObsFix(t)
	f.gate(`{"run_id":"r1","from":"confirmed","to":"warm_ready","target_namespace":"aqs-test","confirmed":"false","reset_verified":"true","workflow_allowed":"true"}`)
	f.gate(`{"run_id":"r2","from":"inferring","to":"rehearsing","target_namespace":"prod","confirmed":"true","workflow_allowed":"true"}`)
	f.gate(okGate)
	_, m := f.get("/metrics")
	for _, want := range []string{
		`aqs_gate_denied_total{reason="not_confirmed"} 1`, `aqs_gate_denied_total{reason="namespace_not_test"} 1`,
		`aqs_gate_decisions_total{decision="allow",to="warm_ready"} 1`, `aqs_audit_entries_total 3`,
		`aqs_audit_retention_days 90`, `aqs_http_requests_total{code="200",method="POST",route="/gates/authorize",service="go-governance"} 3`,
	} {
		if !strings.Contains(m, want+"\n") {
			t.Errorf("falta %q", want)
		}
	}
	if !strings.Contains(f.logs.String(), `"message":"audit"`) || !strings.Contains(f.logs.String(), `"action":"gate.allow"`) {
		t.Error("cada entrada de auditoría debe copiarse al log")
	}
}

func TestMetricsAndLogsHaveNoSecretsNorHighCardinality(t *testing.T) {
	f := newObsFix(t)
	for i := 0; i < 20; i++ {
		f.gate(fmt.Sprintf(`{"run_id":"run-%d","from":"confirmed","to":"warm_ready","target_namespace":"ns-%d"}`, i, i))
		f.get(fmt.Sprintf("/ruta-%d", i))
		req := httptest.NewRequest("PUT", "/policies/events", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer tok-admin")
		f.h.ServeHTTP(httptest.NewRecorder(), req)
	}
	_, m := f.get("/metrics")
	for _, s := range []string{"run-1", "ns-1", "ruta-", svcTok, "tok-admin"} {
		if strings.Contains(m, s) {
			t.Errorf("/metrics contiene %q", s)
		}
	}
	if !strings.Contains(m, `route="unmatched"`) || !strings.Contains(m, `route="/policies/{name}"`) {
		t.Error("las rutas deben salir como patrón")
	}
	if strings.Contains(f.logs.String(), svcTok) || strings.Contains(f.logs.String(), "tok-admin") {
		t.Error("el log contiene un token")
	}
}
