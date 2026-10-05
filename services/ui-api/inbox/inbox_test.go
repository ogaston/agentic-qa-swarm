package inbox

import (
	"testing"

	"pgregory.net/rapid"
)

// TestRapidWired fija rapid como framework PBT de ui-api (lo usa U1-T05).
func TestRapidWired(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 100).Draw(t, "n")
		if n < 0 || n > 100 {
			t.Fatalf("fuera de rango: %d", n)
		}
	})
}
