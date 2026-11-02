package adapters

import (
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

// U11-T03: un prefijo mal formado impide arrancar; vacío = todo rechazado (fail-closed).
func TestParseImagePrefixesMalFormadoOVacio(t *testing.T) {
	for _, bad := range []string{"ghcr.io", "ghcr.io/ogaston", "ghcr.io/ogaston/*", "GHCR.IO/a/b", "ghcr.io/a/b/"} {
		if _, err := ParseImagePrefixes(bad); err == nil {
			t.Errorf("%q debía ser mal formado", bad)
		}
	}
	got, err := ParseImagePrefixes(" ")
	if err != nil || len(got) != 0 {
		t.Fatalf("vacío: %v %v", got, err)
	}
	if err := ValidateImage("published-image", "ghcr.io/ogaston/a:1", nil); err == nil {
		t.Fatal("lista vacía debe rechazar todo")
	}
}

// build-from-repo no es desplegable desde el controlador.
func TestValidateImageSoloPublishedImage(t *testing.T) {
	if err := ValidateImage("build-from-repo", "acme/shop@"+"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", []string{"acme/shop"}); err == nil {
		t.Fatal("build-from-repo debe rechazarse")
	}
}

// U11-T03 F-05: elemento vacío y segmentos "." / ".." son error de configuración (no se omiten ni se aceptan).
func TestParseImagePrefixesElementoVacioYSegmentosPunto(t *testing.T) {
	for _, bad := range []string{"a,,b", "ghcr.io/o/a,", ",ghcr.io/o/a", "ghcr.io/o/a, ,ghcr.io/o/b", "ghcr.io/o/../x", "ghcr.io/o/a/..", "ghcr.io/o/a/./b"} {
		if _, err := ParseImagePrefixes(bad); err == nil {
			t.Errorf("%q debía ser error de configuración", bad)
		}
	}
	ok, err := ParseImagePrefixes("ghcr.io/o/a,ghcr.io/o/b/c")
	if err != nil || len(ok) != 2 {
		t.Fatalf("lista válida: %v %v", ok, err)
	}
}

func TestAllowedPrefixesForRunWithRepoAndWarm(t *testing.T) {
	seed := []string{"ghcr.io/seed/app"}

	// 1. Con run.Artifact.ImagePrefix
	r1 := runctl.Run{Artifact: &runctl.Artifact{Kind: "published-image", Ref: "ghcr.io/custom/app:1.0", ImagePrefix: "ghcr.io/custom/app"}}
	p1 := AllowedPrefixesForRun(r1, seed, "ghcr.io/custom/app:1.0")
	if err := ValidateImage("published-image", "ghcr.io/custom/app:1.0", p1); err != nil {
		t.Fatalf("debería permitir prefijo del artefacto: %v", err)
	}

	// 2. Con run.Metadata["image_prefix"]
	r2 := runctl.Run{Metadata: map[string]string{"image_prefix": "ghcr.io/meta/app"}}
	p2 := AllowedPrefixesForRun(r2, seed, "ghcr.io/meta/app:2.0")
	if err := ValidateImage("published-image", "ghcr.io/meta/app:2.0", p2); err != nil {
		t.Fatalf("debería permitir prefijo de metadata: %v", err)
	}

	// 3. Con run.Warm.Namespace coincidente con el slug del repo
	r3 := runctl.Run{Warm: &runctl.WarmTarget{WarmID: "w-my-service", Namespace: "aqs-w-my-service"}}
	p3 := AllowedPrefixesForRun(r3, seed, "ghcr.io/my/service:v1")
	if err := ValidateImage("published-image", "ghcr.io/my/service:v1", p3); err != nil {
		t.Fatalf("debería permitir prefijo derivado del warm namespace: %v", err)
	}

	// 4. Imagen ajena no permitida (fail-closed)
	p4 := AllowedPrefixesForRun(r3, seed, "ghcr.io/unrelated/other:v1")
	if err := ValidateImage("published-image", "ghcr.io/unrelated/other:v1", p4); err == nil {
		t.Fatal("imagen ajena debe rechazarse")
	}
}
