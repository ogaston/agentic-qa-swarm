package warmmanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// MaxRetries son los reintentos del deploy tras el intento inicial (V8): 3 Jobs como máximo.
const MaxRetries = 2

// Config del servicio.
type Config struct {
	Job              JobConfig
	WarmReadyTimeout time.Duration // 120 s por defecto
	PollInterval     time.Duration
	JobTimeout       time.Duration
}

// Service reúne el dominio y los puertos.
type Service struct {
	Cfg      Config
	State    StateStore
	Probe    HealthProbe
	Runtime  WarmRuntime
	Jobs     Jobs
	Surface  SurfaceProber
	Objects  ObjectStore
	Alerts   Alerter
	Pub      EventPublisher
	Clock    Clock
	Observer Observer

	mu      sync.Mutex
	deploys map[string]*DeployStatus
}

// Observer recibe contadores (métricas); opcional.
type Observer interface {
	WarmState(state string)
	DeployAttempt(result string)
	Handoff(phase string)
}

type nopObserver struct{}

func (nopObserver) WarmState(string)     {}
func (nopObserver) DeployAttempt(string) {}
func (nopObserver) Handoff(string)       {}

// DeployStatus es el estado consultable de un deploy.
type DeployStatus struct {
	RunID    string `json:"run_id"`
	State    string `json:"state"` // pending|done|failed
	Attempts int    `json:"attempts"`
}

func (s *Service) obs() Observer {
	if s.Observer == nil {
		return nopObserver{}
	}
	return s.Observer
}

func (s *Service) pollInterval() time.Duration {
	if s.Cfg.PollInterval > 0 {
		return s.Cfg.PollInterval
	}
	return 2 * time.Second
}

// GetWarm devuelve el WarmState actual.
func (s *Service) GetWarm(ctx context.Context) (WarmState, error) {
	w, err := s.State.Get(ctx)
	if err == nil {
		s.obs().WarmState(w.State)
	}
	return w, err
}

// EnsureWarmReady devuelve el estado y si el warm está listo (ready + reset_verified + sondas verdes).
// Si está idle-escalado lo escala y espera Ready. dirty/cuarentena: no hace reset (es de go-reset).
// Publica warm.ready solo cuando todo está verde.
func (s *Service) EnsureWarmReady(ctx context.Context, trace string) (WarmState, bool, error) {
	w, err := s.GetWarm(ctx)
	if err != nil {
		return WarmState{}, false, err
	}
	if w.State == StateIdleEscalado {
		if err := s.Runtime.ScaleUp(ctx); err != nil {
			return w, false, err
		}
		timeout := s.Cfg.WarmReadyTimeout
		if timeout <= 0 {
			timeout = 120 * time.Second
		}
		deadline := s.Clock.Now().Add(timeout)
		for {
			if s.Probe.Check(ctx) == nil {
				break
			}
			if !s.Clock.Now().Before(deadline) {
				return w, false, ErrWarmTimeout
			}
			if err := s.Clock.Sleep(ctx, s.pollInterval()); err != nil {
				return w, false, err
			}
		}
		nw, err := Transition(w, StateReady, false)
		if err != nil {
			return w, false, err
		}
		if err := s.State.Put(ctx, nw); err != nil {
			return w, false, err
		}
		w = nw
		s.obs().WarmState(w.State)
	}
	if w.State != StateReady || !w.ResetVerified {
		return w, false, nil
	}
	if err := s.Probe.Check(ctx); err != nil {
		return w, false, nil
	}
	ev := newEvent("warm.ready", w.WarmID+"/"+w.BaselineVersion+"/"+fmt.Sprint(s.Clock.Now().UnixNano()), trace, s.Clock.Now(),
		map[string]any{"warm_id": w.WarmID, "state": StateReady, "baseline_version": w.BaselineVersion})
	if err := s.Pub.Publish(ctx, ev); err != nil {
		return w, false, err
	}
	return w, true, nil
}

