package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/ui-api/inbox"
)

type govFake struct {
	srv     *httptest.Server
	mu      sync.Mutex
	calls   int
	methods []string
	paths   []string
	headers []http.Header
	bodies  []string
}

func newGovFake(t *testing.T, handler http.HandlerFunc) *govFake {
	t.Helper()
	f := &govFake{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.calls++
		f.methods = append(f.methods, r.Method)
		f.paths = append(f.paths, r.URL.Path)
		f.headers = append(f.headers, r.Header.Clone())
		b, _ := io.ReadAll(r.Body)
		f.bodies = append(f.bodies, string(b))
		f.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *govFake) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestRepositoriesProxyGetAndPost(t *testing.T) {
	reposJSON := `[{"repo":"acme/shop","image_repo":"ghcr.io/acme/shop","enabled":true,"owner":"u1","warm":{"namespace":"aqs-warm-acme-shop","warm_id":"w1","state":"ready"}}]`
	createdJSON := `{"repo":"acme/shop","name":"Shop","resources":["postgresql","redis"],"webhook_secret_version":1,"image_repo":"ghcr.io/acme/shop","enabled":true,"owner":"u1","warm":{"namespace":"aqs-warm-acme-shop","warm_id":"w1","state":"provisioning"}}`

	f := newGovFake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secretTok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/repositories" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(reposJSON))
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/repositories" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(createdJSON))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	warm := newWarmFake(t, respond(http.StatusOK, `{"warm_id":"w1","state":"ready","reset_verified":true,"baseline_version":"b1"}`))
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte(warmSvcMarker), 0600); err != nil {
		t.Fatal(err)
	}
	e := newEnv(t, nil, func(c *Config) {
		c.GovernanceURL = f.srv.URL
		c.WarmURL = warm.srv.URL
		c.WarmTokenFile = tokenFile
	})

	// GET /repositories
	w := e.do(http.MethodGet, "/repositories", secretTok, "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /repositories: status=%d cuerpo=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "acme/shop") {
		t.Fatalf("GET /repositories: respuesta inesperada %s", w.Body.String())
	}

	// POST /repositories
	postBody := `{"repo":"acme/shop","image_repo":"ghcr.io/acme/shop"}`
	w = e.do(http.MethodPost, "/repositories", secretTok, postBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST /repositories: status=%d cuerpo=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "provisioning") {
		t.Fatalf("POST /repositories: respuesta inesperada %s", w.Body.String())
	}
}

func TestRepositoriesPostFailsBeforeCreationWithoutWebhookKey(t *testing.T) {
	f := newGovFake(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	e := newEnv(t, nil, func(c *Config) {
		c.GovernanceURL = f.srv.URL
		c.WebhookKey = nil
	})

	w := e.do(http.MethodPost, "/repositories", secretTok, `{"repo":"acme/shop"}`)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d cuerpo=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "webhook_credentials_unavailable") {
		t.Fatalf("cuerpo=%s", w.Body.String())
	}
	if got := f.count(); got != 0 {
		t.Fatalf("governance recibió %d POST; la creación debía fallar antes", got)
	}
}

