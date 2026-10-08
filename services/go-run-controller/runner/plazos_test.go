package runner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

// countKube cuenta los Jobs creados y puede fallar la N-ésima creación o los logs.
type countKube struct {
	KubeAPI
	created int
	failAt  int    // 1-based; 0 = nunca
	failJob string // nombre del Job cuya creación siempre falla
	logsErr error
}

func (c *countKube) CreateJob(ctx context.Context, j *batchv1.Job) error {
	if c.failJob != "" && j.Name == c.failJob {
		return errors.New("api caída")
	}
	if c.failAt > 0 && c.created+1 == c.failAt {
		c.created++
		return errors.New("api caída")
	}
	c.created++
	return c.KubeAPI.CreateJob(ctx, j)
}

func (c *countKube) JobLogs(ctx context.Context, name string) ([]byte, error) {
	if c.logsErr != nil {
		return nil, c.logsErr
	}
	return c.KubeAPI.JobLogs(ctx, name)
}

func withCount(r *rig) *countKube {
	ck := &countKube{KubeAPI: r.l.Kube}
	r.l.Kube = ck
	return ck
}

func notMarker(k string) bool {
	return strings.HasSuffix(k, "/logs.txt") || strings.HasSuffix(k, "/result.json")
}

// --- F-01: el plazo de la corrida no se estira cuando vencen y se borran Jobs ---

func TestRunnerRunTimeoutNotStretched(t *testing.T) {
	r := newRig("a", "b", "c", "d", "e", "f")
	r.l.MaxParallel = 1
	r.l.Policy = StaticPolicy{L: Limits{FlowTimeout: time.Minute, RunTimeout: 3 * time.Minute}}
	ck := withCount(r)
	run := passedRun("r-1")
	var out runctl.RunOutcome
	for i := 0; i < 12 && !out.Done; i++ {
		var err error
		if out, err = r.l.Progress(context.Background(), run); err != nil {
			t.Fatal(err)
		}
		r.clk.t = r.clk.t.Add(61 * time.Second) // cada Job vence antes del siguiente paso
	}
	if !out.Done || len(out.URIs) != 12 || out.FailReason != "" {
		t.Fatalf("%+v", out)
	}
	if ck.created > 3 {
		t.Fatalf("jobsCreados=%d con RunTimeout=3m y FlowTimeout=1m (máximo 3)", ck.created)
	}
}

