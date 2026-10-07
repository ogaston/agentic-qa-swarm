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
	"net"
	"net/http"
	"net/url"
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

	readyTO, err := durationEnv(getenv, "WARM_READY_TIMEOUT", wm.DefaultWarmReadyTimeout)
	if err != nil {
		return err
	}
	jobTO, err := durationEnv(getenv, "WARM_JOB_TIMEOUT", 11*time.Minute)
	if err != nil {
		return err
	}
	pollIv, err := durationEnv(getenv, "WARM_POLL_INTERVAL", 2*time.Second)
	if err != nil {
		return err
	}

	if err := validateTimeouts(readyTO, jobTO, pollIv); err != nil {
		return err
	}

	var cs kubernetes.Interface
	seed := safeSeed()
	switch getenv("WARM_KUBE") {
	case "fake":
		if err := checkFakeAllowed(getenv); err != nil {
			return err
		}
		cs = fake.NewClientset()
		if s := getenv("WARM_FAKE_STATE"); s != "" {
			seed.State, seed.ResetVerified = s, s == wm.StateReady
		}
		if !seed.Valid() {
			return errors.New("WARM_FAKE_STATE invalido")
		}
	case "", "incluster":
		if getenv("WARM_DEPLOYER_IMAGE") == "" {
			return errors.New("WARM_DEPLOYER_IMAGE es obligatoria fuera del modo fake (la imagen por defecto es un placeholder)")
		}
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
	case "":
		return errors.New("WARM_OBJECT_STORE es obligatorio y explicito (file|s3)")
	case "file":
		if err := checkFileStoreAllowed(getenv); err != nil {
			return err
		}
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
		s3, err := objstore.NewS3(ep, ak, sk, bucket, getenv("WARM_S3_SECURE") != "false") // seguro por defecto; MinIO in-cluster declara false explicitamente
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
	if err := validateAppURL(appURL); err != nil {
		return err
	}
	metrics := obs.NewMetrics()
	svc := &wm.Service{
		Cfg: wm.Config{Job: wm.JobConfig{AllowedRegistries: registries(getenv), DeployerImage: getenv("WARM_DEPLOYER_IMAGE")},
			WarmReadyTimeout: readyTO, JobTimeout: jobTO, PollInterval: pollIv},
		Log:     log,
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
	// Deploys huerfanos de un arranque anterior: se resuelven antes de aceptar trafico (ver ResolveOrphans).
	if err := svc.ResolveOrphans(ctx, "00000000000000000000000000000000"); err != nil {
		log.Error("resolver deploys huerfanos fallo", "error", err.Error())
		return err
	}
	log.Info("escuchando", "addr", addr)
	return serve(ctx, hs, nil, shutdownGrace)
}

// shutdownGrace es la ventana para que terminen las peticiones en vuelo tras SIGTERM.
const shutdownGrace = 5 * time.Second

// serve atiende hasta que ctx termine y ESPERA a Shutdown antes de volver (si no, el proceso sale
// mientras las peticiones en vuelo siguen). ln nil = ListenAndServe.
func serve(ctx context.Context, hs *http.Server, ln net.Listener, grace time.Duration) error {
	errc := make(chan error, 1)
	go func() {
		if ln != nil {
			errc <- hs.Serve(ln)
		} else {
			errc <- hs.ListenAndServe()
		}
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	c, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()
	if err := hs.Shutdown(c); err != nil {
		return fmt.Errorf("cierre incompleto: %w", err)
	}
	if err := <-errc; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// validateTimeouts acota las duraciones: JobTimeout debe superar activeDeadlineSeconds del Job
// (si no, el Job sigue vivo tras el handoff), con cotas maximas razonables y minimo de sondeo.
func validateTimeouts(ready, job, poll time.Duration) error {
	switch {
	case ready > 10*time.Minute:
		return errors.New("WARM_READY_TIMEOUT no puede superar 10m")
	case poll < 100*time.Millisecond:
		return errors.New("WARM_POLL_INTERVAL no puede ser menor de 100ms")
	case job <= wm.JobDeadline:
		return fmt.Errorf("WARM_JOB_TIMEOUT debe superar el activeDeadlineSeconds del Job (%s)", wm.JobDeadline)
	case job > time.Hour:
		return errors.New("WARM_JOB_TIMEOUT no puede superar 1h")
	}
	return nil
}

// validateAppURL exige http(s)://host: el SurfaceArtifact lo publica como base_url.
func validateAppURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return fmt.Errorf("WARM_APP_URL debe ser http(s)://host (recibido %q)", raw)
	}
	return nil
}

// checkFileStoreAllowed acota el almacen de archivos (solo pruebas): exige WARM_ALLOW_FILE_STORE=true
// y lo rechaza en un cluster o con WARM_ENV prod; su URI file:// no la lee ningun otro servicio.
func checkFileStoreAllowed(getenv func(string) string) error {
	if getenv("WARM_ALLOW_FILE_STORE") != "true" {
		return errors.New("WARM_OBJECT_STORE=file exige WARM_ALLOW_FILE_STORE=true")
	}
	if getenv("KUBERNETES_SERVICE_HOST") != "" {
		return errors.New("WARM_OBJECT_STORE=file prohibido dentro de un cluster")
	}
	switch strings.ToLower(strings.TrimSpace(getenv("WARM_ENV"))) {
	case "prod", "production":
		return errors.New("WARM_OBJECT_STORE=file prohibido con WARM_ENV=prod")
	}
	return nil
}

// safeSeed es el estado si falta el ConfigMap warm-state: el warm NO se declara listo; solo go-reset
// (reset verificado) puede llevarlo a ready. Nunca hay valores optimistas por defecto.
func safeSeed() wm.WarmState {
	return wm.WarmState{WarmID: "warm-1", State: wm.StateDirty, ResetVerified: false, BaselineVersion: "baseline-1"}
}

// checkFakeAllowed acota WARM_KUBE=fake: exige WARM_ALLOW_FAKE_KUBE=true y lo rechaza dentro de un
// clúster (KUBERNETES_SERVICE_HOST) o con WARM_ENV prod/production (sin distinguir mayúsculas ni espacios).
func checkFakeAllowed(getenv func(string) string) error {
	if getenv("WARM_ALLOW_FAKE_KUBE") != "true" {
		return errors.New("WARM_KUBE=fake exige WARM_ALLOW_FAKE_KUBE=true")
	}
	if getenv("KUBERNETES_SERVICE_HOST") != "" {
		return errors.New("WARM_KUBE=fake prohibido dentro de un clúster (KUBERNETES_SERVICE_HOST)")
	}
	switch strings.ToLower(strings.TrimSpace(getenv("WARM_ENV"))) {
	case "prod", "production":
		return errors.New("WARM_KUBE=fake prohibido con WARM_ENV=prod")
	}
	return nil
}

// durationEnv lee una duración; vacía = def; inválida, cero o negativa = error (no arranca).
func durationEnv(getenv func(string) string, key string, def time.Duration) (time.Duration, error) {
	v := strings.TrimSpace(getenv(key))
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s debe ser una duración positiva (recibido %q)", key, v)
	}
	return d, nil
}

// logAlerter registra el handoff (el aviso real a un humano llega con U4-T07).
type logAlerter struct{ log *slog.Logger }

func (a logAlerter) Handoff(_ context.Context, phase, runID, reason string) error {
	a.log.Warn("handoff a humano", "phase", phase, "run_id", runID, "reason", reason)
	return nil
}
