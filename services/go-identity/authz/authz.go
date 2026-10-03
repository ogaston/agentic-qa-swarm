// Package authz decide si un principal puede ejecutar una acción sobre un recurso.
// Es deny-by-default: toda combinación no listada explícitamente se deniega.
package authz

import "github.com/ogaston/agentic-qa-swarm/services/go-identity/principal"

// Action es una acción autorizable.
type Action string

// ResourceType es el tipo de un recurso.
type ResourceType string

// Acciones y tipos conocidos.
const (
	ActionSessionRead Action = "session:read"
	ActionUsersList   Action = "users:list"

	TypeSession ResourceType = "session"
	TypeUsers   ResourceType = "users"
)

// Resource identifica el recurso. Owner es el ID del principal propietario ("" si no tiene: no es de nadie).
type Resource struct {
	Type  ResourceType
	ID    string
	Owner string
}

// Decision es el resultado; su valor cero es Deny.
type Decision bool

// Valores de Decision.
const (
	Deny  Decision = false
	Allow Decision = true
)

// Authorize aplica la tabla de permisos. Principal inválido, acción, tipo o rol desconocidos → Deny.
//
//	session:read / session: user solo si es el propietario; admin sí.
//	users:list   / users:   user no; admin sí.
func Authorize(p principal.Principal, action Action, res Resource) Decision {
	if p.Validate() != nil {
		return Deny
	}
	switch {
	case action == ActionSessionRead && res.Type == TypeSession:
		switch p.Role {
		case principal.RoleAdmin:
			return Allow
		case principal.RoleUser:
			return Decision(res.Owner != "" && res.Owner == p.ID)
		}
	case action == ActionUsersList && res.Type == TypeUsers:
		return Decision(p.Role == principal.RoleAdmin)
	}
	return Deny
}
