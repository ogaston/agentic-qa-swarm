package reset

import (
	"testing"

	"pgregory.net/rapid"
)

// Prueba mínima: fija rapid como dependencia (go.sum no vacío) hasta que la tarea del servicio añada tipos.
func TestSkeleton(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		if rapid.IntRange(0, 5).Draw(t, "n") > 5 {
			t.Fatal("imposible")
		}
	})
}