func TestRepositoryItemProxyGetPatchDelete(t *testing.T) {
	repoJSON := `{"repo":"acme/shop","image_repo":"ghcr.io/acme/shop","enabled":true,"owner":"u1","warm":{"namespace":"aqs-warm-acme-shop","warm_id":"w1","state":"ready"}}`

	f := newGovFake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secretTok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(repoJSON))
		case http.MethodPatch:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(repoJSON))
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	warm := newWarmFake(t, respond(http.StatusOK, `{"warm_id":"w1","state":"ready","reset_verified":true,"baseline_version":"b1"}`))
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte(warmSvcMarker), 0600); err != nil {
		t.Fatal(err)
	}
	e := newEnv(t, nil, func(c *Config) {
		c.GovernanceURL = f.srv.URL
		c.WarmURL = warm.srv.URL
		c.WarmTokenFile = tokenFile
	})

	// GET /repositories/acme/shop
	w := e.do(http.MethodGet, "/repositories/acme/shop", secretTok, "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /repositories/acme/shop: status=%d cuerpo=%s", w.Code, w.Body.String())
	}

	// PATCH /repositories/acme/shop
	patchBody := `{"enabled":false}`
	w = e.do(http.MethodPatch, "/repositories/acme/shop", secretTok, patchBody, "Content-Type", "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH /repositories/acme/shop: status=%d cuerpo=%s", w.Code, w.Body.String())
	}

	// DELETE /repositories/acme/shop
	w = e.do(http.MethodDelete, "/repositories/acme/shop", secretTok, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("DELETE /repositories/acme/shop: status=%d cuerpo=%s", w.Code, w.Body.String())
	}
}

func TestRepositoriesSinGovernanceURLDa503(t *testing.T) {
	e := newEnv(t, nil, nil) // sin GovernanceURL
	w := e.do(http.MethodGet, "/repositories", secretTok, "")
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), GovernanceUnavailable) {
		t.Fatalf("sin GovernanceURL: code=%d cuerpo=%s", w.Code, w.Body.String())
	}
	if w.Header().Get("Retry-After") != "10" {
		t.Fatalf("Retry-After esperado 10, obtuve %q", w.Header().Get("Retry-After"))
	}
}

func TestRepositoriesGovernanceDownDa503ConRetryAfter(t *testing.T) {
	// Fake que aborta la conexión
	f := newGovFake(t, func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	})
	e := newEnv(t, nil, func(c *Config) {
		c.GovernanceURL = f.srv.URL
	})

	w := e.do(http.MethodGet, "/repositories", secretTok, "")
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), GovernanceUnavailable) {
		t.Fatalf("caído: code=%d cuerpo=%s", w.Code, w.Body.String())
	}
	if w.Header().Get("Retry-After") != "10" {
		t.Fatalf("Retry-After esperado 10, obtuve %q", w.Header().Get("Retry-After"))
	}
}

func TestRepositoriesCORSAllowsPatchAndDelete(t *testing.T) {
	e := newEnv(t, nil, nil)
	w := e.do(http.MethodOptions, "/repositories/acme/shop", "", "", "Origin", "https://app.example")
	if w.Code != http.StatusNoContent {
		t.Fatalf("preflight OPTIONS: %d", w.Code)
	}
	methods := w.Header().Get("Access-Control-Allow-Methods")
	if !strings.Contains(methods, "PATCH") || !strings.Contains(methods, "DELETE") {
		t.Fatalf("Access-Control-Allow-Methods debe incluir PATCH y DELETE, obtuve: %q", methods)
	}
}

