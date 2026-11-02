package main

import (
	"context"
	"encoding/json"
	"fmt"
	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/provision"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/warmtarget"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type registryRepo struct {
	Repo      string   `json:"repo"`
	ImageRepo string   `json:"image_repo"`
	Resources []string `json:"resources"`
}

func syncRepositories(ctx context.Context, client *http.Client, base, token string, rec *provision.Reconciler, svc *wm.Service) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/repositories", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("governance respondió %d", resp.StatusCode)
	}
	var repos []registryRepo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&repos); err != nil {
		return err
	}
	if repos == nil {
		return fmt.Errorf("lista de repositorios nula")
	}
	targets := make([]warmtarget.WarmTarget, 0, len(repos))
	seen := map[string]bool{}
	for _, repo := range repos {
		t := warmtarget.TargetForRepoWithImage(repo.Repo, repo.ImageRepo)
		if err := warmtarget.ValidateResources(repo.Resources); err != nil {
			return fmt.Errorf("recursos inválidos para %s: %w", repo.Repo, err)
		}
		t.Resources = warmtarget.NormalizeResources(repo.Resources)
		if repo.Repo == "" || !warmtarget.ValidWarmNamespace(t.Namespace) || seen[t.Namespace] {
			return fmt.Errorf("repositorio inválido o namespace duplicado: %q", repo.Repo)
		}
		seen[t.Namespace] = true
		targets = append(targets, t)
	}
	if _, err := rec.Reconcile(ctx, targets); err != nil {
		return err
	}
	for _, t := range targets {
		svc.RegisterTarget(t)
		svc.SetRepoImage(t.Repo, t.ImageRepo)
	}
	return nil
}
func pollRepositories(ctx context.Context, base, token string, rec *provision.Reconciler, svc *wm.Service, log *slog.Logger) {
	client := &http.Client{Timeout: 10 * time.Second}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		if err := syncRepositories(ctx, client, base, token, rec, svc); err != nil && ctx.Err() == nil {
			log.Error("reconciliación de repositorios falló", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
