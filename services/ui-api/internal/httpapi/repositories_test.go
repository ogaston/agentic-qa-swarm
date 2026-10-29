package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
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
	createdJSON := `{"repo":"acme/shop","image_repo":"ghcr.io/acme/shop","enabled":true,"owner":"u1","warm":{"namespace":"aqs-warm-acme-shop","warm_id":"w1","state":"provisioning"}}`

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

	e := newEnv(t, nil, func(c *Config) {
		c.GovernanceURL = f.srv.URL
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

	e := newEnv(t, nil, func(c *Config) {
		c.GovernanceURL = f.srv.URL
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
}

func TestRoutePatternRepositories(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/repositories", RouteRepositories},
		{"/repositories/acme/shop", RouteRepositoryItem},
		{"/repositories/acme/shop/activity", RouteRepositoryActivity},
		{"/repositories/acme/shop/extra/more", "unmatched"},
	}
	for _, tc := range tests {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		if got := RoutePattern(req); got != tc.want {
			t.Errorf("RoutePattern(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}
