package principal

import "fmt"

// Role es el rol de un principal: user o admin.
type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// Principal es una identidad con un rol.
type Principal struct {
	ID   string `json:"id"`
	Role Role   `json:"role"`
}

// ParseRole acepta solo "user" y "admin", exactos (distingue mayúsculas y espacios).
func ParseRole(s string) (Role, error) {
	switch Role(s) {
	case RoleUser, RoleAdmin:
		return Role(s), nil
	}
	return "", fmt.Errorf("rol inválido %q", s)
}

// Validate exige ID no vacío y rol válido.
func (p Principal) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("id vacío")
	}
	_, err := ParseRole(string(p.Role))
	return err
}
