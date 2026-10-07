// Command go-run-controller es el controlador de corridas (C9): máquina de estados persistida
// con un gate de U4 en cada transición y GET /runs/{id}.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/adapters"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/internal/httpapi"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/internal/obs"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/rehearsal"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runner"
	"k8s.io/client-go/kubernetes"
)

const serviceName = "go-run-controller"

type config struct {
	namespace, dataDir, eventsFile, outboxFile       string
	govURL, govToken, identityURL, addr              string
	real                                             bool
	warmURL, warmToken, resetURL, resetToken         string
	artifactRef, rehearsalImage, rehearsalSA         string
	rehearsalTarget                                  string
	runnerImage, runnerSA                            string
	evEndpoint, evBucket, evAccessFile, evSecretFile string
	maxParallel                                      int
	flowTimeout, runTimeout                          time.Duration
}

// parseSeconds lee una duración en segundos (vacío: def). Valor no numérico o <=0 es un error.
func parseSeconds(name, v string, def int) (time.Duration, error) {
	if v == "" {
		return time.Duration(def) * time.Second, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 86400 {
		return 0, fmt.Errorf("%s debe ser un entero de segundos entre 1 y 86400", name)
	}
	return time.Duration(n) * time.Second, nil
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
	// EVIDENCE_ENDPOINT, si está, se valida siempre (también en fake): un valor no http(s) no arranca.
	c.evEndpoint = env("EVIDENCE_ENDPOINT")
	if c.evEndpoint != "" {
		if u, err := url.Parse(c.evEndpoint); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return c, errors.New("EVIDENCE_ENDPOINT debe ser http(s)://host[:puerto]")
		}
	}
	c.maxParallel = 3
	if v := env("RUN_MAX_PARALLEL_RUNNERS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 20 {
			return c, errors.New("RUN_MAX_PARALLEL_RUNNERS debe ser un entero entre 1 y 20")
		}
		c.maxParallel = n
	}
	var err error
	if c.flowTimeout, err = parseSeconds("RUNNER_FLOW_TIMEOUT_SECONDS", env("RUNNER_FLOW_TIMEOUT_SECONDS"), 300); err != nil {
		return c, err
	}
	if c.runTimeout, err = parseSeconds("RUNNER_RUN_TIMEOUT_SECONDS", env("RUNNER_RUN_TIMEOUT_SECONDS"), 900); err != nil {
		return c, err
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
	case "real":
		c.real = true
		c.warmURL, c.warmToken, c.resetURL, c.resetToken = env("WARM_URL"), env("WARM_SERVICE_TOKEN"), env("RESET_URL"), env("RESET_SERVICE_TOKEN")
		c.artifactRef, c.rehearsalImage, c.rehearsalTarget = env("RUN_ARTIFACT_REF"), env("REHEARSAL_IMAGE"), env("REHEARSAL_TARGET_URL")
		c.rehearsalSA = env("REHEARSAL_SERVICE_ACCOUNT")
		c.runnerImage, c.runnerSA = env("RUNNER_IMAGE"), env("RUNNER_SERVICE_ACCOUNT")
		c.evBucket, c.evAccessFile, c.evSecretFile = env("EVIDENCE_BUCKET"), env("EVIDENCE_ACCESS_KEY_FILE"), env("EVIDENCE_SECRET_KEY_FILE")
		if c.runnerSA == "" {
			c.runnerSA = "aqs-runner"
		}
		if c.rehearsalSA == "" {
			c.rehearsalSA = "aqs-runner"
		}
		for _, m := range []struct{ n, v string }{{"WARM_URL", c.warmURL}, {"WARM_SERVICE_TOKEN", c.warmToken}, {"RESET_URL", c.resetURL},
			{"RESET_SERVICE_TOKEN", c.resetToken}, {"RUN_ARTIFACT_REF", c.artifactRef}, {"REHEARSAL_IMAGE", c.rehearsalImage},
			{"REHEARSAL_TARGET_URL", c.rehearsalTarget},
			{"RUNNER_IMAGE", c.runnerImage}, {"EVIDENCE_ENDPOINT", c.evEndpoint}, {"EVIDENCE_BUCKET", c.evBucket},
			{"EVIDENCE_ACCESS_KEY_FILE", c.evAccessFile}, {"EVIDENCE_SECRET_KEY_FILE", c.evSecretFile}} {
			if m.v == "" {
				return c, fmt.Errorf("RUN_PHASES=real: %s es obligatorio", m.n)
			}
		}
		for _, u := range []struct{ n, v string }{{"WARM_URL", c.warmURL}, {"RESET_URL", c.resetURL}, {"REHEARSAL_TARGET_URL", c.rehearsalTarget}} {
			if _, err := adapters.ValidateHTTPURL(u.v); err != nil {
				return c, fmt.Errorf("%s %w", u.n, err)
			}
		}
		if err := (rehearsal.Config{Namespace: c.namespace, Image: c.rehearsalImage, ServiceAccount: c.rehearsalSA, TargetURL: c.rehearsalTarget}).Validate(); err != nil {
			return c, err
		}
		if err := (runner.Config{Namespace: c.namespace, Image: c.runnerImage, ServiceAccount: c.runnerSA, DeadlineSeconds: int64(c.flowTimeout / time.Second)}).Validate(); err != nil {
			return c, err
		}
	default:
		return c, errors.New("RUN_PHASES es obligatorio: fake (con RUN_ALLOW_FAKE_PHASES=true) o real")
	}
	return c, nil
}

