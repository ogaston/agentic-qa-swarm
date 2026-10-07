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
	}
	reg.MustRegister(m.transitions, m.gate, m.handoff, m.dropped,
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
