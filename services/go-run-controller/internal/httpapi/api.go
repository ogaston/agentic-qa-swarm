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

// Principal es la identidad autenticada que devuelve GET /auth/session.
type Principal struct {
	ID    string // principal_id
	Admin bool   // role == "admin"
}

// TokenVerifier valida un bearer (sin caché) y devuelve el principal autenticado.
type TokenVerifier interface {
	Verify(ctx context.Context, bearer string) (Principal, error)
}

// ExecutionApprover es el controlador de corridas para la confirmación humana de ejecución (U12-T02).
type ExecutionApprover interface {
	ApproveExecution(ctx context.Context, id, principalID string, admin bool) error
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

// Verify: 200 con Session válida -> principal (el Role se valida, no se aplica: C-47); 401 ->
// ErrUnauthenticated; cualquier otra cosa -> ErrUnavailable.
func (v *IdentityVerifier) Verify(ctx context.Context, bearer string) (Principal, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.URL, nil)
	if err != nil {
		return Principal{}, ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	resp, err := v.Client.Do(req)
	if err != nil {
		return Principal{}, ErrUnavailable
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return Principal{}, ErrUnauthenticated
	case http.StatusOK:
	default:
		return Principal{}, ErrUnavailable
	}
	var s struct {
		PrincipalID string `json:"principal_id"`
		Role        string `json:"role"`
		SessionID   string `json:"session_id"`
		ExpiresAt   string `json:"expires_at"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&s) != nil || s.PrincipalID == "" || s.SessionID == "" ||
		(s.Role != "user" && s.Role != "admin") {
		return Principal{}, ErrUnavailable
	}
	if _, err := time.Parse(time.RFC3339, s.ExpiresAt); err != nil {
		return Principal{}, ErrUnavailable
	}
	return Principal{ID: s.PrincipalID, Admin: s.Role == "admin"}, nil
}

// RunView es components.schemas.Run.
type RunView struct {
	ID                string       `json:"id"`
	State             runctl.State `json:"state"`
	TraceID           string       `json:"trace_id,omitempty"`
	Verdict           string       `json:"verdict,omitempty"`
	ReportURI         string       `json:"report_uri,omitempty"`
	AwaitingExecution bool         `json:"awaiting_execution"`
}

// view arma la vista HTTP de una corrida.
func view(run runctl.Run) RunView {
	return RunView{ID: run.ID, State: run.State, TraceID: run.TraceID, Verdict: run.Verdict,
		ReportURI: run.ReportURI, AwaitingExecution: runctl.AwaitingExecution(run)}
}

// New devuelve el handler de la aplicación. exec es el controlador (no el almacén a pelo) porque
// POST /runs/{id}/execute tiene que serializar la aprobación con el resto de la máquina de estados.
func New(store runctl.RunStore, exec ExecutionApprover, v TokenVerifier) http.Handler {
	mux := http.NewServeMux()
	// auth escribe la respuesta de error y devuelve ok=false si la petición no está autenticada.
	auth := func(w http.ResponseWriter, r *http.Request) (Principal, bool) {
		const p = "Bearer "
		h := r.Header.Get("Authorization")
		if len(h) <= len(p) || h[:len(p)] != p {
			writeErr(w, http.StatusUnauthorized, "unauthenticated")
			return Principal{}, false
		}
		principal, err := v.Verify(r.Context(), h[len(p):])
		switch {
		case errors.Is(err, ErrUnauthenticated):
			writeErr(w, http.StatusUnauthorized, "unauthenticated")
			return Principal{}, false
		case err != nil:
			w.Header().Set("Retry-After", "10")
			writeErr(w, http.StatusServiceUnavailable, "identity_unavailable")
			return Principal{}, false
		}
		return principal, true
	}
	mux.HandleFunc("GET /runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth(w, r); !ok {
			return
		}
		run, ok := store.Get(r.PathValue("id"))
		if !ok {
			writeErr(w, http.StatusNotFound, "not_found")
			return
		}
		writeJSON(w, http.StatusOK, view(run))
	})
	// POST /runs/{id}/execute: el humano confirma la ejecución tras el ensayo (U12-T02).
	mux.HandleFunc("POST /runs/{id}/execute", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth(w, r)
		if !ok {
			return
		}
		id := r.PathValue("id")
		switch err := exec.ApproveExecution(r.Context(), id, principal.ID, principal.Admin); {
		case err == nil:
		case errors.Is(err, runctl.ErrNotFound):
			writeErr(w, http.StatusNotFound, "not_found")
			return
		case errors.Is(err, runctl.ErrForbidden):
			writeErr(w, http.StatusForbidden, "forbidden")
			return
		case errors.Is(err, runctl.ErrNotAwaiting):
			writeErr(w, http.StatusConflict, "not_awaiting")
			return
		case errors.Is(err, runctl.ErrPersist):
			writeErr(w, http.StatusServiceUnavailable, "persistence_unavailable")
			return
		default:
			writeErr(w, http.StatusInternalServerError, "internal")
			return
		}
		run, ok := store.Get(id)
		if !ok {
			writeErr(w, http.StatusNotFound, "not_found")
			return
		}
		writeJSON(w, http.StatusAccepted, view(run))
	})
	// GET /runs/{id}/plan devuelve el FlowPlan guardado; nunca llama al planner.
	mux.HandleFunc("GET /runs/{id}/plan", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth(w, r); !ok {
			return
		}
		run, ok := store.Get(r.PathValue("id"))
		if !ok {
			writeErr(w, http.StatusNotFound, "run_not_found")
			return
		}
		if len(run.Plan) == 0 {
			writeErr(w, http.StatusNotFound, "plan_not_ready")
			return
		}
		fp, err := runctl.DecodeStoredPlan(run.Plan, run.ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "plan_invalid")
			return
		}
		writeJSON(w, http.StatusOK, fp)
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
