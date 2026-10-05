// Package intake implementa el webhook de GitHub de go-intake: verificacion de
// firma, clasificacion, notificacion pendiente, persistencia y publicacion de
// notify.created. El almacen JSONL y el outbox son soluciones de transicion
// detras de puertos hasta que C-45 decida transporte y persistencia.
package intake

import "context"

// Valores de Notification.state y github_event segun contracts/openapi.
const (
	StatePending = "pending"

	EventCommit      = "commit"
	EventPullRequest = "pull_request"
	EventTag         = "tag"

	ArtifactBuildFromRepo = "build-from-repo"
)

// Artifact es la referencia al artefacto a desplegar.
type Artifact struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
}

// Notification es el esquema REST Notification (contracts/openapi).
type Notification struct {
	ID          string    `json:"id"`
	GithubEvent string    `json:"github_event"`
	Repo        string    `json:"repo"`
	SHA         string    `json:"sha"`
	State       string    `json:"state"`
	Artifact    *Artifact `json:"artifact,omitempty"`
}

// Record es lo que persiste el almacen: la notificacion mas el estado interno.
type Record struct {
	Notification
	DeliveryID     string `json:"delivery_id"`
	PublishPending bool   `json:"publish_pending"`
}

// NotificationStore es el puerto de persistencia. Put agrega o reemplaza (por
// ID) el registro; GetByDelivery consulta el indice delivery_id -> registro.
type NotificationStore interface {
	GetByDelivery(deliveryID string) (Record, bool)
	Put(rec Record) error
}

// EventData es el campo data de notify.created.
type EventData struct {
	NotificationID string   `json:"notification_id"`
	GithubEvent    string   `json:"github_event"`
	Repo           string   `json:"repo"`
	SHA            string   `json:"sha"`
	Artifact       Artifact `json:"artifact"`
}

// Event es el sobre notify.created v1.
type Event struct {
	EventID    string    `json:"event_id"`
	Type       string    `json:"type"`
	Version    int       `json:"version"`
	OccurredAt string    `json:"occurred_at"`
	TraceID    string    `json:"trace_id"`
	Data       EventData `json:"data"`
}

// EventPublisher es el puerto de publicacion de eventos.
type EventPublisher interface {
	Publish(ctx context.Context, ev Event) error
}

// ArtifactResolver es el puerto de resolucion de artefacto. U1-T03 lo
// reemplaza por la resolucion real (V5).
type ArtifactResolver interface {
	Resolve(ctx context.Context, repo, sha string) (Artifact, error)
}

// StubResolver devuelve siempre build-from-repo con ref <repo>@<sha>.
type StubResolver struct{}

// Resolve implementa ArtifactResolver.
func (StubResolver) Resolve(_ context.Context, repo, sha string) (Artifact, error) {
	return Artifact{Kind: ArtifactBuildFromRepo, Ref: repo + "@" + sha}, nil
}
