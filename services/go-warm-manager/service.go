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
)

// DefaultWarmReadyTimeout es la espera maxima por defecto a que el warm llegue a Ready.
const DefaultWarmReadyTimeout = 120 * time.Second

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

	takeMu  sync.Mutex // serializa ready->dirty
	wakeMu  sync.Mutex // serializa idle-escalado->ready
	Log     *slog.Logger
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
		if w, err = s.wakeWarm(ctx, w); err != nil {
			return w, false, err
		}
	}
	if w.State != StateReady || !w.ResetVerified {
		return w, false, nil
	}
	if err := s.Probe.Check(ctx); err != nil {
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

// wakeWarm saca al warm de idle-escalado. Serializado (un solo ScaleUp efectivo con N ensures
// concurrentes) y tolerante a otras instancias: ante ErrStateConflict relee y sigue evaluando.
func (s *Service) wakeWarm(ctx context.Context, w WarmState) (WarmState, error) {
	s.wakeMu.Lock()
	defer s.wakeMu.Unlock()
	cur, err := s.State.Get(ctx) // otro ensure pudo haberlo despertado mientras esperabamos el cerrojo
	if err != nil {
		return w, err
	}
	if cur.State != StateIdleEscalado {
		return cur, nil
	}
	if err := s.Runtime.ScaleUp(ctx); err != nil {
		s.logger().Error("ensure: ScaleUp fallo", "warm_id", cur.WarmID, "error", err.Error())
		return cur, err
	}
	timeout := s.Cfg.WarmReadyTimeout
	if timeout <= 0 {
		timeout = DefaultWarmReadyTimeout
	}
	deadline := s.Clock.Now().Add(timeout)
	for s.Probe.Check(ctx) != nil {
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
	if err := s.State.CompareAndSwap(ctx, cur, nw); err != nil {
		if errors.Is(err, ErrStateConflict) { // otra instancia lo cambio: se relee y se evalua lo que haya
			re, gerr := s.State.Get(ctx)
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
func (s *Service) StartDeploy(ctx context.Context, runID string, a Artifact, trace string) (DeployStatus, WarmState, error) {
	if err := ValidateRunID(runID); err != nil {
		return DeployStatus{}, WarmState{}, err
	}
	if err := ValidateArtifact(a, s.Cfg.Job.AllowedRegistries); err != nil {
		return DeployStatus{}, WarmState{}, err
	}
	s.mu.Lock()
	if d, ok := s.deploys[runID]; ok {
		out := *d
		s.mu.Unlock()
		return out, WarmState{}, nil
	}
	s.mu.Unlock()
	w, err := s.State.Get(ctx)
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
	d := &DeployStatus{RunID: runID, State: "pending"}
	s.deploys[runID] = d
	out := *d
	s.mu.Unlock()
	go func() {
		// Deploy ya deja el estado failed, el evento y el log; aqui solo se registra el cierre.
		if err := s.Deploy(context.Background(), runID, a, trace); err != nil {
			s.logger().Error("deploy termino con error", "run_id", runID, "trace_id", trace, "error", err.Error())
		}
	}()
	return out, WarmState{}, nil
}

// DeployState devuelve el estado de un deploy: de memoria si esta, y si no derivado de los Jobs
// del run (etiqueta aqs.io/run-id). El estado es memoria + Jobs; no hay otra persistencia.
func (s *Service) DeployState(ctx context.Context, runID string) (DeployStatus, bool, error) {
	s.mu.Lock()
	d, ok := s.deploys[runID]
	if ok {
		out := *d
		s.mu.Unlock()
		return out, true, nil
	}
	s.mu.Unlock()
	jobs, err := s.Jobs.List(ctx, runID)
	if err != nil {
		return DeployStatus{}, false, err
	}
	if len(jobs) == 0 {
		return DeployStatus{}, false, nil
	}
	return deriveStatus(runID, jobs), true, nil
}

func deriveStatus(runID string, jobs []JobView) DeployStatus {
	st := DeployStatus{RunID: runID, Attempts: len(jobs), State: "failed"}
	for _, j := range jobs {
		switch j.Phase {
		case JobSucceeded:
			st.State = "done"
			st.Reason = ""
			return st
		case JobPending:
			st.State, st.Reason = "pending", ""
		case JobFailed:
			if st.State == "failed" {
				st.Reason = "el Job fallo: " + j.Reason
			}
		}
	}
	return st
}

// ResolveOrphans cierra, al arrancar, los deploys que quedaron sin observador (reinicio o SIGTERM
// con un deploy en vuelo). Eleccion fail-closed y simple: no se retoma la observacion; un Job aun en
// vuelo se da por indeterminado y se emite deploy.failed + handoff (el warm sigue dirty, asi que nada
// se despliega encima); un Job terminado reemite su evento terminal (event_id determinista: el
// outbox deduplica). El warm_id sale del estado persistido.
func (s *Service) ResolveOrphans(ctx context.Context, trace string) error {
	jobs, err := s.Jobs.List(ctx, "")
	if err != nil {
		return err
	}
	w, err := s.State.Get(ctx)
	if err != nil {
		return err
	}
	byRun := map[string][]JobView{}
	for _, j := range jobs {
		byRun[j.RunID] = append(byRun[j.RunID], j)
	}
	var errs []error
	for run, js := range byRun {
		s.mu.Lock()
		_, known := s.deploys[run]
		s.mu.Unlock()
		if known {
			continue
		}
		st := deriveStatus(run, js)
		a := js[len(js)-1].Artifact
		switch st.State {
		case "done":
			s.track(run, "done", st.Attempts, "")
			errs = append(errs, s.Pub.Publish(ctx, newEvent("deploy.done", run, trace, s.Clock.Now(), deployData(w.WarmID, run, a, ""))))
		case "pending":
			s.logger().Warn("deploy huerfano tras reinicio: se da por indeterminado", "run_id", run, "trace_id", trace)
			errs = append(errs, s.closeFailed(ctx, run, w.WarmID, a, trace, st.Attempts, "deploy en vuelo huerfano tras reinicio de go-warm-manager: estado indeterminado", true))
		default:
			s.track(run, "failed", st.Attempts, st.Reason)
			errs = append(errs, s.Pub.Publish(ctx, newEvent("deploy.failed", run, trace, s.Clock.Now(), deployData(w.WarmID, run, a, st.Reason))))
		}
	}
	return errors.Join(errs...)
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

// StatusReadRetries es el tope de lecturas fallidas seguidas del estado de un Job antes de darlo por indeterminado.
const StatusReadRetries = 5

// Deploy ejecuta el deploy de forma sincrona: toma el warm (ready->dirty atomico), crea el Job
// deploy-{run}-0 y, si falla, hasta MaxRetries reintentos. Agotados, o ante un intento
// indeterminado (no se pudo crear/leer el Job): deploy.failed + handoff, sin Job adicional.
func (s *Service) Deploy(ctx context.Context, runID string, a Artifact, trace string) error {
	if err := ValidateRunID(runID); err != nil {
		return err
	}
	if err := ValidateArtifact(a, s.Cfg.Job.AllowedRegistries); err != nil {
		return err
	}
	dirty, w, err := s.takeWarm(ctx)
	if err != nil {
		reason := "no se pudo tomar el warm: " + err.Error()
		return errors.Join(err, s.fail(ctx, runID, w.WarmID, a, trace, 0, reason, false))
	}
	s.obs().WarmState(dirty.State)
	s.track(runID, "pending", 0, "")

	var reason string
	attempts := 0
	for n := 0; n <= MaxRetries; n++ {
		m, err := BuildDeployJob(s.Cfg.Job, runID, n, a)
		if err != nil {
			return s.fail(ctx, runID, dirty.WarmID, a, trace, attempts, err.Error(), true)
		}
		attempts = n + 1
		s.track(runID, "pending", attempts, "")
		if err := s.Jobs.Create(ctx, m); err != nil {
			// Un error al crear no prueba que el Job no exista: no se crea otro, se falla cerrado.
			s.obs().DeployAttempt("error")
			return s.fail(ctx, runID, dirty.WarmID, a, trace, attempts, "intento indeterminado, no se pudo crear el Job: "+err.Error(), true)
		}
		phase, why, err := s.waitJob(ctx, JobName(runID, n))
		if err != nil {
			s.obs().DeployAttempt("error")
			return s.fail(ctx, runID, dirty.WarmID, a, trace, attempts, "intento indeterminado, no se pudo leer el Job: "+err.Error(), true)
		}
		if phase == JobSucceeded {
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
		s.obs().DeployAttempt("failure")
		reason = "el Job fallo: " + why
	}
	if reason == "" {
		reason = "deploy fallido"
	}
	return s.fail(ctx, runID, dirty.WarmID, a, trace, attempts, reason, true)
}

// takeWarm hace ready->dirty de forma atomica (mutex del servicio y compare-and-swap del almacen).
// Devuelve el estado previo en w cuando falla.
func (s *Service) takeWarm(ctx context.Context) (dirty, w WarmState, err error) {
	s.takeMu.Lock()
	defer s.takeMu.Unlock()
	w, err = s.State.Get(ctx)
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
	if err := s.State.CompareAndSwap(ctx, w, dirty); err != nil {
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

func (s *Service) waitJob(ctx context.Context, name string) (JobPhase, string, error) {
	timeout := s.Cfg.JobTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	deadline := s.Clock.Now().Add(timeout)
	readFails := 0
	for {
		p, why, err := s.Jobs.Status(ctx, name)
		if err != nil {
			// Error de lectura: el Job puede seguir vivo. Se relee con tope; no cuenta como intento.
			if readFails++; readFails >= StatusReadRetries {
				return "", "", err
			}
		} else {
			readFails = 0
			if p != JobPending {
				return p, why, nil
			}
		}
		if !s.Clock.Now().Before(deadline) {
			return "", "", fmt.Errorf("timeout esperando al Job %s", name)
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

// Errores de la superficie.
var (
	ErrDeployNotDone      = errors.New("el deploy de la corrida no ha terminado bien")
	ErrSurfaceUnreachable = errors.New("la app del warm no responde a ningun sondeo")
	ErrNoSurface          = errors.New("no se encontro ningun endpoint")
)

// InferSurface infiere la superficie consultando solo el exterior del Service del warm. Falla
// cerrado: exige el deploy de la corrida terminado y el warm tomado (dirty); si ningun sondeo
// responde o no hay endpoints, error (nunca surface.ready con 0 endpoints); valida el artefacto
// contra el esquema en tiempo de ejecucion antes de guardar o publicar.
func (s *Service) InferSurface(ctx context.Context, runID, trace string) (SurfaceArtifact, error) {
	if err := ValidateRunID(runID); err != nil {
		return SurfaceArtifact{}, err
	}
	d, ok, err := s.DeployState(ctx, runID)
	if err != nil {
		return SurfaceArtifact{}, err
	}
	if !ok || d.State != "done" {
		return SurfaceArtifact{}, fmt.Errorf("%w: run %s", ErrDeployNotDone, runID)
	}
	w, err := s.State.Get(ctx)
	if err != nil {
		return SurfaceArtifact{}, err
	}
	// Tras un deploy el warm esta dirty (lo usa la corrida); idle/cuarentena/ready no sirven de blanco.
	if w.State != StateDirty {
		return SurfaceArtifact{}, fmt.Errorf("%w: el warm esta %s", ErrNotReady, w.State)
	}
	sa := SurfaceArtifact{RunID: runID, BaseURL: s.Surface.BaseURL(), Endpoints: []Endpoint{}, Source: "probe"}
	answered := 0
	probe := func(p string) (int, []byte, bool) {
		st, body, err := s.Surface.Get(ctx, p)
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

// TrackKnownForTest marca un run como conocido en memoria (solo pruebas de ResolveOrphans).
func (s *Service) TrackKnownForTest(runID string) { s.track(runID, "pending", 1, "") }
