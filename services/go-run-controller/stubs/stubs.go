// Package stubs simula las salidas de U3 (flujos, superficie, evidencia).
// Por defecto cada stub falla cerrado: devuelve ErrNotProgrammed.
package stubs

import (
	"errors"
	"sync"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/plan"
)

// ErrNotProgrammed se devuelve mientras no se programe una respuesta.
var ErrNotProgrammed = errors.New("stub no programado: falla cerrado")

type slot[T any] struct {
	mu  sync.Mutex
	set bool
	val T
	err error
}

func (s *slot[T]) program(v T, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.set, s.val, s.err = true, v, err
}

func (s *slot[T]) get() (T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var zero T
	if !s.set {
		return zero, ErrNotProgrammed
	}
	if s.err != nil {
		return zero, s.err
	}
	return s.val, nil
}

// FakeFlowSource devuelve un FlowPlan sintético por run_id.
type FakeFlowSource struct{ s slot[plan.FlowPlan] }

// Program habilita la respuesta sintética válida.
func (f *FakeFlowSource) Program() { f.s.program(plan.FlowPlan{}, nil) }

// ProgramError programa un error.
func (f *FakeFlowSource) ProgramError(err error) { f.s.program(plan.FlowPlan{}, err) }

// Flows devuelve el plan del run.
func (f *FakeFlowSource) Flows(runID string) (plan.FlowPlan, error) {
	if _, err := f.s.get(); err != nil {
		return plan.FlowPlan{}, err
	}
	return plan.FlowPlan{RunID: runID, Workflow: "checkout", Flows: []plan.Flow{{
		FlowID: "f1", Name: "crear y leer orden",
		Steps: []plan.Step{
			{Method: "POST", Path: "/orders", ExpectStatus: 201},
			{Method: "GET", Path: "/orders/1", ExpectStatus: 200},
		},
		Invariant: "el total coincide con la suma de lineas",
	}}}, nil
}

// FakeSurface devuelve un SurfaceArtifact sintético.
type FakeSurface struct{ s slot[plan.SurfaceArtifact] }

// Program habilita la respuesta sintética válida.
func (f *FakeSurface) Program() { f.s.program(plan.SurfaceArtifact{}, nil) }

// ProgramError programa un error.
func (f *FakeSurface) ProgramError(err error) { f.s.program(plan.SurfaceArtifact{}, err) }

// Surface devuelve la superficie del run.
func (f *FakeSurface) Surface(runID string) (plan.SurfaceArtifact, error) {
	if _, err := f.s.get(); err != nil {
		return plan.SurfaceArtifact{}, err
	}
	return plan.SurfaceArtifact{RunID: runID, BaseURL: "https://warm.example.test",
		Endpoints: []plan.Endpoint{{Method: "GET", Path: "/health"}, {Method: "POST", Path: "/orders"}},
		Source:    "openapi"}, nil
}

// FakeEvidence devuelve EvidenceURIs sintéticas.
type FakeEvidence struct{ s slot[plan.EvidenceURIs] }

// Program habilita la respuesta sintética válida.
func (f *FakeEvidence) Program() { f.s.program(plan.EvidenceURIs{}, nil) }

// ProgramError programa un error.
func (f *FakeEvidence) ProgramError(err error) { f.s.program(plan.EvidenceURIs{}, err) }

// Evidence devuelve la evidencia del run.
func (f *FakeEvidence) Evidence(runID string) (plan.EvidenceURIs, error) {
	if _, err := f.s.get(); err != nil {
		return plan.EvidenceURIs{}, err
	}
	return plan.EvidenceURIs{RunID: runID, URIs: []string{"s3://evidence/" + runID + "/runner-f1.log"}}, nil
}
