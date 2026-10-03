// Command go-governance es el servicio de políticas, gates y auditoría.
// Subcomando: verify-audit <archivo>.
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
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/authz"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/audit"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/auth"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/obs"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/policy"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/server"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/service"
)

func main() {
	if len(os.Args) > 1 {
		if os.Args[1] == "verify-audit" && len(os.Args) == 3 {
			os.Exit(verifyAudit(os.Args[2], os.Stdout, os.Stderr))
		}
		fmt.Fprintln(os.Stderr, "uso: go-governance [verify-audit <archivo>]")
		os.Exit(2)
	}
	log := obs.NewLogger(os.Stdout, server.ServiceName, obs.ParseLevel(os.Getenv("LOG_LEVEL")))
	if err := run(log, os.Getenv); err != nil {
		log.Error("el servicio no arranca", "error", err.Error())
		os.Exit(1)
	}
}

// verifyAudit devuelve 0 si la cadena es íntegra; 1 si no (indicando la línea); 2 si no se pudo leer.
func verifyAudit(path string, out, errw io.Writer) int {
	entries, err := audit.VerifyFile(path)
	var ve *audit.VerifyError
	switch {
	case errors.As(err, &ve):
		fmt.Fprintf(errw, "cadena ROTA en %s: %v\n", path, ve)
		return 1
	case err != nil:
		fmt.Fprintf(errw, "error: %v\n", err)
		return 2
	}
	fmt.Fprintf(out, "cadena íntegra: %d entradas\n", len(entries))
	return 0
}

// config es la configuración ya validada.
type config struct {
	addr, dataDir, testNS string
	verifier              auth.TokenVerifier
	token                 auth.ServiceToken
	identityURL           string
	retentionDays         int
	verifyInterval        time.Duration
}

// MinRetentionDays es la retención mínima de la auditoría; el servicio no arranca con menos.
const MinRetentionDays = 90

// DefaultVerifyInterval es cada cuánto se verifica la cadena de auditoría.
const DefaultVerifyInterval = 15 * time.Minute

func parseRetention(v string) (int, error) {
	if v == "" {
		return MinRetentionDays, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, errors.New("GOVERNANCE_AUDIT_RETENTION_DAYS inválido (entero)")
	}
	if n < MinRetentionDays {
		return 0, fmt.Errorf("GOVERNANCE_AUDIT_RETENTION_DAYS=%d: la retención mínima de la auditoría es %d días", n, MinRetentionDays)
	}
	return n, nil
}

func parseVerifyInterval(v string) (time.Duration, error) {
	if v == "" {
		return DefaultVerifyInterval, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, errors.New("GOVERNANCE_VERIFY_INTERVAL inválido (duración positiva)")
	}
	return d, nil
}

func loadConfig(env func(string) string) (config, error) {
	var c config
	c.addr = env("LISTEN_ADDR")
	if c.addr == "" {
		c.addr = ":8080"
	}
	c.dataDir = env("GOVERNANCE_DATA_DIR")
	if c.dataDir == "" {
		return c, errors.New("GOVERNANCE_DATA_DIR es obligatorio")
	}
	var err error
	if c.retentionDays, err = parseRetention(env("GOVERNANCE_AUDIT_RETENTION_DAYS")); err != nil {
		return c, err
	}
	if c.verifyInterval, err = parseVerifyInterval(env("GOVERNANCE_VERIFY_INTERVAL")); err != nil {
		return c, err
	}
	c.identityURL = env("IDENTITY_URL")
	c.testNS = env("GOVERNANCE_TEST_NAMESPACE")
	if c.testNS == "" {
		c.testNS = authz.DefaultTestNamespace
	}
	switch strings.ToLower(strings.TrimSpace(c.testNS)) {
	case "staging", "prod", "production", "default", "kube-system":
		return c, errors.New("GOVERNANCE_TEST_NAMESPACE no puede ser un namespace de staging/prod")
	}
	tok, err := auth.NewServiceToken(env("GOVERNANCE_SERVICE_TOKEN"))
	if err != nil {
		return c, err
	}
	c.token = tok
	switch env("GOVERNANCE_AUTH") {
	case "identity":
		v, err := auth.NewHTTPTokenVerifier(env("IDENTITY_URL"), 5*time.Second)
		if err != nil {
			return c, err
		}
		c.verifier = v
	case "fake":
		if env("GOVERNANCE_ALLOW_FAKE_AUTH") != "true" {
			return c, errors.New("GOVERNANCE_AUTH=fake exige GOVERNANCE_ALLOW_FAKE_AUTH=true")
		}
		switch strings.ToLower(strings.TrimSpace(env("GOVERNANCE_ENV"))) {
		case "prod", "production":
			return c, errors.New("GOVERNANCE_AUTH=fake no se permite con GOVERNANCE_ENV=prod")
		}
		f, err := auth.ParseFakeTokens(env("GOVERNANCE_FAKE_TOKENS"))
		if err != nil {
			return c, err
		}
		for t := range f.Tokens {
			if c.token.Matches(t) {
				return c, errors.New("un token de persona no puede ser el token de servicio")
			}
		}
		c.verifier = f
	default:
		return c, errors.New("GOVERNANCE_AUTH es obligatorio: identity | fake")
	}
	return c, nil
}

