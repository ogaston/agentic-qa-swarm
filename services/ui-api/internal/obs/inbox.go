package obs

import "github.com/prometheus/client_golang/prometheus"

// InboxStates son los valores cerrados de aqs_inbox_notifications{state}.
var InboxStates = []string{"pending", "confirmed", "rejected"}

// Inbox son las métricas de dominio de ui-api. Un *Inbox nil no hace nada.
type Inbox struct {
	confirmations prometheus.Counter
}

// NewInbox registra aqs_inbox_confirmations_total (en 0) y aqs_inbox_notifications{state}.
// counts se evalúa en cada scrape y devuelve el número de notificaciones por estado;
// los tres estados salen siempre, también en 0.
func NewInbox(reg prometheus.Registerer, counts func() map[string]int) *Inbox {
	m := &Inbox{confirmations: prometheus.NewCounter(prometheus.CounterOpts{Name: "aqs_inbox_confirmations_total",
		Help: "Confirmaciones de notificaciones registradas (201)."})}
	reg.MustRegister(m.confirmations)
	if counts != nil {
		for _, st := range InboxStates {
			reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "aqs_inbox_notifications",
				Help: "Notificaciones del inbox por estado.", ConstLabels: prometheus.Labels{"state": st}},
				func() float64 { return float64(counts()[st]) }))
		}
	}
	return m
}

// Confirmed cuenta una confirmación registrada.
func (m *Inbox) Confirmed() {
	if m != nil {
		m.confirmations.Inc()
	}
}
