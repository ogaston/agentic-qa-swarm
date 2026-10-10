package httpapi

// GET /warm: lectura del estado del warm (go-warm-manager) con el token de SERVICIO. El token de la
// persona nunca sale hacia el warm-manager y el de servicio nunca sale hacia el navegador.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// WarmUnavailable es el code del 503 cuando el warm no puede responder con un WarmState válido.
const WarmUnavailable = "warm_unavailable"

// Parámetros del cliente del warm (mismos que el verificador de identidad).
const (
	WarmDefaultTimeout = 2 * time.Second
	WarmCircuitThresh  = 5
	WarmCircuitOpenFor = 10 * time.Second
	WarmRetryAfter     = 10 // segundos del Retry-After del 503 warm_unavailable
	warmMaxBody        = 16 << 10
)

// WarmState es components.schemas.WarmState (contracts/plans/warm-state.schema.json).
type WarmState struct {
	WarmID          string `json:"warm_id"`
	State           string `json:"state"`
	ResetVerified   bool   `json:"reset_verified"`
	BaselineVersion string `json:"baseline_version"`
}

var warmStates = map[string]bool{"ready": true, "dirty": true, "cuarentena": true, "idle-escalado": true}

// parseWarmState valida el cuerpo contra el esquema: campos required, sin campos extra
// (additionalProperties false), enum de state y minLength 1 en las cadenas.
func parseWarmState(b []byte) (WarmState, bool) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var raw struct {
		WarmID          *string `json:"warm_id"`
		State           *string `json:"state"`
		ResetVerified   *bool   `json:"reset_verified"`
		BaselineVersion *string `json:"baseline_version"`
	}
	if err := dec.Decode(&raw); err != nil {
		return WarmState{}, false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return WarmState{}, false
	}
	if raw.WarmID == nil || raw.State == nil || raw.ResetVerified == nil || raw.BaselineVersion == nil {
		return WarmState{}, false
	}
	if *raw.WarmID == "" || *raw.BaselineVersion == "" || !warmStates[*raw.State] {
		return WarmState{}, false
	}
	return WarmState{WarmID: *raw.WarmID, State: *raw.State, ResetVerified: *raw.ResetVerified, BaselineVersion: *raw.BaselineVersion}, true
}

// warmClient habla con go-warm-manager y lleva el circuito (abre tras WarmCircuitThresh fallos de
// transporte o plazo; cualquier respuesta HTTP cuenta como «alcanzable»).
type warmClient struct {
	url       string // {base}/warm
	tokenFile string
	timeout   time.Duration
	client    *http.Client
	now       func() time.Time

	mu        sync.Mutex
	fails     int
	openUntil time.Time
	probing   bool
}

func newWarmClient(baseURL, tokenFile string, timeout time.Duration, now func() time.Time) (*warmClient, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, errors.New("WARM_URL debe ser una URL http(s) con host")
	}
	if tokenFile == "" {
		return nil, errors.New("WARM_URL exige UIAPI_WARM_TOKEN_FILE")
	}
	if timeout <= 0 {
		timeout = WarmDefaultTimeout
	}
	return &warmClient{
		url: strings.TrimRight(u.String(), "/") + "/warm", tokenFile: tokenFile, timeout: timeout, now: now,
		client: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
}

func (c *warmClient) admit() (probe, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.openUntil.IsZero() {
		return false, true
	}
	if c.now().Before(c.openUntil) || c.probing {
		return false, false
	}
	c.probing = true
	return true, true
}

func (c *warmClient) settle(probe, reachable bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if probe {
		c.probing = false
	}
	if reachable {
		c.fails, c.openUntil = 0, time.Time{}
		return
	}
	c.fails++
	if probe || c.fails >= WarmCircuitThresh {
		c.openUntil = c.now().Add(WarmCircuitOpenFor)
	}
}

func (c *warmClient) abort(probe bool) {
	if probe {
		c.mu.Lock()
		c.probing = false
		c.mu.Unlock()
	}
}

// readToken lee el token de servicio del archivo en cada llamada (rotación sin reinicio).
func (c *warmClient) readToken() (string, error) {
	b, err := os.ReadFile(c.tokenFile)
	if err != nil {
		return "", errors.New("token de servicio ilegible")
	}
	tok := strings.TrimSpace(string(b))
	if tok == "" {
		return "", errors.New("token de servicio vacío")
	}
	return tok, nil
}

// fetch devuelve el WarmState validado, o false (warm_unavailable). Nunca registra el token.
func (h *Handler) fetchWarm(r *http.Request) (WarmState, bool) {
	c := h.warmc
	probe, ok := c.admit()
	if !ok {
		h.logAt(r, slog.LevelWarn, "warm no disponible: circuito abierto")
		return WarmState{}, false
	}
	tok, err := c.readToken()
	if err != nil {
		c.abort(probe)
		h.logAt(r, slog.LevelError, "warm no disponible: "+err.Error())
		return WarmState{}, false
	}
	ctx, cancel := context.WithTimeout(r.Context(), c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		c.abort(probe)
		return WarmState{}, false
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		if r.Context().Err() != nil { // lo abortó el cliente: no es fallo del warm
			c.abort(probe)
			return WarmState{}, false
		}
		c.settle(probe, false)
		h.logAt(r, slog.LevelWarn, "go-warm-manager inalcanzable o con plazo vencido")
		return WarmState{}, false
	}
	defer resp.Body.Close()
	c.settle(probe, true)
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, warmMaxBody))
		h.logAt(r, slog.LevelError, "go-warm-manager rechazó el token de servicio (401): revisar UIAPI_WARM_TOKEN_FILE")
		return WarmState{}, false
	case resp.StatusCode >= 500:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, warmMaxBody))
		h.logAt(r, slog.LevelWarn, "go-warm-manager respondió error", "status", resp.StatusCode)
		return WarmState{}, false
	case resp.StatusCode != http.StatusOK:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, warmMaxBody))
		h.logAt(r, slog.LevelWarn, "go-warm-manager respondió un estado inesperado", "status", resp.StatusCode)
		return WarmState{}, false
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, warmMaxBody+1))
	if err != nil || len(b) > warmMaxBody {
		h.logAt(r, slog.LevelWarn, "respuesta del warm-manager ilegible o demasiado grande")
		return WarmState{}, false
	}
	st, ok := parseWarmState(b)
	if !ok {
		h.logAt(r, slog.LevelWarn, "respuesta del warm-manager no cumple WarmState")
		return WarmState{}, false
	}
	return st, true
}

// warm atiende GET /warm (ya autenticado con el token de la persona).
func (h *Handler) warm(w http.ResponseWriter, r *http.Request) {
	if h.warmc == nil {
		writeError(w, http.StatusServiceUnavailable, WarmUnavailable, "warm no configurado")
		return
	}
	st, ok := h.fetchWarm(r)
	if !ok {
		w.Header().Set("Retry-After", strconv.Itoa(WarmRetryAfter))
		writeError(w, http.StatusServiceUnavailable, WarmUnavailable, "estado del warm no disponible")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// logAt registra en el nivel dado; con Slog lleva request_id y trace_id.
func (h *Handler) logAt(r *http.Request, level slog.Level, msg string, kv ...any) {
	if h.cfg.Slog != nil {
		h.cfg.Slog.Log(r.Context(), level, msg, kv...)
		return
	}
	h.log.Printf("%s %v", msg, kv)
}
