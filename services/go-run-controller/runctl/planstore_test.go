package runctl

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/plan"
)

// countingPlans cuenta las llamadas al planner (U3) y devuelve un plan válido para la corrida.
type countingPlans struct {
	calls int
	err   error
}

func (c *countingPlans) Flows(runID string) (plan.FlowPlan, error) {
	c.calls++
	if c.err != nil {
		return plan.FlowPlan{}, c.err
	}
	return validPlan(runID), nil
}

func validPlan(runID string) plan.FlowPlan {
	return plan.FlowPlan{RunID: runID, Workflow: "checkout", Flows: []plan.Flow{{
		FlowID: "f1", Name: "crear y leer orden", Invariant: "la orden existe",
		Steps: []plan.Step{{Method: "POST", Path: "/orders", ExpectStatus: 201}},
	}}}
}

func TestPlanStoredOnFirstObtain(t *testing.T) {
	st := NewMemStore()
	_ = st.Save(Run{ID: "r-1", State: Confirmed})
	src := &countingPlans{}
	fp, err := StoredPlans{Store: st, Next: src}.Flows("r-1")
	if err != nil {
		t.Fatal(err)
	}
	r, _ := st.Get("r-1")
	if len(r.Plan) == 0 {
		t.Fatal("el plan no quedó guardado en la corrida tras la primera obtención")
	}
	got, err := DecodeStoredPlan(r.Plan, "r-1")
	if err != nil || !reflect.DeepEqual(got, fp) {
		t.Fatalf("el plan guardado no es el obtenido: %v %+v", err, got)
	}
}

func TestPlanStoredNextCalledOnceForRehearseAndRun(t *testing.T) {
	st := NewMemStore()
	_ = st.Save(Run{ID: "r-1", State: Rehearsing})
	src := &countingPlans{}
	sp := StoredPlans{Store: st, Next: src}
	a, err1 := sp.Flows("r-1") // ensayo
	b, err2 := sp.Flows("r-1") // runners
	if err1 != nil || err2 != nil {
		t.Fatal(err1, err2)
	}
	if src.calls != 1 {
		t.Fatalf("planner llamado %d veces; debe ser 1 para ensayo y runners", src.calls)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("ensayo y runners recibieron planes distintos")
	}
}

func TestPlanStoredCorruptFailsClosed(t *testing.T) {
	st := NewMemStore()
	_ = st.Save(Run{ID: "r-1", State: Rehearsing, Plan: json.RawMessage(`{"run_id":"r-1"}`)})
	src := &countingPlans{}
	if _, err := (StoredPlans{Store: st, Next: src}).Flows("r-1"); err == nil {
		t.Fatal("un plan guardado inválido debe fallar cerrado")
	}
	if src.calls != 0 {
		t.Fatal("un plan guardado corrupto no debe reemplazarse por una llamada nueva al planner")
	}
	ajeno, _ := json.Marshal(validPlan("r-otra"))
	_ = st.Save(Run{ID: "r-1", State: Rehearsing, Plan: ajeno})
	if _, err := DecodeStoredPlan(ajeno, "r-1"); err == nil {
		t.Fatal("un plan de otra corrida debe rechazarse al leerse")
	}
}

// planCallingPhases hace lo que el lanzador de ensayo: pide el plan (StoredPlans) dentro de Launch.
type planCallingPhases struct{ plans StoredPlans }

func (p *planCallingPhases) Launch(_ context.Context, _ string, run Run) ([]string, error) {
	_, err := p.plans.Flows(run.ID)
	return nil, err
}

func TestPlanStoredSurvivesDriveSave(t *testing.T) {
	store := NewMemStore()
	_ = store.Save(Run{ID: "r", State: Rehearsing, ConfirmedBy: "u"})
	src := &countingPlans{}
	k, err := New(Config{Gate: factsGate(), Store: store, Publisher: &FakePublisher{}, Warm: &FakeWarm{Fact: True},
		Alerter: &FakeAlerter{}, Phases: &planCallingPhases{plans: StoredPlans{Store: store, Next: src}}})
	if err != nil {
		t.Fatal(err)
	}
	_ = k.Drive(t.Context(), "r")
	got, _ := store.Get("r")
	if len(got.Plan) == 0 {
		t.Fatal("el save de Drive tras Launch borró el plan guardado")
	}
	if src.calls != 1 {
		t.Fatalf("planner llamado %d veces", src.calls)
	}
}
