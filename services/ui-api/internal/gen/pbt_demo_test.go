//go:build pbt_demo

package gen_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/gen"
)

// TestPBT_Demo_Sha40StartsWithA es una propiedad deliberadamente FALSA («todo
// Sha40 generado empieza por a»). Solo existe con -tags pbt_demo: muestra el
// seed, el contraejemplo reducido (shrinking) y la reproducción con
// -rapid.seed. El `go test ./...` normal no la compila.
func TestPBT_Demo_Sha40StartsWithA(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		if s := gen.Sha40().Draw(t, "sha"); s[0] != 'a' {
			t.Fatalf("sha %q no empieza por a", s)
		}
	})
}
