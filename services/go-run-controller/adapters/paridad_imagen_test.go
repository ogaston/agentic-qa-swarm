package adapters

import "testing"

// Paridad (U11-T03): este conjunto de casos es IDÉNTICO en
// services/go-warm-manager/paridad_imagen_test.go y services/go-run-controller/adapters/paridad_imagen_test.go.
// Cada módulo debe decidir igual en cada caso. Si cambias uno, cambia el otro.
var casosParidadImagen = []struct {
	nombre    string
	ref       string
	prefijos  []string
	permitida bool
}{
	{"tag ok", "ghcr.io/ogaston/demo-target-app-qa-swarm:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", []string{"ghcr.io/ogaston/demo-target-app-qa-swarm"}, true},
	{"digest ok", "ghcr.io/ogaston/demo-target-app-qa-swarm@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", []string{"ghcr.io/ogaston/demo-target-app-qa-swarm"}, true},
	{"prefijo con barras", "ghcr.io/ogaston/agentic-qa-swarm/target-app:0.0.0", []string{"ghcr.io/ogaston/demo-target-app-qa-swarm", "ghcr.io/ogaston/agentic-qa-swarm/target-app"}, true},
	{"repo ajeno", "ghcr.io/evil/x:1", []string{"ghcr.io/ogaston/demo-target-app-qa-swarm"}, false},
	{"prefijo sin limite de nombre", "ghcr.io/ogaston/demo-target-app-qa-swarm-evil:1", []string{"ghcr.io/ogaston/demo-target-app-qa-swarm"}, false},
	{"ruta con ..", "ghcr.io/ogaston/demo-target-app-qa-swarm/../x:1", []string{"ghcr.io/ogaston/demo-target-app-qa-swarm"}, false},
	{"owner en mayusculas", "ghcr.io/OGASTON/demo-target-app-qa-swarm:1", []string{"ghcr.io/ogaston/demo-target-app-qa-swarm"}, false},
	{"subruta bajo el prefijo", "ghcr.io/ogaston/demo-target-app-qa-swarm/sub:1", []string{"ghcr.io/ogaston/demo-target-app-qa-swarm"}, false},
	{"tag latest", "ghcr.io/ogaston/demo-target-app-qa-swarm:latest", []string{"ghcr.io/ogaston/demo-target-app-qa-swarm"}, false},
	{"sin tag", "ghcr.io/ogaston/demo-target-app-qa-swarm", []string{"ghcr.io/ogaston/demo-target-app-qa-swarm"}, false},
	{"registro docker.io", "docker.io/ogaston/demo-target-app-qa-swarm:1", []string{"ghcr.io/ogaston/demo-target-app-qa-swarm"}, false},
	{"digest corto", "ghcr.io/ogaston/demo-target-app-qa-swarm@sha256:corto", []string{"ghcr.io/ogaston/demo-target-app-qa-swarm"}, false},
	{"espacio en la ref", "ghcr.io/ogaston/demo-target-app-qa-swarm:1 x", []string{"ghcr.io/ogaston/demo-target-app-qa-swarm"}, false},
	{"lista vacia (fail-closed)", "ghcr.io/ogaston/demo-target-app-qa-swarm:1", nil, false},
}

// TestParidadImagenComoEnWarm: el controlador decide igual que go-warm-manager en cada caso.
func TestParidadImagenComoEnWarm(t *testing.T) {
	for _, c := range casosParidadImagen {
		err := ValidateImage("published-image", c.ref, c.prefijos)
		if (err == nil) != c.permitida {
			t.Errorf("%s (%s): permitida=%v, err=%v", c.nombre, c.ref, c.permitida, err)
		}
	}
}
