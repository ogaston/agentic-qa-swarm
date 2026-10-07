package warmmanager

import (
	"context"
	"time"
)

// StateStore persiste el WarmState (adaptador Kubernetes: ConfigMap warm-state).
type StateStore interface {
	Get(ctx context.Context) (WarmState, error)
	Put(ctx context.Context, s WarmState) error
}

// HealthProbe comprueba app, DB y Redis del warm. nil = todo en verde.
type HealthProbe interface {
	Check(ctx context.Context) error
}

// WarmRuntime escala el warm (solo sale de idle-escalado; el scale-down es de go-reset).
type WarmRuntime interface {
	ScaleUp(ctx context.Context) error
}

// JobPhase es el estado de un Job.
type JobPhase string

const (
	JobPending   JobPhase = "pending"
	JobSucceeded JobPhase = "succeeded"
	JobFailed    JobPhase = "failed"
)

// Manifest es un manifiesto Kubernetes genérico (el dominio no importa client-go).
type Manifest map[string]any

// Jobs crea y consulta Jobs de deploy.
type Jobs interface {
	Create(ctx context.Context, m Manifest) error
	Status(ctx context.Context, name string) (JobPhase, string, error)
}

// ContainerRuntime resuelve el artefacto a una imagen/ref desplegable (build o pull).
type ContainerRuntime interface {
	Prepare(ctx context.Context, a Artifact) (string, error)
}

// SurfaceProber consulta SOLO el exterior del Service del warm (nunca el código fuente).
type SurfaceProber interface {
	BaseURL() string
	Get(ctx context.Context, path string) (status int, body []byte, err error)
}

// ObjectStore guarda objetos y devuelve su URI.
type ObjectStore interface {
	Put(ctx context.Context, key string, data []byte) (uri string, err error)
	Get(ctx context.Context, uri string) ([]byte, error)
}

// Alerter avisa a un humano (handoff) cuando se agotan los reintentos.
type Alerter interface {
	Handoff(ctx context.Context, phase, runID, reason string) error
}

// EventPublisher publica un evento ya serializado.
type EventPublisher interface {
	Publish(ctx context.Context, e Event) error
}

// Clock es el reloj inyectable.
type Clock interface {
	Now() time.Time
	Sleep(ctx context.Context, d time.Duration) error
}
