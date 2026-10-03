package gen

import (
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/go-identity/authz"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/principal"
)

func intBelow(t *rapid.T, label string, n int) int { return rapid.IntRange(0, n-1).Draw(t, label) }

// Role genera roles válidos (2/3) o inválidos (mayúsculas, espacios, desconocidos, vacío).
func Role() *rapid.Generator[principal.Role] {
	return rapid.Custom(func(t *rapid.T) principal.Role {
		if intBelow(t, "valid", 3) < 2 {
			return rapid.SampledFrom([]principal.Role{principal.RoleUser, principal.RoleAdmin}).Draw(t, "role")
		}
		return rapid.SampledFrom([]principal.Role{"", "Admin", "USER", "root", "user ", "admin\n", "superadmin"}).Draw(t, "badrole")
	})
}

// IDs es el conjunto pequeño de IDs de principal (para que coincidan con los propietarios).
var IDs = []string{"u1", "u2", "admin-1", "ü", "u1 "}

// Principal genera principals válidos e inválidos (ID vacío o rol inválido).
func Principal() *rapid.Generator[principal.Principal] {
	return rapid.Custom(func(t *rapid.T) principal.Principal {
		id := rapid.SampledFrom(IDs).Draw(t, "id")
		if intBelow(t, "emptyid", 12) == 0 {
			id = ""
		}
		return principal.Principal{ID: id, Role: Role().Draw(t, "role")}
	})
}

// Actions son las acciones conocidas y desconocidas.
var Actions = []authz.Action{authz.ActionSessionRead, authz.ActionUsersList, "", "session:write", "users:delete", "SESSION:READ", "session:read "}

// Types son los tipos de recurso conocidos y desconocidos.
var Types = []authz.ResourceType{authz.TypeSession, authz.TypeUsers, "", "Session", "secret", "users "}

// Action genera acciones, con las dos conocidas en el 60% de los casos.
func Action() *rapid.Generator[authz.Action] {
	return rapid.Custom(func(t *rapid.T) authz.Action {
		if intBelow(t, "known", 5) < 3 {
			return rapid.SampledFrom(Actions[:2]).Draw(t, "known")
		}
		return rapid.SampledFrom(Actions).Draw(t, "any")
	})
}

// ResourceFor genera recursos para p: sin propietario (25%), propios (35%) o ajenos (40%);
// con tipo conocido en el 70% de los casos.
func ResourceFor(p principal.Principal) *rapid.Generator[authz.Resource] {
	return rapid.Custom(func(t *rapid.T) authz.Resource {
		var owner string
		switch k := intBelow(t, "owner", 20); {
		case k < 5:
		case k < 12:
			owner = p.ID
		default:
			owner = rapid.SampledFrom(IDs).Draw(t, "other")
		}
		typ := rapid.SampledFrom(Types).Draw(t, "type")
		if intBelow(t, "known", 10) < 7 {
			typ = rapid.SampledFrom(Types[:2]).Draw(t, "known")
		}
		return authz.Resource{Type: typ, ID: "r1", Owner: owner}
	})
}

// Secret genera los bytes de un secreto TOTP de 20 a 40 bytes (160 a 320 bits).
func Secret() *rapid.Generator[[]byte] {
	return rapid.Custom(func(t *rapid.T) []byte {
		n := rapid.IntRange(20, 40).Draw(t, "len")
		if intBelow(t, "edge", 4) == 0 {
			n = 20
		}
		return rapid.SliceOfN(rapid.Byte(), n, n).Draw(t, "secret")
	})
}

// ShortSecret genera secretos de 0 a 19 bytes (inválidos: menos de 160 bits).
func ShortSecret() *rapid.Generator[[]byte] {
	return rapid.Custom(func(t *rapid.T) []byte {
		n := rapid.IntRange(0, 19).Draw(t, "len")
		return rapid.SliceOfN(rapid.Byte(), n, n).Draw(t, "short")
	})
}

// Base32 codifica el secreto con variantes que ParseSecret acepta: relleno o no, mayúsculas o minúsculas, espacios en los extremos.
func Base32(t *rapid.T, b []byte) string {
	s := base32.StdEncoding.EncodeToString(b)
	if rapid.Bool().Draw(t, "unpadded") {
		s = strings.TrimRight(s, "=")
	}
	if rapid.Bool().Draw(t, "lower") {
		s = strings.ToLower(s)
	}
	if rapid.Bool().Draw(t, "spaces") {
		s = " " + s + "\t\n"
	}
	return s
}

// Instant genera instantes (segundos Unix) entre 2001 y 2096, sin alinear al periodo TOTP.
func Instant() *rapid.Generator[time.Time] {
	return rapid.Custom(func(t *rapid.T) time.Time {
		return time.Unix(rapid.Int64Range(1_000_000_000, 4_000_000_000).Draw(t, "unix"), 0).UTC()
	})
}

