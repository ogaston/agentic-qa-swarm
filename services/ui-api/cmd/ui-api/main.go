// Command ui-api sirve el inbox: GET /notifications y POST /notifications/{id}/confirm.
//
// Variables de entorno:
//
//	LISTEN_ADDR               (por defecto :8080)
//	UIAPI_AUTH                obligatoria: identity (go-identity real) | fake (solo dev/prueba)
//	IDENTITY_URL              con UIAPI_AUTH=identity: URL http(s) de go-identity (obligatoria)
//	UIAPI_FAKE_TOKENS         con UIAPI_AUTH=fake: "token=id:rol,..."
//	UIAPI_ALLOW_FAKE_AUTH     con UIAPI_AUTH=fake: debe ser "true"; ademas no arranca con UIAPI_ENV=prod
//	UIAPI_ENV                 entorno (prod bloquea el verificador fake)
//	UIAPI_OUTBOX_FILE         outbox JSONL donde se publica run.confirmed (obligatoria)
//	UIAPI_EVENTS_FILE         outbox JSONL de go-intake (obligatoria)
//	UIAPI_DATA_DIR            directorio del registro de confirmaciones (obligatoria)
//	UIAPI_ALLOWED_ORIGINS     origenes CORS separados por coma (vacia = ninguno)
//	UIAPI_RATE_LIMIT_RPS      por defecto 10
//	UIAPI_RATE_LIMIT_BURST    por defecto 20
//	UIAPI_TRUST_PROXY         true para usar X-Forwarded-For
//	LOG_LEVEL                 debug|info|warn|error (por defecto info)
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/ui-api/inbox"
	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/auth"
	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/httpapi"
	"github.com/ogaston/agentic-qa-swarm/services/ui-api/internal/obs"
)

const serviceName = "ui-api"

// RepublishInterval es cada cuánto se reintentan los run.confirmed pendientes.
const RepublishInterval = 5 * time.Second

func main() {
	log := newLog(os.Stdout)
	if err := run(log); err != nil {
		log.Error("el servicio no arranca", "error", err.Error())
		os.Exit(1)
	}
}

func verifierFromEnv() (auth.TokenVerifier, error) {
	switch mode := os.Getenv("UIAPI_AUTH"); mode {
	case "":
		return nil, errors.New("UIAPI_AUTH es obligatorio (no hay modo abierto)")
	case "identity":
		return auth.NewHTTPTokenVerifier(os.Getenv("IDENTITY_URL"), nil)
	case "fake":
		if os.Getenv("UIAPI_ENV") == "prod" {
			return nil, errors.New("UIAPI_AUTH=fake esta prohibido con UIAPI_ENV=prod")
		}
		if os.Getenv("UIAPI_ALLOW_FAKE_AUTH") != "true" {
			return nil, errors.New("UIAPI_AUTH=fake exige UIAPI_ALLOW_FAKE_AUTH=true")
		}
		return auth.NewFakeTokenVerifier(os.Getenv("UIAPI_FAKE_TOKENS"))
	default:
		return nil, fmt.Errorf("UIAPI_AUTH=%q no soportado (identity|fake)", mode)
	}
}

// newLog crea el logger JSON del servicio con LOG_LEVEL del entorno y lo fija como predeterminado.
func newLog(w io.Writer) *slog.Logger {
	log := obs.NewLogger(w, serviceName, obs.ParseLevel(os.Getenv("LOG_LEVEL")))
	slog.SetDefault(log)
	return log
}

// newSubscriber crea el suscriptor del outbox; sus líneas (log.Logger) salen por el handler JSON
// a nivel info, de modo que «evento descartado» se ve con el nivel por defecto.
func newSubscriber(log *slog.Logger, eventsFile string) *inbox.FileSubscriber {
	return &inbox.FileSubscriber{Path: eventsFile, Logger: slog.NewLogLogger(log.Handler(), slog.LevelInfo)}
}

