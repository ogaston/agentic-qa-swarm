// Package httpapi expone POST /resets y PUT /sessions (bearer) más sondas y métricas.
package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/core"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var idRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// Resetter es lo que la API necesita del servicio.
type Resetter interface {
	Reset(ctx context.Context, runID, traceID string) (core.Result, error)
}

type Config struct {
	Token    string
	Service  Resetter
	Sessions core.SessionStore
	Clock    core.Clock
	Log      *slog.Logger
	Gatherer prometheus.Gatherer
}

// New devuelve el handler.
func New(c Config) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	mux.Handle("GET /metrics", promhttp.HandlerFor(c.Gatherer, promhttp.HandlerOpts{}))
	mux.HandleFunc("POST /resets", c.auth(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			RunID   string `json:"run_id"`
			TraceID string `json:"trace_id"`
		}
		if !decode(w, r, &in) || !idRe.MatchString(in.RunID) || (in.TraceID != "" && !idRe.MatchString(in.TraceID)) {
			bad(w)
			return
		}
		res, err := c.Service.Reset(r.Context(), in.RunID, in.TraceID)
		if err != nil {
			c.Log.Error("reset falló", "run_id", in.RunID, "trace_id", in.TraceID, "error", err.Error())
			write(w, 500, map[string]string{"error": "internal"})
			return
		}
		body := map[string]any{"state": res.State.State, "reset_verified": res.State.ResetVerified, "checks": res.Checks, "attempts": res.Attempts}
		if !res.Verified {
			write(w, 409, body)
			return
		}
		write(w, 200, body)
	}))
	mux.HandleFunc("PUT /sessions", c.auth(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			RunID string          `json:"run_id"`
			Plan  json.RawMessage `json:"plan"`
			State string          `json:"state"`
		}
		if !decode(w, r, &in) || !idRe.MatchString(in.RunID) || len(in.State) > 128 {
			bad(w)
			return
		}
		err := c.Sessions.Save(r.Context(), core.Session{RunID: in.RunID, Status: core.SessionActive,
			LastActivity: c.Clock.Now(), Plan: in.Plan, State: in.State})
		if err != nil {
			write(w, 500, map[string]string{"error": "internal"})
			return
		}
		w.WriteHeader(204)
	}))
	return mux
}

func (c Config) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(tok), []byte(c.Token)) != 1 {
			write(w, 401, map[string]string{"error": "unauthorized"})
			return
		}
		h(w, r)
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if dec.Decode(v) != nil {
		return false
	}
	_, err := dec.Token()
	return err == io.EOF
}

func bad(w http.ResponseWriter) { write(w, 400, map[string]string{"error": "bad_request"}) }

func write(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
