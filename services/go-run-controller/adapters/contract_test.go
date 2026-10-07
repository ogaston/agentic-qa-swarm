//go:build contract

package adapters

import (
	"os"
	"strings"
	"testing"
	"time"
)

func contractEnv(t *testing.T) (warm, wtok, reset, rtok string) {
	t.Helper()
	warm, wtok, reset, rtok = os.Getenv("WARM_URL"), os.Getenv("WARM_SERVICE_TOKEN"), os.Getenv("RESET_URL"), os.Getenv("RESET_SERVICE_TOKEN")
	if warm == "" || wtok == "" || reset == "" || rtok == "" {
		t.Skip("sin WARM_URL/RESET_URL y tokens: prueba de contrato omitida")
	}
	return
}

// Contrato contra go-warm-manager real con kubernetes fake. El fake no tiene el Deployment warm-app,
// así que /warm/ensure responde 409 (sonda caída): el adaptador debe traducirlo a fallo de fase.
func TestContractWarmStateReadable(t *testing.T) {
	warm, wtok, _, _ := contractEnv(t)
	w, err := NewWarmClient(warm, wtok)
	if err != nil {
		t.Fatal(err)
	}
	if f, err := w.ResetVerified(t.Context()); err != nil || f != "true" {
		t.Fatalf("GET /warm: %v %v", f, err)
	}
}

func TestContractWarm409EnsureFailsPhase(t *testing.T) {
	warm, wtok, _, _ := contractEnv(t)
	w, _ := NewWarmClient(warm, wtok)
	if err := w.Ensure(t.Context()); err == nil {
		t.Fatal("/warm/ensure 409 debe ser fallo de la fase")
	}
	if err := w.Deploy(t.Context(), "r-contract-0", "published-image", "ghcr.io/x/app:1.0"); err == nil {
		t.Fatal("deploy con ensure en 409 debe fallar sin crear nada")
	}
}

func TestContractWarmDeployFailedAndSurface409FailPhase(t *testing.T) {
	warm, wtok, _, _ := contractEnv(t)
	w, _ := NewWarmClient(warm, wtok)
	w.MaxWait, w.PollEvery = 5*time.Second, 200*time.Millisecond
	// Se toma el warm con un POST /deploys directo (202); el parche falla (no hay Deployment) y el deploy queda failed.
	if _, err := w.s.call(t.Context(), "POST", "/deploys", map[string]any{"run_id": "r-contract-1",
		"artifact": map[string]string{"kind": "published-image", "ref": "ghcr.io/x/app:1.0"}}, 202); err != nil {
		t.Fatalf("POST /deploys: %v", err)
	}
	if err := w.Deploy(t.Context(), "r-contract-1", "published-image", "ghcr.io/x/app:1.0"); err == nil || !strings.Contains(err.Error(), "deploy falló") {
		t.Fatalf("el deploy failed del servicio real debe reportarse como tal: %v", err)
	}
	if _, err := w.Surface(t.Context(), "r-contract-1"); err == nil {
		t.Fatal("/surface sin deploy terminado debe ser fallo")
	}
}

func TestContractWarmWrongTokenFailsPhase(t *testing.T) {
	warm, _, _, _ := contractEnv(t)
	w, _ := NewWarmClient(warm, "token-incorrecto")
	if err := w.Ensure(t.Context()); err == nil {
		t.Fatal("token incorrecto aceptado")
	}
	if _, err := w.ResetVerified(t.Context()); err == nil {
		t.Fatal("token incorrecto aceptado en GET /warm")
	}
}

func TestContractWarmServiceDownFailsPhase(t *testing.T) {
	_, wtok, _, _ := contractEnv(t)
	w, _ := NewWarmClient("http://127.0.0.1:1", wtok)
	if err := w.Ensure(t.Context()); err == nil {
		t.Fatal("servicio detenido aceptado")
	}
}

func TestContractResetVerified(t *testing.T) {
	_, _, reset, rtok := contractEnv(t)
	r, _ := NewResetClient(reset, rtok)
	if err := r.Reset(t.Context(), "r-contract"); err != nil {
		t.Fatalf("POST /resets: %v", err)
	}
}

func TestContractResetWrongTokenAndDownFailPhase(t *testing.T) {
	_, _, reset, rtok := contractEnv(t)
	r, _ := NewResetClient(reset, "token-incorrecto")
	if err := r.Reset(t.Context(), "r-contract"); err == nil {
		t.Fatal("token incorrecto aceptado")
	}
	r, _ = NewResetClient("http://127.0.0.1:1", rtok)
	if err := r.Reset(t.Context(), "r-contract"); err == nil {
		t.Fatal("servicio detenido aceptado")
	}
}