// StartDeploy valida y lanza el deploy en segundo plano; es idempotente por run_id.
func (s *Service) StartDeploy(runID string, a Artifact, trace string) (DeployStatus, error) {
	if err := ValidateRunID(runID); err != nil {
		return DeployStatus{}, err
	}
	if err := ValidateArtifact(a, s.Cfg.Job.AllowedRegistries); err != nil {
		return DeployStatus{}, err
	}
	s.mu.Lock()
	if s.deploys == nil {
		s.deploys = map[string]*DeployStatus{}
	}
	if d, ok := s.deploys[runID]; ok {
		out := *d
		s.mu.Unlock()
		return out, nil
	}
	d := &DeployStatus{RunID: runID, State: "pending"}
	s.deploys[runID] = d
	out := *d
	s.mu.Unlock()
	go func() { _ = s.Deploy(context.Background(), runID, a, trace) }()
	return out, nil
}

// DeployState devuelve el estado de un deploy conocido.
func (s *Service) DeployState(runID string) (DeployStatus, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.deploys[runID]
	if !ok {
		return DeployStatus{}, false
	}
	return *d, true
}

func (s *Service) track(runID, state string, attempts int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deploys == nil {
		s.deploys = map[string]*DeployStatus{}
	}
	d := s.deploys[runID]
	if d == nil {
		d = &DeployStatus{RunID: runID}
		s.deploys[runID] = d
	}
	d.State, d.Attempts = state, attempts
}

// Deploy ejecuta el deploy de forma síncrona: marca el warm dirty (la corrida lo usa), crea el
// Job deploy-{run}-0 y, si falla, hasta MaxRetries reintentos; agotados: deploy.failed + handoff.
func (s *Service) Deploy(ctx context.Context, runID string, a Artifact, trace string) error {
	if err := ValidateRunID(runID); err != nil {
		return err
	}
	if err := ValidateArtifact(a, s.Cfg.Job.AllowedRegistries); err != nil {
		return err
	}
	w, err := s.State.Get(ctx)
	if err != nil {
		return err
	}
	if w.State != StateReady || !w.ResetVerified {
		s.track(runID, "failed", 0)
		return fmt.Errorf("%w: estado %s", ErrNotReady, w.State)
	}
	dirty, err := Transition(w, StateDirty, false)
	if err != nil {
		return err
	}
	if err := s.State.Put(ctx, dirty); err != nil {
		return err
	}
	s.obs().WarmState(dirty.State)
	s.track(runID, "pending", 0)

	var reason string
	attempts := 0
	for n := 0; n <= MaxRetries; n++ {
		m, err := BuildDeployJob(s.Cfg.Job, runID, n, a)
		if err != nil {
			return err
		}
		attempts = n + 1
		s.track(runID, "pending", attempts)
		if err := s.Jobs.Create(ctx, m); err != nil {
			reason = "no se pudo crear el Job: " + err.Error()
			s.obs().DeployAttempt("error")
			continue
		}
		phase, why, err := s.waitJob(ctx, JobName(runID, n))
		if err == nil && phase == JobSucceeded {
			s.obs().DeployAttempt("success")
			s.track(runID, "done", attempts)
			return s.Pub.Publish(ctx, newEvent("deploy.done", runID, trace, s.Clock.Now(), deployData(dirty.WarmID, runID, a, "")))
		}
		s.obs().DeployAttempt("failure")
		if err != nil {
			reason = err.Error()
		} else {
			reason = "el Job fallo: " + why
		}
	}
	s.track(runID, "failed", attempts)
	if reason == "" {
		reason = "deploy fallido"
	}
	if err := s.Alerts.Handoff(ctx, "deploy", runID, reason); err != nil {
		return err
	}
	s.obs().Handoff("deploy")
	if err := s.Pub.Publish(ctx, newEvent("deploy.failed", runID, trace, s.Clock.Now(), deployData(dirty.WarmID, runID, a, reason))); err != nil {
		return err
	}
	return fmt.Errorf("deploy agotado tras %d intentos: %s", attempts, reason)
}

