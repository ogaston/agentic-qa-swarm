package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// Config del servicio.
type Config struct {
	DefaultWarmID   string
	BaselineVersion string
	ReadyTimeout    time.Duration
	ReadyInterval   time.Duration
	Grace           time.Duration
}

// Service orquesta reset, idle, higiene y rebuild.
type Service struct {
	Kube     KubeAPI
	DB       DatabaseCleaner
	Cache    CacheFlusher
	State    StateStore
	Sessions SessionStore
	Events   EventPublisher
	Alert    Alerter
	Clock    Clock
	Metrics  Metrics
	Log      *slog.Logger // opcional
	Cfg      Config
	mu       sync.Mutex
	baseline string // versión del baseline del último intento verificado
}

// Result es el resultado de un reset o rebuild.
type Result struct {
	Verified bool
	State    WarmState
	Checks   []string
	Attempts int
	Reason   string
}

const (
	maxAttempts     = 2             // 1 intento + 1 reintento (US-M7.1)
	unknownBaseline = "desconocida" // solo en estados no verificados; nunca llega a un evento
)

func (s *Service) metrics() Metrics {
	if s.Metrics == nil {
		return NopMetrics{}
	}
	return s.Metrics
}

func (s *Service) log() *slog.Logger {
	if s.Log == nil {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return s.Log
}

// unreadable registra un warm-state ilegible (log + aqs_warm_state_unreadable_total).
func (s *Service) unreadable(where string) {
	s.log().Error("warm-state ilegible: se trata como desconocido (dirty, no verificado)", "where", where)
	s.metrics().StateUnreadable()
}

// current devuelve el estado del warm. Un error transitorio de la API se devuelve como error (no se
// asume nada); un contenido ilegible o un almacén ausente se interpretan como desconocido => dirty.
func (s *Service) current(ctx context.Context) (WarmState, error) {
	snap, ok, err := s.State.Get(ctx)
	if err != nil {
		return WarmState{}, err
	}
	if ok && snap.Unreadable {
		s.unreadable("current")
		ok = false
	}
	if !ok {
		bv := s.Cfg.BaselineVersion
		if bv == "" {
			bv = unknownBaseline
		}
		return WarmState{WarmID: s.Cfg.DefaultWarmID, State: StateDirty, BaselineVersion: bv}, nil
	}
	return snap.WarmState, nil
}

// Reset ejecuta el reset verificado de la corrida runID.
func (s *Service) Reset(ctx context.Context, runID, traceID string) (Result, error) {
	if runID == "" {
		return Result{}, errors.New("run_id vacío")
	}
	return s.run(ctx, "reset", runID, traceID, s.restart)
}

// Rebuild reconstruye el warm desde la imagen base (mode "rebuild") o lo destruye y reprovisiona ("teardown").
func (s *Service) Rebuild(ctx context.Context, mode, traceID string) (Result, error) {
	var first func(context.Context) error
	switch mode {
	case "rebuild":
		first = s.Kube.Rebuild
	case "teardown":
		first = s.Kube.Teardown
	default:
		return Result{}, fmt.Errorf("modo %q desconocido", mode)
	}
	return s.run(ctx, mode, "", traceID, first)
}

func (s *Service) restart(ctx context.Context) error {
	n, err := s.Kube.Replicas(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return s.Kube.ScaleApp(ctx, 1)
	}
	return s.Kube.RestartApp(ctx)
}

func (s *Service) run(ctx context.Context, kind, runID, traceID string, first func(context.Context) error) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.current(ctx)
	if err != nil {
		return Result{}, err
	}
	if st.State != StateQuarantine {
		st.State = StateDirty
	}
	st.ResetVerified = false
	if err := s.State.Put(ctx, st, s.Clock.Now()); err != nil {
		return Result{}, err
	}
	var checks []string
	var lastErr error
	attempts := 0
	for attempts < maxAttempts {
		attempts++
		checks, lastErr = s.attempt(ctx, first)
		if lastErr == nil {
			break
		}
	}
	res := Result{Attempts: attempts}
	if traceID == "" {
		traceID = "tr-" + kindID(kind, runID, st.WarmID)
	}
	if lastErr != nil {
		st.State, st.ResetVerified = StateQuarantine, false
		putErr := s.State.Put(ctx, st, s.Clock.Now())
		reason := lastErr.Error()
		if putErr != nil { // el estado queda dirty (seguro), pero el aviso no se pierde
			s.log().Error("no se pudo escribir la cuarentena", "err", putErr)
			reason += " (además no se pudo escribir la cuarentena: " + putErr.Error() + ")"
		}
		s.Alert.Quarantine(ctx, st.WarmID, runID, reason)
		s.metrics().Reset("quarantined")
		res.State, res.Reason = st, reason
		if putErr != nil {
			return res, putErr
		}
		return res, nil
	}
	st.State, st.ResetVerified, st.BaselineVersion = StateReady, true, s.baseline
	if err := s.State.Put(ctx, st, s.Clock.Now()); err != nil {
		return res, err
	}
	var ev Event
	if kind == "reset" {
		ev = ResetVerifiedEvent(runID, st.WarmID, traceID, checks, s.Clock.Now())
	} else {
		ev = TeardownVerifiedEvent(st.WarmID, kind, traceID, s.Clock.Now())
	}
	res.Verified, res.State, res.Checks = true, st, checks
	if err := s.Events.Publish(ctx, ev); err != nil {
		return res, fmt.Errorf("publicar %s: %w", ev.Type, err)
	}
	s.metrics().Reset("verified") // solo tras un Publish exitoso
	return res, nil
}

