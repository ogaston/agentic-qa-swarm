// Package adapters contiene los adaptadores reales de los puertos de runctl que tocan el
// exterior: HTTP hacia go-governance, diario y eventos en archivos.
package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

// GateTimeout es el plazo de cada llamada al gate.
const GateTimeout = 2 * time.Second

// HTTPGate llama a POST {base}/gates/authorize de go-governance con el token de servicio.
type HTTPGate struct {
	url, token string
	client     *http.Client
	timeout    time.Duration
}

var _ runctl.GateClient = (*HTTPGate)(nil)

// ValidateHTTPURL exige una URL http(s) con host y sin credenciales.
func ValidateHTTPURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, errors.New("debe ser una URL http(s) con host")
	}
	return u, nil
}

// NewHTTPGate valida la URL y el token.
func NewHTTPGate(base, token string) (*HTTPGate, error) {
	u, err := ValidateHTTPURL(base)
	if err != nil {
		return nil, fmt.Errorf("GOVERNANCE_URL %w", err)
	}
	if token == "" {
		return nil, errors.New("GOVERNANCE_SERVICE_TOKEN es obligatorio")
	}
	return &HTTPGate{url: strings.TrimRight(u.String(), "/") + "/gates/authorize", token: token, timeout: GateTimeout,
		client: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// SetTimeout cambia el plazo (pruebas).
func (g *HTTPGate) SetTimeout(d time.Duration) { g.timeout = d }

// Authorize implementa GateClient: solo 200 con cuerpo válido es una decisión; todo lo demás es error.
func (g *HTTPGate) Authorize(ctx context.Context, in runctl.GateRequest) (runctl.GateDecision, error) {
	body, err := json.Marshal(in)
	if err != nil {
		return runctl.GateDecision{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.url, bytes.NewReader(body))
	if err != nil {
		return runctl.GateDecision{}, err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Content-Type", "application/json")
	if in.TraceID != "" {
		req.Header.Set("traceparent", "00-"+in.TraceID+"-0000000000000001-01")
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return runctl.GateDecision{}, errors.New("gate inalcanzable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return runctl.GateDecision{}, fmt.Errorf("gate respondió %d", resp.StatusCode)
	}
	var raw map[string]any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&raw); err != nil {
		return runctl.GateDecision{}, errors.New("gate: cuerpo inválido")
	}
	allow, ok1 := raw["allow"].(bool)
	reason, ok2 := raw["reason"].(string)
	ref, ok3 := raw["audit_ref"].(string)
	if !ok1 || !ok2 || !ok3 || (!allow && reason == "") {
		return runctl.GateDecision{}, errors.New("gate: respuesta no cumple GateDecision")
	}
	return runctl.GateDecision{Allow: allow, Reason: reason, AuditRef: ref}, nil
}
