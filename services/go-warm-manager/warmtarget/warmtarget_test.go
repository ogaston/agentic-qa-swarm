package warmtarget_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/warmtarget"
)

type slugCase struct {
	Repo      string `json:"repo"`
	Slug      string `json:"slug"`
	Namespace string `json:"namespace"`
	WarmID    string `json:"warm_id"`
}

func TestSlugsAgainstGoldenVectors(t *testing.T) {
	data, err := os.ReadFile("../../../contracts/examples/slugs.json")
	if err != nil {
		t.Fatalf("leyendo contracts/examples/slugs.json: %v", err)
	}

	var cases []slugCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("parseando slugs.json: %v", err)
	}

	for _, c := range cases {
		t.Run(c.Repo, func(t *testing.T) {
			gotSlug := warmtarget.Slug(c.Repo)
			if gotSlug != c.Slug {
				t.Errorf("Slug(%q) = %q, esperado %q", c.Repo, gotSlug, c.Slug)
			}
			gotNS := warmtarget.NamespaceForRepo(c.Repo)
			if gotNS != c.Namespace {
				t.Errorf("NamespaceForRepo(%q) = %q, esperado %q", c.Repo, gotNS, c.Namespace)
			}
			gotWID := warmtarget.WarmIDForRepo(c.Repo)
			if gotWID != c.WarmID {
				t.Errorf("WarmIDForRepo(%q) = %q, esperado %q", c.Repo, gotWID, c.WarmID)
			}
			if !warmtarget.ValidWarmNamespace(gotNS) {
				t.Errorf("ValidWarmNamespace(%q) debería ser true", gotNS)
			}
		})
	}
}

func TestValidWarmNamespace(t *testing.T) {
	valids := []string{"aqs-test", "aqs-w-acme-shop", "aqs-w-app-1", "aqs-w-a"}
	for _, ns := range valids {
		if !warmtarget.ValidWarmNamespace(ns) {
			t.Errorf("ValidWarmNamespace(%q) debería ser true", ns)
		}
	}

	invalids := []string{"", "default", "kube-system", "aqs-system", "aqs-w-", "aqs-w-App", "aqs-w-a-", "aqs-w-" + string(make([]byte, 50))}
	for _, ns := range invalids {
		if warmtarget.ValidWarmNamespace(ns) {
			t.Errorf("ValidWarmNamespace(%q) debería ser false", ns)
		}
	}
}
