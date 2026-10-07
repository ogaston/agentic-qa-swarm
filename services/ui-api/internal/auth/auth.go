// Package auth define el puerto TokenVerifier de ui-api, el verificador real contra
// go-identity (HTTPTokenVerifier) y un verificador de prueba.
package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
)

// Roles de las personas (sin roles inventados). Informativos en esta tarea.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// Principal es la identidad autenticada de una peticion.
type Principal struct {
	ID   string
	Role string
}

// ErrUnauthenticated es el error generico de un token no aceptado (401).
var ErrUnauthenticated = errors.New("no autenticado")

// ErrUnavailable indica que no se pudo decidir (identidad caida, lenta, respuesta invalida o
// circuito abierto): la capa HTTP responde 503. Nunca implica acceso.
var ErrUnavailable = errors.New("identidad no disponible")

// TokenVerifier valida un bearer. Cualquier error implica rechazo (fail-closed):
// ErrUnavailable -> 503; el resto -> 401.
type TokenVerifier interface {
	Verify(ctx context.Context, bearer string) (Principal, error)
}

// FakeTokenVerifier acepta tokens fijos. Solo para pruebas y dev local.
type FakeTokenVerifier struct {
	tokens []fakeToken
}

type fakeToken struct {
	token string
	p     Principal
}

var _ TokenVerifier = (*FakeTokenVerifier)(nil)

// NewFakeTokenVerifier parsea "tok=id:rol,tok2=id2:rol2".
func NewFakeTokenVerifier(spec string) (*FakeTokenVerifier, error) {
	v := &FakeTokenVerifier{}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		tok, rest, ok := strings.Cut(part, "=")
		id, role, ok2 := strings.Cut(rest, ":")
		if !ok || !ok2 || tok == "" || id == "" {
			return nil, errors.New("formato de tokens falso invalido (esperado token=id:rol)")
		}
		if role != RoleUser && role != RoleAdmin {
			return nil, fmt.Errorf("rol invalido %q (user|admin)", role)
		}
		v.tokens = append(v.tokens, fakeToken{tok, Principal{ID: id, Role: role}})
	}
	if len(v.tokens) == 0 {
		return nil, errors.New("sin tokens falsos configurados")
	}
	return v, nil
}

// Verify compara contra todos los tokens en tiempo constante.
func (v *FakeTokenVerifier) Verify(_ context.Context, bearer string) (Principal, error) {
	var found Principal
	matched := false
	for _, t := range v.tokens {
		if subtle.ConstantTimeCompare([]byte(bearer), []byte(t.token)) == 1 {
			found, matched = t.p, true
		}
	}
	if !matched {
		return Principal{}, ErrUnauthenticated
	}
	return found, nil
}

// BearerToken extrae el token de un header Authorization con esquema Bearer.
func BearerToken(header string) (string, bool) {
	scheme, tok, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || tok == "" || strings.ContainsAny(tok, " \t") {
		return "", false
	}
	return tok, true
}
