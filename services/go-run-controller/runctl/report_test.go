package runctl

import (
	"context"
	"errors"
	"testing"
)

// reportPhases es un PhaseLauncher que además produce el veredicto (ReportLauncher).
type reportPhases struct {
	verdict string
	err     error
	calls   int
}

func (p *reportPhases) Launch(context.Context, string, Run) ([]string, error) { return nil, nil }

func (p *reportPhases) LaunchReport(_ context.Context, _ Run) (ReportResult, error) {
	p.calls++
	return ReportResult{Verdict: p.verdict, ReportURI: "s3://evidence/reports/r.json"}, p.err
}

func reportingRun(store RunStore) {
	_ = store.Save(Run{ID: "r", State: Reporting, ConfirmedBy: "u", Flows: []string{"f"}, EnsayoPassed: True,
		Evidence: []string{"s3://evidence/r/f1/result.json"}, Launched: map[string]bool{PhaseReset: true}})
}

// El veredicto de report queda en la corrida y la evidencia de run no se sobrescribe.
func TestReportGuardaVeredictoYConservaEvidencia(t *testing.T) {
	store := NewMemStore()
	reportingRun(store)
	ph := &reportPhases{verdict: "bug"}
	k, err := New(Config{Gate: AllowAll(), Store: store, Publisher: &FakePublisher{}, Warm: &FakeWarm{Fact: True},
		Alerter: &FakeAlerter{}, Phases: ph})
	if err != nil {
		t.Fatal(err)
	}
	if err := k.Drive(context.Background(), "r"); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Get("r")
	if ph.calls != 1 || got.Verdict != "bug" || got.ReportURI != "s3://evidence/reports/r.json" || got.State != Done {
		t.Fatalf("calls=%d verdict=%q uri=%q state=%s", ph.calls, got.Verdict, got.ReportURI, got.State)
	}
	if len(got.Evidence) != 1 || got.Evidence[0] != "s3://evidence/r/f1/result.json" {
		t.Fatalf("evidencia sobrescrita: %v", got.Evidence)
	}
}

// Fail-closed: un error del reporte no deja veredicto ni marca la fase como lanzada.
func TestReportErrorNoDejaVeredicto(t *testing.T) {
	store := NewMemStore()
	reportingRun(store)
	ph := &reportPhases{err: errors.New("reporter caído")}
	k, _ := New(Config{Gate: AllowAll(), Store: store, Publisher: &FakePublisher{}, Warm: &FakeWarm{Fact: True},
		Alerter: &FakeAlerter{}, Phases: ph})
	_ = k.Drive(context.Background(), "r")
	got, _ := store.Get("r")
	if got.Verdict != "" || got.Launched[PhaseReport] || got.State == Done {
		t.Fatalf("reporte fallido dejó huella: verdict=%q launched=%v state=%s", got.Verdict, got.Launched[PhaseReport], got.State)
	}
}
