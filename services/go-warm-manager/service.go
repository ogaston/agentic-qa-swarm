package warmmanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/warmtarget"
)

// DefaultWarmReadyTimeout es la espera maxima por defecto a que el warm llegue a Ready.
const DefaultWarmReadyTimeout = 120 * time.Second

// DefaultDeployTimeout es la espera maxima por defecto del rollout de un intento de deploy.
const DefaultDeployTimeout = 10 * time.Minute

// MaxRetries son los reintentos del deploy tras el intento inicial (V8): 3 intentos como máximo.
const MaxRetries = 2

// Config del servicio.
type Config struct {
	AllowedRegistries []string
	// AllowedImagePrefixes: WARM_ALLOWED_IMAGE_PREFIXES (U11-T03). Vacía = ninguna published-image permitida.
	AllowedImagePrefixes []string
	WarmReadyTimeout     time.Duration // 120 s por defecto
	PollInterval         time.Duration
	DeployTimeout        time.Duration // espera maxima del rollout de cada intento (10 min por defecto)
}

// Service reúne el dominio y los puertos.
type Service struct {
	Cfg      Config
	State    StateStore
	Probe    HealthProbe
	Runtime  WarmRuntime
	Deployer Deployer
	Surface  SurfaceProber
	Objects  ObjectStore
	Alerts   Alerter
	Pub      EventPublisher
	Clock    Clock
	Observer Observer

	takeMu        sync.Mutex // serializa ready->dirty
	wakeMu        sync.Mutex // serializa idle-escalado->ready
	warmMus       map[string]*sync.Mutex
	Log           *slog.Logger
	mu            sync.Mutex
	deploys       map[string]*DeployStatus
	deployTargets map[string]warmtarget.WarmTarget
	targets       map[string]warmtarget.WarmTarget
	repoImages    map[string]string
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
	Reason   string `json:"reason,omitempty"`
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

func (s *Service) getWarmMus(warmID string) (take, wake *sync.Mutex) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if warmID == "" || warmID == warmtarget.LegacyWarmID {
		return &s.takeMu, &s.wakeMu
	}
	if s.warmMus == nil {
		s.warmMus = make(map[string]*sync.Mutex)
	}
	take, ok := s.warmMus[warmID+":take"]
	if !ok {
		take = &sync.Mutex{}
		s.warmMus[warmID+":take"] = take
	}
	wake, ok = s.warmMus[warmID+":wake"]
	if !ok {
		wake = &sync.Mutex{}
		s.warmMus[warmID+":wake"] = wake
	}
	return take, wake
}

func (s *Service) resolveTarget(targetOrRepo ...any) warmtarget.WarmTarget {
	if len(targetOrRepo) == 0 || targetOrRepo[0] == nil {
		return warmtarget.LegacyTarget()
	}
	switch v := targetOrRepo[0].(type) {
	case warmtarget.WarmTarget:
		if v.Namespace == "" && v.Repo != "" {
			return warmtarget.TargetForRepo(v.Repo)
		}
		if v.Namespace == "" {
			return warmtarget.LegacyTarget()
		}
		return v
	case string:
		str := strings.TrimSpace(v)
		if str == "" || str == LegacyNamespace || str == warmtarget.LegacyWarmID {
			return warmtarget.LegacyTarget()
		}
		s.mu.Lock()
		if s.targets != nil {
			if t, ok := s.targets[str]; ok {
				s.mu.Unlock()
				return t
			}
		}
		s.mu.Unlock()
		if warmtarget.ValidWarmNamespace(str) && strings.HasPrefix(str, "aqs-w-") {
			slug := strings.TrimPrefix(str, "aqs-w-")
			return warmtarget.WarmTarget{
				Namespace: str,
				WarmID:    "w-" + slug,
				AppURL:    fmt.Sprintf("http://warm-app.%s.svc:8080", str),
			}
		}
		if strings.HasPrefix(str, "w-") {
			slug := strings.TrimPrefix(str, "w-")
			ns := "aqs-w-" + slug
			return warmtarget.WarmTarget{
				Namespace: ns,
				WarmID:    str,
				AppURL:    fmt.Sprintf("http://warm-app.%s.svc:8080", ns),
			}
		}
		return warmtarget.TargetForRepo(str)
	default:
		return warmtarget.LegacyTarget()
	}
}

