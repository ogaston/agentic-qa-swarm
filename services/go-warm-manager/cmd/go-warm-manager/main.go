// Command go-warm-manager gestiona el entorno warm y el deploy por corrida.
// Subcomando: render-deploy-job --run <id> --artifact-kind <k> --artifact-ref <ref>.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/adapters/kube"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/adapters/objstore"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/adapters/outbox"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/adapters/probe"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/internal/api"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/internal/obs"
)

func main() {
	if len(os.Args) > 1 {
		if os.Args[1] == "render-deploy-job" {
			if err := renderDeployJob(os.Args[2:], os.Getenv, os.Stdout); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
			return
		}
		fmt.Fprintln(os.Stderr, "uso: go-warm-manager [render-deploy-job --run <id> --artifact-kind <k> --artifact-ref <ref>]")
		os.Exit(2)
	}
	log := obs.NewLogger(os.Stdout)
	if err := run(os.Getenv, log); err != nil {
		log.Error("el servicio no arranca", "error", err.Error())
		os.Exit(1)
	}
}

func registries(getenv func(string) string) []string {
	v := getenv("WARM_ALLOWED_REGISTRIES")
	if strings.TrimSpace(v) == "" {
		return wm.DefaultAllowedRegistries
	}
	var out []string
	for _, r := range strings.Split(v, ",") {
		if r = strings.TrimSpace(r); r != "" {
			out = append(out, r)
		}
	}
	return out
}

func renderDeployJob(args []string, getenv func(string) string, out io.Writer) error {
	fs := flag.NewFlagSet("render-deploy-job", flag.ContinueOnError)
	runID := fs.String("run", "", "run_id")
	kind := fs.String("artifact-kind", "", "build-from-repo|published-image")
	ref := fs.String("artifact-ref", "", "referencia del artefacto")
	if err := fs.Parse(args); err != nil {
		return err
	}
	m, err := wm.BuildDeployJob(wm.JobConfig{AllowedRegistries: registries(getenv), DeployerImage: getenv("WARM_DEPLOYER_IMAGE")},
		*runID, 0, wm.Artifact{Kind: *kind, Ref: *ref})
	if err != nil {
		return err
	}
	b, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	_, err = out.Write(b)
	return err
}

func run(getenv func(string) string, log *slog.Logger) error {
	ns := getenv("WARM_NAMESPACE")
	if ns != wm.Namespace {
		return fmt.Errorf("WARM_NAMESPACE debe ser %q", wm.Namespace)
	}
	token := getenv("WARM_SERVICE_TOKEN")
	if token == "" {
		return errors.New("WARM_SERVICE_TOKEN es obligatorio")
	}
	outFile := getenv("WARM_OUTBOX_FILE")
	if outFile == "" {
		return errors.New("WARM_OUTBOX_FILE es obligatorio")
	}
	addr := getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	var cs kubernetes.Interface
	seed := wm.WarmState{WarmID: "warm-1", State: wm.StateReady, ResetVerified: true, BaselineVersion: "baseline-1"}
	switch getenv("WARM_KUBE") {
	case "fake":
		cs = fake.NewClientset()
		if s := getenv("WARM_FAKE_STATE"); s != "" {
			seed.State, seed.ResetVerified = s, s == wm.StateReady
		}
		if !seed.Valid() {
			return errors.New("WARM_FAKE_STATE invalido")
		}
	case "", "incluster":
		cfg, err := rest.InClusterConfig()
		if err != nil {
			return fmt.Errorf("sin configuracion de cluster: %w", err)
		}
		if cs, err = kubernetes.NewForConfig(cfg); err != nil {
			return err
		}
	default:
		return errors.New("WARM_KUBE debe ser incluster|fake")
	}

	var objs wm.ObjectStore
	switch getenv("WARM_OBJECT_STORE") {
	case "file", "":
		dir := getenv("WARM_OBJECT_DIR")
		if dir == "" {
			dir = os.TempDir() + "/warm-objects"
		}
		objs = &objstore.File{Dir: dir}
	case "s3":
		ep, ak, sk := getenv("WARM_S3_ENDPOINT"), getenv("WARM_S3_ACCESS_KEY"), getenv("WARM_S3_SECRET_KEY")
		if ep == "" || ak == "" || sk == "" {
			return errors.New("WARM_OBJECT_STORE=s3 exige WARM_S3_ENDPOINT, WARM_S3_ACCESS_KEY y WARM_S3_SECRET_KEY")
		}
		bucket := getenv("WARM_S3_BUCKET")
		if bucket == "" {
			bucket = "evidence"
		}
		s3, err := objstore.NewS3(ep, ak, sk, bucket, getenv("WARM_S3_SECURE") == "true")
		if err != nil {
			return err
		}
		objs = s3
	default:
		return errors.New("WARM_OBJECT_STORE debe ser file|s3")
	}

	appURL := getenv("WARM_APP_URL")
	if appURL == "" {
		appURL = "http://warm-app.aqs-test.svc"
	}
	metrics := obs.NewMetrics()
	svc := &wm.Service{
		Cfg:     wm.Config{Job: wm.JobConfig{AllowedRegistries: registries(getenv), DeployerImage: getenv("WARM_DEPLOYER_IMAGE")}},
		State:   &kube.StateStore{C: cs, Seed: seed},
		Probe:   &kube.Health{C: cs},
		Runtime: &kube.Runtime{C: cs},
		Jobs:    &kube.Jobs{C: cs},
		Surface: &probe.HTTP{Base: appURL},
		Objects: objs,
		Alerts:  logAlerter{log},
		Pub:     &outbox.File{Path: outFile},
		Clock:   wm.RealClock{}, Observer: metrics,
	}
	srv := &api.Server{Svc: svc, Token: token, Log: log, Registry: metrics.Reg}
	hs := srv.HTTPServer(addr)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hs.Shutdown(c)
	}()
	log.Info("escuchando", "addr", addr)
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// logAlerter registra el handoff (el aviso real a un humano llega con U4-T07).
type logAlerter struct{ log *slog.Logger }

func (a logAlerter) Handoff(_ context.Context, phase, runID, reason string) error {
	a.log.Warn("handoff a humano", "phase", phase, "run_id", runID, "reason", reason)
	return nil
}
