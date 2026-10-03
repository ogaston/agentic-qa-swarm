package policy_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/gen"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/policy"
)

// El "Parse" de una política es policy.Validate (JSON -> JSON canónico) y el
// "Marshal" es encoding/json sobre la estructura de dominio.

func decodeLike(name string, raw []byte) (any, error) {
	var out any
	switch name {
	case policy.Events:
		out = new(policy.EventsValue)
	case policy.ConfirmRequired:
		out = new(policy.ConfirmRequiredValue)
	case policy.WarmQuotas:
		out = new(policy.WarmQuotasValue)
	default:
		out = new(policy.WorkflowsValue)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, err
	}
	return reflect.ValueOf(out).Elem().Interface(), nil
}

func TestPBT_RoundTrip_Policies(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p := gen.ValidPolicy().Draw(t, "policy")
		raw1, err := json.Marshal(p.Value)
		if err != nil {
			t.Fatal(err)
		}
		raw2, _ := json.Marshal(p.Value)
		if !bytes.Equal(raw1, raw2) {
			t.Fatalf("Marshal inestable: %s vs %s", raw1, raw2)
		}
		norm, err := policy.Validate(p.Name, raw1)
		if err != nil {
			t.Fatalf("política generada válida rechazada: %v (%s)", err, raw1)
		}
		if !bytes.Equal(norm, raw1) {
			t.Fatalf("Validate no es identidad sobre la forma canónica: %s -> %s", raw1, norm)
		}
		again, err := policy.Validate(p.Name, norm)
		if err != nil || !bytes.Equal(again, norm) {
			t.Fatalf("Validate no es idempotente: %s -> %s (%v)", norm, again, err)
		}
		back, err := decodeLike(p.Name, norm)
		if err != nil || !reflect.DeepEqual(back, p.Value) {
			t.Fatalf("round-trip distinto: %#v vs %#v (%v)", back, p.Value, err)
		}
	})
}

func TestPBT_RoundTrip_Workflows(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		v := gen.WorkflowsPolicy().Draw(t, "workflows")
		raw, _ := json.Marshal(v)
		norm, err := policy.Validate(policy.Workflows, raw)
		if err != nil {
			t.Fatalf("rechazada: %v (%s)", err, raw)
		}
		var got policy.WorkflowsValue
		if err := json.Unmarshal(norm, &got); err != nil || !reflect.DeepEqual(got, v) {
			t.Fatalf("round-trip distinto: %#v vs %#v (%v)", got, v, err)
		}
		for i, w := range got.Workflows { // el orden y los nombres (hasta 63 caracteres) se conservan
			if w.Name != v.Workflows[i].Name || len(w.Name) > 63 {
				t.Fatalf("nombre alterado: %q", w.Name)
			}
		}
	})
}

// Límite de longitud del nombre: 63 válido, 64 inválido (ejemplo fijo).
func TestPBT_Examples_WorkflowNameLimit(t *testing.T) {
	mk := func(n int) []byte {
		name := "a" + string(bytes.Repeat([]byte("b"), n-1))
		raw, _ := json.Marshal(policy.WorkflowsValue{Workflows: []policy.WorkflowItem{{Name: name, Complexity: "low", MaxRunsPerDay: 1, TimeoutSeconds: 1}}})
		return raw
	}
	if _, err := policy.Validate(policy.Workflows, mk(63)); err != nil {
		t.Errorf("63 caracteres debe ser válido: %v", err)
	}
	if _, err := policy.Validate(policy.Workflows, mk(64)); !errors.Is(err, policy.ErrInvalid) {
		t.Errorf("64 caracteres debe ser inválido: %v", err)
	}
}

func TestPBT_Validate_RejectsInvalid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p := gen.InvalidPolicyValue().Draw(t, "invalid")
		if norm, err := policy.Validate(p.Name, p.Raw); !errors.Is(err, policy.ErrInvalid) {
			t.Fatalf("valor inválido (%s) aceptado o con otro error: %v -> %s: %s", p.Why, err, norm, p.Raw)
		}
	})
}
