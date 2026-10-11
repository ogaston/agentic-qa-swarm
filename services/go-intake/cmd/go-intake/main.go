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

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
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
	dataDir := os.Getenv("INTAKE_DATA_DIR")
	pub, ready, err := publisherFromEnv(dataDir, []byte(secret))
	if err != nil {
		return err
	}
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	handler, err := newHandlerWith(log, []byte(secret), dataDir, os.Getenv("ARTIFACT_REGISTRY"), pub, ready)
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

// publisherFromEnv elige el transporte de eventos (EVENTS_TRANSPORT): file (por defecto, outbox JSONL
// de INTAKE_EVENTS_FILE) o nats (JetStream en NATS_URL, U8-T02). Devuelve el publicador y los
// chequeos de /readyz de ese transporte.
func publisherFromEnv(dataDir string, secret []byte) (intake.EventPublisher, []obs.Check, error) {
	switch os.Getenv("EVENTS_TRANSPORT") {
	case "", "file":
		eventsFile := os.Getenv("INTAKE_EVENTS_FILE")
		if dataDir == "" || eventsFile == "" {
			return nil, nil, errors.New("INTAKE_DATA_DIR e INTAKE_EVENTS_FILE son obligatorios")
		}
		return intake.NewOutbox(eventsFile), obs.ReadyChecks(dataDir, eventsFile, secret), nil
	case "nats":
		if dataDir == "" {
			return nil, nil, errors.New("INTAKE_DATA_DIR es obligatorio")
		}
		nc, err := intake.ConnectNATS(os.Getenv("NATS_URL"))
		if err != nil {
			return nil, nil, err
		}
		js, err := jetstream.New(nc)
		if err != nil {
			return nil, nil, err
		}
		return intake.NewNATSPublisher(js), natsReadyChecks(dataDir, secret, nc), nil
	default:
		return nil, nil, errors.New("EVENTS_TRANSPORT debe ser file o nats")
	}
}

// natsReadyChecks son los chequeos de /readyz con NATS: los del servicio sin el del outbox de archivo,
// más "nats" (falla sin conexión).
func natsReadyChecks(dataDir string, secret []byte, nc *nats.Conn) []obs.Check {
	var out []obs.Check
	for _, c := range obs.ReadyChecks(dataDir, "", secret) {
		if c.Name != "outbox" {
			out = append(out, c)
		}
	}
	return append(out, obs.Check{Name: "nats", Fn: func(context.Context) error { return intake.NATSReady(nc) }})
}

// newHandler arma la cadena completa con el transporte de archivo (firma histórica).
func newHandler(log *slog.Logger, secret []byte, dataDir, eventsFile, artifactRegistry string) (http.Handler, error) {
	return newHandlerWith(log, secret, dataDir, artifactRegistry, intake.NewOutbox(eventsFile), obs.ReadyChecks(dataDir, eventsFile, secret))
}

// newHandlerWith arma la cadena completa: webhook + obs.Wrap, que sirve /healthz, /readyz y /metrics
// (sin token) antes de la aplicación y añade identificadores, log de acceso y métricas HTTP.
func newHandlerWith(log *slog.Logger, secret []byte, dataDir, artifactRegistry string, pub intake.EventPublisher, ready []obs.Check) (http.Handler, error) {
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
		Publisher: pub,
		Resolver:  intake.NewArtifactResolver(artifactRegistry),
		Log:       log,
		Metrics:   obs.NewIntake(reg),
	})
	if err != nil {
		return nil, err
	}
	return obs.Wrap(obs.Config{Service: serviceName, Log: log, Registry: reg, Metrics: httpMetrics,
		Ready: ready, Route: intake.RoutePattern}, h), nil
}
