package warmmanager_test

import (
	"context"
	"errors"
	"testing"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/internal/fakes"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/warmtarget"
)

func TestMultiTargetStateAndOperations(t *testing.T) {
	ctx := context.Background()

	target := warmtarget.TargetForRepoWithImage("acme/payment-svc", "ghcr.io/acme/payment-svc")
	if target.Namespace != "aqs-w-acme-payment-svc" {
		t.Fatalf("namespace inesperado: %s", target.Namespace)
	}
	if target.WarmID != "w-acme-payment-svc" {
		t.Fatalf("warm_id inesperado: %s", target.WarmID)
	}

	r := newRig(ws("ready", true))
	r.svc.RegisterTarget(target)

	// Estado inicial para target
	st, err := r.svc.GetWarm(ctx, target)
	if err != nil {
		t.Fatalf("GetWarm con target: %v", err)
	}
	if st.WarmID != target.WarmID || st.Namespace != target.Namespace {
		t.Fatalf("estado target no coincide: %+v", st)
	}

	// Lookup por namespace
	stNS, err := r.svc.GetWarm(ctx, target.Namespace)
	if err != nil {
		t.Fatalf("GetWarm con namespace: %v", err)
	}
	if stNS.WarmID != target.WarmID {
		t.Fatalf("lookup por namespace falló: %+v", stNS)
	}

	// Lookup por repo
	stRepo, err := r.svc.GetWarm(ctx, target.Repo)
	if err != nil {
		t.Fatalf("GetWarm con repo: %v", err)
	}
	if stRepo.WarmID != target.WarmID {
		t.Fatalf("lookup por repo falló: %+v", stRepo)
	}

	// EnsureWarmReady para target
	readySt, ok, err := r.svc.EnsureWarmReady(ctx, "tr-1", target)
	if err != nil || !ok || readySt.State != "ready" {
		t.Fatalf("EnsureWarmReady para target: st=%+v ok=%v err=%v", readySt, ok, err)
	}

	// Image prefix validation:
	// La imagen configurada para el repo ("ghcr.io/acme/payment-svc") debe permitirse.
	artOk := wm.Artifact{Kind: "published-image", Ref: "ghcr.io/acme/payment-svc:1.0.0"}
	dep, _, err := r.svc.StartDeploy(ctx, "r-target-1", artOk, "tr-1", target)
	if err != nil || dep.State != "pending" {
		t.Fatalf("StartDeploy con imagen configurada falló: dep=%+v err=%v", dep, err)
	}

	// Una imagen no permitida para ese repo debe rechazarse (ErrImageNotAllowed)
	artBad := wm.Artifact{Kind: "published-image", Ref: "ghcr.io/evil/payment-svc:1.0.0"}
	_, _, err = r.svc.StartDeploy(ctx, "r-target-bad", artBad, "tr-1", target)
	if !errors.Is(err, wm.ErrImageNotAllowed) {
		t.Fatalf("StartDeploy con imagen ajena debía dar ErrImageNotAllowed, got: %v", err)
	}
}

func TestMultiTargetImageValidationFallback(t *testing.T) {
	ctx := context.Background()
	// Target sin ImageRepo configurado: fallback a Cfg.AllowedImagePrefixes
	target := warmtarget.TargetForRepo("other/service")
	r := newRig(ws("ready", true))
	r.svc.RegisterTarget(target)

	// "ghcr.io/acme/shop" está en AllowedImagePrefixes del rig
	artSeed := wm.Artifact{Kind: "published-image", Ref: "ghcr.io/acme/shop:1.0.0"}
	dep, _, err := r.svc.StartDeploy(ctx, "r-fallback-1", artSeed, "tr-2", target)
	if err != nil || dep.State != "pending" {
		t.Fatalf("StartDeploy fallback a AllowedImagePrefixes falló: dep=%+v err=%v", dep, err)
	}

	// Imagen fuera de seed
	artOther := wm.Artifact{Kind: "published-image", Ref: "ghcr.io/unknown/service:1.0.0"}
	_, _, err = r.svc.StartDeploy(ctx, "r-fallback-bad", artOther, "tr-2", target)
	if !errors.Is(err, wm.ErrImageNotAllowed) {
		t.Fatalf("StartDeploy con imagen desconocida debía fallar con ErrImageNotAllowed, got: %v", err)
	}
}

func TestMultiTargetInferSurface(t *testing.T) {
	ctx := context.Background()
	target := warmtarget.TargetForRepoWithImage("acme/orders", "ghcr.io/acme/orders")

	r := newRig(ws("ready", true))
	r.svc.RegisterTarget(target)

	art := wm.Artifact{Kind: "published-image", Ref: "ghcr.io/acme/orders:1.0.0"}
	// Despliega sobre target
	err := r.svc.Deploy(ctx, "r-orders-1", art, "tr-1", target)
	if err != nil {
		t.Fatalf("Deploy falló: %v", err)
	}

	// Registra rutas para la URL de target
	openAPI := `{"openapi":"3.0.0","paths":{"/orders":{"get":{}}}}`
	r.web.Routes["/openapi.json"] = fakes.FakeResponse{Status: 200, Body: openAPI}
	r.web.Routes[target.AppURL+"/openapi.json"] = fakes.FakeResponse{Status: 200, Body: openAPI}

	sa, err := r.svc.InferSurface(ctx, "r-orders-1", "tr-1", target)
	if err != nil {
		t.Fatalf("InferSurface sobre target: %v", err)
	}
	if sa.BaseURL != target.AppURL {
		t.Fatalf("BaseURL esperado %s, got %s", target.AppURL, sa.BaseURL)
	}
	if len(sa.Endpoints) != 1 || sa.Endpoints[0].Path != "/orders" {
		t.Fatalf("endpoints inesperados: %+v", sa.Endpoints)
	}
}
