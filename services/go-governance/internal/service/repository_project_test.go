package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/authz"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/repos"
)

func TestProjectMetadataAndWebhookRotation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	created, err := e.svc.CreateRepository(ctx, "alice", repos.RepositoryInput{
		Repo: "acme/shop", Name: "Tienda", Description: "QA de checkout", Resources: []string{repos.ResourceRedis},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "Tienda" || created.WebhookSecretVersion != 1 || !reflect.DeepEqual(created.Resources, []string{repos.ResourceRedis}) {
		t.Fatalf("proyecto creado = %+v", created)
	}
	if _, err := e.svc.RotateWebhookSecret(ctx, "bob", authz.RoleUser, created.Repo); !errors.Is(err, repos.ErrForbidden) {
		t.Fatalf("rotación sin permisos = %v", err)
	}
	rotated, err := e.svc.RotateWebhookSecret(ctx, "alice", authz.RoleUser, created.Repo)
	if err != nil || rotated.WebhookSecretVersion != 2 {
		t.Fatalf("rotación owner = %+v, %v", rotated, err)
	}
	rotated, err = e.svc.RotateWebhookSecret(ctx, "admin", authz.RoleAdmin, created.Repo)
	if err != nil || rotated.WebhookSecretVersion != 3 {
		t.Fatalf("rotación admin = %+v, %v", rotated, err)
	}
}

func TestLegacyRepositoryDefaults(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	legacy := repos.Repository{Repo: "acme/legacy", Owner: "alice", Warm: repos.WarmEnvironment{Namespace: "aqs-w-acme-legacy", WarmID: "w-acme-legacy"}}
	if err := e.svc.repos.Create(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	got, found, err := e.svc.GetRepository(ctx, legacy.Repo)
	if err != nil || !found || got.Name != legacy.Repo || got.WebhookSecretVersion != 0 || len(got.Resources) != 2 {
		t.Fatalf("legacy = %+v, found=%v, err=%v", got, found, err)
	}
}
