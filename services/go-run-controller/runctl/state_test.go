package runctl

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func openapiStates(t *testing.T) []State {
	t.Helper()
	b, err := os.ReadFile("../../../contracts/openapi/control-plane.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Components struct {
			Schemas struct {
				Run struct {
					Properties struct {
						State struct {
							Enum []string `yaml:"enum"`
						} `yaml:"state"`
					} `yaml:"properties"`
				} `yaml:"Run"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	var out []State
	for _, s := range doc.Components.Schemas.Run.Properties.State.Enum {
		out = append(out, State(s))
	}
	return out
}

func TestStatesMatchOpenAPIEnum(t *testing.T) {
	got := openapiStates(t)
	if len(got) != len(AllStates) {
		t.Fatalf("enum %v vs %v", got, AllStates)
	}
	for i := range got {
		if got[i] != AllStates[i] {
			t.Fatalf("enum difiere en %d: %s vs %s", i, got[i], AllStates[i])
		}
	}
}

// legalPairs es la lista literal de las 19 transiciones legales de la tarea.
var legalPairs = func() map[[2]State]bool {
	m := map[[2]State]bool{}
	for _, p := range [][2]State{{Confirmed, WarmReady}, {WarmReady, Deploying}, {Deploying, Inferring}, {Inferring, Rehearsing},
		{Rehearsing, Running}, {Running, Resetting}, {Resetting, Reporting}, {Reporting, Done},
		{Deploying, Resetting}, {Inferring, Resetting}, {Rehearsing, Resetting}} {
		m[p] = true
	}
	for _, s := range AllStates {
		if !s.Terminal() {
			m[[2]State{s, Failed}] = true
		}
	}
	return m
}()

func TestLegalCount(t *testing.T) {
	if len(legalPairs) != 19 {
		t.Fatalf("tabla de prueba: %d", len(legalPairs))
	}
}

func TestExhaustiveStateByState(t *testing.T) {
	for _, from := range openapiStates(t) {
		for _, to := range openapiStates(t) {
			want := legalPairs[[2]State{from, to}]
			if got := Legal(from, to); got != want {
				t.Errorf("Legal(%s,%s)=%v, esperado %v", from, to, got, want)
			}
		}
	}
}

func TestIllegalTransitionLeavesStateAndSkipsGate(t *testing.T) {
	for _, from := range AllStates {
		for _, to := range AllStates {
			if legalPairs[[2]State{from, to}] {
				continue
			}
			store, gate := NewMemStore(), AllowAll()
			k := newCtl(t, store, gate)
			_ = store.Save(Run{ID: "r", State: from, ConfirmedBy: "u"})
			err := k.Transition(t.Context(), "r", to)
			if err == nil {
				t.Fatalf("%s->%s debía rechazarse", from, to)
			}
			if r, _ := store.Get("r"); r.State != from || len(gate.Calls) != 0 {
				t.Fatalf("%s->%s tocó estado o llamó al gate", from, to)
			}
		}
	}
}
