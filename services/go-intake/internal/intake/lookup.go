package intake

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RepoLookup determina si un repositorio tiene permiso para ser procesado por el intake.
type RepoLookup interface {
	Allowed(ctx context.Context, repo string) bool
}

// StaticRepoLookup es un lookup estático basado en una lista de repositorios permitidos.
type StaticRepoLookup struct {
	allowed map[string]struct{}
}

var _ RepoLookup = (*StaticRepoLookup)(nil)

// NewStaticRepoLookup crea un lookup estático. Fail-closed si repos está vacío.
func NewStaticRepoLookup(repos []string) *StaticRepoLookup {
	m := make(map[string]struct{}, len(repos))
	for _, r := range repos {
		if tr := strings.ToLower(strings.TrimSpace(r)); tr != "" {
			m[tr] = struct{}{}
		}
	}
	return &StaticRepoLookup{allowed: m}
}

// Allowed implementa RepoLookup de forma fail-closed (lista vacía = false).
func (s *StaticRepoLookup) Allowed(_ context.Context, repo string) bool {
	if s == nil || len(s.allowed) == 0 {
		return false
	}
	_, ok := s.allowed[strings.ToLower(strings.TrimSpace(repo))]
	return ok
}

// HTTPDoer es la interfaz para realizar peticiones HTTP (lo cumple *http.Client).
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// RegistryLookupConfig configura el lookup respaldado por go-governance.
type RegistryLookupConfig struct {
	GovernanceURL string
	ServiceToken  string
	Client        HTTPDoer
	Seed          []string
	CacheTTL      time.Duration    // default 15s
	LkgTTL        time.Duration    // default 5m
	Now           func() time.Time // default time.Now
	Log           *slog.Logger
}

// RegistryRepoLookup consulta y cachea los repositorios de go-governance con 15s de caché,
// 5 minutos de last-known-good, y fail-closed si la consulta falla y el last-known-good expiró.
type RegistryRepoLookup struct {
	cfg         RegistryLookupConfig
	mu          sync.RWMutex
	cached      map[string]bool
	lastFetched time.Time
	lastGood    time.Time
}

var _ RepoLookup = (*RegistryRepoLookup)(nil)

// NewRegistryRepoLookup construye un lookup respaldado por el registro de go-governance.
func NewRegistryRepoLookup(cfg RegistryLookupConfig) *RegistryRepoLookup {
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 15 * time.Second
	}
	if cfg.LkgTTL <= 0 {
		cfg.LkgTTL = 5 * time.Minute
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 5 * time.Second}
	}
	if cfg.Log == nil {
		cfg.Log = slog.New(slog.DiscardHandler)
	}

	cached := make(map[string]bool)
	for _, s := range cfg.Seed {
		if tr := strings.ToLower(strings.TrimSpace(s)); tr != "" {
			cached[tr] = true
		}
	}
	r := &RegistryRepoLookup{
		cfg:    cfg,
		cached: cached,
	}
	if len(cached) > 0 {
		r.lastGood = cfg.Now()
	}
	return r
}

type governanceRepoItem struct {
	Repo    string `json:"repo"`
	Enabled bool   `json:"enabled"`
}

func (r *RegistryRepoLookup) fetch(ctx context.Context) (map[string]bool, error) {
	url := fmt.Sprintf("%s/v1/repositories", strings.TrimRight(r.cfg.GovernanceURL, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if r.cfg.ServiceToken != "" {
		req.Header.Set("Authorization", "Bearer "+r.cfg.ServiceToken)
	}

	resp, err := r.cfg.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("governance respondió HTTP %d", resp.StatusCode)
	}

	var items []governanceRepoItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("decodificando repositorios de governance: %w", err)
	}

	m := make(map[string]bool, len(items)+len(r.cfg.Seed))
	for _, s := range r.cfg.Seed {
		if tr := strings.ToLower(strings.TrimSpace(s)); tr != "" {
			m[tr] = true
		}
	}
	for _, it := range items {
		tr := strings.ToLower(strings.TrimSpace(it.Repo))
		if tr != "" {
			m[tr] = it.Enabled
		}
	}
	return m, nil
}

// Allowed implementa RepoLookup.
func (r *RegistryRepoLookup) Allowed(ctx context.Context, repo string) bool {
	repo = strings.ToLower(strings.TrimSpace(repo))
	if repo == "" {
		return false
	}
	now := r.cfg.Now()

	// 1. Verificación rápida de caché bajo RLock (< 15s)
	r.mu.RLock()
	if !r.lastFetched.IsZero() && now.Sub(r.lastFetched) < r.cfg.CacheTTL {
		allowed := r.cached[repo]
		r.mu.RUnlock()
		return allowed
	}
	r.mu.RUnlock()

	// 2. Caché vencida o primera consulta
	r.mu.Lock()
	defer r.mu.Unlock()

	// Doble comprobación bajo lock
	now = r.cfg.Now()
	if !r.lastFetched.IsZero() && now.Sub(r.lastFetched) < r.cfg.CacheTTL {
		return r.cached[repo]
	}

	fetched, err := r.fetch(ctx)
	if err == nil {
		r.cached = fetched
		r.lastFetched = now
		r.lastGood = now
		return r.cached[repo]
	}

	// Consulta fallida
	r.lastFetched = now
	r.cfg.Log.WarnContext(ctx, "fallo al consultar repositorios en governance", "error", err.Error())

	// Comprobar last-known-good (5 minutos)
	if !r.lastGood.IsZero() && now.Sub(r.lastGood) <= r.cfg.LkgTTL {
		return r.cached[repo]
	}

	// Fail-closed si no hay last-known-good o ya expiró
	return false
}
