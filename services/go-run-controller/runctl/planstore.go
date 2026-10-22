package runctl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/plan"
)

// PlanSource es el planner de flujos (U3) tal como lo usan los lanzadores.
type PlanSource interface {
	Flows(runID string) (plan.FlowPlan, error)
}

// StoredPlans es la fuente de FlowPlan de ensayo y runners. La primera vez que obtiene un plan válido
// para una corrida lo guarda en Run.Plan (mismo store y misma escritura atómica que el estado de la
// corrida). Las obtenciones siguientes lo leen del store y nunca llaman a Next: lo ensayado es lo
// ejecutado, byte a byte. Un plan guardado que no valida al leerse falla cerrado.
//
// Formato: Run.Plan es el JSON compacto de plan.FlowPlan tal como lo produce encoding/json de Go
// (campos run_id, workflow, flows); el diario lo guarda dentro de la línea de la corrida.
type StoredPlans struct {
	Store RunStore
	Next  PlanSource
}

// Flows implementa la fuente de FlowPlan con persistencia.
func (s StoredPlans) Flows(runID string) (plan.FlowPlan, error) {
	r, ok := s.Store.Get(runID)
	if !ok {
		return plan.FlowPlan{}, ErrNotFound
	}
	if len(r.Plan) > 0 {
		return DecodeStoredPlan(r.Plan, runID)
	}
	fp, err := s.Next.Flows(runID)
	if err != nil {
		return plan.FlowPlan{}, err
	}
	if err := plan.ValidateFlowPlan(fp); err != nil {
		return plan.FlowPlan{}, err
	}
	if fp.RunID != runID {
		return plan.FlowPlan{}, errors.New("flow-plan de otra corrida")
	}
	raw, err := json.Marshal(fp)
	if err != nil {
		return plan.FlowPlan{}, err
	}
	r.Plan = raw
	if err := s.Store.Save(r); err != nil {
		// Sin plan persistido no se lanza nada: lo ensayado y lo ejecutado no pueden divergir.
		return plan.FlowPlan{}, fmt.Errorf("guardando el plan de la corrida: %w", err)
	}
	return DecodeStoredPlan(raw, runID)
}

// DecodeStoredPlan lee un plan guardado: JSON estricto (campos desconocidos y datos tras el objeto
// son error), validación contra el esquema y comprobación de la corrida. Falla cerrado.
func DecodeStoredPlan(raw json.RawMessage, runID string) (plan.FlowPlan, error) {
	var fp plan.FlowPlan
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&fp); err != nil {
		return plan.FlowPlan{}, errors.New("plan guardado ilegible")
	}
	if _, err := dec.Token(); err != io.EOF {
		return plan.FlowPlan{}, errors.New("plan guardado ilegible: datos tras el objeto")
	}
	if err := plan.ValidateFlowPlan(fp); err != nil {
		return plan.FlowPlan{}, fmt.Errorf("plan guardado inválido: %w", err)
	}
	if fp.RunID != runID {
		return plan.FlowPlan{}, errors.New("plan guardado de otra corrida")
	}
	return fp, nil
}