// dataDirCheck comprueba que el directorio de datos es escribible: abre el log de auditoría
// para añadir (sin crear ni truncar nada) y lo sincroniza.
func dataDirCheck(dir string) func(context.Context) error {
	return func(context.Context) error {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			return errors.New("directorio de datos inaccesible")
		}
		f, err := os.OpenFile(filepath.Join(dir, "audit.jsonl"), os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			return fmt.Errorf("auditoría no escribible: %w", err)
		}
		defer f.Close()
		return f.Sync()
	}
}

// policyCheck comprueba que el almacén de políticas es utilizable: sin envenenar y con
// policies.jsonl legible y coherente (policy.FileStore.Healthy).
func policyCheck(ps interface{ Healthy() error }) func(context.Context) error {
	return func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := ps.Healthy(); err != nil {
			return fmt.Errorf("políticas no disponibles: %w", err)
		}
		return nil
	}
}

// identityCheck comprueba que go-identity responde (GET {base}/healthz) dentro del plazo de la petición.
func identityCheck(base string) func(context.Context) error {
	cl := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/healthz", nil)
		if err != nil {
			return err
		}
		resp, err := cl.Do(req)
		if err != nil {
			return errors.New("identidad inalcanzable")
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("identidad respondió %d", resp.StatusCode)
		}
		return nil
	}
}

func run(log *slog.Logger, env func(string) string) error {
	c, err := loadConfig(env)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.dataDir, 0o700); err != nil {
		return err
	}
	reg := obs.NewRegistry()
	gov := obs.NewGov(reg, c.retentionDays, log)
	al, err := audit.Open(filepath.Join(c.dataDir, "audit.jsonl"), nil)
	if err != nil {
		return err // cadena rota o truncada: no se continúa sobre ella
	}
	defer al.Close()
	al.OnAppend = gov.AuditAppended
	gov.SetLastAppend(al.LastAt())
	ps, err := policy.OpenFileStore(filepath.Join(c.dataDir, "policies.jsonl"), nil)
	if err != nil {
		return err
	}
	defer ps.Close()
	mon := obs.NewChainMonitor(al.VerifyNow, gov, 10*time.Second, nil, log)
	_ = mon.Verify() // estado inicial de aqs_audit_chain_ok (Open ya exigió una cadena íntegra)
	checks := []obs.Check{
		{Name: "data_dir", Fn: dataDirCheck(c.dataDir)},
		{Name: "policies", Fn: policyCheck(ps)},
		{Name: "audit_chain", Fn: mon.Check},
	}
	if c.identityURL != "" {
		checks = append(checks, obs.Check{Name: "identity", Fn: identityCheck(c.identityURL)})
	}
	svc := service.New(service.Config{Policies: ps, Audit: al, TestNamespace: c.testNS, Observer: gov})
	srv := server.New(server.Config{Service: svc, Verifier: c.verifier, Token: c.token, Logger: log, Registry: reg, Ready: checks})
	hs := &http.Server{
		Addr:              c.addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go mon.Run(ctx, c.verifyInterval)
	errc := make(chan error, 1)
	go func() { errc <- hs.ListenAndServe() }()
	log.Info("go-governance escuchando", "addr", c.addr, "test_namespace", c.testNS, "env", strconv.Quote(env("GOVERNANCE_ENV")),
		"audit_retention_days", c.retentionDays)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		sc, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return hs.Shutdown(sc)
	}
}
