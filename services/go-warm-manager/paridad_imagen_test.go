package warmmanager_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
)

// U11-T03 F-02: fuente ÚNICA de casos de paridad, testdata/imagen_casos.json, copiada byte a byte en
// services/go-run-controller/adapters/testdata/imagen_casos.json. El SHA-256 fijado aquí (y en el controlador)
// obliga a que las dos copias sean idénticas: si cambia una sola, la prueba de su módulo falla.
// Para cambiar un caso: editar AMBAS copias y actualizar la constante en AMBOS tests.
const casosImagenSHA256 = "d71a94d52ed8206cbd7959c152258ff3a537d681ae42fbe4aae2c6050f768888"

type casoImagen struct {
	Nombre    string   `json:"nombre"`
	Kind      string   `json:"kind"`
	Ref       string   `json:"ref"`
	Prefijos  []string `json:"prefijos"`
	Permitida bool     `json:"permitida"`
}

// TestParidadImagenCasosIdenticos: go-warm-manager decide igual que el controlador en cada caso, y la copia de
// casos es la fijada. "Desplegable" = kind published-image y ValidateArtifact sin error.
func TestParidadImagenCasosIdenticos(t *testing.T) {
	raw, err := os.ReadFile("testdata/imagen_casos.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if got := hex.EncodeToString(sum[:]); got != casosImagenSHA256 {
		t.Fatalf("testdata/imagen_casos.json cambió (sha256 %s, fijado %s): actualiza AMBAS copias y la constante", got, casosImagenSHA256)
	}
	var casos []casoImagen
	if err := json.Unmarshal(raw, &casos); err != nil {
		t.Fatal(err)
	}
	for _, c := range casos {
		err := wm.ValidateArtifact(wm.Artifact{Kind: c.Kind, Ref: c.Ref}, []string{"ghcr.io"}, c.Prefijos)
		desplegable := err == nil && c.Kind == wm.KindPublishedImage
		if desplegable != c.Permitida {
			t.Errorf("%s (kind=%q ref=%q): desplegable=%v, esperado %v (err=%v)", c.Nombre, c.Kind, c.Ref, desplegable, c.Permitida, err)
		}
	}
}
