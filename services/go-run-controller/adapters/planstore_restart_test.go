package adapters

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/plan"
	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

type restartPlans struct{ calls int }

func (c *restartPlans) Flows(runID string) (plan.FlowPlan, error) {
	c.calls++
	return plan.FlowPlan{RunID: runID, Workflow: "checkout", Flows: []plan.Flow{{
		FlowID: "f1", Name: "crear", Invariant: "la orden existe",
		Steps: []plan.Step{{Method: "POST", Path: "/orders", ExpectStatus: 201}},
	}}}, nil
}

func TestPlanStoredSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	j, err := OpenJournal(filepath.Join(dir))
	if err != nil {
		t.Fatal(err)
	}
	_ = j.Save(runctl.Run{ID: "r-1", State: runctl.Rehearsing})
	src := &restartPlans{}
	first, err := runctl.StoredPlans{Store: j, Next: src}.Flows("r-1")
	if err != nil {
		t.Fatal(err)
	}
	_ = j.Close()

	j2, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j2.Close()
	again := &restartPlans{}
	after, err := runctl.StoredPlans{Store: j2, Next: again}.Flows("r-1")
	if err != nil {
		t.Fatal(err)
	}
	if again.calls != 0 {
		t.Fatalf("tras reiniciar se volvió a pedir el plan al planner (%d llamadas)", again.calls)
	}
	if !reflect.DeepEqual(first, after) {
		t.Fatal("el plan cambió tras reiniciar el controlador")
	}
}