func run(log *slog.Logger) error {
	verifier, err := verifierFromEnv()
	if err != nil {
		return err
	}
	eventsFile, dataDir := os.Getenv("UIAPI_EVENTS_FILE"), os.Getenv("UIAPI_DATA_DIR")
	outboxFile := os.Getenv("UIAPI_OUTBOX_FILE")
	if eventsFile == "" || dataDir == "" || outboxFile == "" {
		return errors.New("UIAPI_EVENTS_FILE, UIAPI_DATA_DIR y UIAPI_OUTBOX_FILE son obligatorios")
	}
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	rps, burst := 10.0, 20
	if v := os.Getenv("UIAPI_RATE_LIMIT_RPS"); v != "" {
		if rps, err = strconv.ParseFloat(v, 64); err != nil || rps <= 0 {
			return errors.New("UIAPI_RATE_LIMIT_RPS invalido")
		}
	}
	if v := os.Getenv("UIAPI_RATE_LIMIT_BURST"); v != "" {
		if burst, err = strconv.Atoi(v); err != nil || burst <= 0 {
			return errors.New("UIAPI_RATE_LIMIT_BURST invalido")
		}
	}
	var origins []string
	for _, o := range strings.Split(os.Getenv("UIAPI_ALLOWED_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			origins = append(origins, o)
		}
	}

	store, err := inbox.OpenStore(dataDir, nil)
	if err != nil {
		return err
	}
	if store.Skipped > 0 {
		log.Warn("registro de confirmaciones: lineas ilegibles ignoradas", "count", store.Skipped)
	}
	pub := inbox.NewPublisher(store, inbox.NewOutbox(outboxFile), log)
	h, err := newHandler(log, httpapi.Config{
		Store: store, Verifier: verifier, Publisher: pub, AllowedOrigins: origins,
		RateRPS: rps, RateBurst: burst, TrustProxy: os.Getenv("UIAPI_TRUST_PROXY") == "true",
	}, dataDir, eventsFile)
	if err != nil {
		return err
	}

	sub := newSubscriber(log, eventsFile)
	handle := func(ev inbox.NotifyCreated) { store.Apply(ev) }
	if _, err := sub.Drain(handle); err != nil { // carga inicial antes de servir
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() { _ = sub.Run(ctx, handle) }()
	go pub.Run(ctx, RepublishInterval) // al arrancar y cada 5 s: run.confirmed pendientes

	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	log.Info("ui-api escuchando", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// newHandler arma la cadena completa. obs.Wrap queda POR FUERA de httpapi: /healthz, /readyz y
// /metrics no pasan por el token, el limitador por IP ni las cabeceras de seguridad de la API
// (excepción documentada en el README); el resto de las rutas sí.
func newHandler(log *slog.Logger, cfg httpapi.Config, dataDir, eventsFile string) (http.Handler, error) {
	reg := obs.NewRegistry()
	httpMetrics := obs.NewHTTPMetrics(reg, serviceName)
	// httpapi registra por Slog (con request_id y trace_id); su Logger clásico solo es el respaldo
	// de quien no configura Slog, así que aquí no se usa.
	cfg.Slog = log // las líneas dentro de una petición llevan request_id y trace_id
	cfg.Metrics = obs.NewInbox(reg, cfg.Store.Counts)
	// Métricas de la integración con identidad (solo si el verificador es el real).
	if iv, ok := cfg.Verifier.(*auth.HTTPTokenVerifier); ok {
		iv.SetObserver(obs.NewIdentity(reg, iv.CircuitOpen))
	}
	obs.RegisterPublishPending(reg, cfg.Store.PublishPendingCount)
	app, err := httpapi.New(cfg)
	if err != nil {
		return nil, err
	}
	return obs.Wrap(obs.Config{Service: serviceName, Log: log, Registry: reg, Metrics: httpMetrics,
		Ready: obs.ReadyChecks(dataDir, eventsFile, func() bool { return cfg.Verifier != nil }), Route: httpapi.RoutePattern}, app), nil
}
