// Package core es el dominio de go-reset: reset verificado, cuarentena, idle,
// higiene y rebuild/teardown. No importa client-go ni net/http.
package core

import (
	"context"
	"encoding/json"
	"fmt"
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
	CheckAppReady      = "app-ready"
	CheckDBBaseline    = "db-baseline"
	CheckCacheEmpty    = "cache-empty"
	ResourcePostgreSQL = "postgresql"
	ResourceRedis      = "redis"
)

// NormalizeResources aplica el comportamiento legacy a una selección omitida.
func NormalizeResources(resources []string) []string {
	if resources == nil {
		return []string{ResourcePostgreSQL, ResourceRedis}
	}
	out := make([]string, 0, len(resources))
	for _, canonical := range []string{ResourcePostgreSQL, ResourceRedis} {
		for _, selected := range resources {
			if selected == canonical {
				out = append(out, canonical)
				break
			}
		}
	}
	return out
}

// HasResource aplica la selección sin confundir una lista vacía con una omitida.
func HasResource(resources []string, resource string) bool {
	for _, selected := range NormalizeResources(resources) {
		if selected == resource {
			return true
		}
	}
	return false
}

// ValidateResources rechaza dependencias que la plataforma no provisiona.
func ValidateResources(resources []string) error {
	seen := map[string]bool{}
	for _, resource := range resources {
		if resource != ResourcePostgreSQL && resource != ResourceRedis {
			return fmt.Errorf("recurso no soportado %q", resource)
		}
		if seen[resource] {
			return fmt.Errorf("recurso duplicado %q", resource)
		}
		seen[resource] = true
	}
	return nil
}

// Estados de sesión.
const (
	SessionActive     = "activa"
	SessionIncomplete = "incompleta"
	SessionDone       = "terminada"
)

// WarmState es el WarmState del contrato.
type WarmState struct {
	WarmID              string `json:"warm_id"`
	State               string `json:"state"`
	ResetVerified       bool   `json:"reset_verified"`
	BaselineVersion     string `json:"baseline_version"`
	InfrastructureReady bool   `json:"infrastructure_ready"`
	Namespace           string `json:"namespace,omitempty"`
	Repo                string `json:"repo,omitempty"`
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
	Resources          []string
}

// StatelessDatabase evita llamadas nil cuando PostgreSQL no fue seleccionado.
type StatelessDatabase struct{}

func (StatelessDatabase) Clean(context.Context) error       { return nil }
func (StatelessDatabase) Diff(context.Context) (int, error) { return 0, nil }
func (StatelessDatabase) Version(context.Context) (string, error) {
	return "stateless", nil
}

// StatelessCache evita llamadas nil cuando Redis no fue seleccionado.
type StatelessCache struct{}

func (StatelessCache) Flush(context.Context) error           { return nil }
func (StatelessCache) DBSize(context.Context) (int64, error) { return 0, nil }

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
	// KubeAPI opera sobre el namespace configurado (aqs-test o aqs-w-*).
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
