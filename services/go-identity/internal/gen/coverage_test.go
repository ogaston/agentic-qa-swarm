package gen_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/go-identity/authz"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/gen"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/passhash"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/principal"
)

// En 500 sorteos aparecen las clases de dominio de cada generador y las dos
// decisiones de Authorize para cada rol válido.
func TestPBT_GeneratorCoverage(t *testing.T) {
	gen.AtLeastChecks(t, 500)
	c := map[string]int{}
	n := 0
	rapid.Check(t, func(t *rapid.T) {
		n++
		p := gen.Principal().Draw(t, "p")
		a := gen.Action().Draw(t, "a")
		r := gen.ResourceFor(p).Draw(t, "r")
		d := authz.Authorize(p, a, r)
		switch {
		case p.ID == "":
			c["principal sin ID"]++
		}
		if _, err := principal.ParseRole(string(p.Role)); err != nil {
			c["rol inválido"]++
		} else {
			c["rol "+string(p.Role)]++
			if d == authz.Allow {
				c["Allow "+string(p.Role)]++
			} else {
				c["Deny "+string(p.Role)]++
			}
		}
		if a == authz.ActionSessionRead || a == authz.ActionUsersList {
			c["acción conocida"]++
		} else {
			c["acción desconocida"]++
		}
		switch {
		case r.Owner == "":
			c["sin propietario"]++
		case r.Owner == p.ID:
			c["propietario propio"]++
		default:
			c["propietario ajeno"]++
		}
		if r.Type == authz.TypeSession || r.Type == authz.TypeUsers {
			c["tipo conocido"]++
		} else {
			c["tipo desconocido"]++
		}
		if len(gen.Secret().Draw(t, "secret")) == 20 {
			c["secreto de 160 bits"]++
		}
		if len(gen.ShortSecret().Draw(t, "short")) < 20 {
			c["secreto corto"]++
		}
		if err := passhash.Validate(gen.PHC().Draw(t, "phc")); err == nil {
			c["PHC válido"]++
		}
		if err := passhash.Validate(gen.BadPHC().Draw(t, "badphc")); err != nil {
			c["PHC inválido"]++
		}
		for _, u := range gen.ValidUsers().Draw(t, "users") {
			c["usuario "+u.Role]++
			if u.MFASecret == nil {
				c["usuario sin mfa"]++
			}
		}
		c["inválido: "+gen.InvalidUsers().Draw(t, "invalid").Why]++
	})
	want := []string{"principal sin ID", "rol inválido", "rol user", "rol admin", "Allow user", "Deny user", "Allow admin", "Deny admin",
		"acción conocida", "acción desconocida", "sin propietario", "propietario propio", "propietario ajeno", "tipo conocido", "tipo desconocido",
		"secreto de 160 bits", "secreto corto", "PHC válido", "PHC inválido", "usuario admin", "usuario user", "usuario sin mfa",
		"inválido: hash no argon2id o débil", "inválido: duplicado sin distinguir mayúsculas", "inválido: rol fuera de {user,admin}",
		"inválido: admin sin mfa_secret", "inválido: mfa_secret de menos de 160 bits", "inválido: username vacío", "inválido: username demasiado largo"}
	for _, k := range want {
		if c[k] == 0 {
			t.Errorf("clase %q nunca generada en %d sorteos", k, n)
		}
	}
}
