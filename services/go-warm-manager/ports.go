package warmmanager

import (
	"context"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/warmtarget"
)

// StateStore persiste el WarmState (adaptador Kubernetes: ConfigMap warm-state).
type StateStore interface {
	Get(ctx context.Context) (WarmState, error)
	Put(ctx context.Context, s WarmState) error
	// CompareAndSwap escribe next solo si el estado persistido sigue siendo expect;
	// si otro lo cambió devuelve ErrStateConflict (nunca pisa).
	CompareAndSwap(ctx context.Context, expect, next WarmState) error
}

// TargetStateStore añade soporte de múltiples entornos warm a StateStore.
type TargetStateStore interface {
	StateStore
	GetForTarget(ctx context.Context, target warmtarget.WarmTarget) (WarmState, error)
	PutForTarget(ctx context.Context, target warmtarget.WarmTarget, s WarmState) error
	CompareAndSwapForTarget(ctx context.Context, target warmtarget.WarmTarget, expect, next WarmState) error
	LookupState(ctx context.Context, idOrNS string) (WarmState, error)
}

// HealthProbe comprueba app, DB y Redis del warm. nil = todo en verde.
type HealthProbe interface {
	Check(ctx context.Context) error
}

// TargetHealthProbe añade comprobación por namespace a HealthProbe.
type TargetHealthProbe interface {
	HealthProbe
	CheckInNamespace(ctx context.Context, ns string) error
}

// WarmRuntime escala el warm (solo sale de idle-escalado; el scale-down es de go-reset).
type WarmRuntime interface {
	ScaleUp(ctx context.Context) error
}

// TargetWarmRuntime añade escalado por namespace a WarmRuntime.
type TargetWarmRuntime interface {
	WarmRuntime
	ScaleUpInNamespace(ctx context.Context, ns string) error
}

// Deployer parchea la imagen del Deployment warm-app y consulta su rollout (deploy en proceso).
type Deployer interface {
	// SetImage cambia la imagen del contenedor warm-app a ref (parche; falla si el contenedor no existe).
	SetImage(ctx context.Context, ref string) error
	// RolloutComplete dice si el rollout de ref esta COMPLETO: generacion observada, replicas
	// actualizadas y disponibles, sin pods extra, todos con la imagen ref y Ready.
	RolloutComplete(ctx context.Context, ref string) (bool, error)
}

// TargetDeployer añade despliegue por namespace a Deployer.
type TargetDeployer interface {
	Deployer
	SetImageInNamespace(ctx context.Context, ns, ref string) error
	RolloutCompleteInNamespace(ctx context.Context, ns, ref string) (bool, error)
}

// SurfaceProber consulta SOLO el exterior del Service del warm (nunca el código fuente).
type SurfaceProber interface {
	BaseURL() string
	Get(ctx context.Context, path string) (status int, body []byte, err error)
}

// TargetSurfaceProber añade sondeo con URL base personalizada a SurfaceProber.
type TargetSurfaceProber interface {
	SurfaceProber
	GetWithBaseURL(ctx context.Context, baseURL, path string) (status int, body []byte, err error)
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
