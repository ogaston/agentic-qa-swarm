//go:build contract

package main

// Contrato U1<->U4. Solo corre con IDENTITY_URL (go-identity real); sin el, se omite.
//
//	IDENTITY_URL=http://127.0.0.1:18200 IDENTITY_TEST_USER=marta IDENTITY_TEST_PASSWORD=... \
//	  go test -tags contract -run Contract -v ./...
//
// Las pruebas «token expirado» e «identidad caida» NO usan IDENTITY_URL: compilan y arrancan su
// propio go-identity desde ../go-identity (TTL de 1 s; y se detiene a mitad de la prueba). Asi no
// dependen de que el entorno del CI sepa esperar ni matar el servicio compartido.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/ui-api/inbox"
	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/auth"
	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/httpapi"
	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/obs"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

const contracts = "../../../../contracts"

type identityEnv struct{ url, user, pass string }

func envIdentity(t *testing.T) identityEnv {
	t.Helper()
	u := os.Getenv("IDENTITY_URL")
	if u == "" {
		t.Skip("IDENTITY_URL no definido: prueba de contrato omitida")
	}
	return identityEnv{u, os.Getenv("IDENTITY_TEST_USER"), os.Getenv("IDENTITY_TEST_PASSWORD")}
}

func compileSchema(t *testing.T, doc any) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()
	if err := c.AddResource("mem:///s.json", doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("mem:///s.json")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func sessionSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(contracts, "openapi/control-plane.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := yaml.Unmarshal(b, &root); err != nil {
		t.Fatal(err)
	}
	doc := root["components"].(map[string]any)["schemas"].(map[string]any)["Session"]
	if doc == nil {
		t.Fatal("components.schemas.Session no existe")
	}
	return compileSchema(t, doc)
}

