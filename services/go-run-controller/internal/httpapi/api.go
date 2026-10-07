// Package httpapi sirve GET /runs/{id} autenticado contra go-identity.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

// Errores del verificador.
var (
	ErrUnauthenticated = errors.New("no autenticado")
	ErrUnavailable     = errors.New("identidad no disponible")
)

// TokenVerifier valida un bearer (sin caché).
type TokenVerifier interface {
	Verify(ctx context.Context, bearer string) error
}

// IdentityVerifier valida contra GET {base}/auth/session (mismo contrato que U1-T07), 2 s.
type IdentityVerifier struct {
	URL    string
	Client *http.Client
}

// NewIdentityVerifier construye el verificador (base ya validada).
func NewIdentityVerifier(base string) *IdentityVerifier {
	return &IdentityVerifier{URL: base + "/auth/session", Client: &http.Client{Timeout: 2 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

// Verify: 200 con Session válida -> nil (el Role se valida, no se aplica: C-47); 401 -> ErrUnauthenticated;
// cualquier otra cosa -> ErrUnavailable.
func (v *IdentityVerifier) Verify(ctx context.Context, bearer string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.URL, nil)
	if err != nil {
		return ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	resp, err := v.Client.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return ErrUnauthenticated
	case http.StatusOK:
	default:
		return ErrUnavailable
	}
	var s struct {
		PrincipalID string `json:"principal_id"`
		Role        string `json:"role"`
		SessionID   string `json:"session_id"`
		ExpiresAt   string `json:"expires_at"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&s) != nil || s.PrincipalID == "" || s.SessionID == "" ||
		(s.Role != "user" && s.Role != "admin") {
		return ErrUnavailable
	}
	if _, err := time.Parse(time.RFC3339, s.ExpiresAt); err != nil {
		return ErrUnavailable
	}
	return nil
}

// RunView es components.schemas.Run.
type RunView struct {
	ID      string       `json:"id"`
	State   runctl.State `json:"state"`
	TraceID string       `json:"trace_id,omitempty"`
}

// New devuelve el handler de la aplicación.
func New(store runctl.RunStore, v TokenVerifier) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		const p = "Bearer "
		h := r.Header.Get("Authorization")
		if len(h) <= len(p) || h[:len(p)] != p {
			writeErr(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		switch err := v.Verify(r.Context(), h[len(p):]); {
		case errors.Is(err, ErrUnauthenticated):
			writeErr(w, http.StatusUnauthorized, "unauthenticated")
			return
		case err != nil:
			w.Header().Set("Retry-After", "10")
			writeErr(w, http.StatusServiceUnavailable, "identity_unavailable")
			return
		}
		run, ok := store.Get(r.PathValue("id"))
		if !ok {
			writeErr(w, http.StatusNotFound, "not_found")
			return
		}
		writeJSON(w, http.StatusOK, RunView{ID: run.ID, State: run.State, TraceID: run.TraceID})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, c string) {
	writeJSON(w, code, map[string]string{"code": c, "message": c})
}