func TestRepositoriesSinAutenticacionDa401(t *testing.T) {
	f := newGovFake(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	e := newEnv(t, nil, func(c *Config) {
		c.GovernanceURL = f.srv.URL
	})
	w := e.do(http.MethodGet, "/repositories", "", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("sin auth: %d", w.Code)
	}
	if f.count() != 0 {
		t.Fatalf("no debió llamar a gobernanza sin autenticación: calls=%d", f.count())
	}
}

func TestRepositoriesNombreInvalidoDa400(t *testing.T) {
	e := newEnv(t, nil, func(c *Config) {
		c.GovernanceURL = "http://example.invalid"
	})
	// Nombre con caracteres inválidos
	w := e.do(http.MethodGet, "/repositories/Invalid_Owner/repo", secretTok, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("repo inválido: %d", w.Code)
	}
}

func TestRepositoriesActivity(t *testing.T) {
	e := newEnv(t, nil, nil)
	// En newEnv ya se creó la notificación n-1 para acme/shop
	// 1. Repo con actividad (acme/shop)
	w := e.do(http.MethodGet, "/repositories/acme/shop/activity", secretTok, "")
	if w.Code != http.StatusOK {
		t.Fatalf("activity acme/shop: %d %s", w.Code, w.Body.String())
	}
	var act struct {
		Repo       string  `json:"repo"`
		HasWebhook bool    `json:"has_webhook"`
		LastEvent  *string `json:"last_event"`
		LastSHA    *string `json:"last_sha"`
		LastSeenAt *string `json:"last_seen_at"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &act); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	if act.Repo != "acme/shop" || !act.HasWebhook || act.LastEvent == nil || *act.LastEvent != "commit" || act.LastSHA == nil || *act.LastSHA != sha {
		t.Fatalf("actividad inesperada para acme/shop: %+v", act)
	}

	// 2. Repo sin actividad (otra/app)
	w = e.do(http.MethodGet, "/repositories/otra/app/activity", secretTok, "")
	if w.Code != http.StatusOK {
		t.Fatalf("activity otra/app: %d %s", w.Code, w.Body.String())
	}
	var actEmpty struct {
		Repo       string `json:"repo"`
		HasWebhook bool   `json:"has_webhook"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &actEmpty); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	if actEmpty.Repo != "otra/app" || actEmpty.HasWebhook {
		t.Fatalf("actividad inesperada para otra/app: %+v", actEmpty)
	}

	// 3. Repo con actividad de ping (otra/app recibe webhook.validated)
	e.store.ApplyWebhookValidated(inbox.WebhookValidated{
		EventID:    "e-ping-otra",
		Type:       "webhook.validated",
		Version:    1,
		OccurredAt: time.Date(2026, 11, 2, 2, 0, 0, 0, time.UTC),
		TraceID:    "trace-otra",
		Data: inbox.WebhookValidatedData{
			Repo:        "otra/app",
			GithubEvent: "ping",
			DeliveryID:  "deliv-otra",
		},
	})
	w = e.do(http.MethodGet, "/repositories/otra/app/activity", secretTok, "")
	if w.Code != http.StatusOK {
		t.Fatalf("activity otra/app tras ping: %d %s", w.Code, w.Body.String())
	}
	var actPing struct {
		Repo       string  `json:"repo"`
		HasWebhook bool    `json:"has_webhook"`
		LastEvent  *string `json:"last_event"`
		LastSHA    *string `json:"last_sha"`
		LastSeenAt *string `json:"last_seen_at"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &actPing); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	if actPing.Repo != "otra/app" || !actPing.HasWebhook || actPing.LastEvent == nil || *actPing.LastEvent != "ping" || actPing.LastSHA != nil {
		t.Fatalf("actividad inesperada de ping para otra/app: %+v", actPing)
	}
	if strings.Contains(w.Body.String(), "last_sha") {
		t.Fatalf("last_sha no debe aparecer en el JSON cuando no hay SHA: %s", w.Body.String())
	}
}

func TestRoutePatternRepositories(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/repositories", RouteRepositories},
		{"/repositories/acme/shop", RouteRepositoryItem},
		{"/repositories/acme/shop/activity", RouteRepositoryActivity},
		{"/repositories/acme/shop/webhook-secret/rotate", RouteRepositoryRotateSecret},
		{"/repositories/acme/shop/extra/more", "unmatched"},
	}
	for _, tc := range tests {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		if got := RoutePattern(req); got != tc.want {
			t.Errorf("RoutePattern(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestRepositoryWarmOverlay(t *testing.T) {
	gov := newGovFake(t, respond(http.StatusOK, `[{"repo":"acme/shop","warm":{"namespace":"ns-one","warm_id":"w","state":"provisioning"}},{"repo":"acme/other","warm":{"namespace":"ns-two","warm_id":"w","state":"ready"}}]`))
	warm := newWarmFake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+warmSvcMarker {
			t.Error("service token missing")
		}
		state := "ready"
		if r.URL.Query().Get("namespace") == "ns-two" {
			state = "dirty"
		}
		verified := state == "ready"
		_, _ = w.Write([]byte(fmt.Sprintf(`{"warm_id":"w","state":%q,"reset_verified":%t,"baseline_version":"b"}`, state, verified)))
	})
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte(warmSvcMarker), 0600); err != nil {
		t.Fatal(err)
	}
	e := newEnv(t, nil, func(c *Config) { c.GovernanceURL = gov.srv.URL; c.WarmURL = warm.srv.URL; c.WarmTokenFile = tokenFile })
	w := e.do(http.MethodGet, "/repositories?ignored=yes", secretTok, "")
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), warmSvcMarker) {
		t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
	}
	var repos []struct {
		Warm struct {
			State           string `json:"state"`
			ResetVerified   bool   `json:"reset_verified"`
			BaselineVersion string `json:"baseline_version"`
		} `json:"warm"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &repos); err != nil || len(repos) != 2 || repos[0].Warm.State != "ready" || !repos[0].Warm.ResetVerified || repos[0].Warm.BaselineVersion != "b" || repos[1].Warm.State != "dirty" || repos[1].Warm.ResetVerified {
		t.Fatalf("overlay: %s", w.Body.String())
	}
	if warm.query(0).Encode() != "namespace=ns-one" || warm.query(1).Encode() != "namespace=ns-two" {
		t.Fatal("wrong namespace query")
	}
}

func TestRepositoryWarmUnavailableFailsClosed(t *testing.T) {
	gov := newGovFake(t, respond(http.StatusOK, `{"repo":"acme/shop","warm":{"namespace":"ns-one","state":"ready"}}`))
	e := newEnv(t, nil, func(c *Config) { c.GovernanceURL = gov.srv.URL })
	w := e.do(http.MethodGet, "/repositories/acme/shop", secretTok, "")
	if w.Code != http.StatusServiceUnavailable || strings.Contains(w.Body.String(), `"ready"`) {
		t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
	}
}

func TestRepositoriesCreateAndRotateWebhookCredentials(t *testing.T) {
	repoV1JSON := `{"repo":"acme/shop","name":"Acme Shop","resources":["postgresql","redis"],"webhook_secret_version":1,"image_repo":"ghcr.io/acme/shop","enabled":true,"owner":"u1","created_at":"2026-11-02T01:00:00Z","warm":{"namespace":"aqs-w-acme-shop","warm_id":"w-acme-shop","state":"ready","infrastructure_ready":true}}`
	repoV2JSON := `{"repo":"acme/shop","name":"Acme Shop","resources":["postgresql","redis"],"webhook_secret_version":2,"image_repo":"ghcr.io/acme/shop","enabled":true,"owner":"u1","created_at":"2026-11-02T01:00:00Z","warm":{"namespace":"aqs-w-acme-shop","warm_id":"w-acme-shop","state":"ready","infrastructure_ready":true}}`

	var govHandler http.HandlerFunc
	gov := newGovFake(t, func(w http.ResponseWriter, r *http.Request) {
		govHandler(w, r)
	})

	e := newEnv(t, nil, func(c *Config) {
		c.GovernanceURL = gov.srv.URL
		c.WebhookKey = []byte("contract-platform-webhook-key")
	})

	t.Run("POST /repositories entrega secreto derivado", func(t *testing.T) {
		govHandler = respond(http.StatusCreated, repoV1JSON)
		body := `{"repo":"acme/shop","name":"Acme Shop","resources":["postgresql","redis"]}`
		w := e.do(http.MethodPost, "/repositories", secretTok, body)
		if w.Code != http.StatusCreated {
			t.Fatalf("status=%d %s", w.Code, w.Body.String())
		}
		if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
			t.Fatalf("Cache-Control=%q, quiero no-store", cc)
		}
		var creds struct {
			Repository struct {
				Repo                 string   `json:"repo"`
				Name                 string   `json:"name"`
				Resources            []string `json:"resources"`
				WebhookSecretVersion int      `json:"webhook_secret_version"`
			} `json:"repository"`
			WebhookSecret string `json:"webhook_secret"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &creds); err != nil {
			t.Fatalf("json invalido: %v", err)
		}
		if creds.Repository.Repo != "acme/shop" || creds.Repository.WebhookSecretVersion != 1 {
			t.Fatalf("repo devuelto inesperado: %+v", creds.Repository)
		}
		// Validar contra golden vector 1
		wantSec := "whsec_ba62d9c1355a81a091d6fc1204063a880cc8c6586dc5c19ae639cd49c0217971"
		if creds.WebhookSecret != wantSec {
			t.Fatalf("webhook_secret=%q, quiero %q", creds.WebhookSecret, wantSec)
		}
	})

	t.Run("POST /repositories/{owner}/{repo}/webhook-secret/rotate rota y entrega version 2", func(t *testing.T) {
		govHandler = respond(http.StatusOK, repoV2JSON)
		w := e.do(http.MethodPost, "/repositories/acme/shop/webhook-secret/rotate", secretTok, "")
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d %s", w.Code, w.Body.String())
		}
		var creds struct {
			Repository struct {
				Repo                 string `json:"repo"`
				WebhookSecretVersion int    `json:"webhook_secret_version"`
			} `json:"repository"`
			WebhookSecret string `json:"webhook_secret"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &creds); err != nil {
			t.Fatalf("json invalido: %v", err)
		}
		if creds.Repository.WebhookSecretVersion != 2 {
			t.Fatalf("version=%d, quiero 2", creds.Repository.WebhookSecretVersion)
		}
		// Validar contra golden vector 2
		wantSec := "whsec_c6c8791253a1dabfcc16a7f331fb4e029ce9f91f43747381ea7dc9d122152f0c"
		if creds.WebhookSecret != wantSec {
			t.Fatalf("webhook_secret=%q, quiero %q", creds.WebhookSecret, wantSec)
		}
	})

	t.Run("rotate con 403 forbidden de governance se reenvia intacto", func(t *testing.T) {
		govHandler = respond(http.StatusForbidden, `{"code":"forbidden","message":"rol o permisos insuficientes"}`)
		w := e.do(http.MethodPost, "/repositories/acme/shop/webhook-secret/rotate", secretTok, "")
		if w.Code != http.StatusForbidden {
			t.Fatalf("status=%d, quiero 403", w.Code)
		}
	})

	t.Run("rotate con 404 no encontrado", func(t *testing.T) {
		govHandler = respond(http.StatusNotFound, `{"code":"not_found","message":"repositorio no encontrado"}`)
		w := e.do(http.MethodPost, "/repositories/acme/inexistente/webhook-secret/rotate", secretTok, "")
		if w.Code != http.StatusNotFound {
			t.Fatalf("status=%d, quiero 404", w.Code)
		}
	})

	t.Run("GET /repositories y GET item no exponen secretos", func(t *testing.T) {
		govHandler = respond(http.StatusOK, fmt.Sprintf(`[%s]`, repoV1JSON))
		wList := e.do(http.MethodGet, "/repositories", secretTok, "")
		if strings.Contains(wList.Body.String(), "whsec_") {
			t.Fatalf("GET /repositories no debe contener whsec_: %s", wList.Body.String())
		}

		govHandler = respond(http.StatusOK, repoV1JSON)
		wItem := e.do(http.MethodGet, "/repositories/acme/shop", secretTok, "")
		if strings.Contains(wItem.Body.String(), "whsec_") {
			t.Fatalf("GET /repositories/{owner}/{repo} no debe contener whsec_: %s", wItem.Body.String())
		}
	})

	t.Run("ausencia de secretos en logs de ui-api", func(t *testing.T) {
		logs := e.logs.String()
		if strings.Contains(logs, "whsec_") || strings.Contains(logs, "contract-platform-webhook-key") {
			t.Fatalf("fuga de secreto en logs de ui-api: %s", logs)
		}
	})
}
