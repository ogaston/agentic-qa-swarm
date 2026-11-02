package repos_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/repos"
)

type slugCase struct {
	Repo      string `json:"repo"`
	Slug      string `json:"slug"`
	Namespace string `json:"namespace"`
	WarmID    string `json:"warm_id"`
}

func TestSlugsAgainstGoldenVectors(t *testing.T) {
	data, err := os.ReadFile("../../../../contracts/examples/slugs.json")
	if err != nil {
		t.Fatalf("leyendo contracts/examples/slugs.json: %v", err)
	}

	var cases []slugCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("parseando slugs.json: %v", err)
	}

	for _, c := range cases {
		t.Run(c.Repo, func(t *testing.T) {
			gotSlug := repos.Slug(c.Repo)
			if gotSlug != c.Slug {
				t.Errorf("Slug(%q) = %q, esperado %q", c.Repo, gotSlug, c.Slug)
			}
			gotNS := repos.NamespaceForRepo(c.Repo)
			if gotNS != c.Namespace {
				t.Errorf("NamespaceForRepo(%q) = %q, esperado %q", c.Repo, gotNS, c.Namespace)
			}
			gotWID := repos.WarmIDForRepo(c.Repo)
			if gotWID != c.WarmID {
				t.Errorf("WarmIDForRepo(%q) = %q, esperado %q", c.Repo, gotWID, c.WarmID)
			}
		})
	}
}

func TestValidation(t *testing.T) {
	validRepos := []string{"acme/shop", "ogaston/demo-target-app-qa-swarm", "my-org/service.v2", "a/b"}
	for _, r := range validRepos {
		if err := repos.ValidateRepoName(r); err != nil {
			t.Errorf("ValidateRepoName(%q) no debería fallar: %v", r, err)
		}
	}

	invalidRepos := []string{"", "acme", "acme/", "/shop", "Acme/Shop", "acme/shop!", "a//b", "a/b/c"}
	for _, r := range invalidRepos {
		if err := repos.ValidateRepoName(r); err == nil {
			t.Errorf("ValidateRepoName(%q) debería fallar", r)
		}
	}

	if err := repos.ValidateImageRepo("acme/shop", "ghcr.io/acme/shop"); err != nil {
		t.Errorf("ValidateImageRepo igual debería pasar: %v", err)
	}
	if err := repos.ValidateImageRepo("acme/shop", "ghcr.io/acme/shop:v1"); err != nil {
		t.Errorf("ValidateImageRepo prefijo debería pasar: %v", err)
	}
	if err := repos.ValidateImageRepo("acme/shop", "docker.io/acme/shop"); err == nil {
		t.Errorf("ValidateImageRepo con otro registry debería fallar")
	}
	if err := repos.ValidateImageRepo("acme/shop", "ghcr.io/other/repo"); err == nil {
		t.Errorf("ValidateImageRepo con otro repo debería fallar")
	}
}

func TestStoreCRUDAndPersistence(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "repos.json")

	st, err := repos.OpenFileStore(filePath)
	if err != nil {
		t.Fatalf("OpenFileStore: %v", err)
	}
	ctx := context.Background()

	r1 := repos.Repository{
		Repo:      "acme/shop",
		ImageRepo: "ghcr.io/acme/shop",
		Enabled:   true,
		Owner:     "u-alice",
		CreatedAt: time.Now().UTC(),
		Warm: repos.WarmEnvironment{
			Repo:      "acme/shop",
			Namespace: repos.NamespaceForRepo("acme/shop"),
			WarmID:    repos.WarmIDForRepo("acme/shop"),
			State:     "provisioning",
		},
	}

	if err := st.Create(ctx, r1); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Duplicado debe fallar con ErrAlreadyExists
	if err := st.Create(ctx, r1); err != repos.ErrAlreadyExists {
		t.Fatalf("esperado ErrAlreadyExists, obtenido: %v", err)
	}

	// Get
	got, found, err := st.Get(ctx, "acme/shop")
	if err != nil || !found {
		t.Fatalf("Get: found=%v, err=%v", found, err)
	}
	if got.Owner != "u-alice" {
		t.Errorf("Owner = %q, esperado %q", got.Owner, "u-alice")
	}

	// List
	list, err := st.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("List: len=%d, err=%v", len(list), err)
	}

	// Update
	r1.Enabled = false
	if err := st.Update(ctx, r1); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _, _ = st.Get(ctx, "acme/shop")
	if got.Enabled {
		t.Errorf("esperado Enabled=false tras Update")
	}

	// Reabrir archivo para verificar persistencia
	st2, err := repos.OpenFileStore(filePath)
	if err != nil {
		t.Fatalf("OpenFileStore reload: %v", err)
	}
	list2, err := st2.List(ctx)
	if err != nil || len(list2) != 1 || list2[0].Enabled {
		t.Fatalf("persistencia falló: len=%d, enabled=%v", len(list2), list2[0].Enabled)
	}

	// Delete
	deleted, err := st2.Delete(ctx, "acme/shop")
	if err != nil || !deleted {
		t.Fatalf("Delete: deleted=%v, err=%v", deleted, err)
	}

	list3, err := st2.List(ctx)
	if err != nil || len(list3) != 0 {
		t.Fatalf("List tras delete: len=%d", len(list3))
	}
}
