package service_test

import (
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/gen"
)

// TestMain fija y registra el seed de rapid (PBT-08).
func TestMain(m *testing.M) { gen.Main(m) }
