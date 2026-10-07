// Package obs: métricas Prometheus, logger JSON y la alerta de cuarentena (registro + métrica).
package obs

import (
	"context"
	"io"
	"log/slog"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics implementa core.Metrics.
type Metrics struct {
	resets *prometheus.CounterVec
	idle   prometheus.Counter
	closed prometheus.Counter
	unread prometheus.Counter
}

// New registra las métricas; quarantined se evalúa en cada scrape.
func New(reg prometheus.Registerer, quarantined func() float64) *Metrics {
	m := &Metrics{
		resets: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aqs_reset_total", Help: "Resets por resultado."}, []string{"result"}),
		idle:   prometheus.NewCounter(prometheus.CounterOpts{Name: "aqs_warm_idle_scaled_total", Help: "Escalados a idle."}),
		closed: prometheus.NewCounter(prometheus.CounterOpts{Name: "aqs_housekeeping_sessions_closed_total", Help: "Sesiones cerradas por higiene."}),
		unread: prometheus.NewCounter(prometheus.CounterOpts{Name: "aqs_warm_state_unreadable_total", Help: "Lecturas de warm-state con contenido ilegible (tratado como dirty)."}),
	}
	reg.MustRegister(m.resets, m.idle, m.closed, m.unread)
	if quarantined != nil {
		reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "aqs_warm_quarantined", Help: "1 si el warm está en cuarentena."}, quarantined))
	}
	m.resets.WithLabelValues("verified")
	m.resets.WithLabelValues("quarantined")
	return m
}

func (m *Metrics) Reset(r string)       { m.resets.WithLabelValues(r).Inc() }
func (m *Metrics) IdleScaled()          { m.idle.Inc() }
func (m *Metrics) SessionsClosed(n int) { m.closed.Add(float64(n)) }
func (m *Metrics) StateUnreadable()     { m.unread.Inc() }

// NewLogger crea un logger JSON.
func NewLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, nil)).With("service", "go-reset")
}

// LogAlerter registra la alerta warm.quarantined.
type LogAlerter struct{ Log *slog.Logger }

func (a LogAlerter) Quarantine(_ context.Context, warmID, runID, reason string) {
	a.Log.Error("alerta", "alert", "warm.quarantined", "warm_id", warmID, "run_id", runID, "trace_id", "tr-"+runID, "reason", strings.TrimSpace(reason))
}