func kindID(kind, runID, warm string) string {
	if runID != "" {
		return runID
	}
	return kind + "-" + warm
}

func (s *Service) attempt(ctx context.Context, first func(context.Context) error) ([]string, error) {
	if err := first(ctx); err != nil {
		return nil, fmt.Errorf("restart: %w", err)
	}
	if err := s.DB.Clean(ctx); err != nil {
		return nil, fmt.Errorf("db clean: %w", err)
	}
	if err := s.Cache.Flush(ctx); err != nil {
		return nil, fmt.Errorf("cache flush: %w", err)
	}
	checks, err := s.Verify(ctx)
	if err != nil {
		return nil, err
	}
	v, err := s.DB.Version(ctx)
	if err != nil || v == "" {
		return nil, fmt.Errorf("versión del baseline desconocida: no se publica reset.verified con una inventada (%v)", err)
	}
	s.baseline = v
	return checks, nil
}

// Verify lee de vuelta el estado real: pod Ready, filas de la DB frente al baseline y DBSIZE.
// No mira el resultado de los pasos anteriores.
func (s *Service) Verify(ctx context.Context) ([]string, error) {
	var checks []string
	var fails []string
	deadline := s.Clock.Now().Add(s.Cfg.ReadyTimeout)
	ready := false
	for {
		ok, err := s.Kube.AppReady(ctx)
		if err != nil {
			fails = append(fails, CheckAppReady+": "+err.Error())
			break
		}
		if ok {
			ready = true
			break
		}
		if !s.Clock.Now().Before(deadline) {
			fails = append(fails, CheckAppReady+": pod no Ready")
			break
		}
		if err := s.Clock.Sleep(ctx, s.Cfg.ReadyInterval); err != nil {
			return nil, err
		}
	}
	if ready {
		checks = append(checks, CheckAppReady)
	}
	if n, err := s.DB.Diff(ctx); err != nil {
		fails = append(fails, CheckDBBaseline+": "+err.Error())
	} else if n != 0 {
		fails = append(fails, fmt.Sprintf("%s: %d filas difieren del baseline", CheckDBBaseline, n))
	} else {
		checks = append(checks, CheckDBBaseline)
	}
	if n, err := s.Cache.DBSize(ctx); err != nil {
		fails = append(fails, CheckCacheEmpty+": "+err.Error())
	} else if n != 0 {
		fails = append(fails, fmt.Sprintf("%s: %d claves", CheckCacheEmpty, n))
	} else {
		checks = append(checks, CheckCacheEmpty)
	}
	if len(fails) > 0 {
		return nil, fmt.Errorf("verificación fallida: %v", fails)
	}
	return checks, nil
}

