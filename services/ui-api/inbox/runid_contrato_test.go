package inbox

import (
	"regexp"
	"strings"
	"testing"
)

// Contrato cruzado de run_id (U8-T08a). Los validadores de los otros servicios
// viven en paquetes internos o en módulos distintos, así que sus regex se copian
// literalmente aquí. Si cambia un validador, cambia también su copia y este
// test lo delata. La regla: todo id que acuña newRunID debe pasar cada validador.

// formatoRunID es el formato que acuña ui-api: "run-" + 16 hex (64 bits).
var formatoRunID = regexp.MustCompile(`^run-[0-9a-f]{16}$`)

// runIDWarmManager: services/go-warm-manager/types.go:53 (tope 32).
var runIDWarmManager = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$`)

// runIDGoReset: services/go-reset/internal/jobspec/job.go:32 (tope 40 para nombres de Job).
var runIDGoReset = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,38}[a-z0-9])?$`)

// runIDRunController: services/go-run-controller/adapters/phases.go:25.
var runIDRunController = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// idsGenerados devuelve una muestra de ids acuñados por el generador real.
func idsGenerados(t *testing.T, n int) []string {
	t.Helper()
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		id, err := newRunID()
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func TestRunIDContratoFormato(t *testing.T) {
	for _, id := range idsGenerados(t, 200) {
		if len(id) != 20 || !formatoRunID.MatchString(id) {
			t.Fatalf("run_id %q (len %d) no cumple ^run-[0-9a-f]{16}$ de 20 caracteres", id, len(id))
		}
	}
}

// TestRunIDContratoWarmManager: go-warm-manager rechaza > 32 con 400 invalid_run_id.
func TestRunIDContratoWarmManager(t *testing.T) {
	for _, id := range idsGenerados(t, 200) {
		if !runIDWarmManager.MatchString(id) {
			t.Fatalf("go-warm-manager (types.go:53) rechazaría %q", id)
		}
	}
}

// TestRunIDContratoGoReset: go-reset valida el id antes de nombrar el Job.
func TestRunIDContratoGoReset(t *testing.T) {
	for _, id := range idsGenerados(t, 200) {
		if !runIDGoReset.MatchString(id) {
			t.Fatalf("go-reset (job.go:32) rechazaría %q", id)
		}
	}
}

// TestRunIDContratoRunController: go-run-controller valida el id en cada fase.
func TestRunIDContratoRunController(t *testing.T) {
	for _, id := range idsGenerados(t, 200) {
		if !runIDRunController.MatchString(id) {
			t.Fatalf("go-run-controller (phases.go:25) rechazaría %q", id)
		}
	}
}

// Guarda: el id de 36 caracteres que daba 400 no debe volver a pasar el validador
// de go-warm-manager. Documenta por qué el generador se acortó.
func TestRunIDContratoIdLargoRechazado(t *testing.T) {
	viejo := "run-" + strings.Repeat("a", 32)
	if runIDWarmManager.MatchString(viejo) {
		t.Fatalf("el validador de go-warm-manager ya no rechaza %d caracteres; revisar el contrato", len(viejo))
	}
}

// Unicidad con 64 bits: 10 000 ids acuñados por newRunID sin colisión.
func TestRunIDContratoUnicidad(t *testing.T) {
	vistos := make(map[string]bool, 10000)
	for _, id := range idsGenerados(t, 10000) {
		if vistos[id] {
			t.Fatalf("colisión de run_id %q dentro de 10 000 generados", id)
		}
		vistos[id] = true
	}
}
