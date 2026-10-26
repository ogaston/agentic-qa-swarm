package adapters

import "testing"

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
