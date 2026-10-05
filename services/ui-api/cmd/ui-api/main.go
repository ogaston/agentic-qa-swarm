// Command ui-api sirve el inbox: GET /notifications y POST /notifications/{id}/confirm.
//
// Variables de entorno:
//
//	LISTEN_ADDR               (por defecto :8080)
//	UIAPI_AUTH                obligatoria; unico valor soportado hoy: fake
//	UIAPI_FAKE_TOKENS         con UIAPI_AUTH=fake: "token=id:rol,..." (solo dev/prueba)
//	UIAPI_EVENTS_FILE         outbox JSONL de go-intake (obligatoria)
//	UIAPI_DATA_DIR            directorio del registro de confirmaciones (obligatoria)
//	UIAPI_ALLOWED_ORIGINS     origenes CORS separados por coma (vacia = ninguno)
//	UIAPI_RATE_LIMIT_RPS      por defecto 10
//	UIAPI_RATE_LIMIT_BURST    por defecto 20
//	UIAPI_TRUST_PROXY         true para usar X-Forwarded-For
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
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
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("ui-api: %v", err)
	}
}

func verifierFromEnv() (auth.TokenVerifier, error) {
	switch mode := os.Getenv("UIAPI_AUTH"); mode {
	case "":
		return nil, errors.New("UIAPI_AUTH es obligatorio (no hay modo abierto)")
	case "fake":
		return auth.NewFakeTokenVerifier(os.Getenv("UIAPI_FAKE_TOKENS"))
	default:
		return nil, fmt.Errorf("UIAPI_AUTH=%q no soportado (la verificacion real llega con U1-T07)", mode)
	}
}

func run() error {
	verifier, err := verifierFromEnv()
	if err != nil {
		return err
	}
	eventsFile, dataDir := os.Getenv("UIAPI_EVENTS_FILE"), os.Getenv("UIAPI_DATA_DIR")
	if eventsFile == "" || dataDir == "" {
		return errors.New("UIAPI_EVENTS_FILE y UIAPI_DATA_DIR son obligatorios")
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
		log.Printf("registro de confirmaciones: %d lineas ilegibles ignoradas", store.Skipped)
	}
	h, err := httpapi.New(httpapi.Config{
		Store: store, Verifier: verifier, AllowedOrigins: origins,
		RateRPS: rps, RateBurst: burst, TrustProxy: os.Getenv("UIAPI_TRUST_PROXY") == "true",
		Logger: log.Default(),
	})
	if err != nil {
		return err
	}

	sub := &inbox.FileSubscriber{Path: eventsFile, Logger: log.Default()}
	handle := func(ev inbox.NotifyCreated) { store.Apply(ev) }
	if _, err := sub.Drain(handle); err != nil { // carga inicial antes de servir
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() { _ = sub.Run(ctx, handle) }()

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
	log.Printf("ui-api escuchando en %s", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
