package obs

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/authz"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/audit"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/policy"
)

// Etiquetas acotadas de las métricas de gobierno.
var (
	// Decisions son los valores de aqs_gate_decisions_total{decision}.
	Decisions = []string{"allow", "deny", "error"}
	// DenyReasons son los valores de aqs_gate_denied_total{reason}.
	DenyReasons = []string{authz.DenyNotConfirmed, authz.DenyResetNotVerified, authz.DenyEnsayoNotPassed,
		authz.DenyNamespaceNotTest, authz.DenyWorkflowNotAllowed, authz.DenyIllegalTransition,
		DenyAuditFailed, authz.DenyInternalError}
	policyNames   = []string{policy.Events, policy.ConfirmRequired, policy.WarmQuotas, policy.Workflows, "unknown"}
	policyResults = []string{"accepted", "rejected"}
)

// DenyAuditFailed: la decisión no pudo registrarse en la auditoría (fail-closed).
const DenyAuditFailed = "audit_failed"

// Gov son las métricas de seguridad de go-governance. Todas las series con contador
// arrancan en 0. Un *Gov nil no hace nada.
type Gov struct {
	decisions *prometheus.CounterVec
	denied    *prometheus.CounterVec
	policies  *prometheus.CounterVec
	entries   prometheus.Counter
	failures  prometheus.Counter
	lastAt    prometheus.Gauge
	chainOK   prometheus.Gauge
	retention prometheus.Gauge
	log       *slog.Logger
}

// NewGov registra las métricas en reg. retentionDays es el mínimo configurado.
// log recibe una línea "audit" por cada entrada de auditoría (segunda copia hacia Loki).
func NewGov(reg prometheus.Registerer, retentionDays int, log *slog.Logger) *Gov {
	g := &Gov{
		decisions: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aqs_gate_decisions_total",
			Help: "Decisiones de gate por resultado y estado destino."}, []string{"decision", "to"}),
		denied: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aqs_gate_denied_total",
			Help: "Decisiones de gate no permitidas, por motivo."}, []string{"reason"}),
		policies: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aqs_policy_changes_total",
			Help: "Cambios de política aceptados o rechazados."}, []string{"name", "result"}),
		entries: prometheus.NewCounter(prometheus.CounterOpts{Name: "aqs_audit_entries_total",
			Help: "Entradas añadidas a la auditoría por este proceso."}),
		failures: prometheus.NewCounter(prometheus.CounterOpts{Name: "aqs_audit_append_failures_total",
			Help: "Escrituras de auditoría fallidas."}),
		lastAt: prometheus.NewGauge(prometheus.GaugeOpts{Name: "aqs_audit_last_append_timestamp_seconds",
			Help: "Marca de tiempo (Unix) de la última entrada de auditoría."}),
		chainOK: prometheus.NewGauge(prometheus.GaugeOpts{Name: "aqs_audit_chain_ok",
			Help: "1 si la cadena de auditoría es íntegra, 0 si está rota."}),
		retention: prometheus.NewGauge(prometheus.GaugeOpts{Name: "aqs_audit_retention_days",
			Help: "Días mínimos de retención de la auditoría (garantía de no borrado)."}),
		log: log,
	}
	reg.MustRegister(g.decisions, g.denied, g.policies, g.entries, g.failures, g.lastAt, g.chainOK, g.retention)
	for _, d := range Decisions {
		for _, to := range append(stateLabels(), "invalid") {
			g.decisions.WithLabelValues(d, to)
		}
	}
	for _, r := range DenyReasons {
		g.denied.WithLabelValues(r)
	}
	for _, n := range policyNames {
		for _, r := range policyResults {
			g.policies.WithLabelValues(n, r)
		}
	}
	g.retention.Set(float64(retentionDays))
	g.chainOK.Set(1)
	return g
}

func stateLabels() []string {
	out := make([]string, len(authz.AllStates))
	for i, s := range authz.AllStates {
		out[i] = string(s)
	}
	return out
}

