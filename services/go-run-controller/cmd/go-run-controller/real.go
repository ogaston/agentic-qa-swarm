package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/adapters"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/plan"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/rehearsal"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// noFlows falla cerrado mientras U3 no entregue el FlowPlan: sin plan no hay ensayo y sin ensayo no hay runner.
type noFlows struct{}

func (noFlows) Flows(string) (plan.FlowPlan, error) {
	return plan.FlowPlan{}, errors.New("sin fuente de FlowPlan (U3): falla cerrado")
}

// wireReal sustituye los puertos de fase y de warm por los adaptadores reales.
func wireReal(cfg *runctl.Config, c config) error {
	wc, err := adapters.NewWarmClient(c.warmURL, c.warmToken)
	if err != nil {
		return err
	}
	rc, err := adapters.NewResetClient(c.resetURL, c.resetToken)
	if err != nil {
		return err
	}
	rest, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("RUN_PHASES=real requiere ejecutarse en el clúster: %w", err)
	}
	cs, err := kubernetes.NewForConfig(rest)
	if err != nil {
		return err
	}
	l := &rehearsal.Launcher{Kube: rehearsal.ClientGo{CS: cs, NS: c.namespace}, Plans: noFlows{},
		Cfg: rehearsal.Config{Namespace: c.namespace, Image: c.rehearsalImage, ServiceAccount: c.rehearsalSA, TargetURL: c.rehearsalTarget}}
	ref := c.artifactRef
	cfg.Warm = wc
	cfg.Results = l
	cfg.Phases = &adapters.RealPhases{Warm: wc, Reset: rc, Rehearse: l,
		Artifact: func(string) (string, string, error) { return "published-image", ref, nil }}
	return nil
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