func deployData(warmID, runID string, a Artifact, reason string) map[string]any {
	d := map[string]any{"run_id": runID, "warm_id": warmID, "artifact": map[string]any{"kind": a.Kind, "ref": a.Ref}}
	if reason != "" {
		d["reason"] = reason
	}
	return d
}

func (s *Service) waitJob(ctx context.Context, name string) (JobPhase, string, error) {
	timeout := s.Cfg.JobTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	deadline := s.Clock.Now().Add(timeout)
	for {
		p, why, err := s.Jobs.Status(ctx, name)
		if err != nil {
			return "", "", err
		}
		if p != JobPending {
			return p, why, nil
		}
		if !s.Clock.Now().Before(deadline) {
			return JobFailed, "timeout esperando el Job", nil
		}
		if err := s.Clock.Sleep(ctx, s.pollInterval()); err != nil {
			return "", "", err
		}
	}
}

// SurfaceArtifact es el SurfaceArtifact del contrato.
type SurfaceArtifact struct {
	RunID     string     `json:"run_id"`
	BaseURL   string     `json:"base_url"`
	Endpoints []Endpoint `json:"endpoints"`
	Source    string     `json:"source"`
}

// Endpoint es un par método/ruta.
type Endpoint struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

var openAPIPaths = []string{"/openapi.json", "/v3/api-docs", "/swagger.json"}
var probePaths = []string{"/", "/health", "/healthz", "/api", "/status"}
var methods = map[string]string{"get": "GET", "post": "POST", "put": "PUT", "patch": "PATCH", "delete": "DELETE"}

// InferSurface infiere la superficie consultando solo el exterior del Service del warm.
func (s *Service) InferSurface(ctx context.Context, runID, trace string) (SurfaceArtifact, error) {
	if err := ValidateRunID(runID); err != nil {
		return SurfaceArtifact{}, err
	}
	sa := SurfaceArtifact{RunID: runID, BaseURL: s.Surface.BaseURL(), Endpoints: []Endpoint{}, Source: "probe"}
	for _, p := range openAPIPaths {
		st, body, err := s.Surface.Get(ctx, p)
		if err != nil || st != 200 {
			continue
		}
		if eps, ok := parseOpenAPI(body); ok {
			sa.Source, sa.Endpoints = "openapi", eps
			break
		}
	}
	if sa.Source == "probe" {
		for _, p := range probePaths {
			st, _, err := s.Surface.Get(ctx, p)
			if err == nil && st > 0 && st < 400 {
				sa.Endpoints = append(sa.Endpoints, Endpoint{Method: "GET", Path: p})
			}
		}
	}
	raw, err := json.Marshal(sa)
	if err != nil {
		return SurfaceArtifact{}, err
	}
	uri, err := s.Objects.Put(ctx, runID+"/surface.json", raw)
	if err != nil {
		return SurfaceArtifact{}, err
	}
	ev := newEvent("surface.ready", runID, trace, s.Clock.Now(),
		map[string]any{"run_id": runID, "surface_uri": uri, "endpoint_count": len(sa.Endpoints)})
	if err := s.Pub.Publish(ctx, ev); err != nil {
		return SurfaceArtifact{}, err
	}
	return sa, nil
}

func parseOpenAPI(body []byte) ([]Endpoint, bool) {
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if json.Unmarshal(body, &doc) != nil || doc.Paths == nil {
		return nil, false
	}
	eps := []Endpoint{}
	for p, ops := range doc.Paths {
		if !strings.HasPrefix(p, "/") {
			continue
		}
		for m := range ops {
			if M, ok := methods[strings.ToLower(m)]; ok {
				eps = append(eps, Endpoint{Method: M, Path: p})
			}
		}
	}
	sort.Slice(eps, func(i, j int) bool {
		if eps[i].Path != eps[j].Path {
			return eps[i].Path < eps[j].Path
		}
		return eps[i].Method < eps[j].Method
	})
	return eps, true
}

var _ = errors.Is
