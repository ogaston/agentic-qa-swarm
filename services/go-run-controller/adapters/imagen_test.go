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
