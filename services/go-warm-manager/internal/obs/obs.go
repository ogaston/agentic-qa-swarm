// Package obs: métricas Prometheus y logger JSON del servicio.
package obs

import (
	"io"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
)

// ServiceName es el nombre del servicio en logs.
const ServiceName = "go-warm-manager"

var warmStates = []string{"ready", "dirty", "cuarentena", "idle-escalado"}

// Metrics implementa warmmanager.Observer.
type Metrics struct {
	Reg      *prometheus.Registry
	state    *prometheus.GaugeVec
	attempts *prometheus.CounterVec
	handoff  *prometheus.CounterVec
}

// NewMetrics registra aqs_warm_state{state}, aqs_deploy_attempts_total{result}, aqs_handoff_total{phase}.
func NewMetrics() *Metrics {
	m := &Metrics{Reg: prometheus.NewRegistry(),
		state:    prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "aqs_warm_state", Help: "1 para el estado actual del warm."}, []string{"state"}),
		attempts: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aqs_deploy_attempts_total", Help: "Intentos de deploy por resultado."}, []string{"result"}),
		handoff:  prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aqs_handoff_total", Help: "Handoffs a humano por fase."}, []string{"phase"})}
	m.Reg.MustRegister(m.state, m.attempts, m.handoff)
	for _, s := range warmStates {
		m.state.WithLabelValues(s).Set(0)
	}
	for _, r := range []string{"success", "failure", "error"} {
		m.attempts.WithLabelValues(r)
	}
	m.handoff.WithLabelValues("deploy")
	return m
}

// WarmState fija el estado actual.
func (m *Metrics) WarmState(cur string) {
	for _, s := range warmStates {
		v := 0.0
		if s == cur {
			v = 1
		}
		m.state.WithLabelValues(s).Set(v)
	}
}

// DeployAttempt cuenta un intento.
func (m *Metrics) DeployAttempt(result string) { m.attempts.WithLabelValues(result).Inc() }

// Handoff cuenta un handoff.
func (m *Metrics) Handoff(phase string) { m.handoff.WithLabelValues(phase).Inc() }

// NewLogger devuelve un logger JSON con el campo service.
func NewLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, nil)).With("service", ServiceName)
}
