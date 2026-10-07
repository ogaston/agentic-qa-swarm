package core

import (
	"context"
	"errors"
	"fmt"
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
	Cfg      Config
	mu       sync.Mutex
}

// Result es el resultado de un reset o rebuild.
type Result struct {
	Verified bool
	State    WarmState
	Checks   []string
	Attempts int
	Reason   string
}

const maxAttempts = 2 // 1 intento + 1 reintento

func (s *Service) metrics() Metrics {
	if s.Metrics == nil {
		return NopMetrics{}
	}
	return s.Metrics
}

func (s *Service) current(ctx context.Context) (WarmState, error) {
	snap, ok, err := s.State.Get(ctx)
	if err != nil {
		return WarmState{}, err
	}
	if !ok {
		return WarmState{WarmID: s.Cfg.DefaultWarmID, State: StateDirty, BaselineVersion: s.Cfg.BaselineVersion}, nil
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
		if err := s.State.Put(ctx, st, s.Clock.Now()); err != nil {
			return res, err
		}
		s.Alert.Quarantine(ctx, st.WarmID, runID, lastErr.Error())
		s.metrics().Reset("quarantined")
		res.State, res.Reason = st, lastErr.Error()
		return res, nil
	}
	st.State, st.ResetVerified = StateReady, true
	if err := s.State.Put(ctx, st, s.Clock.Now()); err != nil {
		return res, err
	}
	var ev Event
	if kind == "reset" {
		ev = ResetVerifiedEvent(runID, st.WarmID, traceID, checks, s.Clock.Now())
	} else {
		ev = TeardownVerifiedEvent(st.WarmID, kind, traceID, s.Clock.Now())
	}
	s.metrics().Reset("verified")
	res.Verified, res.State, res.Checks = true, st, checks
	if err := s.Events.Publish(ctx, ev); err != nil {
		return res, fmt.Errorf("publicar %s: %w", ev.Type, err)
	}
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
	return s.Verify(ctx)
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
func (s *Service) IdleCheck(ctx context.Context) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, ok, err := s.State.Get(ctx)
	if err != nil || !ok || snap.State != StateReady {
		return false, err
	}
	pol, err := s.Kube.WarmPolicy(ctx)
	if err != nil {
		return false, err
	}
	sessions, err := s.Sessions.List(ctx)
	if err != nil {
		return false, err
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
	if err := s.Kube.ScaleApp(ctx, pol.MinReplicasIdle); err != nil {
		return false, err
	}
	snap.State = StateIdle
	if err := s.State.Put(ctx, snap.WarmState, s.Clock.Now()); err != nil {
		return false, err
	}
	s.metrics().IdleScaled()
	return true, nil
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
