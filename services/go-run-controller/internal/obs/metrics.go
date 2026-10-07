package obs

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/runctl"
)

// RunMetrics implementa runctl.Observer con las métricas de la tarea.
type RunMetrics struct {
	transitions *prometheus.CounterVec
	gate        *prometheus.CounterVec
	handoff     *prometheus.CounterVec
	dropped     *prometheus.CounterVec
	persist     prometheus.Counter
	tail        prometheus.Counter
}

var _ runctl.Observer = (*RunMetrics)(nil)

// NewRunMetrics registra aqs_run_transitions_total, aqs_gate_calls_total, aqs_handoff_total y
// aqs_runs_active (gauge calculado en cada scrape con active()).
func NewRunMetrics(reg prometheus.Registerer, active func() float64) *RunMetrics {
	m := &RunMetrics{
		transitions: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aqs_run_transitions_total",
			Help: "Transiciones de corrida por origen, destino y resultado."}, []string{"from", "to", "result"}),
		gate: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aqs_gate_calls_total",
			Help: "Llamadas al gate de go-governance por resultado."}, []string{"result"}),
		handoff: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aqs_handoff_total",
			Help: "Handoffs humanos por fase."}, []string{"phase"}),
		dropped: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aqs_events_dropped_total",
			Help: "Eventos de entrada descartados por llegar fuera de estado."}, []string{"type"}),
		persist: prometheus.NewCounter(prometheus.CounterOpts{Name: "aqs_persist_errors_total",
			Help: "Guardados del diario de corridas que fallaron."}),
		tail: prometheus.NewCounter(prometheus.CounterOpts{Name: "aqs_journal_tail_discarded_total",
			Help: "Colas incompletas (sin salto de línea) del diario descartadas al arrancar."}),
	}
	reg.MustRegister(m.transitions, m.gate, m.handoff, m.dropped, m.persist, m.tail,
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "aqs_runs_active", Help: "Corridas no terminales."}, active))
	// Series iniciales para que existan desde el arranque.
	m.gate.WithLabelValues(runctl.ResAllow)
	m.handoff.WithLabelValues(runctl.PhaseTransport)
	return m
}

// Transition implementa runctl.Observer.
func (m *RunMetrics) Transition(from, to runctl.State, res string) {
	m.transitions.WithLabelValues(string(from), string(to), res).Inc()
}

// GateCall implementa runctl.Observer.
func (m *RunMetrics) GateCall(res string) { m.gate.WithLabelValues(res).Inc() }

// Handoff implementa runctl.Observer.
func (m *RunMetrics) Handoff(phase string) { m.handoff.WithLabelValues(phase).Inc() }

// EventDropped implementa runctl.Observer.
func (m *RunMetrics) EventDropped(t string) { m.dropped.WithLabelValues(t).Inc() }

// PersistError implementa runctl.Observer.
func (m *RunMetrics) PersistError() { m.persist.Inc() }

// JournalTailDiscarded cuenta una cola de diario descartada al arrancar.
func (m *RunMetrics) JournalTailDiscarded() { m.tail.Inc() }

// RunnerMetrics implementa runner.Observer: aqs_runner_jobs_total{result},
// aqs_evidence_objects_total{result} y aqs_run_duration_seconds.
type RunnerMetrics struct {
	jobs, objs *prometheus.CounterVec
	dur        prometheus.Histogram
}

// NewRunnerMetrics registra las métricas de runners y evidencia.
func NewRunnerMetrics(reg prometheus.Registerer) *RunnerMetrics {
	m := &RunnerMetrics{
		jobs: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aqs_runner_jobs_total", Help: "Jobs runner resueltos por resultado."}, []string{"result"}),
		objs: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aqs_evidence_objects_total", Help: "Flujos con evidencia guardada o fallida."}, []string{"result"}),
		dur: prometheus.NewHistogram(prometheus.HistogramOpts{Name: "aqs_run_duration_seconds", Help: "Duración de la fase run.",
			Buckets: []float64{10, 30, 60, 120, 300, 600, 1200, 3600}}),
	}
	reg.MustRegister(m.jobs, m.objs, m.dur)
	for _, r := range []string{"passed", "failed", "timeout"} {
		m.jobs.WithLabelValues(r)
	}
	for _, r := range []string{"stored", "error"} {
		m.objs.WithLabelValues(r)
	}
	return m
}

// RunnerJob cuenta un Job runner resuelto.
func (m *RunnerMetrics) RunnerJob(result string) { m.jobs.WithLabelValues(result).Inc() }

// EvidenceObject cuenta la evidencia de un flujo.
func (m *RunnerMetrics) EvidenceObject(result string) { m.objs.WithLabelValues(result).Inc() }

// RunDuration observa la duración de la fase run.
func (m *RunnerMetrics) RunDuration(s float64) { m.dur.Observe(s) }
