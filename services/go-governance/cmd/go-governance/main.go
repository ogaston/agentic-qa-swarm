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
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
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

func run(log *slog.Logger, env func(string) string) error {
	c, err := loadConfig(env)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.dataDir, 0o700); err != nil {
		return err
	}
	al, err := audit.Open(filepath.Join(c.dataDir, "audit.jsonl"), nil)
	if err != nil {
		return err // cadena rota o truncada: no se continúa sobre ella
	}
	defer al.Close()
	ps, err := policy.OpenFileStore(filepath.Join(c.dataDir, "policies.jsonl"), nil)
	if err != nil {
		return err
	}
	defer ps.Close()
	svc := service.New(service.Config{Policies: ps, Audit: al, TestNamespace: c.testNS})
	srv := server.New(server.Config{Service: svc, Verifier: c.verifier, Token: c.token, Logger: log})
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
	errc := make(chan error, 1)
	go func() { errc <- hs.ListenAndServe() }()
	log.Info("go-governance escuchando", "addr", c.addr, "test_namespace", c.testNS, "env", strconv.Quote(env("GOVERNANCE_ENV")))
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		sc, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return hs.Shutdown(sc)
	}
}