// IdleCheck escala el warm a minReplicasIdle si lleva idleScaleDownAfter sin corrida activa.
// Devuelve true si escaló. Nunca escala un warm que no esté ready ni uno con corrida activa.
//
// Orden (F-11): primero se escribe idle-escalado y luego se escala, de modo que en ningún instante el
// almacén diga ready con 0 réplicas. Si el escalado falla se compensa (ver compensate).
func (s *Service) IdleCheck(ctx context.Context) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, ok, err := s.State.Get(ctx)
	if err != nil || !ok {
		return false, err
	}
	if snap.Unreadable {
		s.unreadable("IdleCheck")
		return false, nil // fail-closed: no se escala con un estado que no se entiende
	}
	if snap.State != StateReady {
		return false, nil
	}
	pol, err := s.Kube.WarmPolicy(ctx)
	if err != nil {
		return false, err
	}
	if pol.IdleScaleDownAfter <= 0 {
		err := fmt.Errorf("idleScaleDownAfter %v inválido (debe ser > 0): no se escala", pol.IdleScaleDownAfter)
		s.log().Error("warm-policy inválida", "err", err)
		return false, err
	}
	sessions, err := s.Sessions.List(ctx)
	if err != nil {
		return false, err
	}
	if snap.UpdatedAt.IsZero() {
		return false, nil // updated_at ausente o ilegible: fail-closed, no se sabe desde cuándo está idle
	}
	last := snap.UpdatedAt
	for _, se := range sessions {
		if se.Status == SessionActive {
			return false, nil
		}
		if se.LastActivity.After(last) {
			last = se.LastActivity
		}
	}
	if s.Clock.Now().Sub(last) < pol.IdleScaleDownAfter {
		return false, nil
	}
	ready := snap.WarmState
	idle := ready
	idle.State = StateIdle
	if err := s.State.Put(ctx, idle, s.Clock.Now()); err != nil {
		return false, err // nada se escaló: el almacén sigue diciendo ready con las réplicas intactas
	}
	if err := s.Kube.ScaleApp(ctx, pol.MinReplicasIdle); err != nil {
		return false, s.compensate(ctx, ready, err)
	}
	s.metrics().IdleScaled()
	return true, nil
}

// compensate deshace el estado idle-escalado cuando el escalado falló: vuelve a ready solo si el warm
// sigue con >= 1 réplica verificada (Replicas y AppReady leídos de vuelta); si no, lo deja dirty.
// Si ni siquiera puede escribir, el almacén queda en idle-escalado, que nunca es un ready falso.
func (s *Service) compensate(ctx context.Context, ready WarmState, scaleErr error) error {
	back := ready
	n, err := s.Kube.Replicas(ctx)
	ok := false
	if err == nil && n >= 1 {
		ok, err = s.Kube.AppReady(ctx)
	}
	if err != nil || !ok {
		back.State, back.ResetVerified = StateDirty, false
	}
	if perr := s.State.Put(ctx, back, s.Clock.Now()); perr != nil {
		s.log().Error("IdleCheck: no se pudo compensar el estado", "err", perr)
		return fmt.Errorf("escalar: %w; compensar: %v", scaleErr, perr)
	}
	return scaleErr
}

// HousekeepingReport resume una pasada de higiene.
type HousekeepingReport struct {
	Closed    []string
	ResetRun  bool
	WarmState string
}

// Housekeeping cierra sesiones abandonadas (> Grace sin actividad) y deja el warm en ready o cuarentena, nunca dirty.
func (s *Service) Housekeeping(ctx context.Context) (HousekeepingReport, error) {
	var rep HousekeepingReport
	sessions, err := s.Sessions.List(ctx)
	if err != nil {
		return rep, err
	}
	now := s.Clock.Now()
	var abandoned []Session
	fresh := false
	for _, se := range sessions {
		if se.Status != SessionActive {
			continue
		}
		if now.Sub(se.LastActivity) > s.Cfg.Grace {
			abandoned = append(abandoned, se)
		} else {
			fresh = true
		}
	}
	sort.Slice(abandoned, func(i, j int) bool { return abandoned[i].RunID < abandoned[j].RunID })
	st, err := s.current(ctx)
	if err != nil {
		return rep, err
	}
	rep.WarmState = st.State
	needReset := !fresh && (len(abandoned) > 0 || st.State == StateDirty)
	if needReset {
		id := "housekeeping"
		if len(abandoned) > 0 {
			id = abandoned[0].RunID
		}
		res, err := s.Reset(ctx, id, "")
		if err != nil {
			return rep, err
		}
		rep.ResetRun, rep.WarmState = true, res.State.State
	}
	for _, se := range abandoned {
		t := now
		se.Status, se.ClosedAt, se.WarmState = SessionIncomplete, &t, rep.WarmState
		if err := s.Sessions.Save(ctx, se); err != nil {
			return rep, err
		}
		rep.Closed = append(rep.Closed, se.RunID)
	}
	s.metrics().SessionsClosed(len(rep.Closed))
	return rep, nil
}