// RegisterTarget registra un WarmTarget en memoria para resolución por repo, warm_id o namespace.
func (s *Service) RegisterTarget(target warmtarget.WarmTarget) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.targets == nil {
		s.targets = make(map[string]warmtarget.WarmTarget)
	}
	if target.Repo != "" {
		s.targets[target.Repo] = target
	}
	if target.WarmID != "" {
		s.targets[target.WarmID] = target
	}
	if target.Namespace != "" {
		s.targets[target.Namespace] = target
	}
}

// SetRepoImage registra el image_repo configurado para un repositorio.
func (s *Service) SetRepoImage(repo, imageRepo string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.repoImages == nil {
		s.repoImages = make(map[string]string)
	}
	s.repoImages[strings.ToLower(strings.TrimSpace(repo))] = strings.TrimSpace(imageRepo)
}

func (s *Service) allowedPrefixesForTarget(target warmtarget.WarmTarget) []string {
	if target.ImageRepo != "" {
		return []string{target.ImageRepo}
	}
	s.mu.Lock()
	if s.repoImages != nil && target.Repo != "" {
		if img, ok := s.repoImages[target.Repo]; ok && img != "" {
			s.mu.Unlock()
			return []string{img}
		}
	}
	if s.targets != nil {
		if t, ok := s.targets[target.Repo]; ok && t.ImageRepo != "" {
			s.mu.Unlock()
			return []string{t.ImageRepo}
		}
	}
	s.mu.Unlock()
	return s.Cfg.AllowedImagePrefixes
}

func (s *Service) getState(ctx context.Context, target warmtarget.WarmTarget) (WarmState, error) {
	if ts, ok := s.State.(TargetStateStore); ok && target.Namespace != LegacyNamespace && target.Namespace != "" {
		w, err := ts.GetForTarget(ctx, target)
		if err == nil {
			if w.Namespace == "" {
				w.Namespace = target.Namespace
			}
			if w.Repo == "" {
				w.Repo = target.Repo
			}
		}
		return w, err
	}
	return s.State.Get(ctx)
}

func (s *Service) casState(ctx context.Context, target warmtarget.WarmTarget, expect, next WarmState) error {
	if ts, ok := s.State.(TargetStateStore); ok && target.Namespace != LegacyNamespace && target.Namespace != "" {
		return ts.CompareAndSwapForTarget(ctx, target, expect, next)
	}
	return s.State.CompareAndSwap(ctx, expect, next)
}

func (s *Service) checkProbe(ctx context.Context, target warmtarget.WarmTarget) error {
	if hp, ok := s.Probe.(TargetHealthProbe); ok && target.Namespace != LegacyNamespace && target.Namespace != "" {
		return hp.CheckInNamespace(ctx, target.Namespace)
	}
	return s.Probe.Check(ctx)
}

func (s *Service) scaleUp(ctx context.Context, target warmtarget.WarmTarget) error {
	if wr, ok := s.Runtime.(TargetWarmRuntime); ok && target.Namespace != LegacyNamespace && target.Namespace != "" {
		return wr.ScaleUpInNamespace(ctx, target.Namespace)
	}
	return s.Runtime.ScaleUp(ctx)
}

func (s *Service) setImage(ctx context.Context, target warmtarget.WarmTarget, ref string) error {
	if d, ok := s.Deployer.(TargetDeployer); ok && target.Namespace != LegacyNamespace && target.Namespace != "" {
		return d.SetImageInNamespace(ctx, target.Namespace, ref)
	}
	return s.Deployer.SetImage(ctx, ref)
}