// toLabel acota el estado destino: un valor fuera del enum se agrupa en "invalid".
func toLabel(to authz.State) string {
	if to.Valid() {
		return string(to)
	}
	return "invalid"
}

// GateDecision registra una decisión (reason solo si no es allow).
func (g *Gov) GateDecision(decision string, to authz.State, reason string) {
	if g == nil {
		return
	}
	g.decisions.WithLabelValues(decision, toLabel(to)).Inc()
	if decision != "allow" {
		g.denied.WithLabelValues(reasonLabel(reason)).Inc()
	}
}

func reasonLabel(r string) string {
	for _, k := range DenyReasons {
		if r == k {
			return r
		}
	}
	return authz.DenyInternalError
}

// PolicyChange registra un cambio de política (name desconocido se agrupa en "unknown").
func (g *Gov) PolicyChange(name, result string) {
	if g == nil {
		return
	}
	if !policy.Known(name) {
		name = "unknown"
	}
	g.policies.WithLabelValues(name, result).Inc()
}

// AuditAppended es el observador de audit.Log: cuenta, marca el tiempo y emite la copia en el log.
// No incluye el detalle de los hechos (puede traer nombres de namespace y workflow).
func (g *Gov) AuditAppended(e audit.Entry, err error) {
	if g == nil {
		return
	}
	if err != nil {
		g.failures.Inc()
		if g.log != nil {
			g.log.Error("audit: escritura fallida", "error", err.Error())
		}
		return
	}
	g.entries.Inc()
	g.lastAt.Set(float64(e.At.UnixNano()) / 1e9)
	if g.log != nil {
		g.log.Info("audit", "action", e.Action, "run_id", e.RunID, "actor", e.Actor)
	}
}

// SetLastAppend fija el gauge con la última entrada ya existente al arrancar.
func (g *Gov) SetLastAppend(t time.Time) {
	if g != nil && !t.IsZero() {
		g.lastAt.Set(float64(t.UnixNano()) / 1e9)
	}
}

// SetChainOK fija aqs_audit_chain_ok.
func (g *Gov) SetChainOK(ok bool) {
	if g == nil {
		return
	}
	v := 0.0
	if ok {
		v = 1
	}
	g.chainOK.Set(v)
}

// ChainMonitor verifica la cadena de auditoría y mantiene aqs_audit_chain_ok.
type ChainMonitor struct {
	verify func() error
	gov    *Gov
	ttl    time.Duration
	now    func() time.Time
	log    *slog.Logger

	mu      sync.Mutex
	checked time.Time
	last    error
}

// NewChainMonitor: verify recorre la cadena; ttl es la vigencia del último resultado para
// /readyz (0: verifica en cada llamada). now nil usa time.Now.
func NewChainMonitor(verify func() error, gov *Gov, ttl time.Duration, now func() time.Time, log *slog.Logger) *ChainMonitor {
	if now == nil {
		now = time.Now
	}
	return &ChainMonitor{verify: verify, gov: gov, ttl: ttl, now: now, log: log}
}

// Verify fuerza una verificación y actualiza el gauge.
func (m *ChainMonitor) Verify() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.verifyLocked()
}

func (m *ChainMonitor) verifyLocked() error {
	err := m.verify()
	m.checked, m.last = m.now(), err
	m.gov.SetChainOK(err == nil)
	if err != nil && m.log != nil {
		var ve *audit.VerifyError
		if errors.As(err, &ve) {
			m.log.Error("audit: cadena rota", "line", ve.Line, "reason", ve.Reason)
		} else {
			m.log.Error("audit: cadena no verificable", "error", err.Error())
		}
	}
	return err
}

// Check es el chequeo de /readyz: reutiliza el resultado si es más reciente que ttl.
func (m *ChainMonitor) Check(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.checked.IsZero() && m.now().Sub(m.checked) < m.ttl {
		return m.last
	}
	return m.verifyLocked()
}

// Run verifica cada interval hasta que ctx termine.
func (m *ChainMonitor) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = m.Verify()
		}
	}
}
