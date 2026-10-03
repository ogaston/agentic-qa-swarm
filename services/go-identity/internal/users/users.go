// Package users carga y valida el archivo de usuarios (IDENTITY_USERS_FILE).
package users

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/passhash"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/totp"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/principal"
)

const maxFileBytes = 1 << 20

// User es un usuario ya validado.
type User struct {
	Name         string // tal como figura en el archivo
	PasswordHash string
	Role         principal.Role
	MFASecret    []byte // solo presente si el archivo lo trae; obligatorio para admin
}

type raw struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	Role         string `json:"role"`
	MFASecret    string `json:"mfa_secret"`
}

// Key normaliza un nombre de usuario para comparar y contar (minúsculas).
func Key(username string) string { return strings.ToLower(username) }

// LoadFile lee y valida el archivo. Cualquier incumplimiento es un error: el servicio no arranca.
func LoadFile(path string) (map[string]*User, error) {
	if path == "" {
		return nil, errors.New("IDENTITY_USERS_FILE no está definido")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("archivo de usuarios: %w", err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("archivo de usuarios: %w", err)
	}
	if len(b) > maxFileBytes {
		return nil, errors.New("archivo de usuarios demasiado grande")
	}
	return Parse(b)
}

// Parse valida el JSON de usuarios.
func Parse(b []byte) (map[string]*User, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var list []raw
	if err := dec.Decode(&list); err != nil {
		return nil, fmt.Errorf("archivo de usuarios no es JSON válido: %w", err)
	}
	if len(list) == 0 {
		return nil, errors.New("archivo de usuarios vacío: no hay usuarios por defecto")
	}
	out := make(map[string]*User, len(list))
	for i, r := range list {
		if r.Username == "" || len(r.Username) > 256 {
			return nil, fmt.Errorf("usuario #%d: username vacío o demasiado largo", i)
		}
		k := Key(r.Username)
		if _, dup := out[k]; dup {
			return nil, fmt.Errorf("usuario #%d: username duplicado", i)
		}
		if err := passhash.Validate(r.PasswordHash); err != nil {
			return nil, fmt.Errorf("usuario #%d: %w", i, err)
		}
		role, err := principal.ParseRole(r.Role)
		if err != nil {
			return nil, fmt.Errorf("usuario #%d: %w", i, err)
		}
		u := &User{Name: r.Username, PasswordHash: r.PasswordHash, Role: role}
		if r.MFASecret != "" {
			if u.MFASecret, err = totp.ParseSecret(r.MFASecret); err != nil {
				return nil, fmt.Errorf("usuario #%d: %w", i, err)
			}
		}
		if role == principal.RoleAdmin && u.MFASecret == nil {
			return nil, fmt.Errorf("usuario #%d: un admin requiere mfa_secret", i)
		}
		out[k] = u
	}
	return out, nil
}