func (s *Service) rolloutComplete(ctx context.Context, target warmtarget.WarmTarget, ref string) (bool, error) {
	if d, ok := s.Deployer.(TargetDeployer); ok && target.Namespace != LegacyNamespace && target.Namespace != "" {
		return d.RolloutCompleteInNamespace(ctx, target.Namespace, ref)
	}
	return s.Deployer.RolloutComplete(ctx, ref)
}

func (s *Service) probeSurface(ctx context.Context, target warmtarget.WarmTarget, path string) (int, []byte, error) {
	baseURL := s.Surface.BaseURL()
	if target.Namespace != "" && target.Namespace != LegacyNamespace && target.AppURL != "" {
		baseURL = target.AppURL
	}
	if p, ok := s.Surface.(TargetSurfaceProber); ok && baseURL != "" {
		return p.GetWithBaseURL(ctx, baseURL, path)
	}
	return s.Surface.Get(ctx, path)
}

// GetWarm devuelve el WarmState actual.
func (s *Service) GetWarm(ctx context.Context, targetOrRepo ...any) (WarmState, error) {
	target := s.resolveTarget(targetOrRepo...)
	w, err := s.getState(ctx, target)
	if err == nil {
		s.obs().WarmState(w.State)
	}
	return w, err
}

// EnsureWarmReady devuelve el estado y si el warm está listo (ready + reset_verified + sondas verdes).
// Si está idle-escalado lo escala y espera Ready. dirty/cuarentena: no hace reset (es de go-reset).
// Publica warm.ready solo cuando todo está verde.
func (s *Service) EnsureWarmReady(ctx context.Context, trace string, targetOrRepo ...any) (WarmState, bool, error) {
	target := s.resolveTarget(targetOrRepo...)
	w, err := s.GetWarm(ctx, target)
	if err != nil {
		return WarmState{}, false, err
	}
	if w.State == StateIdleEscalado {
		if w, err = s.wakeWarm(ctx, w, target); err != nil {
			return w, false, err
		}
	}
	if w.State != StateReady || !w.ResetVerified {
		return w, false, nil
	}
	if err := s.checkProbe(ctx, target); err != nil {
		s.logger().Warn("ensure: sonda del warm caida", "trace_id", trace, "warm_id", w.WarmID, "error", err.Error())
		return w, false, nil
	}
	ev := newEvent("warm.ready", w.WarmID+"/"+w.BaselineVersion+"/"+fmt.Sprint(s.Clock.Now().UnixNano()), trace, s.Clock.Now(),
		map[string]any{"warm_id": w.WarmID, "state": StateReady, "baseline_version": w.BaselineVersion})
	if err := s.Pub.Publish(ctx, ev); err != nil {
		s.logger().Error("publicar warm.ready fallo", "trace_id", trace, "error", err.Error())
		return w, false, err
	}
	return w, true, nil
}

// wakeWarm saca al warm de idle-escalado.
func (s *Service) wakeWarm(ctx context.Context, w WarmState, target warmtarget.WarmTarget) (WarmState, error) {
	_, wakeMu := s.getWarmMus(target.WarmID)
	wakeMu.Lock()
	defer wakeMu.Unlock()
	cur, err := s.getState(ctx, target)
	if err != nil {
		return w, err
	}
	if cur.State != StateIdleEscalado {
		return cur, nil
	}
	if err := s.scaleUp(ctx, target); err != nil {
		s.logger().Error("ensure: ScaleUp fallo", "warm_id", cur.WarmID, "error", err.Error())
		return cur, err
	}
	timeout := s.Cfg.WarmReadyTimeout
	if timeout <= 0 {
		timeout = DefaultWarmReadyTimeout
	}
	deadline := s.Clock.Now().Add(timeout)
	for s.checkProbe(ctx, target) != nil {
		if !s.Clock.Now().Before(deadline) {
			return cur, ErrWarmTimeout
		}
		if err := s.Clock.Sleep(ctx, s.pollInterval()); err != nil {
			return cur, err
		}
	}
	nw, err := Transition(cur, StateReady, false)
	if err != nil {
		return cur, err
	}
	if err := s.casState(ctx, target, cur, nw); err != nil {
		if errors.Is(err, ErrStateConflict) {
			re, gerr := s.getState(ctx, target)
			if gerr != nil {
				return cur, gerr
			}
			return re, nil
		}
		return cur, err
	}
	s.obs().WarmState(nw.State)
	return nw, nil
}

