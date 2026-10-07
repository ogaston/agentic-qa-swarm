// Command go-run-controller es el controlador de corridas (C9): máquina de estados persistida
// con un gate de U4 en cada transición y GET /runs/{id}.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/adapters"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/internal/httpapi"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/internal/obs"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

const serviceName = "go-run-controller"

type config struct {
	namespace, dataDir, eventsFile, outboxFile string
	govURL, govToken, identityURL, addr        string
}

func loadConfig(env func(string) string) (config, error) {
	c := config{namespace: env("RUN_TEST_NAMESPACE"), dataDir: env("RUN_DATA_DIR"), eventsFile: env("RUN_EVENTS_FILE"),
		outboxFile: env("RUN_OUTBOX_FILE"), govURL: env("GOVERNANCE_URL"), govToken: env("GOVERNANCE_SERVICE_TOKEN"),
		identityURL: env("IDENTITY_URL"), addr: env("LISTEN_ADDR")}
	if c.namespace == "" {
		c.namespace = runctl.DefaultNamespace
	}
	switch strings.ToLower(strings.TrimSpace(c.namespace)) {
	case "staging", "prod", "production", "default", "kube-system":
		return c, errors.New("RUN_TEST_NAMESPACE no puede ser un namespace de staging/prod")
	}
	for _, m := range []struct{ n, v string }{{"RUN_DATA_DIR", c.dataDir}, {"RUN_EVENTS_FILE", c.eventsFile},
		{"RUN_OUTBOX_FILE", c.outboxFile}, {"GOVERNANCE_URL", c.govURL}, {"GOVERNANCE_SERVICE_TOKEN", c.govToken},
		{"IDENTITY_URL", c.identityURL}, {"LISTEN_ADDR", c.addr}} {
		if m.v == "" {
			return c, fmt.Errorf("%s es obligatorio", m.n)
		}
	}
	for _, u := range []struct{ n, v string }{{"GOVERNANCE_URL", c.govURL}, {"IDENTITY_URL", c.identityURL}} {
		if _, err := adapters.ValidateHTTPURL(u.v); err != nil {
			return c, fmt.Errorf("%s %w", u.n, err)
		}
	}
	switch env("RUN_PHASES") {
	case "fake":
		if env("RUN_ALLOW_FAKE_PHASES") != "true" {
			return c, errors.New("RUN_PHASES=fake exige RUN_ALLOW_FAKE_PHASES=true")
		}
		switch strings.ToLower(strings.TrimSpace(env("RUN_ENV"))) {
		case "prod", "production":
			return c, errors.New("RUN_PHASES=fake no se permite con RUN_ENV=prod")
		}
	default:
		return c, errors.New("RUN_PHASES es obligatorio: los adaptadores reales de fase llegan con U2-T03 a T06 (solo fake)")
	}
	return c, nil
}

type logAlerter struct{ log *slog.Logger }

func (a logAlerter) Handoff(ctx context.Context, runID, phase, reason string) {
	a.log.ErrorContext(ctx, "handoff registrado", "run_id", runID, "phase", phase, "reason", reason)
}

func main() {
	log := obs.NewLogger(os.Stdout, serviceName, obs.ParseLevel(os.Getenv("LOG_LEVEL")))
	if err := run(log, os.Getenv); err != nil {
		log.Error("el servicio no arranca", "error", err.Error())
		os.Exit(1)
	}
}

func run(log *slog.Logger, env func(string) string) error {
	c, err := loadConfig(env)
	if err != nil {
		return err
	}
	store, err := adapters.OpenJournal(c.dataDir)
	if err != nil {
		return err
	}
	defer store.Close()
	gate, err := adapters.NewHTTPGate(c.govURL, c.govToken)
	if err != nil {
		return err
	}
	reg := obs.NewRegistry()
	active := func() float64 {
		n := 0
		for _, r := range store.List() {
			if !r.State.Terminal() {
				n++
			}
		}
		return float64(n)
	}
	metrics := obs.NewRunMetrics(reg, active)
	ctl, err := runctl.New(runctl.Config{Namespace: c.namespace, Gate: gate, Store: store,
		Publisher: &adapters.Outbox{Path: c.outboxFile}, Warm: &runctl.FakeWarm{Fact: runctl.True},
		Alerter: logAlerter{log}, Phases: &runctl.FakePhases{}, Observer: metrics, Log: log})
	if err != nil {
		return err
	}
	src := &adapters.FileSource{Path: c.eventsFile}
	checks := []obs.Check{{Name: "journal", Fn: func(context.Context) error { return store.Healthy() }}}
	app := httpapi.New(store, httpapi.NewIdentityVerifier(strings.TrimRight(c.identityURL, "/")))
	h := obs.Wrap(obs.Config{Service: serviceName, Log: log, Registry: reg, Metrics: obs.NewHTTPMetrics(reg, serviceName), Ready: checks,
		Route: func(r *http.Request) string {
			if strings.HasPrefix(r.URL.Path, "/runs/") {
				return "/runs/{id}"
			}
			return "unmatched"
		}}, app)
	hs := &http.Server{Addr: c.addr, Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go loop(ctx, log, ctl, src)
	errc := make(chan error, 1)
	go func() { errc <- hs.ListenAndServe() }()
	log.Info("go-run-controller escuchando", "addr", c.addr, "namespace", c.namespace)
	select {
	case <-ctx.Done():
		sc, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return hs.Shutdown(sc)
	case err := <-errc:
		return err
	}
}

func loop(ctx context.Context, log *slog.Logger, ctl *runctl.Controller, src runctl.EventSource) {
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	for {
		evs, err := src.Poll(ctx)
		if err != nil {
			log.WarnContext(ctx, "leyendo eventos", "error", err.Error())
		}
		for _, ev := range evs {
			if err := ctl.Apply(ctx, ev); err != nil {
				log.WarnContext(ctx, "evento no aplicado", "type", ev.Type, "run_id", ev.RunID, "error", err.Error())
			}
		}
		ctl.DriveAll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
