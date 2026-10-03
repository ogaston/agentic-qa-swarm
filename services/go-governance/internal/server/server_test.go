package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/authz"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/audit"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/auth"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/policy"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/service"
)

const svcTok = "s3rv1ce-t0ken-s3rv1ce-t0ken-s3rv1ce-t0ken"

type downVerifier struct{}

func (downVerifier) Verify(context.Context, string) (authz.Principal, error) {
	return authz.Principal{}, fmt.Errorf("%w: caída", auth.ErrUnavailable)
}

type fix struct {
	h    http.Handler
	log  *audit.Log
	logs *bytes.Buffer
}

func newFix(t *testing.T, v auth.TokenVerifier) *fix {
	t.Helper()
	dir := t.TempDir()
	al, err := audit.Open(filepath.Join(dir, "audit.jsonl"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { al.Close() })
	ps, err := policy.OpenFileStore(filepath.Join(dir, "policies.jsonl"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ps.Close() })
	tok, _ := auth.NewServiceToken(svcTok)
	if v == nil {
		v = auth.FakeTokenVerifier{Tokens: map[string]authz.Principal{
			"tok-admin": {ID: "a1", Role: authz.RoleAdmin}, "tok-user": {ID: "u1", Role: authz.RoleUser}}}
	}
	buf := &bytes.Buffer{}
	s := server_New(service.New(service.Config{Policies: ps, Audit: al, TestNamespace: "aqs-test"}), v, tok, buf)
	return &fix{h: s.Handler(), log: al, logs: buf}
}

func server_New(svc *service.Service, v auth.TokenVerifier, tok auth.ServiceToken, buf *bytes.Buffer) *Server {
	return New(Config{Service: svc, Verifier: v, Token: tok, Logger: slog.New(slog.NewJSONHandler(buf, nil))})
}

func (f *fix) do(method, path, token, body string) (int, string) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

const okGate = `{"run_id":"r1","from":"confirmed","to":"warm_ready","target_namespace":"aqs-test","confirmed":"true","reset_verified":"true","workflow_allowed":"true"}`

func TestGateAuthAndErrors(t *testing.T) {
	f := newFix(t, nil)
	for name, tok := range map[string]string{"sin token": "", "persona": "tok-admin", "otro": "x"} {
		if c, _ := f.do("POST", "/gates/authorize", tok, okGate); c != 401 {
			t.Errorf("%s: %d", name, c)
		}
	}
	if c, b := f.do("POST", "/gates/authorize", svcTok, okGate); c != 200 || !strings.Contains(b, `"allow":true`) {
		t.Fatalf("%d %s", c, b)
	}
	for name, body := range map[string]string{"roto": `{rota`, "extra": `{"run_id":"r","x":1}`, "hecho inválido": `{"run_id":"r","confirmed":"maybe"}`,
		"hecho bool": `{"run_id":"r","confirmed":true}`, "doble": okGate + okGate, "vacío": ``} {
		c, b := f.do("POST", "/gates/authorize", svcTok, body)
		if c != 400 || strings.Contains(b, `"allow"`) {
			t.Errorf("%s: %d %s", name, c, b)
		}
	}
	if c, _ := f.do("POST", "/gates/authorize", svcTok, `{"run_id":"r","pad":"`+strings.Repeat("a", MaxBody)+`"}`); c != 413 {
		t.Errorf("cuerpo grande: %d", c)
	}
	if c, b := f.do("POST", "/gates/authorize", svcTok, `{"run_id":"r1","from":"nope","to":"warm_ready"}`); c != 200 || !strings.Contains(b, `"allow":false`) {
		t.Errorf("estado desconocido: %d %s", c, b)
	}
	for _, m := range []string{"GET", "PUT", "DELETE"} {
		if c, _ := f.do(m, "/gates/authorize", svcTok, ""); c != 405 {
			t.Errorf("%s gate: %d", m, c)
		}
	}
}

type brokenAudit struct{ *audit.Log }

func (brokenAudit) Append(audit.Entry) (audit.Entry, error) {
	return audit.Entry{}, errors.New("disco")
}

func TestGateFailsClosedWhenAuditBroken(t *testing.T) {
	dir := t.TempDir()
	al, _ := audit.Open(filepath.Join(dir, "a"), nil)
	defer al.Close()
	ps, _ := policy.OpenFileStore(filepath.Join(dir, "p"), nil)
	defer ps.Close()
	tok, _ := auth.NewServiceToken(svcTok)
	s := server_New(service.New(service.Config{Policies: ps, Audit: brokenAudit{al}, TestNamespace: "aqs-test"}),
		auth.FakeTokenVerifier{}, tok, &bytes.Buffer{})
	req := httptest.NewRequest("POST", "/gates/authorize", strings.NewReader(okGate))
	req.Header.Set("Authorization", "Bearer "+svcTok)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 503 || !strings.Contains(rec.Body.String(), `"allow":false`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestPoliciesRolesAndValidation(t *testing.T) {
	f := newFix(t, nil)
	put := func(name, tok, body string) int { c, _ := f.do("PUT", "/policies/"+name, tok, body); return c }
	ev := `{"value":{"enabled_events":["tag"]}}`
	if put("events", "tok-user", ev) != 403 || put("events", "", ev) != 401 || put("events", "x", ev) != 401 || put("events", "tok-admin", ev) != 200 {
		t.Fatal("roles del PUT")
	}
	for name, c := range map[string][3]string{
		"confirm false": {"confirm_required", `{"value":{"required":false}}`, "422"},
		"sin value":     {"events", `{}`, "422"},
		"value null":    {"events", `{"value":null}`, "422"},
		"campo extra":   {"events", `{"value":{"enabled_events":[]},"x":1}`, "422"},
		"desconocida":   {"inventada", `{"value":{}}`, "404"},
	} {
		if got := fmt.Sprint(put(c[0], "tok-admin", c[1])); got != c[2] {
			t.Errorf("%s: %s", name, got)
		}
	}
	if put("events", "tok-admin", `{"value":{"enabled_events":["`+strings.Repeat("a", MaxBody)+`"]}}`) != 413 {
		t.Error("413")
	}
	if c, b := f.do("GET", "/policies/events", "tok-user", ""); c != 200 || !strings.Contains(b, `"value":{"enabled_events":["tag"]}`) || !strings.Contains(b, `"version":1`) {
		t.Errorf("GET: %d %s", c, b)
	}
	if c, _ := f.do("GET", "/policies/workflows", "tok-user", ""); c != 404 {
		t.Error("sin política: 404")
	}
	if c, _ := f.do("GET", "/policies/events", "", ""); c != 401 {
		t.Error("GET sin token")
	}
	if c, _ := f.do("DELETE", "/policies/events", "tok-admin", ""); c != 405 {
		t.Error("DELETE de política: 405")
	}
}

func TestPolicyVersionsOverHTTP(t *testing.T) {
	f := newFix(t, nil)
	ver := func(v string) string {
		_, b := f.do("PUT", "/policies/events", "tok-admin", `{"value":{"enabled_events":`+v+`}}`)
		var r struct {
			Name    string
			Version int
		}
		json.Unmarshal([]byte(b), &r)
		return fmt.Sprint(r.Name, "@", r.Version)
	}
	if got := strings.Join([]string{ver(`["tag"]`), ver(`["tag"]`), ver(`["commit"]`)}, " "); got != "events@1 events@1 events@2" {
		t.Fatal(got)
	}
}

func TestAuditEndpoint(t *testing.T) {
	f := newFix(t, nil)
	f.do("POST", "/gates/authorize", svcTok, strings.Replace(okGate, `"true","reset`, `"false","reset`, 1))
	f.do("POST", "/gates/authorize", svcTok, okGate)
	f.do("PUT", "/policies/events", "tok-admin", `{"value":{"enabled_events":[]}}`)
	f.do("PUT", "/policies/events", "tok-user", `{"value":{"enabled_events":[]}}`)
	c, b := f.do("GET", "/audit?run=r1", "tok-user", "")
	var es []map[string]string
	json.Unmarshal([]byte(b), &es)
	if c != 200 || len(es) != 2 || es[0]["action"] != "gate.deny" || es[1]["action"] != "gate.allow" {
		t.Fatalf("%d %s", c, b)
	}
	if len(es[0]) != 4 {
		t.Fatalf("solo los campos del contrato: %v", es[0])
	}
	_, b = f.do("GET", "/audit", "tok-admin", "")
	json.Unmarshal([]byte(b), &es)
	var acts []string
	for _, e := range es {
		acts = append(acts, e["action"])
	}
	if got := strings.Join(acts, ","); got != "gate.deny,gate.allow,policy.set:events@v1,policy.rejected" {
		t.Fatalf("acciones: %s", got)
	}
	_, b = f.do("GET", "/audit?limit=1", "tok-user", "")
	if json.Unmarshal([]byte(b), &es); len(es) != 1 || es[0]["action"] != "policy.rejected" {
		t.Fatalf("limit: %s", b)
	}
	for _, q := range []string{"limit=0", "limit=501", "limit=x", "limit=-1"} {
		if c, _ := f.do("GET", "/audit?"+q, "tok-user", ""); c != 400 {
			t.Errorf("%s: %d", q, c)
		}
	}
	if c, _ := f.do("GET", "/audit?limit=500", "tok-user", ""); c != 200 {
		t.Error("limit 500 válido")
	}
	if c, _ := f.do("GET", "/audit", "", ""); c != 401 {
		t.Error("sin token")
	}
	for _, m := range []string{"DELETE", "PUT", "POST", "PATCH"} {
		if c, _ := f.do(m, "/audit", "tok-admin", ""); c != 405 {
			t.Errorf("%s /audit: %d", m, c)
		}
	}
	if len(f.log.Query("", 500)) != 4 {
		t.Error("el log de auditoría debe tener 4 entradas")
	}
}

func TestIdentityUnavailableIs503(t *testing.T) {
	f := newFix(t, downVerifier{})
	for _, r := range [][3]string{{"PUT", "/policies/events", `{"value":{"enabled_events":[]}}`}, {"GET", "/policies/events", ""}, {"GET", "/audit", ""}} {
		if c, b := f.do(r[0], r[1], "tok", r[2]); c != 503 || !strings.Contains(b, "identity_unavailable") {
			t.Errorf("%v: %d %s", r[:2], c, b)
		}
	}
	// el gate no depende de identidad
	if c, _ := f.do("POST", "/gates/authorize", svcTok, okGate); c != 200 {
		t.Error("el gate usa token de servicio, no identidad")
	}
}

// Política de roles aislada del middleware: la función pura decide por sí sola.
func TestPersonPermitsPolicyIsolated(t *testing.T) {
	cases := []struct {
		a    Access
		r    authz.Role
		want bool
	}{
		{AccessAuthenticated, authz.RoleUser, true}, {AccessAuthenticated, authz.RoleAdmin, true},
		{AccessAdmin, authz.RoleUser, false}, {AccessAdmin, authz.RoleAdmin, true},
		{AccessAuthenticated, "root", false}, {AccessAdmin, "", false}, {AccessAdmin, "ADMIN", false},
		{AccessService, authz.RoleAdmin, false}, {0, authz.RoleAdmin, false}, {99, authz.RoleAdmin, false},
	}
	for _, c := range cases {
		if got := PersonPermits(c.a, c.r); got != c.want {
			t.Errorf("%v/%q: %v", c.a, c.r, got)
		}
	}
}

func TestRoutingMisc(t *testing.T) {
	f := newFix(t, nil)
	if c, _ := f.do("GET", "/nada", "tok-admin", ""); c != 404 {
		t.Error("ruta desconocida")
	}
	if c, _ := f.do("GET", "/policies/", "tok-admin", ""); c != 404 {
		t.Error("nombre vacío")
	}
	if c, _ := f.do("GET", "/policies/events/extra", "tok-admin", ""); c != 404 {
		t.Error("subruta")
	}
}

func TestSecretsNeverInLogsOrErrors(t *testing.T) {
	f := newFix(t, downVerifier{})
	var all strings.Builder
	for _, r := range [][3]string{
		{"PUT", "/policies/events", svcTok}, {"GET", "/audit", "tok-secreto-persona"}, {"POST", "/gates/authorize", "tok-secreto-persona"},
		{"POST", "/gates/authorize", svcTok},
	} {
		_, b := f.do(r[0], r[1], r[2], `{rota`)
		all.WriteString(b)
	}
	f2 := newFix(t, nil)
	_, b := f2.do("POST", "/gates/authorize", "tok-secreto-persona", okGate)
	all.WriteString(b)
	all.WriteString(f.logs.String())
	all.WriteString(f2.logs.String())
	for _, e := range f.log.Query("", 500) {
		j, _ := json.Marshal(e)
		all.Write(j)
	}
	for _, secret := range []string{svcTok, "tok-secreto-persona", "tok-admin"} {
		if strings.Contains(all.String(), secret) {
			t.Fatalf("el secreto %q aparece en logs, errores o auditoría", secret)
		}
	}
}
