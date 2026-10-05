// Package gen contiene los generadores de dominio de las pruebas basadas en
// propiedades (PBT-07) de ui-api. Es código solo de pruebas: ningún
// binario lo importa. Se duplica en go-intake a propósito (sin go.work ni replace).
package gen

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"
)

// Main es el TestMain de los paquetes con propiedades (PBT-08). Si no se pasó
// -rapid.seed, elige uno, lo imprime como `rapid seed: <n>` y lo fija, de modo
// que `-rapid.seed=<n>` repite la ejecución idéntica. Desactiva los failfiles de
// rapid (no se escriben archivos en el árbol); el shrinking sigue activo.
func Main(m *testing.M) {
	flag.Parse()
	if f := flag.Lookup("rapid.nofailfile"); f != nil {
		_ = f.Value.Set("true")
	}
	if f := flag.Lookup("rapid.seed"); f != nil {
		if n, err := strconv.ParseUint(f.Value.String(), 10, 64); err != nil || n == 0 {
			seed := uint64(time.Now().UnixNano())
			if seed == 0 {
				seed = 1
			}
			_ = f.Value.Set(strconv.FormatUint(seed, 10))
		}
		fmt.Printf("rapid seed: %s\n", f.Value.String())
	}
	os.Exit(m.Run())
}

func setChecks(t *testing.T, want func(cur int) (int, bool)) {
	t.Helper()
	f := flag.Lookup("rapid.checks")
	if f == nil {
		return
	}
	old := f.Value.String()
	cur, err := strconv.Atoi(old)
	if err != nil {
		return
	}
	if n, change := want(cur); change {
		_ = f.Value.Set(strconv.Itoa(n))
		t.Cleanup(func() { _ = f.Value.Set(old) })
	}
}

// AtLeastChecks sube -rapid.checks a n si es menor (propiedades baratas que
// necesitan más sorteos para hallar casos fronterizos).
func AtLeastChecks(t *testing.T, n int) {
	t.Helper()
	setChecks(t, func(cur int) (int, bool) { return n, cur < n })
}

// AtMostChecks baja -rapid.checks a n si es mayor (propiedades caras, p. ej. argon2id).
func AtMostChecks(t *testing.T, n int) {
	t.Helper()
	setChecks(t, func(cur int) (int, bool) { return n, cur > n })
}
