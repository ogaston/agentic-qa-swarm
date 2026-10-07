package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Parametros del verificador real (NF-SEG-08: sin cache de validaciones).
const (
	IdentityTimeout     = 2 * time.Second
	CircuitThreshold    = 5
	CircuitOpenDuration = 10 * time.Second
	maxSessionBody      = 16 << 10
)

// Resultados de aqs_identity_calls_total{result}.
const (
	ResultOK           = "ok"
	ResultUnauthorized = "unauthorized"
	ResultError        = "error"
	ResultCircuitOpen  = "circuit_open"
)

// CallObserver recibe el resultado de cada validacion (metricas). Puede ser nil.
type CallObserver interface{ IdentityCall(result string) }

// HTTPTokenVerifier valida CADA peticion contra GET {base}/auth/session de go-identity.
// 200 con cuerpo valido -> Principal; 401 -> ErrUnauthenticated; cualquier otra cosa
// (otro codigo, cuerpo invalido, rol desconocido, timeout, red, circuito abierto) -> ErrUnavailable.
type HTTPTokenVerifier struct {
	url     string
	client  *http.Client
	now     func() time.Time
	obs     CallObserver
	timeout time.Duration

	mu        sync.Mutex
	fails     int
	openUntil time.Time // cero = cerrado
	probing   bool      // una sonda medio-abierta en vuelo
}

var _ TokenVerifier = (*HTTPTokenVerifier)(nil)

// NewHTTPTokenVerifier valida baseURL (solo http/https, con host) y construye el verificador.
// now puede ser nil (reloj real).
func NewHTTPTokenVerifier(baseURL string, now func() time.Time) (*HTTPTokenVerifier, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, errors.New("IDENTITY_URL debe ser una URL http(s) con host")
	}
	if now == nil {
		now = time.Now
	}
	return &HTTPTokenVerifier{
		url: strings.TrimRight(u.String(), "/") + "/auth/session", now: now, timeout: IdentityTimeout,
		client: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

// SetObserver fija el observador de metricas.
func (v *HTTPTokenVerifier) SetObserver(o CallObserver) { v.obs = o }

// CircuitOpen indica si el circuito esta abierto (rechazando sin llamar a identidad).
func (v *HTTPTokenVerifier) CircuitOpen() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return !v.openUntil.IsZero()
}

func (v *HTTPTokenVerifier) record(r string) {
	if v.obs != nil {
		v.obs.IdentityCall(r)
	}
}

// admit decide si se puede llamar a identidad. Abierto y vigente: no. Vencido: una sonda a la vez.
func (v *HTTPTokenVerifier) admit() (probe, ok bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.openUntil.IsZero() {
		return false, true
	}
	if v.now().Before(v.openUntil) || v.probing {
		return false, false
	}
	v.probing = true
	return true, true
}

// settle registra el resultado de una llamada de transporte (healthy = identidad respondio de forma usable).
func (v *HTTPTokenVerifier) settle(probe, healthy bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if probe {
		v.probing = false
	}
	if healthy {
		v.fails, v.openUntil = 0, time.Time{}
		return
	}
	v.fails++
	if probe || v.fails >= CircuitThreshold {
		v.openUntil = v.now().Add(CircuitOpenDuration)
	}
}

// Verify implementa TokenVerifier.
func (v *HTTPTokenVerifier) Verify(ctx context.Context, bearer string) (Principal, error) {
	probe, ok := v.admit()
	if !ok {
		v.record(ResultCircuitOpen)
		return Principal{}, ErrUnavailable
	}
	pr, status, err := v.call(ctx, bearer)
	switch {
	case err == nil:
		v.settle(probe, true)
		v.record(ResultOK)
		return pr, nil
	case status == http.StatusUnauthorized:
		v.settle(probe, true)
		v.record(ResultUnauthorized)
		return Principal{}, ErrUnauthenticated
	default:
		v.settle(probe, false)
		v.record(ResultError)
		return Principal{}, ErrUnavailable
	}
}

// call hace la peticion. Devuelve el status (0 si no hubo respuesta); err != nil salvo en 200 valido.
func (v *HTTPTokenVerifier) call(ctx context.Context, bearer string) (Principal, int, error) {
	ctx, cancel := context.WithTimeout(ctx, v.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.url, nil)
	if err != nil {
		return Principal{}, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Accept", "application/json")
	resp, err := v.client.Do(req)
	if err != nil {
		// El error de net/http incluye la URL, nunca el token; aun asi se descarta el detalle.
		return Principal{}, 0, errors.New("identidad inalcanzable")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxSessionBody))
		return Principal{}, resp.StatusCode, ErrUnauthenticated
	}
	if resp.StatusCode != http.StatusOK {
		return Principal{}, resp.StatusCode, fmt.Errorf("identidad respondio %d", resp.StatusCode)
	}
	pr, err := parseSession(io.LimitReader(resp.Body, maxSessionBody))
	return pr, resp.StatusCode, err
}

// parseSession valida el cuerpo contra components.schemas.Session del OpenAPI:
// required [principal_id, role, session_id, expires_at]; role en {user, admin}; expires_at date-time.
func parseSession(r io.Reader) (Principal, error) {
	dec := json.NewDecoder(r)
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return Principal{}, errors.New("sesion: cuerpo invalido")
	}
	str := func(k string) string { s, _ := raw[k].(string); return s }
	id, role, sid, exp := str("principal_id"), str("role"), str("session_id"), str("expires_at")
	if id == "" || sid == "" {
		return Principal{}, errors.New("sesion: principal_id/session_id ausentes")
	}
	if role != RoleUser && role != RoleAdmin {
		return Principal{}, errors.New("sesion: rol desconocido")
	}
	if _, err := time.Parse(time.RFC3339, exp); err != nil {
		return Principal{}, errors.New("sesion: expires_at invalido")
	}
	return Principal{ID: id, Role: role}, nil
}
