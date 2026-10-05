package inbox

import "time"

// State es el estado de una notificacion (enum del OpenAPI).
type State string

// Estados validos. En este MVP solo se llega a pending y confirmed;
// rejected existe en el contrato pero ninguna ruta lo produce todavia.
const (
	StatePending   State = "pending"
	StateConfirmed State = "confirmed"
	StateRejected  State = "rejected"
)

// Valid indica si s es un valor del enum del contrato.
func (s State) Valid() bool {
	return s == StatePending || s == StateConfirmed || s == StateRejected
}

// Artifact es el artefacto a desplegar (forma de notify.created y del OpenAPI).
type Artifact struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
}

// Notification es la forma REST del OpenAPI (components.schemas.Notification).
type Notification struct {
	ID          string    `json:"id"`
	GithubEvent string    `json:"github_event"`
	Repo        string    `json:"repo"`
	SHA         string    `json:"sha"`
	Artifact    *Artifact `json:"artifact,omitempty"`
	State       State     `json:"state"`
}

// Receipt es el ConfirmationReceipt del OpenAPI.
type Receipt struct {
	RunID          string    `json:"run_id"`
	NotificationID string    `json:"notification_id"`
	ConfirmedBy    string    `json:"confirmed_by"`
	ConfirmedAt    time.Time `json:"confirmed_at"`
}
