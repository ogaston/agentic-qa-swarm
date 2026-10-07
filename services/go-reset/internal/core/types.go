// Package core es el dominio de go-reset: reset verificado, cuarentena, idle,
// higiene y rebuild/teardown. No importa client-go ni net/http.
package core

import (
	"context"
	"encoding/json"
	"time"
)

// Estados del warm (contracts/plans/warm-state.schema.json).
const (
	StateReady      = "ready"
	StateDirty      = "dirty"
	StateQuarantine = "cuarentena"
	StateIdle       = "idle-escalado"
)

// Nombres de las comprobaciones de verificación.
const (
	CheckAppReady   = "app-ready"
	CheckDBBaseline = "db-baseline"
	CheckCacheEmpty = "cache-empty"
)

// Estados de sesión.
const (
	SessionActive     = "activa"
	SessionIncomplete = "incompleta"
	SessionDone       = "terminada"
)

// WarmState es el WarmState del contrato.
type WarmState struct {
	WarmID          string `json:"warm_id"`
	State           string `json:"state"`
	ResetVerified   bool   `json:"reset_verified"`
	BaselineVersion string `json:"baseline_version"`
}

// Snapshot es el estado más la hora de su última escritura.
type Snapshot struct {
	WarmState
	UpdatedAt time.Time
	// Unreadable: el almacén existe pero su contenido no se puede interpretar (F-10). El estado se
	// trata como desconocido: dirty y no verificado; nunca como ready.
	Unreadable bool
}

// Policy es la warm-policy (ConfigMap).
type Policy struct {
	IdleScaleDownAfter time.Duration
	MinReplicasIdle    int32
}

// Session es la sesión de una corrida.
type Session struct {
	RunID        string          `json:"run_id"`
	Status       string          `json:"status"`
	LastActivity time.Time       `json:"last_activity"`
	Plan         json.RawMessage `json:"plan,omitempty"`
	State        string          `json:"state,omitempty"`
	WarmState    string          `json:"warm_state,omitempty"`
	ClosedAt     *time.Time      `json:"closed_at,omitempty"`
}

// Event es un evento listo para el outbox.
type Event struct {
	EventID    string         `json:"event_id"`
	Type       string         `json:"type"`
	Version    int            `json:"version"`
	OccurredAt time.Time      `json:"occurred_at"`
	TraceID    string         `json:"trace_id"`
	Data       map[string]any `json:"data"`
}

// Puertos.
type (
	// KubeAPI opera solo sobre el namespace aqs-test.
	KubeAPI interface {
		Replicas(ctx context.Context) (int32, error)
		RestartApp(ctx context.Context) error
		ScaleApp(ctx context.Context, n int32) error
		AppReady(ctx context.Context) (bool, error)
		Rebuild(ctx context.Context) error
		Teardown(ctx context.Context) error
		WarmPolicy(ctx context.Context) (Policy, error)
	}
	// DatabaseCleaner limpia la DB al baseline y lee de vuelta cuántas filas difieren.
	DatabaseCleaner interface {
		Clean(ctx context.Context) error
		Diff(ctx context.Context) (int, error)
		// Version es la versión del baseline contra la que se limpió; error si no se conoce.
		Version(ctx context.Context) (string, error)
	}
	// CacheFlusher hace FLUSHALL y lee DBSIZE.
	CacheFlusher interface {
		Flush(ctx context.Context) error
		DBSize(ctx context.Context) (int64, error)
	}
	StateStore interface {
		Get(ctx context.Context) (Snapshot, bool, error)
		Put(ctx context.Context, s WarmState, at time.Time) error
	}
	SessionStore interface {
		Save(ctx context.Context, s Session) error
		List(ctx context.Context) ([]Session, error)
	}
	EventPublisher interface {
		Publish(ctx context.Context, e Event) error
	}
	Alerter interface {
		Quarantine(ctx context.Context, warmID, runID, reason string)
	}
	Clock interface {
		Now() time.Time
		Sleep(ctx context.Context, d time.Duration) error
	}
	Metrics interface {
		Reset(result string)
		IdleScaled()
		SessionsClosed(n int)
		StateUnreadable()
	}
)

// NopMetrics no hace nada.
type NopMetrics struct{}

func (NopMetrics) Reset(string)       {}
func (NopMetrics) IdleScaled()        {}
func (NopMetrics) SessionsClosed(int) {}
func (NopMetrics) StateUnreadable()   {}
