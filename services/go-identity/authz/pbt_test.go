package authz_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/go-identity/authz"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/gen"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/principal"
)

// oracle es la tabla de permisos escrita a mano desde la especificación (deny-by-default).
func oracle(p principal.Principal, a authz.Action, r authz.Resource) authz.Decision {
	if p.ID == "" || (p.Role != "user" && p.Role != "admin") {
		return authz.Deny
	}
	switch {
	case a == "session:read" && r.Type == "session":
		return authz.Decision(p.Role == "admin" || (r.Owner != "" && r.Owner == p.ID))
	case a == "users:list" && r.Type == "users":
		return authz.Decision(p.Role == "admin")
	}
	return authz.Deny
}

func TestPBT_Invariant_AuthZ(t *testing.T) {
	gen.AtLeastChecks(t, 2000)
	rapid.Check(t, func(t *rapid.T) {
		p := gen.Principal().Draw(t, "principal")
		a := gen.Action().Draw(t, "action")
		r := gen.ResourceFor(p).Draw(t, "resource")
		got := authz.Authorize(p, a, r)
		if want := oracle(p, a, r); got != want {
			t.Fatalf("Authorize(%+v, %q, %+v)=%v, la tabla dice %v", p, a, r, got, want)
		}
		if got == authz.Allow {
			// nunca se permite con rol, acción o tipo desconocidos, ni a un user lo reservado a admin
			if (p.Role != "user" && p.Role != "admin") || p.ID == "" {
				t.Fatalf("Allow a un principal inválido: %+v", p)
			}
			if a != authz.ActionSessionRead && a != authz.ActionUsersList {
				t.Fatalf("Allow con acción desconocida %q", a)
			}
			if r.Type != authz.TypeSession && r.Type != authz.TypeUsers {
				t.Fatalf("Allow con tipo desconocido %q", r.Type)
			}
			if p.Role == "user" && (a == authz.ActionUsersList || r.Owner == "" || r.Owner != p.ID) {
				t.Fatalf("Allow a un user fuera de lo suyo: %+v %q %+v", p, a, r)
			}
		}
		// un user nunca obtiene lo que un admin sin propiedad no obtendría de otro modo: users:list es solo admin.
		if p.Role == "user" && a == authz.ActionUsersList && got == authz.Allow {
			t.Fatalf("users:list es solo admin")
		}
	})
}

// Ejemplos fijos de la tabla.
func TestPBT_Examples_AuthZ(t *testing.T) {
	u1 := principal.Principal{ID: "u1", Role: principal.RoleUser}
	adm := principal.Principal{ID: "a", Role: principal.RoleAdmin}
	cases := []struct {
		p    principal.Principal
		a    authz.Action
		r    authz.Resource
		want authz.Decision
	}{
		{u1, authz.ActionSessionRead, authz.Resource{Type: authz.TypeSession, Owner: "u1"}, authz.Allow},
		{u1, authz.ActionSessionRead, authz.Resource{Type: authz.TypeSession, Owner: "u2"}, authz.Deny},
		{u1, authz.ActionSessionRead, authz.Resource{Type: authz.TypeSession}, authz.Deny},
		{u1, authz.ActionUsersList, authz.Resource{Type: authz.TypeUsers}, authz.Deny},
		{adm, authz.ActionUsersList, authz.Resource{Type: authz.TypeUsers}, authz.Allow},
		{adm, authz.ActionSessionRead, authz.Resource{Type: authz.TypeSession}, authz.Allow},
		{adm, "session:write", authz.Resource{Type: authz.TypeSession}, authz.Deny},
		{principal.Principal{ID: "u1", Role: "Admin"}, authz.ActionUsersList, authz.Resource{Type: authz.TypeUsers}, authz.Deny},
	}
	for i, c := range cases {
		if got := authz.Authorize(c.p, c.a, c.r); got != c.want {
			t.Errorf("caso %d: %v, esperado %v", i, got, c.want)
		}
	}
}