type logAlerter struct{ log *slog.Logger }

func (a logAlerter) Handoff(ctx context.Context, runID, phase, reason string) {
	a.log.ErrorContext(ctx, "handoff registrado", "run_id", runID, "phase", phase, "reason", reason)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "render-rehearsal-job" {
		if err := renderRehearsalJob(os.Stdout, os.Args[2:], os.Getenv); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "render-runner-job" {
		if err := renderRunnerJob(os.Stdout, os.Args[2:], os.Getenv); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
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
	gate, err := adapters.NewHTTPGate(c.govURL, c.govToken)
	if err != nil {
		return err
	}
	reg := obs.NewRegistry()
	var store *adapters.JournalStore
	active := func() float64 {
		n := 0
		if store == nil {
			return 0
		}
		for _, r := range store.List() {
			if !r.State.Terminal() {
				n++
			}
		}
		return float64(n)
	}
	metrics := obs.NewRunMetrics(reg, active)
	store, err = openJournal(c.dataDir, log, metrics)
	if err != nil {
		return err
	}
	defer store.Close()
	var cs kubernetes.Interface
	if c.real {
		if cs, err = inClusterClient(); err != nil {
			return err
		}
	}
	cfg, err := buildConfig(c, cs, gate, store, &adapters.Outbox{Path: c.outboxFile}, logAlerter{log}, metrics, obs.NewRunnerMetrics(reg), log)
	if err != nil {
		return err
	}
	ctl, err := runctl.New(cfg)
	if err != nil {
		return err
	}
	src := &adapters.FileSource{Path: c.eventsFile}
	checks := readyChecks(store, ctl)
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

// openJournal abre el diario registrando la cola descartada (log y aqs_journal_tail_discarded_total).
func openJournal(dir string, log *slog.Logger, m interface{ JournalTailDiscarded() }) (*adapters.JournalStore, error) {
	j, err := adapters.OpenJournalOpts(dir, adapters.JournalOptions{Log: log})
	if err != nil {
		return nil, err
	}
	noteJournalTail(j, m)
	return j, nil
}

// noteJournalTail cuenta en aqs_journal_tail_discarded_total la cola incompleta descartada al abrir.
func noteJournalTail(j interface{ TailDiscarded() int }, m interface{ JournalTailDiscarded() }) {
	if j.TailDiscarded() > 0 {
		m.JournalTailDiscarded()
	}
}

// readyChecks son los chequeos de /readyz: el diario escribible y el último Save exitoso.
func readyChecks(journal interface{ Healthy() error }, ctl interface{ PersistHealthy() error }) []obs.Check {
	return []obs.Check{
		{Name: "journal", Fn: func(context.Context) error { return journal.Healthy() }},
		{Name: "persist", Fn: func(context.Context) error { return ctl.PersistHealthy() }}, // el último Save salió bien
	}
}

// tickEvery es el periodo del lazo (variable solo para las pruebas).
var tickEvery = 500 * time.Millisecond

func loop(ctx context.Context, log *slog.Logger, ctl *runctl.Controller, src runctl.EventSource) {
	t := time.NewTicker(tickEvery)
	defer t.Stop()
	for {
		tick(ctx, log, ctl, src)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// tick es un paso del lazo: entrega eventos y avanza las corridas. El evento se confirma (Ack)
// solo cuando Apply no falló por el almacén; con ErrPersist se corta y se vuelve a entregar.
func tick(ctx context.Context, log *slog.Logger, ctl *runctl.Controller, src runctl.EventSource) {
	evs, err := src.Poll(ctx)
	if err != nil {
		log.WarnContext(ctx, "leyendo eventos", "error", err.Error())
	}
	for _, ev := range evs {
		err := ctl.Apply(ctx, ev)
		if errors.Is(err, runctl.ErrPersist) {
			log.WarnContext(ctx, "evento no aplicado por el almacén; se reintenta", "type", ev.Type, "run_id", ev.RunID, "error", err.Error())
			break
		}
		if err != nil {
			log.WarnContext(ctx, "evento no aplicado", "type", ev.Type, "run_id", ev.RunID, "error", err.Error())
		}
		src.Ack(ev)
	}
	ctl.DriveAll(ctx)
}
