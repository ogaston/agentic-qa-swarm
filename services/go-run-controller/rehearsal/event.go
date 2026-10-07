package rehearsal

import (
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

// EventJSON traduce el resultado de un Job a rehearsal.passed|failed (contracts/events/rehearsal.schema.json).
// evidence es un URI al Job de Kubernetes. Solo se llama con Done=true.
func EventJSON(run runctl.Run, flowID, jobName string, out runctl.RehearsalOutcome, now time.Time) map[string]any {
	typ := "rehearsal.failed"
	if out.Passed {
		typ = "rehearsal.passed"
	}
	return map[string]any{
		"event_id": out.EventID, "type": typ, "version": 1,
		"occurred_at": now.UTC().Format(time.RFC3339), "trace_id": nonEmpty(run.TraceID, run.ID),
		"data": map[string]any{"run_id": run.ID, "flow_id": flowID, "passed": out.Passed,
			"evidence": "k8s://aqs-test/jobs/" + jobName},
	}
}

func nonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