func login(t *testing.T, base, user, pass string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": user, "password": pass})
	resp, err := http.Post(base+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != 200 || out.Token == "" {
		t.Fatalf("login %d", resp.StatusCode)
	}
	return out.Token
}

func getSession(t *testing.T, base, token string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest("GET", base+"/auth/session", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// uiAPI arma la cadena real de ui-api (newHandler) con el verificador HTTP contra base.
func uiAPI(t *testing.T, base string) (*httptest.Server, *inbox.Store, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := inbox.OpenStore(filepath.Join(dir, "d"), nil)
	if err != nil {
		t.Fatal(err)
	}
	e := inbox.NotifyCreated{EventID: "e-1", Type: "notify.created", Version: 1, OccurredAt: time.Now(), TraceID: "t"}
	e.Data.NotificationID, e.Data.GithubEvent, e.Data.Repo = "n-1", "commit", "acme/shop"
	e.Data.SHA = strings.Repeat("a", 40)
	e.Data.Artifact = &inbox.Artifact{Kind: "build-from-repo", Ref: "acme/shop@" + e.Data.SHA}
	st.Apply(e)
	v, err := auth.NewHTTPTokenVerifier(base, nil)
	if err != nil {
		t.Fatal(err)
	}
	outbox := filepath.Join(dir, "out.jsonl")
	log := obs.NewLogger(io.Discard, serviceName, slog.LevelInfo)
	h, err := newHandler(log, httpapi.Config{Store: st, Verifier: v, Publisher: inbox.NewPublisher(st, inbox.NewOutbox(outbox), log)},
		filepath.Join(dir, "d"), filepath.Join(dir, "e.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, st, outbox
}

func code(t *testing.T, method, url, token, body string) int {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestContractSessionMatchesOpenAPI(t *testing.T) {
	id := envIdentity(t)
	tok := login(t, id.url, id.user, id.pass)
	status, b := getSession(t, id.url, tok)
	if status != 200 {
		t.Fatalf("status %d", status)
	}
	var inst any
	if err := json.Unmarshal(b, &inst); err != nil {
		t.Fatal(err)
	}
	if err := sessionSchema(t).Validate(inst); err != nil {
		t.Fatalf("la respuesta real de /auth/session no valida contra Session: %v", err)
	}
}

func TestContractUnauthenticatedTokens(t *testing.T) {
	id := envIdentity(t)
	srv, _, _ := uiAPI(t, id.url)
	closed := login(t, id.url, id.user, id.pass)
	req, _ := http.NewRequest("POST", id.url+"/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+closed)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	for name, tok := range map[string]string{"sin token": "", "invalido": "token-invalido", "cerrado por logout": closed} {
		if c := code(t, "GET", srv.URL+"/notifications", tok, ""); c != 401 {
			t.Errorf("%s: %d, quiero 401", name, c)
		}
	}
}

func TestContractRunConfirmedValidAndConfirmedByIsIdentityPrincipal(t *testing.T) {
	id := envIdentity(t)
	srv, st, outbox := uiAPI(t, id.url)
	tok := login(t, id.url, id.user, id.pass)
	req, _ := http.NewRequest("POST", srv.URL+"/notifications/n-1/confirm", strings.NewReader(`{"flows":["checkout","refund"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 201 || st.PublishPendingCount() != 0 {
		t.Fatalf("status %d pendientes %d", resp.StatusCode, st.PublishPendingCount())
	}
	b, err := os.ReadFile(outbox)
	if err != nil || bytes.Count(b, []byte("\n")) != 1 {
		t.Fatalf("outbox: %v %q", err, b)
	}
	var inst any
	if err := json.Unmarshal(bytes.TrimSpace(b), &inst); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(contracts, "events/run.confirmed.schema.json"))
	var sch any
	_ = json.Unmarshal(raw, &sch)
	if err := compileSchema(t, sch).Validate(inst); err != nil {
		t.Fatalf("run.confirmed no valida: %v", err)
	}
	_, sb := getSession(t, id.url, tok)
	var sess struct {
		PrincipalID string `json:"principal_id"`
	}
	_ = json.Unmarshal(sb, &sess)
	got := inst.(map[string]any)["data"].(map[string]any)["confirmed_by"]
	if got != sess.PrincipalID || sess.PrincipalID == "" {
		t.Fatalf("confirmed_by=%v, principal de identidad=%q", got, sess.PrincipalID)
	}
}

// startIdentity compila y arranca un go-identity propio (usuario y TTL de prueba generados aqui).
func startIdentity(t *testing.T, ttl string) (base string, stop func(), user, pass string) {
	t.Helper()
	dir := t.TempDir()
	idDir := filepath.Join("..", "..", "..", "go-identity")
	bin := filepath.Join(dir, "go-identity")
	build := exec.Command("go", "build", "-o", bin, "./cmd/go-identity")
	build.Dir = idDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build go-identity: %v\n%s", err, out)
	}
	user, pass = "contrato", "Cl4ve-de-Contrato-1!"
	hp := exec.Command(bin, "hash-password")
	hp.Stdin = strings.NewReader(pass)
	hash, err := hp.Output()
	if err != nil {
		t.Fatal(err)
	}
	users, _ := json.Marshal([]map[string]string{{"username": user, "password_hash": strings.TrimSpace(string(hash)), "role": "user"}})
	uf := filepath.Join(dir, "users.json")
	if err := os.WriteFile(uf, users, 0o600); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "IDENTITY_USERS_FILE="+uf, "LISTEN_ADDR="+addr, "IDENTITY_SESSION_TTL="+ttl)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	stop = func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }
	t.Cleanup(stop)
	base = "http://" + addr
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for ctx.Err() == nil {
		if resp, err := http.Get(base + "/healthz"); err == nil {
			resp.Body.Close()
			return base, stop, user, pass
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("go-identity propio no arranco")
	return
}

func TestContractExpiredTokenIs401(t *testing.T) {
	envIdentity(t)
	base, _, user, pass := startIdentity(t, "1s")
	srv, _, _ := uiAPI(t, base)
	tok := login(t, base, user, pass)
	if c := code(t, "GET", srv.URL+"/notifications", tok, ""); c != 200 {
		t.Fatalf("token fresco: %d", c)
	}
	time.Sleep(2500 * time.Millisecond)
	if c := code(t, "GET", srv.URL+"/notifications", tok, ""); c != 401 {
		t.Fatalf("token expirado: %d, quiero 401", c)
	}
}

func TestContractIdentityDownIs503(t *testing.T) {
	envIdentity(t)
	base, stop, user, pass := startIdentity(t, "30m")
	srv, _, _ := uiAPI(t, base)
	tok := login(t, base, user, pass)
	if c := code(t, "GET", srv.URL+"/notifications", tok, ""); c != 200 {
		t.Fatalf("con identidad viva: %d", c)
	}
	stop()
	for i := 0; i < 7; i++ { // cruza el umbral del circuito: antes y despues de abrir, siempre 503
		if c := code(t, "GET", srv.URL+"/notifications", tok, ""); c != 503 {
			t.Fatalf("peticion %d con identidad detenida: %d, quiero 503", i, c)
		}
	}
	if c := code(t, "POST", srv.URL+"/notifications/n-1/confirm", tok, `{"flows":["f"]}`); c != 503 {
		t.Fatalf("confirmar con identidad caida: %d", c)
	}
}
