package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/adapters"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/plan"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/rehearsal"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runner"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// noFlows falla cerrado mientras U3 no entregue el FlowPlan: sin plan no hay ensayo y sin ensayo no hay runner.
type noFlows struct{}

func (noFlows) Flows(string) (plan.FlowPlan, error) {
	return plan.FlowPlan{}, errors.New("sin fuente de FlowPlan (U3): falla cerrado")
}

// inClusterClient crea el clientset del clúster (solo en RUN_PHASES=real).
func inClusterClient() (kubernetes.Interface, error) {
	rc, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("RUN_PHASES=real requiere ejecutarse en el clúster: %w", err)
	}
	return kubernetes.NewForConfig(rc)
}

// buildConfig elige en UNA rama los puertos de warm y de fase: o todos reales (con cs) o todos fakes.
func buildConfig(c config, cs kubernetes.Interface, gate runctl.GateClient, store runctl.RunStore, pub runctl.EventPublisher,
	al runctl.Alerter, obs runctl.Observer, rm runner.Observer, log *slog.Logger) (runctl.Config, error) {
	base := runctl.Config{Namespace: c.namespace, Gate: gate, Store: store, Publisher: pub, Alerter: al, Observer: obs, Log: log}
	if !c.real {
		base.Warm, base.Phases = &runctl.FakeWarm{Fact: runctl.True}, &runctl.FakePhases{}
		return base, nil
	}
	if cs == nil {
		return base, errors.New("RUN_PHASES=real exige un clientset de Kubernetes")
	}
	wc, err := adapters.NewWarmClient(c.warmURL, c.warmToken)
	if err != nil {
		return base, err
	}
	rc, err := adapters.NewResetClient(c.resetURL, c.resetToken)
	if err != nil {
		return base, err
	}
	l := &rehearsal.Launcher{Kube: rehearsal.ClientGo{CS: cs, NS: c.namespace}, Plans: noFlows{},
		Cfg: rehearsal.Config{Namespace: c.namespace, Image: c.rehearsalImage, ServiceAccount: c.rehearsalSA, TargetURL: c.rehearsalTarget}}
	ev, err := runner.NewS3(c.evEndpoint, c.evBucket, c.evAccessFile, c.evSecretFile)
	if err != nil {
		return base, err
	}
	rl := &runner.Launcher{Kube: runner.ClientGo{CS: cs, NS: c.namespace}, Plans: noFlows{}, Exec: runner.HTTPStepsExecutor{Target: c.rehearsalTarget},
		Evidence: ev, Gate: gate, Policy: runner.StaticPolicy{L: runner.Limits{FlowTimeout: c.flowTimeout, RunTimeout: c.runTimeout}},
		MaxParallel: c.maxParallel, Obs: rm, Log: log,
		Cfg: runner.Config{Namespace: c.namespace, Image: c.runnerImage, ServiceAccount: c.runnerSA}}
	ref := c.artifactRef
	base.Warm = wc
	base.Results = l
	base.Runners = rl
	base.Phases = &adapters.RealPhases{Warm: wc, Reset: rc, Rehearse: l, Run: rl,
		Artifact: func(string) (string, string, error) { return "published-image", ref, nil }}
	return base, nil
}

// renderRehearsalJob imprime el Job de ensayo como YAML sin tocar un clúster.
func renderRehearsalJob(w io.Writer, args []string, env func(string) string) error {
	var runID, flow string
	for i := 0; i+1 < len(args); i += 2 {
		switch args[i] {
		case "--run":
			runID = args[i+1]
		case "--flow":
			flow = args[i+1]
		default:
			return fmt.Errorf("argumento desconocido %q", args[i])
		}
	}
	if runID == "" || flow == "" || len(args)%2 != 0 {
		return errors.New("uso: render-rehearsal-job --run <id> --flow <id>")
	}
	cfg := rehearsal.Config{Namespace: runctl.DefaultNamespace, ServiceAccount: "aqs-runner",
		Image: env("REHEARSAL_IMAGE"), TargetURL: env("REHEARSAL_TARGET_URL")}
	if cfg.Image == "" {
		cfg.Image = "ghcr.io/ogaston/aqs-rehearsal:0.1.0"
	}
	if cfg.TargetURL == "" {
		cfg.TargetURL = "http://warm-app.aqs-test.svc:8080"
	}
	j, err := rehearsal.BuildJob(cfg, runID, plan.Flow{FlowID: flow, Name: flow, Invariant: "el flujo responde con los estados esperados",
		Steps: []plan.Step{{Method: "GET", Path: "/health", ExpectStatus: 200}}}, 1)
	if err != nil {
		return err
	}
	b, err := yaml.Marshal(j)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(strings.TrimSpace(string(b)) + "\n"))
	return err
}

// renderRunnerJob imprime el Job runner como YAML sin tocar un clúster.
func renderRunnerJob(w io.Writer, args []string, env func(string) string) error {
	var runID, flow string
	for i := 0; i+1 < len(args); i += 2 {
		switch args[i] {
		case "--run":
			runID = args[i+1]
		case "--flow":
			flow = args[i+1]
		default:
			return fmt.Errorf("argumento desconocido %q", args[i])
		}
	}
	if runID == "" || flow == "" || len(args)%2 != 0 {
		return errors.New("uso: render-runner-job --run <id> --flow <id>")
	}
	cfg := runner.Config{Namespace: runctl.DefaultNamespace, ServiceAccount: "aqs-runner", Image: env("RUNNER_IMAGE"), DeadlineSeconds: 300}
	if cfg.Image == "" {
		cfg.Image = "ghcr.io/ogaston/aqs-runner:0.1.0"
	}
	target := env("REHEARSAL_TARGET_URL")
	if target == "" {
		target = "http://warm-app.aqs-test.svc:8080"
	}
	f := plan.Flow{FlowID: flow, Name: flow, Invariant: "el flujo responde con los estados esperados",
		Steps: []plan.Step{{Method: "GET", Path: "/health", ExpectStatus: 200}}}
	spec, err := runner.HTTPStepsExecutor{Target: target}.Plan(f)
	if err != nil {
		return err
	}
	j, err := runner.BuildJob(cfg, runID, flow, spec, time.Now())
	if err != nil {
		return err
	}
	b, err := yaml.Marshal(j)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(strings.TrimSpace(string(b)) + "\n"))
	return err
}
