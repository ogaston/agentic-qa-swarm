package obs

import "github.com/prometheus/client_golang/prometheus"

// RejectReasons son los valores cerrados de aqs_intake_webhook_rejected_total{reason}.
var RejectReasons = []string{"invalid_signature", "unsupported_event", "unresolvable_artifact", "bad_request", "too_large"}

// Intake son las métricas de dominio de go-intake. Todas las series con contador arrancan en 0.
// Un *Intake nil no hace nada.
type Intake struct {
	created         prometheus.Counter
	rejected        *prometheus.CounterVec
	publishFailures prometheus.Counter
}

// NewIntake registra las métricas de dominio de go-intake e inicializa en 0 cada serie.
func NewIntake(reg prometheus.Registerer) *Intake {
	m := &Intake{
		created: prometheus.NewCounter(prometheus.CounterOpts{Name: "aqs_intake_notifications_created_total",
			Help: "Notificaciones nuevas creadas a partir de un webhook de GitHub."}),
		rejected: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aqs_intake_webhook_rejected_total",
			Help: "Webhooks rechazados, por motivo."}, []string{"reason"}),
		publishFailures: prometheus.NewCounter(prometheus.CounterOpts{Name: "aqs_intake_publish_failures_total",
			Help: "Fallos al publicar notify.created."}),
	}
	reg.MustRegister(m.created, m.rejected, m.publishFailures)
	for _, r := range RejectReasons {
		m.rejected.WithLabelValues(r)
	}
	return m
}

// Created cuenta una notificación nueva.
func (m *Intake) Created() {
	if m != nil {
		m.created.Inc()
	}
}

// Rejected cuenta un rechazo; un motivo fuera de RejectReasons se agrupa en bad_request (cardinalidad cerrada).
func (m *Intake) Rejected(reason string) {
	if m == nil {
		return
	}
	for _, r := range RejectReasons {
		if r == reason {
			m.rejected.WithLabelValues(reason).Inc()
			return
		}
	}
	m.rejected.WithLabelValues("bad_request").Inc()
}

// PublishFailed cuenta un fallo de publicación.
func (m *Intake) PublishFailed() {
	if m != nil {
		m.publishFailures.Inc()
	}
}
