// Command go-identity es el servicio de autenticación. Subcomando: hash-password.
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
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/guard"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/passhash"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/server"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/session"
	"github.com/ogaston/agentic-qa-swarm/services/go-identity/internal/users"
)

func main() {
	if len(os.Args) > 1 {
		if os.Args[1] == "hash-password" && len(os.Args) == 2 {
			if err := hashPassword(os.Stdin, os.Stdout); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
			return
		}
		fmt.Fprintln(os.Stderr, "uso: go-identity [hash-password < contraseña]")
		os.Exit(2)
	}
	log := newLogger(os.Stdout)
	if err := run(log); err != nil {
		log.Error("el servicio no arranca", "error", err.Error())
		os.Exit(1)
	}
}

func newLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.MessageKey {
			a.Key = "message"
		}
		return a
	}}))
}

// hashPassword lee la contraseña de stdin (nunca de argumentos) y escribe el hash PHC.
func hashPassword(in io.Reader, out io.Writer) error {
	b, err := io.ReadAll(io.LimitReader(in, 4096))
	if err != nil {
		return err
	}
	pw := string(b)
	for _, suf := range []string{"\r\n", "\n"} {
		if len(pw) >= len(suf) && pw[len(pw)-len(suf):] == suf {
			pw = pw[:len(pw)-len(suf)]
			break
		}
	}
	if n := utf8.RuneCountInString(pw); n < 8 || n > 128 {
		return errors.New("la contraseña debe tener entre 8 y 128 caracteres")
	}
	h, err := passhash.Hash(pw, nil)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, h)
	return err
}

func envInt(name string, def int) (int, error) {
	v := os.Getenv(name)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%s inválido", name)
	}
	return n, nil
}

func envDur(name string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(name)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s inválido", name)
	}
	return d, nil
}

func run(log *slog.Logger) error {
	us, err := users.LoadFile(os.Getenv("IDENTITY_USERS_FILE"))
	if err != nil {
		return err
	}
	maxFail, err := envInt("IDENTITY_MAX_FAILURES", 5)
	if err != nil {
		return err
	}
	maxSess, err := envInt("IDENTITY_MAX_SESSIONS", 5)
	if err != nil {
		return err
	}
	ttl, err := envDur("IDENTITY_SESSION_TTL", 30*time.Minute)
	if err != nil {
		return err
	}
	idle, err := envDur("IDENTITY_IDLE_TTL", 15*time.Minute)
	if err != nil {
		return err
	}
	trust := false
	if v := os.Getenv("IDENTITY_TRUST_PROXY"); v != "" {
		if trust, err = strconv.ParseBool(v); err != nil {
			return errors.New("IDENTITY_TRUST_PROXY inválido")
		}
	}
	decoy, err := server.NewDecoy()
	if err != nil {
		return err
	}
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	srv := server.New(server.Config{
		Users:      us,
		Guard:      guard.New(maxFail, nil),
		Sessions:   session.New(session.Config{AbsoluteTTL: ttl, IdleTTL: idle, MaxPerUser: maxSess}),
		Decoy:      decoy,
		TrustProxy: trust,
		Logger:     log,
	})
	hs := &http.Server{
		Addr:              addr,
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
	log.Info("go-identity escuchando", "addr", addr, "users", len(us))
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		sc, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return hs.Shutdown(sc)
	}
}
