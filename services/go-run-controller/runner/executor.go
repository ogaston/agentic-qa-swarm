// Package runner construye y lanza los Jobs `runner-<run>-<flow>`, recolecta su evidencia a un
// almacén de objetos (leída de vuelta y comparada por hash) y resuelve el resultado de la fase run.
package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/plan"
)

// EnvVar es una variable de entorno NO sensible que aporta el motor.
type EnvVar struct{ Name, Value string }

// JobSpec es lo único que un Executor aporta al Job: comando, argumentos y entorno no sensible.
// EnvFrom existe solo para que el constructor lo RECHACE si un motor intenta usarlo.
type JobSpec struct {
	Command []string
	Args    []string
	Env     []EnvVar
	EnvFrom []string
}

// Executor es el motor enchufable: cambiarlo no cambia el constructor de Jobs de seguridad.
type Executor interface {
	Plan(flow plan.Flow) (JobSpec, error)
}

// ErrSensitiveEnv marca un env sensible o un envFrom pedido por el motor.
var ErrSensitiveEnv = errors.New("el motor pidió un env sensible o envFrom")

var sensitiveRe = regexp.MustCompile(`(?i)LLM|API_KEY|TOKEN|SECRET|PASSWORD`)

// checkSpec rechaza env cuyo nombre case con LLM|API_KEY|TOKEN|SECRET|PASSWORD y todo envFrom.
func checkSpec(s JobSpec) error {
	if len(s.EnvFrom) > 0 {
		return fmt.Errorf("%w: envFrom", ErrSensitiveEnv)
	}
	for _, e := range s.Env {
		if e.Name == "" || sensitiveRe.MatchString(e.Name) {
			return fmt.Errorf("%w: %q", ErrSensitiveEnv, e.Name)
		}
	}
	return nil
}

// HTTPStepsExecutor ejecuta los pasos HTTP del flujo con la imagen RUNNER_IMAGE.
type HTTPStepsExecutor struct{ Target string }

// Plan implementa Executor.
func (h HTTPStepsExecutor) Plan(f plan.Flow) (JobSpec, error) {
	steps, err := json.Marshal(f.Steps)
	if err != nil {
		return JobSpec{}, err
	}
	return JobSpec{
		Args: []string{"http-steps", "--flow", f.FlowID, "--target", h.Target},
		Env:  []EnvVar{{"RUNNER_STEPS", string(steps)}, {"RUNNER_INVARIANT", f.Invariant}},
	}, nil
}

// FakeExecutor es el motor de pruebas: devuelve Spec tal cual (o Err).
type FakeExecutor struct {
	Spec JobSpec
	Err  error
}

// Plan implementa Executor.
func (f FakeExecutor) Plan(plan.Flow) (JobSpec, error) {
	if f.Err != nil {
		return JobSpec{}, f.Err
	}
	if f.Spec.Args == nil && f.Spec.Command == nil {
		f.Spec.Args = []string{"fake"}
	}
	return f.Spec, nil
}