// StartDeploy valida y, si el warm esta listo, lanza el deploy en segundo plano (idempotente por run_id).
// Si el warm no esta listo devuelve ErrNotReady junto con el WarmState y NO crea estado pending.
func (s *Service) StartDeploy(ctx context.Context, runID string, a Artifact, trace string, targetOrRepo ...any) (DeployStatus, WarmState, error) {
	target := s.resolveTarget(targetOrRepo...)
	if err := ValidateRunID(runID); err != nil {
		return DeployStatus{}, WarmState{}, err
	}
	prefixes := s.allowedPrefixesForTarget(target)
	if err := ValidateArtifact(a, s.Cfg.AllowedRegistries, prefixes); err != nil {
		return DeployStatus{}, WarmState{}, err
	}
	if a.Kind == KindBuildFromRepo {
		return DeployStatus{}, WarmState{}, ErrBuildNotSupported
	}
	s.mu.Lock()
	if d, ok := s.deploys[runID]; ok {
		out := *d
		s.mu.Unlock()
		return out, WarmState{}, nil
	}
	s.mu.Unlock()
	w, err := s.getState(ctx, target)
	if err != nil {
		return DeployStatus{}, WarmState{}, err
	}
	if w.State != StateReady || !w.ResetVerified {
		return DeployStatus{}, w, ErrNotReady
	}
	s.mu.Lock()
	if d, ok := s.deploys[runID]; ok { // otra peticion del mismo run gano la carrera
		out := *d
		s.mu.Unlock()
		return out, WarmState{}, nil
	}
	if s.deploys == nil {
		s.deploys = map[string]*DeployStatus{}
	}
	if s.deployTargets == nil {
		s.deployTargets = map[string]warmtarget.WarmTarget{}
	}
	s.deployTargets[runID] = target
	d := &DeployStatus{RunID: runID, State: "pending"}
	s.deploys[runID] = d
	out := *d
	s.mu.Unlock()
	go func() {
		// Deploy ya deja el estado failed, el evento y el log; aqui solo se registra el cierre.
		if err := s.Deploy(context.Background(), runID, a, trace, target); err != nil {
			s.logger().Error("deploy termino con error", "run_id", runID, "trace_id", trace, "error", err.Error())
		}
	}()
	return out, WarmState{}, nil
}

// DeployState devuelve el estado de un deploy. Vive solo en memoria: tras un reinicio no hay
// deploys conocidos (404) y el warm persistido esta dirty, asi que nada se despliega hasta un reset verificado.
func (s *Service) DeployState(_ context.Context, runID string) (DeployStatus, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.deploys[runID]
	if !ok {
		return DeployStatus{}, false, nil
	}
	return *d, true, nil
}

func (s *Service) track(runID, state string, attempts int, reason string) {
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
	d.State, d.Attempts, d.Reason = state, attempts, reason
}

