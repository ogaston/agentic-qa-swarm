package obs

import "github.com/prometheus/client_golang/prometheus"

// IdentityResults son los valores cerrados de aqs_identity_calls_total{result}.
var IdentityResults = []string{"ok", "unauthorized", "error", "circuit_open"}

// Identity son las métricas de la integración con go-identity. Un *Identity nil no hace nada.
type Identity struct{ calls *prometheus.CounterVec }

// NewIdentity registra aqs_identity_calls_total{result} (series en 0) y aqs_identity_circuit_open
// (1 si el circuito está abierto; circuitOpen se evalúa en cada scrape).
func NewIdentity(reg prometheus.Registerer, circuitOpen func() bool) *Identity {
	m := &Identity{calls: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aqs_identity_calls_total",
		Help: "Validaciones de token contra go-identity por resultado."}, []string{"result"})}
	for _, r := range IdentityResults {
		m.calls.WithLabelValues(r)
	}
	reg.MustRegister(m.calls)
	reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "aqs_identity_circuit_open",
		Help: "1 si el circuito hacia go-identity está abierto."}, func() float64 {
		if circuitOpen != nil && circuitOpen() {
			return 1
		}
		return 0
	}))
	return m
}

// IdentityCall cuenta una validación (implementa auth.CallObserver).
func (m *Identity) IdentityCall(result string) {
	if m != nil {
		m.calls.WithLabelValues(result).Inc()
	}
}

// RegisterPublishPending registra aqs_inbox_publish_pending (confirmaciones con run.confirmed sin publicar).
func RegisterPublishPending(reg prometheus.Registerer, pending func() int) {
	reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "aqs_inbox_publish_pending",
		Help: "Confirmaciones cuyo run.confirmed aún no se publicó."}, func() float64 { return float64(pending()) }))
}
