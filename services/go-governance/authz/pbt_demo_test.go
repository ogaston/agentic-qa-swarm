//go:build pbt_demo

package authz_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/gen"
)

// TestPBT_Demo_NamespaceIsAlwaysTest es una propiedad deliberadamente FALSA
// («todo namespace generado es aqs-test»). Solo existe con -tags pbt_demo: sirve
// para mostrar el seed, el contraejemplo reducido (shrinking) y la reproducción
// con -rapid.seed. El `go test ./...` normal no la compila.
func TestPBT_Demo_NamespaceIsAlwaysTest(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		if ns := gen.Namespace().Draw(t, "ns"); ns != gen.TestNS {
			t.Fatalf("namespace %q no es %q", ns, gen.TestNS)
		}
	})
}