func (s *Service) logger() *slog.Logger {
	if s.Log == nil {
		return slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	return s.Log
}

// fail cierra un deploy como fallido: estado visible con razon, log, deploy.failed y (si handoff) aviso.
// Ningun error se pierde: se registran en el log y se devuelven unidos.
func (s *Service) fail(ctx context.Context, runID, warmID string, a Artifact, trace string, attempts int, reason string, handoff bool) error {
	return errors.Join(fmt.Errorf("deploy fallido tras %d intentos: %s", attempts, reason),
		s.closeFailed(ctx, runID, warmID, a, trace, attempts, reason, handoff))
}

// closeFailed hace los efectos de cerrar un deploy como fallido y devuelve solo los errores de esos
// efectos (handoff, publicacion).
func (s *Service) closeFailed(ctx context.Context, runID, warmID string, a Artifact, trace string, attempts int, reason string, handoff bool) error {
	s.track(runID, "failed", attempts, reason)
	s.logger().Error("deploy fallido", "run_id", runID, "trace_id", trace, "reason", reason, "attempts", attempts)
	var errs []error
	if handoff {
		if err := s.Alerts.Handoff(ctx, "deploy", runID, reason); err != nil {
			s.logger().Error("handoff fallo", "run_id", runID, "trace_id", trace, "error", err.Error())
			errs = append(errs, err)
		} else {
			s.obs().Handoff("deploy")
		}
	}
	if warmID == "" { // sin warm_id el evento no seria valido (no se pudo ni leer el estado)
		return errors.Join(errs...)
	}
	ev := newEvent("deploy.failed", runID, trace, s.Clock.Now(), deployData(warmID, runID, a, reason))
	if err := s.Pub.Publish(ctx, ev); err != nil {
		s.logger().Error("publicar deploy.failed fallo", "run_id", runID, "trace_id", trace, "error", err.Error())
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// StatusReadRetries es el tope de lecturas fallidas seguidas del rollout antes de dar el intento por fallido.
const StatusReadRetries = 5

// Deploy ejecuta el deploy de forma sincrona: toma el warm (ready->dirty atomico ANTES de parchear),
// parchea la imagen de warm-app y espera el rollout completo.
func (s *Service) Deploy(ctx context.Context, runID string, a Artifact, trace string, targetOrRepo ...any) error {
	target := s.resolveTarget(targetOrRepo...)
	if err := ValidateRunID(runID); err != nil {
		return err
	}
	prefixes := s.allowedPrefixesForTarget(target)
	if err := ValidateArtifact(a, s.Cfg.AllowedRegistries, prefixes); err != nil {
		return err
	}
	if a.Kind == KindBuildFromRepo {
		return ErrBuildNotSupported
	}
	dirty, w, err := s.takeWarm(ctx, target)
	if err != nil {
		reason := "no se pudo tomar el warm: " + err.Error()
		return errors.Join(err, s.fail(ctx, runID, w.WarmID, a, trace, 0, reason, false))
	}
	s.obs().WarmState(dirty.State)
	s.track(runID, "pending", 0, "")

	reason := "deploy fallido"
	attempts := 0
	for n := 0; n <= MaxRetries; n++ {
		attempts = n + 1
		s.track(runID, "pending", attempts, "")
		if err := s.setImage(ctx, target, a.Ref); err != nil {
			s.obs().DeployAttempt("failure")
			reason = "no se pudo parchear la imagen de warm-app: " + err.Error()
			continue
		}
		ok, why := s.waitRollout(ctx, target, a.Ref)
		if !ok {
			s.obs().DeployAttempt("failure")
			reason = why
			continue
		}
		s.obs().DeployAttempt("success")
		ev := newEvent("deploy.done", runID, trace, s.Clock.Now(), deployData(dirty.WarmID, runID, a, ""))
		if err := s.Pub.Publish(ctx, ev); err != nil {
			s.logger().Error("publicar deploy.done fallo", "run_id", runID, "trace_id", trace, "error", err.Error())
			s.track(runID, "failed", attempts, "deploy ok pero no se pudo publicar deploy.done: "+err.Error())
			return err
		}
		s.track(runID, "done", attempts, "")
		return nil
	}
	return s.fail(ctx, runID, dirty.WarmID, a, trace, attempts, reason, true)
}

// takeWarm hace ready->dirty de forma atomica.
func (s *Service) takeWarm(ctx context.Context, target warmtarget.WarmTarget) (dirty, w WarmState, err error) {
	takeMu, _ := s.getWarmMus(target.WarmID)
	takeMu.Lock()
	defer takeMu.Unlock()
	w, err = s.getState(ctx, target)
	if err != nil {
		return WarmState{}, w, err
	}
	if w.State != StateReady || !w.ResetVerified {
		return WarmState{}, w, fmt.Errorf("%w: estado %s", ErrNotReady, w.State)
	}
	dirty, err = Transition(w, StateDirty, false)
	if err != nil {
		return WarmState{}, w, err
	}
	if err := s.casState(ctx, target, w, dirty); err != nil {
		if errors.Is(err, ErrStateConflict) {
			return WarmState{}, w, fmt.Errorf("%w: %v", ErrNotReady, err)
		}
		return WarmState{}, w, err
	}
	return dirty, w, nil
}

func deployData(warmID, runID string, a Artifact, reason string) map[string]any {
	d := map[string]any{"run_id": runID, "warm_id": warmID, "artifact": map[string]any{"kind": a.Kind, "ref": a.Ref}}
	if reason != "" {
		d["reason"] = reason
	}
	return d
}

// waitRollout espera el rollout completo de ref hasta DeployTimeout.
func (s *Service) waitRollout(ctx context.Context, target warmtarget.WarmTarget, ref string) (bool, string) {
	timeout := s.Cfg.DeployTimeout
	if timeout <= 0 {
		timeout = DefaultDeployTimeout
	}
	deadline := s.Clock.Now().Add(timeout)
	readFails := 0
	for {
		done, err := s.rolloutComplete(ctx, target, ref)
		switch {
		case err != nil:
			if readFails++; readFails >= StatusReadRetries {
				return false, fmt.Sprintf("no se pudo leer el rollout de warm-app (imagen %s): %s", ref, err.Error())
			}
		case done:
			return true, ""
		default:
			readFails = 0
		}
		if !s.Clock.Now().Before(deadline) {
			return false, fmt.Sprintf("timeout esperando el rollout completo de warm-app (imagen %s) tras %s", ref, timeout)
		}
		if err := s.Clock.Sleep(ctx, s.pollInterval()); err != nil {
			return false, "espera del rollout interrumpida: " + err.Error()
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
	Method      string          `json:"method"`
	Path        string          `json:"path"`
	RequestBody json.RawMessage `json:"request_body,omitempty"`
}

var openAPIPaths = []string{"/openapi.json", "/v3/api-docs", "/swagger.json"}
var probePaths = []string{"/", "/health", "/healthz", "/api", "/status"}
var methods = map[string]string{"get": "GET", "post": "POST", "put": "PUT", "patch": "PATCH", "delete": "DELETE"}

// Errores de la superficie.
var (
	ErrDeployNotDone      = errors.New("el deploy de la corrida no ha terminado bien")
	ErrSurfaceUnreachable = errors.New("la app del warm no responde a ningun sondeo")
	ErrNoSurface          = errors.New("no se encontro ningun endpoint")
)

// InferSurface infiere la superficie consultando solo el exterior del Service del warm.
func (s *Service) InferSurface(ctx context.Context, runID, trace string, targetOrRepo ...any) (SurfaceArtifact, error) {
	if err := ValidateRunID(runID); err != nil {
		return SurfaceArtifact{}, err
	}
	var target warmtarget.WarmTarget
	if len(targetOrRepo) > 0 && targetOrRepo[0] != nil {
		target = s.resolveTarget(targetOrRepo...)
	} else {
		s.mu.Lock()
		if t, ok := s.deployTargets[runID]; ok {
			target = t
		} else {
			target = warmtarget.LegacyTarget()
		}
		s.mu.Unlock()
	}

	d, ok, err := s.DeployState(ctx, runID)
	if err != nil {
		return SurfaceArtifact{}, err
	}
	if !ok || d.State != "done" {
		return SurfaceArtifact{}, fmt.Errorf("%w: run %s", ErrDeployNotDone, runID)
	}
	w, err := s.getState(ctx, target)
	if err != nil {
		return SurfaceArtifact{}, err
	}
	// Tras un deploy el warm esta dirty (lo usa la corrida); idle/cuarentena/ready no sirven de blanco.
	if w.State != StateDirty {
		return SurfaceArtifact{}, fmt.Errorf("%w: el warm esta %s", ErrNotReady, w.State)
	}
	baseURL := s.Surface.BaseURL()
	if target.Namespace != "" && target.Namespace != LegacyNamespace && target.AppURL != "" {
		baseURL = target.AppURL
	}
	sa := SurfaceArtifact{RunID: runID, BaseURL: baseURL, Endpoints: []Endpoint{}, Source: "probe"}
	answered := 0
	probe := func(p string) (int, []byte, bool) {
		st, body, err := s.probeSurface(ctx, target, p)
		if err != nil {
			s.logger().Warn("sondeo de superficie fallo", "run_id", runID, "trace_id", trace, "path", p, "error", err.Error())
			return 0, nil, false
		}
		answered++
		return st, body, true
	}
	for _, p := range openAPIPaths {
		st, body, ok := probe(p)
		if !ok || st != 200 {
			continue
		}
		if eps, ok := parseOpenAPI(body); ok && len(eps) > 0 {
			sa.Source, sa.Endpoints = "openapi", eps
			break
		}
	}
	if sa.Source == "probe" {
		for _, p := range probePaths {
			if st, _, ok := probe(p); ok && st > 0 && st < 400 {
				sa.Endpoints = append(sa.Endpoints, Endpoint{Method: "GET", Path: p})
			}
		}
	}
	if answered == 0 {
		return SurfaceArtifact{}, ErrSurfaceUnreachable
	}
	if len(sa.Endpoints) == 0 {
		return SurfaceArtifact{}, ErrNoSurface
	}
	if err := ValidateSurface(sa); err != nil {
		s.logger().Error("superficie invalida", "run_id", runID, "trace_id", trace, "error", err.Error())
		return SurfaceArtifact{}, err
	}
	raw, err := json.Marshal(sa)
	if err != nil {
		return SurfaceArtifact{}, err
	}
	uri, err := s.Objects.Put(ctx, runID+"/surface.json", raw)
	if err != nil {
		s.logger().Error("guardar la superficie fallo", "run_id", runID, "trace_id", trace, "error", err.Error())
		return SurfaceArtifact{}, err
	}
	ev := newEvent("surface.ready", runID, trace, s.Clock.Now(),
		map[string]any{"run_id": runID, "surface_uri": uri, "endpoint_count": len(sa.Endpoints)})
	if err := s.Pub.Publish(ctx, ev); err != nil {
		s.logger().Error("publicar surface.ready fallo", "run_id", runID, "trace_id", trace, "error", err.Error())
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
		for m, raw := range ops {
			if M, ok := methods[strings.ToLower(m)]; ok {
				eps = append(eps, Endpoint{Method: M, Path: p, RequestBody: openAPIRequestBody(raw)})
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

// openAPIRequestBody extrae el esquema JSON de requestBody (application/json) de una operacion.
// Sin requestBody, sin application/json, con esquema ausente, no objeto o con $ref (local o externo)
// devuelve nil: el endpoint sale sin cuerpo y la inferencia no falla. No resuelve referencias.
// Un $ref ANIDADO dentro del esquema (p. ej. properties.id.$ref) se conserva tal cual: el planner
// exige body para ese endpoint pero no comprueba el cuerpo (ver validate.py _checkable).
func openAPIRequestBody(op json.RawMessage) json.RawMessage {
	var o struct {
		RequestBody struct {
			Content map[string]struct {
				Schema json.RawMessage `json:"schema"`
			} `json:"content"`
		} `json:"requestBody"`
	}
	if json.Unmarshal(op, &o) != nil {
		return nil
	}
	media, ok := o.RequestBody.Content["application/json"]
	if !ok {
		return nil
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(media.Schema, &obj) != nil || obj == nil {
		return nil
	}
	if _, ref := obj["$ref"]; ref {
		return nil
	}
	return media.Schema
}

var _ = errors.Is
