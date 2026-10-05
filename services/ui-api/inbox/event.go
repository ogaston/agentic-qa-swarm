package inbox

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"time"
)

// NotifyCreated es el evento notify.created v1 (contracts/events).
type NotifyCreated struct {
	EventID    string    `json:"event_id"`
	Type       string    `json:"type"`
	Version    int       `json:"version"`
	OccurredAt time.Time `json:"occurred_at"`
	TraceID    string    `json:"trace_id"`
	Data       struct {
		NotificationID string    `json:"notification_id"`
		GithubEvent    string    `json:"github_event"`
		Repo           string    `json:"repo"`
		SHA            string    `json:"sha"`
		Artifact       *Artifact `json:"artifact"`
	} `json:"data"`
}

var (
	uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	shaRe  = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// ErrInvalidEvent marca un evento que no cumple notify.created.schema.json.
var ErrInvalidEvent = errors.New("evento notify.created invalido")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidEvent, fmt.Sprintf(format, a...))
}

// exactKeys exige que el objeto tenga EXACTAMENTE las claves de want, con
// los nombres sensibles a mayusculas (encoding/json las casa sin distinguir
// mayusculas, a diferencia del esquema). Devuelve los valores crudos.
func exactKeys(raw []byte, where string, want ...string) (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return nil, invalid("%s: no es un objeto", where)
	}
	if len(m) != len(want) {
		return nil, invalid("%s: claves distintas de las del esquema", where)
	}
	for _, k := range want {
		if _, ok := m[k]; !ok {
			return nil, invalid("%s: falta la clave %q (nombres exactos)", where, k)
		}
	}
	return m, nil
}

// ParseNotifyCreated decodifica y valida una linea contra el esquema
// contracts/events/notify.created.schema.json (propiedades requeridas, enums,
// patrones y additionalProperties=false), con nombres de campo exactos
// (sensibles a mayusculas) en la raiz, en data y en artifact. La prueba de
// contrato compara este validador con el esquema real. Nunca incluye el
// contenido en el error.
//
// Es a proposito MAS estricto que el esquema en dos puntos menores, que se
// aceptan asi: version debe ser el entero 1 (rechaza 1.0 y 1e0) y occurred_at
// debe ser RFC 3339 de Go (rechaza "t"/"z" en minuscula).
func ParseNotifyCreated(line []byte) (NotifyCreated, error) {
	var ev NotifyCreated
	root, err := exactKeys(line, "evento", "event_id", "type", "version", "occurred_at", "trace_id", "data")
	if err != nil {
		return ev, err
	}
	data, err := exactKeys(root["data"], "data", "notification_id", "github_event", "repo", "sha", "artifact")
	if err != nil {
		return ev, err
	}
	if _, err := exactKeys(data["artifact"], "data.artifact", "kind", "ref"); err != nil {
		return ev, err
	}
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ev); err != nil {
		return ev, invalid("json: %v", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return ev, invalid("datos tras el objeto")
	}
	// version debe ser exactamente 1 y los campos string no pueden faltar.
	switch {
	case !uuidRe.MatchString(ev.EventID):
		return ev, invalid("event_id")
	case ev.Type != "notify.created":
		return ev, invalid("type")
	case ev.Version != 1:
		return ev, invalid("version")
	case ev.OccurredAt.IsZero():
		return ev, invalid("occurred_at")
	case ev.TraceID == "":
		return ev, invalid("trace_id")
	case ev.Data.NotificationID == "":
		return ev, invalid("data.notification_id")
	case ev.Data.GithubEvent != "commit" && ev.Data.GithubEvent != "pull_request" && ev.Data.GithubEvent != "tag":
		return ev, invalid("data.github_event")
	case ev.Data.Repo == "":
		return ev, invalid("data.repo")
	case !shaRe.MatchString(ev.Data.SHA):
		return ev, invalid("data.sha")
	case ev.Data.Artifact == nil:
		return ev, invalid("data.artifact")
	case ev.Data.Artifact.Kind != "build-from-repo" && ev.Data.Artifact.Kind != "published-image":
		return ev, invalid("data.artifact.kind")
	case ev.Data.Artifact.Ref == "":
		return ev, invalid("data.artifact.ref")
	}
	return ev, nil
}
