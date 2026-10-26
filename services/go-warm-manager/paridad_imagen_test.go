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

// U11-T03 F-05: el PARSER decide igual que el controlador sobre el valor crudo de WARM_ALLOWED_IMAGE_PREFIXES.
// testdata/imagen_parse_casos.json es copia byte a byte de la del controlador; el SHA-256 fijado obliga a que
// ambas sean iguales.
const parseCasosSHA256 = "dbaaad966c0e814071c5f1ae79a8c20e085acf30064132298445c680bbd7236a"

type casoParse struct {
	Nombre string   `json:"nombre"`
	Crudo  string   `json:"crudo"`
	Error  bool     `json:"error"`
	Lista  []string `json:"lista"`
}

func TestParidadParserCasosIdenticos(t *testing.T) {
	raw, err := os.ReadFile("testdata/imagen_parse_casos.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if got := hex.EncodeToString(sum[:]); got != parseCasosSHA256 {
		t.Fatalf("testdata/imagen_parse_casos.json cambió (sha256 %s, fijado %s): actualiza AMBAS copias y la constante", got, parseCasosSHA256)
	}
	var casos []casoParse
	if err := json.Unmarshal(raw, &casos); err != nil {
		t.Fatal(err)
	}
	for _, c := range casos {
		lista, err := wm.ParseImagePrefixes(c.Crudo)
		if (err != nil) != c.Error {
			t.Errorf("%s (crudo=%q): error=%v, esperado error=%v", c.Nombre, c.Crudo, err, c.Error)
			continue
		}
		if err == nil && !equalStrings(lista, c.Lista) {
			t.Errorf("%s (crudo=%q): lista=%v, esperada %v", c.Nombre, c.Crudo, lista, c.Lista)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
