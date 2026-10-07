package warmmanager_test

import (
	"errors"
	"testing"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
)

func ws(state string, rv bool) wm.WarmState {
	return wm.WarmState{WarmID: "warm-1", State: state, ResetVerified: rv, BaselineVersion: "b1"}
}

func TestTransitionLegal(t *testing.T) {
	cases := []struct {
		name   string
		from   wm.WarmState
		to     string
		rv     bool
		wantRV bool
	}{
		{"ready->dirty", ws("ready", true), "dirty", false, false},
		{"ready->dirty ignora rv", ws("ready", true), "dirty", true, false},
		{"dirty->ready con reset", ws("dirty", false), "ready", true, true},
		{"dirty->cuarentena", ws("dirty", false), "cuarentena", false, false},
		{"cuarentena->ready con accion humana", ws("cuarentena", false), "ready", true, true},
		{"ready->idle", ws("ready", true), "idle-escalado", false, true},
		{"idle->ready", ws("idle-escalado", true), "ready", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := wm.Transition(c.from, c.to, c.rv)
			if err != nil || got.State != c.to || got.ResetVerified != c.wantRV {
				t.Fatalf("got %+v err=%v", got, err)
			}
		})
	}
}

func TestTransitionIllegal(t *testing.T) {
	states := []string{"ready", "dirty", "cuarentena", "idle-escalado"}
	legal := map[[2]string]bool{{"ready", "dirty"}: true, {"dirty", "ready"}: true, {"dirty", "cuarentena"}: true,
		{"cuarentena", "ready"}: true, {"ready", "idle-escalado"}: true, {"idle-escalado", "ready"}: true}
	for _, a := range states {
		for _, b := range states {
			if legal[[2]string{a, b}] {
				continue
			}
			t.Run(a+"->"+b, func(t *testing.T) {
				cur := ws(a, false)
				got, err := wm.Transition(cur, b, true)
				if !errors.Is(err, wm.ErrIllegalTransition) || got != cur {
					t.Fatalf("debia rechazar: %+v %v", got, err)
				}
			})
		}
	}
}

func TestTransitionToReadyNeedsResetVerified(t *testing.T) {
	for _, from := range []string{"dirty", "cuarentena"} {
		if _, err := wm.Transition(ws(from, false), "ready", false); !errors.Is(err, wm.ErrIllegalTransition) {
			t.Fatalf("%s->ready sin reset_verified debia fallar: %v", from, err)
		}
	}
}

func TestValidateArtifact(t *testing.T) {
	reg := []string{"ghcr.io"}
	sha := "acme/shop@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	cases := []struct {
		name string
		a    wm.Artifact
		want error
	}{
		{"imagen ok", wm.Artifact{Kind: "published-image", Ref: "ghcr.io/acme/shop:1.2.3"}, nil},
		{"digest ok", wm.Artifact{Kind: "published-image", Ref: "ghcr.io/acme/shop@sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}, nil},
		{"repo ok", wm.Artifact{Kind: "build-from-repo", Ref: sha}, nil},
		{"latest", wm.Artifact{Kind: "published-image", Ref: "ghcr.io/acme/shop:latest"}, wm.ErrInvalidArtifact},
		{"sin tag", wm.Artifact{Kind: "published-image", Ref: "ghcr.io/acme/shop"}, wm.ErrInvalidArtifact},
		{"registro no permitido", wm.Artifact{Kind: "published-image", Ref: "docker.io/evil/x:1"}, wm.ErrRegistryNotAllowed},
		{"sin registro", wm.Artifact{Kind: "published-image", Ref: "shop:1"}, wm.ErrInvalidArtifact},
		{"kind desconocido", wm.Artifact{Kind: "x", Ref: "a"}, wm.ErrInvalidArtifact},
		{"registro por sufijo evilghcr.io", wm.Artifact{Kind: "published-image", Ref: "evilghcr.io/a/b:1"}, wm.ErrRegistryNotAllowed},
		{"registro con subdominio", wm.Artifact{Kind: "published-image", Ref: "x.ghcr.io/a/b:1"}, wm.ErrRegistryNotAllowed},
		{"registro con puerto", wm.Artifact{Kind: "published-image", Ref: "ghcr.io:5000/a/b:1"}, wm.ErrRegistryNotAllowed},
		{"registro con userinfo", wm.Artifact{Kind: "published-image", Ref: "ghcr.io@evil.com/a/b:1"}, wm.ErrRegistryNotAllowed},
		{"registro con userinfo 2", wm.Artifact{Kind: "published-image", Ref: "evil.com@ghcr.io/a/b:1"}, wm.ErrRegistryNotAllowed},
		{"registro en mayusculas", wm.Artifact{Kind: "published-image", Ref: "GHCR.IO/a/b:1"}, wm.ErrRegistryNotAllowed},
		{"repo sin sha", wm.Artifact{Kind: "build-from-repo", Ref: "acme/shop"}, wm.ErrInvalidArtifact},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := wm.ValidateArtifact(c.a, reg)
			if (c.want == nil) != (err == nil) || (c.want != nil && !errors.Is(err, c.want)) {
				t.Fatalf("err=%v want=%v", err, c.want)
			}
		})
	}
}

func TestValidateRunID(t *testing.T) {
	for _, ok := range []string{"r-1", "a", "run42"} {
		if wm.ValidateRunID(ok) != nil {
			t.Errorf("%q debia valer", ok)
		}
	}
	for _, bad := range []string{"", "R1", "-a", "a-", "a_b", "a/b", "0123456789012345678901234567890123"} {
		if wm.ValidateRunID(bad) == nil {
			t.Errorf("%q debia fallar", bad)
		}
	}
}
