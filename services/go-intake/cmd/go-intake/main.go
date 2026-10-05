// Command go-intake sirve POST /webhooks/github.
package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/githubsig"
	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/intake"
	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/obs"
)

const serviceName = "go-intake"

func main() {
	log := newLog(os.Stdout)
	if err := run(log); err != nil {
		log.Error("el servicio no arranca", "error", err.Error())
		os.Exit(1)
	}
}

// newLog crea el logger JSON del servicio con LOG_LEVEL del entorno y lo fija como predeterminado,
// de modo que los log.Printf residuales del paquete intake salen también en JSON.
func newLog(w io.Writer) *slog.Logger {
	log := obs.NewLogger(w, serviceName, obs.ParseLevel(os.Getenv("LOG_LEVEL")))
	slog.SetDefault(log)
	return log
}

func run(log *slog.Logger) error {
	secret := os.Getenv("GITHUB_WEBHOOK_SECRET")
	if secret == "" {
		return errors.New("GITHUB_WEBHOOK_SECRET es obligatorio")
	}
	dataDir, eventsFile := os.Getenv("INTAKE_DATA_DIR"), os.Getenv("INTAKE_EVENTS_FILE")
	if dataDir == "" || eventsFile == "" {
		return errors.New("INTAKE_DATA_DIR e INTAKE_EVENTS_FILE son obligatorios")
	}
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	handler, err := newHandler(log, []byte(secret), dataDir, eventsFile, os.Getenv("ARTIFACT_REGISTRY"))
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	log.Info("go-intake escuchando", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// newHandler arma la cadena completa: webhook + obs.Wrap, que sirve /healthz, /readyz y /metrics
// (sin token) antes de la aplicación y añade identificadores, log de acceso y métricas HTTP.
func newHandler(log *slog.Logger, secret []byte, dataDir, eventsFile, artifactRegistry string) (http.Handler, error) {
	store, err := intake.OpenJSONLStore(dataDir)
	if err != nil {
		return nil, err
	}
	reg := obs.NewRegistry()
	httpMetrics := obs.NewHTTPMetrics(reg, serviceName)
	h, err := intake.NewHandler(intake.Deps{
		Secret:    secret,
		Verifier:  githubsig.HMACVerifier{},
		Store:     store,
		Publisher: intake.NewOutbox(eventsFile),
		Resolver:  intake.NewArtifactResolver(artifactRegistry),
		Log:       log,
		Metrics:   obs.NewIntake(reg),
	})
	if err != nil {
		return nil, err
	}
	return obs.Wrap(obs.Config{Service: serviceName, Log: log, Registry: reg, Metrics: httpMetrics,
		Ready: obs.ReadyChecks(dataDir, eventsFile, secret), Route: intake.RoutePattern}, h), nil
}
