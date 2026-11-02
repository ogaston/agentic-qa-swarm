package repos_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/repos"
)

func TestProjectResourceValidationAndDefaults(t *testing.T) {
	both := []string{repos.ResourcePostgreSQL, repos.ResourceRedis}
	if got := repos.NormalizeResources(nil); !reflect.DeepEqual(got, both) {
		t.Fatalf("recursos omitidos = %v, esperado %v", got, both)
	}
	if got := repos.NormalizeResources([]string{}); len(got) != 0 || got == nil {
		t.Fatalf("lista vacía explícita debe conservarse: %#v", got)
	}
	if got := repos.NormalizeResources([]string{repos.ResourceRedis, repos.ResourcePostgreSQL}); !reflect.DeepEqual(got, both) {
		t.Fatalf("orden canónico = %v", got)
	}
	for _, input := range [][]string{{"mysql"}, {repos.ResourceRedis, repos.ResourceRedis}} {
		if err := repos.ValidateProjectDetails("Proyecto", "", input); !errors.Is(err, repos.ErrInvalid) {
			t.Fatalf("ValidateProjectDetails(%v) = %v", input, err)
		}
	}
	if err := repos.ValidateProjectDetails("Proyecto", "", []string{}); err != nil {
		t.Fatalf("proyecto stateless rechazado: %v", err)
	}
}

func TestNormalizeLegacyRepository(t *testing.T) {
	legacy := repos.NormalizeRepository(repos.Repository{Repo: "ACME/SHOP"})
	if legacy.Name != "acme/shop" || legacy.WebhookSecretVersion != 0 {
		t.Fatalf("legacy normalizado incorrectamente: %+v", legacy)
	}
	if !reflect.DeepEqual(legacy.Resources, []string{repos.ResourcePostgreSQL, repos.ResourceRedis}) {
		t.Fatalf("recursos legacy = %v", legacy.Resources)
	}
}
