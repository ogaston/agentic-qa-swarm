package authz

import (
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-identity/principal"
)

var (
	user  = principal.Principal{ID: "marta", Role: principal.RoleUser}
	admin = principal.Principal{ID: "julian", Role: principal.RoleAdmin}
)

func TestAuthZ_Table(t *testing.T) {
	cases := []struct {
		name string
		p    principal.Principal
		a    Action
		r    Resource
		want Decision
	}{
		{"user lee su sesion", user, ActionSessionRead, Resource{TypeSession, "s1", "marta"}, Allow},
		{"user no lee sesion ajena", user, ActionSessionRead, Resource{TypeSession, "s1", "luis"}, Deny},
		{"user no lee sesion sin propietario", user, ActionSessionRead, Resource{TypeSession, "s1", ""}, Deny},
		{"user propietario distingue mayusculas", user, ActionSessionRead, Resource{TypeSession, "s1", "Marta"}, Deny},
		{"admin lee sesion ajena", admin, ActionSessionRead, Resource{TypeSession, "s1", "marta"}, Allow},
		{"admin lee sesion sin propietario", admin, ActionSessionRead, Resource{TypeSession, "s1", ""}, Allow},
		{"admin lee su sesion", admin, ActionSessionRead, Resource{TypeSession, "s1", "julian"}, Allow},
		{"user no lista usuarios", user, ActionUsersList, Resource{Type: TypeUsers}, Deny},
		{"user propietario no lista usuarios", user, ActionUsersList, Resource{TypeUsers, "x", "marta"}, Deny},
		{"admin lista usuarios", admin, ActionUsersList, Resource{Type: TypeUsers}, Allow},
		{"accion desconocida user", user, "session:delete", Resource{TypeSession, "s1", "marta"}, Deny},
		{"accion desconocida admin", admin, "users:delete", Resource{Type: TypeUsers}, Deny},
		{"accion vacia admin", admin, "", Resource{TypeSession, "s1", "marta"}, Deny},
		{"tipo desconocido admin", admin, ActionSessionRead, Resource{Type: "repo", ID: "r"}, Deny},
		{"tipo vacio admin", admin, ActionUsersList, Resource{}, Deny},
		{"accion con tipo cruzado admin", admin, ActionSessionRead, Resource{Type: TypeUsers}, Deny},
		{"accion con tipo cruzado user", user, ActionUsersList, Resource{TypeSession, "s", "marta"}, Deny},
		{"rol vacio", principal.Principal{ID: "x"}, ActionSessionRead, Resource{TypeSession, "s", "x"}, Deny},
		{"rol desconocido", principal.Principal{ID: "x", Role: "root"}, ActionUsersList, Resource{Type: TypeUsers}, Deny},
		{"rol con mayusculas", principal.Principal{ID: "x", Role: "Admin"}, ActionUsersList, Resource{Type: TypeUsers}, Deny},
		{"id vacio con rol admin", principal.Principal{Role: principal.RoleAdmin}, ActionUsersList, Resource{Type: TypeUsers}, Deny},
		{"principal vacio", principal.Principal{}, ActionSessionRead, Resource{TypeSession, "s", ""}, Deny},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Authorize(c.p, c.a, c.r); got != c.want {
				t.Fatalf("Authorize = %v, esperado %v", got, c.want)
			}
		})
	}
}

func TestAuthZ_ZeroDecisionIsDeny(t *testing.T) {
	var d Decision
	if d != Deny {
		t.Fatal("el valor cero debe ser Deny")
	}
}
