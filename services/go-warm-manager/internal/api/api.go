// Package api es la API REST servicio a servicio de go-warm-manager.
package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
)

// Server expone el servicio.
type Server struct {
	Svc      *wm.Service
	Token    string
	Log      *slog.Logger
	Registry *prometheus.Registry
}

var traceRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

func newTrace() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const p = "Bearer "
		h := r.Header.Get("Authorization")
		got := ""
		if strings.HasPrefix(h, p) {
			got = h[len(p):]
		}
		if s.Token == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.Token)) != 1 {
			writeJSON(w, 401, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("basura tras el JSON")
	}
	return nil
}

func trace(r *http.Request) string {
	if t := r.Header.Get("X-Trace-Id"); traceRe.MatchString(t) {
		return t
	}
	return newTrace()
}

// Handler devuelve el mux completo.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if _, err := s.Svc.State.Get(r.Context()); err != nil {
			writeJSON(w, 503, map[string]string{"status": "unavailable"})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ready"})
	})
	mux.Handle("GET /metrics", promhttp.HandlerFor(s.Registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("GET /warm", s.auth(func(w http.ResponseWriter, r *http.Request) {
		st, err := s.Svc.GetWarm(r.Context())
		if err != nil {
			writeJSON(w, 503, map[string]string{"error": "state_unavailable"})
			return
		}
		writeJSON(w, 200, st)
	}))
	mux.HandleFunc("POST /warm/ensure", s.auth(func(w http.ResponseWriter, r *http.Request) {
		if n, _ := io.Copy(io.Discard, io.LimitReader(r.Body, 1<<10)); n > 0 {
			writeJSON(w, 400, map[string]string{"error": "body_not_allowed"})
			return
		}
		st, ok, err := s.Svc.EnsureWarmReady(r.Context(), trace(r))
		switch {
		case errors.Is(err, wm.ErrWarmTimeout):
			writeJSON(w, 409, st)
		case err != nil:
			s.Log.Error("ensure fallo", "error", err.Error())
			writeJSON(w, 503, map[string]string{"error": "ensure_failed"})
		case ok:
			writeJSON(w, 200, st)
		default:
			writeJSON(w, 409, st)
		}
	}))
	mux.HandleFunc("POST /deploys", s.auth(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RunID    string      `json:"run_id"`
			Artifact wm.Artifact `json:"artifact"`
		}
		if err := decode(r, &req); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid_body"})
			return
		}
		st, err := s.Svc.StartDeploy(req.RunID, req.Artifact, trace(r))
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid_request"})
			return
		}
		s.Log.Info("deploy aceptado", "run_id", req.RunID, "trace_id", trace(r))
		writeJSON(w, 202, st)
	}))
	mux.HandleFunc("GET /deploys/{run_id}", s.auth(func(w http.ResponseWriter, r *http.Request) {
		if wm.ValidateRunID(r.PathValue("run_id")) != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid_run_id"})
			return
		}
		st, ok := s.Svc.DeployState(r.PathValue("run_id"))
		if !ok {
			writeJSON(w, 404, map[string]string{"error": "not_found"})
			return
		}
		writeJSON(w, 200, st)
	}))
	mux.HandleFunc("POST /surface", s.auth(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RunID string `json:"run_id"`
		}
		if err := decode(r, &req); err != nil || wm.ValidateRunID(req.RunID) != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid_body"})
			return
		}
		sa, err := s.Svc.InferSurface(r.Context(), req.RunID, trace(r))
		if err != nil {
			s.Log.Error("superficie fallo", "run_id", req.RunID, "error", err.Error())
			writeJSON(w, 502, map[string]string{"error": "surface_failed"})
			return
		}
		writeJSON(w, 200, sa)
	}))
	// Autenticacion antes del enrutado: una ruta protegida sin token valido es 401 aunque el metodo sea otro.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz", "/readyz", "/metrics":
		default:
			if !s.authorized(r) {
				writeJSON(w, 401, map[string]string{"error": "unauthorized"})
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) authorized(r *http.Request) bool {
	const p = "Bearer "
	h := r.Header.Get("Authorization")
	got := ""
	if strings.HasPrefix(h, p) {
		got = h[len(p):]
	}
	return s.Token != "" && subtle.ConstantTimeCompare([]byte(got), []byte(s.Token)) == 1
}

// HTTPServer devuelve un http.Server con timeouts.
func (s *Server) HTTPServer(addr string) *http.Server {
	return &http.Server{Addr: addr, Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 60 * time.Second}
}
