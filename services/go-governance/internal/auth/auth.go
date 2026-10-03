// Package auth autentica a las personas (vía go-identity) y al servicio llamante del gate.
package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/authz"
)

var (
	// ErrInvalidToken: el token no corresponde a una sesión vigente (401).
	ErrInvalidToken = errors.New("token inválido")
	// ErrUnavailable: no se pudo verificar el token (503); nunca se permite.
	ErrUnavailable = errors.New("identity_unavailable")
)

// TokenVerifier resuelve un token de persona en su Principal.
type TokenVerifier interface {
	Verify(ctx context.Context, token string) (authz.Principal, error)
}

// HTTPTokenVerifier llama a GET {BaseURL}/auth/session en cada petición (sin caché).
type HTTPTokenVerifier struct {
	BaseURL string
	Client  *http.Client
}

// NewHTTPTokenVerifier valida la URL y construye el verificador (sin seguir redirecciones).
func NewHTTPTokenVerifier(base string, timeout time.Duration) (*HTTPTokenVerifier, error) {
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("IDENTITY_URL inválida (se espera http(s)://host[:puerto])")
	}
	return &HTTPTokenVerifier{
		BaseURL: strings.TrimRight(base, "/"),
		Client: &http.Client{Timeout: timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
}

// Verify implementa TokenVerifier.
func (v *HTTPTokenVerifier) Verify(ctx context.Context, token string) (authz.Principal, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.BaseURL+"/auth/session", nil)
	if err != nil {
		return authz.Principal{}, ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := v.Client.Do(req)
	if err != nil {
		return authz.Principal{}, fmt.Errorf("%w: %s", ErrUnavailable, "sin respuesta de identidad")
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return authz.Principal{}, ErrInvalidToken
	default:
		return authz.Principal{}, fmt.Errorf("%w: identidad respondió %d", ErrUnavailable, resp.StatusCode)
	}
	var s struct {
		PrincipalID string     `json:"principal_id"`
		Role        authz.Role `json:"role"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&s); err != nil {
		return authz.Principal{}, fmt.Errorf("%w: respuesta de identidad ilegible", ErrUnavailable)
	}
	if s.PrincipalID == "" || !s.Role.Valid() {
		return authz.Principal{}, fmt.Errorf("%w: sesión sin principal o rol válido", ErrUnavailable)
	}
	return authz.Principal{ID: s.PrincipalID, Role: s.Role}, nil
}

// FakeTokenVerifier es para pruebas: token -> principal.
type FakeTokenVerifier struct{ Tokens map[string]authz.Principal }

// Verify implementa TokenVerifier.
func (f FakeTokenVerifier) Verify(_ context.Context, token string) (authz.Principal, error) {
	p, ok := f.Tokens[token]
	if !ok {
		return authz.Principal{}, ErrInvalidToken
	}
	return p, nil
}

// ParseFakeTokens parsea 'tok=id:role,tok2=id2:role2'.
func ParseFakeTokens(s string) (FakeTokenVerifier, error) {
	f := FakeTokenVerifier{Tokens: map[string]authz.Principal{}}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		tok, rest, ok := strings.Cut(part, "=")
		id, role, ok2 := strings.Cut(rest, ":")
		if !ok || !ok2 || tok == "" || id == "" || !authz.Role(role).Valid() {
			return f, errors.New("GOVERNANCE_FAKE_TOKENS inválido (se espera token=id:user|admin,...)")
		}
		f.Tokens[tok] = authz.Principal{ID: id, Role: authz.Role(role)}
	}
	if len(f.Tokens) == 0 {
		return f, errors.New("GOVERNANCE_FAKE_TOKENS vacío")
	}
	return f, nil
}

// MinServiceTokenLen es la longitud mínima del token de servicio.
const MinServiceTokenLen = 32

// ServiceToken compara en tiempo constante el token del servicio llamante.
type ServiceToken struct{ sum [32]byte }

// NewServiceToken exige al menos MinServiceTokenLen caracteres.
func NewServiceToken(tok string) (ServiceToken, error) {
	if len(tok) < MinServiceTokenLen {
		return ServiceToken{}, fmt.Errorf("GOVERNANCE_SERVICE_TOKEN debe tener al menos %d caracteres", MinServiceTokenLen)
	}
	return ServiceToken{sum: sha256.Sum256([]byte(tok))}, nil
}

// Matches compara en tiempo constante (vía digests de igual longitud).
func (s ServiceToken) Matches(tok string) bool {
	got := sha256.Sum256([]byte(tok))
	return subtle.ConstantTimeCompare(got[:], s.sum[:]) == 1
}

// BearerToken extrae el token de "Authorization: Bearer <t>".
func BearerToken(h http.Header) (string, bool) {
	v := h.Get("Authorization")
	const p = "Bearer "
	if len(v) <= len(p) || !strings.EqualFold(v[:len(p)], p) {
		return "", false
	}
	t := strings.TrimSpace(v[len(p):])
	return t, t != ""
}
