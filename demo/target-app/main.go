// Package main es la app de referencia del warm (U8-T04): una API transaccional minima de pedidos
// sobre la Postgres del warm. Publica /openapi.json, /health, GET/POST /products y GET/POST /orders.
// Invariante: el stock nunca es negativo. Defecto sembrado: TARGET_APP_BUG=oversell permite vender
// mas del stock. Un listener aparte (BASELINE_LISTEN_ADDR, :9090 por defecto) expone los endpoints
// de baseline (clean|verify|version) protegidos por un token; lo consume baseline.sh desde go-reset.
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

//go:embed openapi.json
var openAPIDoc []byte

// config viene del entorno. Todo lo obligatorio falla al arrancar (fail closed).
type config struct {
	DatabaseURL     string
	ListenAddr      string
	BaselineAddr    string
	BaselineTokFile string
	Bug             string
}

func loadConfig() (config, error) {
	c := config{
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		ListenAddr:      envOr("LISTEN_ADDR", ":80"),
		BaselineAddr:    envOr("BASELINE_LISTEN_ADDR", ":9090"),
		BaselineTokFile: os.Getenv("BASELINE_TOKEN_FILE"),
		Bug:             os.Getenv("TARGET_APP_BUG"),
	}
	if c.DatabaseURL == "" {
		return c, errors.New("falta DATABASE_URL")
	}
	if c.BaselineTokFile == "" {
		return c, errors.New("falta BASELINE_TOKEN_FILE")
	}
	if c.Bug != "" && c.Bug != "oversell" {
		return c, errors.New("TARGET_APP_BUG desconocido: " + c.Bug)
	}
	return c, nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := loadConfig()
	if err != nil {
		log.Error("configuracion invalida", "error", err.Error())
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := openStore(ctx, cfg.DatabaseURL, cfg.Bug == "oversell")
	if err != nil {
		log.Error("no se pudo abrir la base", "error", err.Error())
		os.Exit(1)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		log.Error("migracion fallida", "error", err.Error())
		os.Exit(1)
	}

	tok, err := readToken(cfg.BaselineTokFile)
	if err != nil {
		log.Error("token de baseline", "error", err.Error())
		os.Exit(2)
	}

	srv := &http.Server{Addr: cfg.ListenAddr, Handler: appMux(st), ReadHeaderTimeout: 5 * time.Second}
	base := &http.Server{Addr: cfg.BaselineAddr, Handler: baselineMux(st, tok), ReadHeaderTimeout: 5 * time.Second}
	errc := make(chan error, 2)
	go func() { errc <- srv.ListenAndServe() }()
	go func() { errc <- base.ListenAndServe() }()
	log.Info("target-app arriba", "listen", cfg.ListenAddr, "baseline", cfg.BaselineAddr, "bug", cfg.Bug)

	select {
	case <-ctx.Done():
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error("servidor caido", "error", err.Error())
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	_ = base.Shutdown(shutdownCtx)
}

func appMux(st *store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"service": "target-app", "version": "ref-v1"})
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		if err := st.Ping(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db_down"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(openAPIDoc)
	})
	mux.HandleFunc("GET /products", func(w http.ResponseWriter, r *http.Request) {
		ps, err := st.ListProducts(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, ps)
	})
	mux.HandleFunc("POST /products", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name  string `json:"name"`
			Stock int    `json:"stock"`
		}
		if !decodeJSON(w, r, &in) || strings.TrimSpace(in.Name) == "" || in.Stock < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
			return
		}
		p, err := st.CreateProduct(r.Context(), in.Name, in.Stock)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusCreated, p)
	})
	mux.HandleFunc("GET /orders", func(w http.ResponseWriter, r *http.Request) {
		list, err := st.ListOrders(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, list)
	})
	mux.HandleFunc("POST /orders", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ProductID int64 `json:"product_id"`
			Qty       int   `json:"qty"`
		}
		if !decodeJSON(w, r, &in) || in.ProductID <= 0 || in.Qty <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
			return
		}
		o, err := st.PlaceOrder(r.Context(), in.ProductID, in.Qty)
		switch {
		case errors.Is(err, errNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "product_not_found"})
		case errors.Is(err, errInsufficientStock):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "insufficient_stock"})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		default:
			writeJSON(w, http.StatusCreated, o)
		}
	})
	return mux
}

// baselineMux: todo exige Authorization: Bearer <token>. Sin token o con token ajeno, 401.
func baselineMux(st *store, token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /__aqs/baseline/clean", func(w http.ResponseWriter, r *http.Request) {
		if err := st.Clean(r.Context()); err != nil {
			http.Error(w, "clean fallido\n", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /__aqs/baseline/verify", func(w http.ResponseWriter, r *http.Request) {
		n, err := st.Diff(r.Context())
		if err != nil {
			http.Error(w, "verify fallido\n", http.StatusInternalServerError)
			return
		}
		writeText(w, "%d\n", n)
	})
	mux.HandleFunc("GET /__aqs/baseline/version", func(w http.ResponseWriter, _ *http.Request) {
		writeText(w, "%s\n", baselineVersion)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authorized(r, token) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func authorized(r *http.Request, token string) bool {
	h := r.Header.Get("Authorization")
	const pfx = "Bearer "
	if !strings.HasPrefix(h, pfx) {
		return false
	}
	return constantTimeEq(strings.TrimPrefix(h, pfx), token)
}

func constantTimeEq(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

func readToken(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	t := strings.TrimSpace(string(b))
	if t == "" {
		return "", errors.New("token vacio")
	}
	return t, nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	return json.NewDecoder(r.Body).Decode(v) == nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeText(w http.ResponseWriter, format string, args ...any) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, format, args...)
}