// PHC arma un hash PHC argon2id sintácticamente válido sin calcular argon2 (solo para el archivo de usuarios).
func PHC() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		m := 19*1024 + 1024*intBelow(t, "m", 4)
		tt := 2 + intBelow(t, "t", 3)
		p := 1 + intBelow(t, "p", 3)
		salt := rapid.SliceOfN(rapid.Byte(), 8, 24).Draw(t, "salt")
		key := rapid.SliceOfN(rapid.Byte(), 16, 64).Draw(t, "key")
		b := base64.RawStdEncoding
		return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", m, tt, p, b.EncodeToString(salt), b.EncodeToString(key))
	})
}

// BadPHC genera hashes que no son argon2id PHC válidos o con parámetros insuficientes.
func BadPHC() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		good := PHC().Draw(t, "good")
		switch intBelow(t, "how", 8) {
		case 0:
			return ""
		case 1:
			return "$2a$12$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ012345"
		case 2:
			return strings.Replace(good, "argon2id", "argon2i", 1)
		case 3:
			return strings.Replace(good, "argon2id", "argon2d", 1)
		case 4:
			parts := strings.Split(good, "$")
			parts[3] = "m=1024,t=2,p=1" // memoria insuficiente
			return strings.Join(parts, "$")
		case 5:
			parts := strings.Split(good, "$")
			parts[3] = "m=19456,t=1,p=1" // pasadas insuficientes
			return strings.Join(parts, "$")
		case 6:
			return good + "$extra"
		}
		return rapid.String().Draw(t, "any")
	})
}

// User es un usuario del archivo, con los campos crudos.
type User struct {
	Username  string
	Hash      string
	Role      string
	MFASecret []byte // nil = sin mfa_secret
	MFAText   string // representación en el archivo
}

// Username genera nombres no vacíos de hasta 256 bytes, con Unicode y mayúsculas.
func Username() *rapid.Generator[string] {
	return rapid.OneOf(
		rapid.StringN(1, 12, 256),
		rapid.StringMatching(`[A-Za-z][A-Za-z0-9._-]{0,15}`),
	)
}

func asciiUpper(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' {
			return r - 32
		}
		return r
	}, s)
}

// ValidUser genera un usuario válido: admin siempre con mfa_secret; user con o sin.
func ValidUser() *rapid.Generator[User] {
	return rapid.Custom(func(t *rapid.T) User {
		u := User{Username: Username().Draw(t, "username"), Hash: PHC().Draw(t, "hash")}
		if rapid.Bool().Draw(t, "admin") {
			u.Role = "admin"
		} else {
			u.Role = "user"
		}
		if u.Role == "admin" || rapid.Bool().Draw(t, "mfa") {
			u.MFASecret = Secret().Draw(t, "secret")
			u.MFAText = Base32(t, u.MFASecret)
		}
		return u
	})
}

// Key es la normalización de nombres de users.Key (minúsculas).
func Key(s string) string { return strings.ToLower(s) }

// ValidUsers genera de 1 a 5 usuarios válidos, sin duplicados sin distinguir mayúsculas.
func ValidUsers() *rapid.Generator[[]User] {
	return rapid.SliceOfNDistinct(ValidUser(), 1, 5, func(u User) string { return Key(u.Username) })
}

// InvalidUsers devuelve una lista que rompe exactamente una regla del archivo, y la regla.
func InvalidUsers() *rapid.Generator[struct {
	Users []User
	Why   string
}] {
	type out = struct {
		Users []User
		Why   string
	}
	return rapid.Custom(func(t *rapid.T) out {
		us := ValidUsers().Draw(t, "users")
		i := rapid.IntRange(0, len(us)-1).Draw(t, "victim")
		why := ""
		switch intBelow(t, "how", 7) {
		case 0:
			us[i].Hash, why = BadPHC().Draw(t, "badhash"), "hash no argon2id o débil"
		case 1:
			d := us[i]
			d.Username = asciiUpper(d.Username)
			// el duplicado difiere de la víctima solo en mayúsculas ASCII (o es idéntico)
			us, why = append(us, d), "duplicado sin distinguir mayúsculas"
		case 2:
			us[i].Role, why = rapid.SampledFrom([]string{"", "Admin", "USER", "root", "user ", "admin\n", "superuser"}).Draw(t, "badrole"), "rol fuera de {user,admin}"
		case 3:
			us[i].Role, us[i].MFASecret, us[i].MFAText, why = "admin", nil, "", "admin sin mfa_secret"
		case 4:
			us[i].MFASecret = ShortSecret().Draw(t, "short")
			us[i].MFAText, why = base32.StdEncoding.EncodeToString(us[i].MFASecret), "mfa_secret de menos de 160 bits"
			if len(us[i].MFASecret) == 0 { // "" equivale a ausente: se usa un texto no base32
				us[i].MFAText = "!!!no-base32!!!"
			}
		case 5:
			us[i].Username, why = "", "username vacío"
		default:
			us[i].Username, why = strings.Repeat("x", 257)+fmt.Sprint(i), "username demasiado largo"
		}
		return out{us, why}
	})
}