func TestRunnerExpiredJobsKeptUntilSettled(t *testing.T) {
	r := newRig("a")
	r.ev.FailPut = func(k string) error {
		if notMarker(k) {
			return errors.New("bucket caído")
		}
		return nil
	}
	run := passedRun("r-1")
	if _, err := r.l.Progress(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	r.clk.t = t0.Add(2 * time.Minute)
	out, err := r.l.Progress(context.Background(), run)
	if err != nil || !out.Done || out.FailReason == "" {
		t.Fatalf("%+v %v", out, err)
	}
	if n := len(r.jobs(t)); n != 1 {
		t.Fatalf("el Job vencido se borró sin evidencia escrita: quedan %d", n)
	}
	// con la evidencia sana, el mismo Job vencido sí se borra al resolverse
	r2 := newRig("a")
	_, _ = r2.l.Progress(context.Background(), run)
	r2.clk.t = t0.Add(2 * time.Minute)
	if out, err := r2.l.Progress(context.Background(), run); err != nil || !out.Done || len(r2.jobs(t)) != 0 {
		t.Fatalf("%+v %v jobs=%d", out, err, len(r2.jobs(t)))
	}
}

// --- F-02: un lanzamiento parcial no deja Jobs vivos ---

func TestRunnerInvalidFlowCreatesNoJobs(t *testing.T) {
	r := newRig("a", "B_malo")
	ck := withCount(r)
	if _, err := r.l.Launch(context.Background(), runctl.PhaseRun, passedRun("r-1")); err == nil {
		t.Fatal("flow_id inválido aceptado")
	}
	if n := len(r.jobs(t)); n != 0 || ck.created != 0 {
		t.Fatalf("jobs vivos %d, creados %d", n, ck.created)
	}
}

func TestRunnerPartialLaunchLeavesNoJobs(t *testing.T) {
	r := newRig("a", "b", "c")
	ck := withCount(r)
	ck.failAt = 2
	if _, err := r.l.Launch(context.Background(), runctl.PhaseRun, passedRun("r-1")); err == nil {
		t.Fatal("el fallo de CreateJob se tragó")
	}
	if n := len(r.jobs(t)); n != 0 {
		t.Fatalf("jobs vivos: %d", n)
	}
	// por el camino del controlador (reintentos y reset) tampoco queda ninguno
	r = newRig("a", "b", "c")
	ck = withCount(r)
	ck.failJob = "runner-r-1-b"
	c := newCtl(t, r, runctl.Running)
	for i := 0; i < 30 && c.state() != runctl.Failed && c.state() != runctl.Done; i++ {
		c.step(t)
	}
	if n := len(r.jobs(t)); n != 0 || !has(c.seen, runctl.Resetting) || len(c.pub.Events) != 0 {
		t.Fatalf("estado %s jobs vivos %d eventos %d", c.state(), n, len(c.pub.Events))
	}
}

// --- F-03 (5, 6): plazo del Job, status passed, cuota una vez por corrida ---

func TestRunnerJobDeadlineEqualsFlowTimeout(t *testing.T) {
	r := newRig("a")
	r.l.Policy = StaticPolicy{L: Limits{FlowTimeout: 45 * time.Second, RunTimeout: 90 * time.Second}}
	if _, err := r.l.Launch(context.Background(), runctl.PhaseRun, passedRun("r-1")); err != nil {
		t.Fatal(err)
	}
	js := r.jobs(t)
	if len(js) != 1 || js[0].Spec.ActiveDeadlineSeconds == nil || *js[0].Spec.ActiveDeadlineSeconds != 45 {
		t.Fatalf("%+v", js)
	}
}

func TestRunnerResultStatusPassed(t *testing.T) {
	r := newRig("a")
	run := passedRun("r-1")
	_, _ = r.l.Progress(context.Background(), run)
	r.finish(t, "runner-r-1-a", true)
	if out, err := r.l.Progress(context.Background(), run); err != nil || !out.Done {
		t.Fatalf("%+v %v", out, err)
	}
	b, _ := r.ev.Get(context.Background(), "runs/r-1/a/result.json")
	if !strings.Contains(string(b), `"status":"passed"`) {
		t.Fatalf("result.json %s", b)
	}
}

func TestRunnerQuotaOncePerRun(t *testing.T) {
	for name, expire := range map[string]bool{"jobs completados": false, "jobs vencidos y borrados": true} {
		r := newRig("a", "b", "c")
		r.l.MaxParallel = 1
		run := passedRun("r-1")
		var out runctl.RunOutcome
		for i := 0; i < 20 && !out.Done; i++ {
			var err error
			if out, err = r.l.Progress(context.Background(), run); err != nil {
				t.Fatal(err)
			}
			if expire {
				r.clk.t = r.clk.t.Add(61 * time.Second)
			} else {
				for _, j := range r.jobs(t) {
					if d, _ := jobState(j); !d {
						r.finish(t, j.Name, true)
					}
				}
			}
		}
		if !out.Done || len(r.gt.Calls) != 1 {
			t.Errorf("%s: done=%v consultas de cuota=%d (una por corrida)", name, out.Done, len(r.gt.Calls))
		}
	}
}

// --- F-03 (4): logs.txt antes que result.json, también con hash distinto ---

func TestEvidenceMarkerLast(t *testing.T) {
	for name, mut := range map[string]func(*MemEvidence){
		"logs no sube": func(e *MemEvidence) {
			e.FailPut = func(k string) error {
				if strings.HasSuffix(k, "/logs.txt") {
					return errors.New("x")
				}
				return nil
			}
		},
		"logs hash distinto": func(e *MemEvidence) {
			e.Corrupt = func(k string) []byte {
				if strings.HasSuffix(k, "/logs.txt") {
					return []byte("otro")
				}
				return nil
			}
		},
	} {
		r := newRig("a")
		mut(r.ev)
		run := passedRun("r-1")
		_, _ = r.l.Progress(context.Background(), run)
		r.finish(t, "runner-r-1-a", true)
		out, err := r.l.Progress(context.Background(), run)
		if err != nil || !out.Done || out.FailReason == "" || len(out.URIs) != 0 {
			t.Errorf("%s: %+v %v", name, out, err)
		}
		if _, ok := r.ev.Objs["runs/r-1/a/result.json"]; ok {
			t.Errorf("%s: existe result.json sin logs.txt verificado", name)
		}
	}
}

// --- AMARILLO: logs ilegibles se marcan ---

func TestRunnerLogsUnavailable(t *testing.T) {
	r := newRig("a")
	ck := withCount(r)
	run := passedRun("r-1")
	_, _ = r.l.Progress(context.Background(), run)
	ck.logsErr = errors.New("pods/log denegado")
	r.finish(t, "runner-r-1-a", true)
	if out, err := r.l.Progress(context.Background(), run); err != nil || !out.Done {
		t.Fatalf("%+v %v", out, err)
	}
	b, _ := r.ev.Get(context.Background(), "runs/r-1/a/result.json")
	if !strings.Contains(string(b), `"logs_unavailable":true`) {
		t.Fatalf("result.json %s", b)
	}
	// con logs legibles no se marca
	r2 := newRig("a")
	_, _ = r2.l.Progress(context.Background(), run)
	r2.finish(t, "runner-r-1-a", true)
	_, _ = r2.l.Progress(context.Background(), run)
	b, _ = r2.ev.Get(context.Background(), "runs/r-1/a/result.json")
	if strings.Contains(string(b), "logs_unavailable") {
		t.Fatalf("marcado sin falla: %s", b)
	}
}
