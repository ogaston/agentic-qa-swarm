// Command go-intake sirve POST /webhooks/github.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/githubsig"
	"github.com/ogaston/agentic-qa-swarm/services/go-intake/internal/intake"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("go-intake: %v", err)
	}
}

func run() error {
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
	store, err := intake.OpenJSONLStore(dataDir)
	if err != nil {
		return err
	}
	h, err := intake.NewHandler(intake.Deps{
		Secret:    []byte(secret),
		Verifier:  githubsig.HMACVerifier{},
		Store:     store,
		Publisher: intake.NewOutbox(eventsFile),
		Resolver:  intake.StubResolver{},
	})
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
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
	log.Printf("go-intake escuchando en %s", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
