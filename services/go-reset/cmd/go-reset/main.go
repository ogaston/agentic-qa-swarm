// Command go-reset sirve la API de reset y expone: idle-check, housekeeping, rebuild, teardown,
// reset-run --run <id> y render-reset-job --run <id>.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/adapters/kube"
	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/adapters/outbox"
	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/adapters/redis"
	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/adapters/script"
	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/adapters/sessions"
	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/config"
	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/core"
	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/fakes"
	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/httpapi"
	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/jobspec"
	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/obs"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }
func (realClock) Sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "":
		return serve()
	case "render-reset-job":
		fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
		runID := fs.String("run", "", "run_id")
		image := fs.String("image", os.Getenv("RESET_JOB_IMAGE"), "imagen con tag fijado (por defecto RESET_JOB_IMAGE)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		j, err := jobspec.Build(*runID, *image)
		if err != nil {
			return err
		}
		b, err := jobspec.YAML(j)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(b)
		return err
	case "idle-check", "housekeeping", "rebuild", "teardown", "reset-run":
		return oneShot(cmd, args[1:])
	}
	return fmt.Errorf("subcomando %q desconocido", cmd)
}

type wiring struct {
	svc      *core.Service
	sessions core.SessionStore
	state    core.StateStore
	reg      *prometheus.Registry
}

func build(c config.Config) (*wiring, error) {
	log := obs.NewLogger(os.Stdout)
	ss, err := sessions.New(c.DataDir)
	if err != nil {
		return nil, err
	}
	w := &wiring{sessions: ss, reg: prometheus.NewRegistry()}
	svc := &core.Service{Sessions: ss, Events: outbox.New(c.OutboxFile), Alert: obs.LogAlerter{Log: log},
		Clock: realClock{}, Cfg: core.Config{DefaultWarmID: c.WarmID, BaselineVersion: c.BaselineVersion,
			ReadyTimeout: c.ReadyTimeout, ReadyInterval: c.ReadyInterval, Grace: c.Grace}}
	if c.Backend == "fake" {
		b := fakes.NewBundle()
		b.State.Exists = false
		svc.Kube, svc.DB, svc.Cache, w.state = b.Kube, b.DB, b.Cache, b.State
		b.DB.Ver = c.BaselineVersion
	} else {
		rc, err := rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
		cs, err := kubernetes.NewForConfig(rc)
		if err != nil {
			return nil, err
		}
		k, err := kube.New(cs, c.Namespace, nil)
		if err != nil {
			return nil, err
		}
		svc.Kube, w.state = k, k
		svc.DB = &script.Cleaner{Path: c.BaselineScript, Timeout: c.ScriptTimeout, StaticVersion: c.BaselineVersion}
		svc.Cache = &redis.Flusher{Addr: c.RedisAddr, Timeout: c.RedisTimeout}
	}
	svc.State = w.state
	svc.Metrics = obs.New(w.reg, func() float64 {
		s, ok, err := w.state.Get(context.Background())
		if err == nil && ok && s.State == core.StateQuarantine {
			return 1
		}
		return 0
	})
	w.svc = svc
	return w, nil
}

func serve() error {
	c, err := config.Load(true)
	if err != nil {
		return err
	}
	w, err := build(c)
	if err != nil {
		return err
	}
	h := httpapi.New(httpapi.Config{Token: c.Token, Service: w.svc, Sessions: w.sessions, Clock: realClock{},
		Log: obs.NewLogger(os.Stdout), Gatherer: w.reg})
	srv := &http.Server{Addr: c.Listen, Handler: h, ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func oneShot(cmd string, args []string) error {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	runID := fs.String("run", "", "run_id (reset-run)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := config.Load(false)
	if err != nil {
		return err
	}
	w, err := build(c)
	if err != nil {
		return err
	}
	ctx := context.Background()
	switch cmd {
	case "idle-check":
		ok, err := w.svc.IdleCheck(ctx)
		fmt.Println("scaled:", ok)
		return err
	case "housekeeping":
		rep, err := w.svc.Housekeeping(ctx)
		fmt.Printf("closed=%d reset=%v warm=%s\n", len(rep.Closed), rep.ResetRun, rep.WarmState)
		return err
	case "reset-run":
		res, err := w.svc.Reset(ctx, *runID, "")
		fmt.Printf("verified=%v state=%s\n", res.Verified, res.State.State)
		if err == nil && !res.Verified {
			err = errors.New("reset no verificado: warm en cuarentena")
		}
		return err
	default:
		res, err := w.svc.Rebuild(ctx, cmd, "")
		fmt.Printf("verified=%v state=%s\n", res.Verified, res.State.State)
		if err == nil && !res.Verified {
			err = errors.New("verificación fallida: warm en cuarentena")
		}
		return err
	}
}
