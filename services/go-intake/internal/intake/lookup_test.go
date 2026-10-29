package intake_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/intake"
)

type mockHTTPClient struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.doFunc(req)
}

func TestStaticRepoLookup(t *testing.T) {
	ctx := context.Background()

	// Vacío -> fail-closed
	empty := intake.NewStaticRepoLookup(nil)
	if empty.Allowed(ctx, "acme/shop") {
		t.Error("lookup vacío debe denegar")
	}

	// Semilla con mayúsculas y espacios
	l := intake.NewStaticRepoLookup([]string{" Acme/Shop ", "my-org/app"})
	if !l.Allowed(ctx, "acme/shop") {
		t.Error("debe permitir acme/shop")
	}
	if !l.Allowed(ctx, "ACME/SHOP") {
		t.Error("debe ser insensible a mayúsculas")
	}
	if !l.Allowed(ctx, "my-org/app") {
		t.Error("debe permitir my-org/app")
	}
	if l.Allowed(ctx, "other/repo") {
		t.Error("no debe permitir other/repo")
	}
	if l.Allowed(ctx, "") {
		t.Error("no debe permitir repo vacío")
	}
}

func TestRegistryRepoLookup(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	callCount := 0
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			callCount++
			if req.Header.Get("Authorization") != "Bearer test-token" {
				return &http.Response{
					StatusCode: http.StatusUnauthorized,
					Body:       io.NopCloser(bytes.NewBufferString(`{"code":"unauthorized"}`)),
				}, nil
			}
			body := `[
				{"repo":"acme/shop","image_repo":"ghcr.io/acme/shop","enabled":true},
				{"repo":"acme/disabled","image_repo":"ghcr.io/acme/disabled","enabled":false}
			]`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(body)),
			}, nil
		},
	}

	cfg := intake.RegistryLookupConfig{
		GovernanceURL: "http://governance.test",
		ServiceToken:  "test-token",
		Client:        mockClient,
		Seed:          []string{"seed/legacy"},
		CacheTTL:      15 * time.Second,
		LkgTTL:        5 * time.Minute,
		Now:           clock,
	}

	lookup := intake.NewRegistryRepoLookup(cfg)

	// 1. Primera consulta: debe llamar a governance
	if !lookup.Allowed(ctx, "acme/shop") {
		t.Error("acme/shop debe ser permitido")
	}
	if callCount != 1 {
		t.Fatalf("callCount = %d, esperado 1", callCount)
	}

	// Repo deshabilitado debe ser denegado
	if lookup.Allowed(ctx, "acme/disabled") {
		t.Error("acme/disabled debe ser denegado")
	}

	// Semilla original debe ser permitida
	if !lookup.Allowed(ctx, "seed/legacy") {
		t.Error("seed/legacy debe ser permitido")
	}

	// 2. Caché fresca (< 15s): no debe hacer segunda llamada
	now = now.Add(10 * time.Second)
	if !lookup.Allowed(ctx, "acme/shop") {
		t.Error("acme/shop debe seguir permitido desde caché")
	}
	if callCount != 1 {
		t.Fatalf("callCount = %d tras 10s, esperado 1 (caché viva)", callCount)
	}

	// 3. Caché vencida (> 15s): debe refrescar
	now = now.Add(10 * time.Second) // total 20s desde la primera llamada
	if !lookup.Allowed(ctx, "acme/shop") {
		t.Error("acme/shop debe seguir permitido tras refresco")
	}
	if callCount != 2 {
		t.Fatalf("callCount = %d tras 20s, esperado 2", callCount)
	}

	// 4. Governance falla (p.ej. error 500) pero dentro del last-known-good (5m)
	mockClient.doFunc = func(req *http.Request) (*http.Response, error) {
		callCount++
		return nil, errors.New("red caída")
	}

	// Avanzamos 20s (caché vencida)
	now = now.Add(20 * time.Second)
	// Debe usar last-known-good
	if !lookup.Allowed(ctx, "acme/shop") {
		t.Error("acme/shop debe ser permitido vía last-known-good durante caída de governance")
	}
	if callCount != 3 {
		t.Fatalf("callCount = %d, esperado 3", callCount)
	}

	// 5. Governance sigue caído y pasa el límite de last-known-good (> 5 minutos)
	now = now.Add(6 * time.Minute)
	// Debe fallar cerrado (fail-closed)
	if lookup.Allowed(ctx, "acme/shop") {
		t.Error("acme/shop debe denegar tras expirar el last-known-good (fail-closed)")
	}
}
