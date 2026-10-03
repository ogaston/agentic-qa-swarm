package obs

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/authz"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/audit"
)

func TestGovMetricsStartAtZero(t *testing.T) {
	reg := NewRegistry()
	g := NewGov(reg, 90, nil)
	for _, r := range DenyReasons {
		if v := testutil.ToFloat64(g.denied.WithLabelValues(r)); v != 0 {
			t.Errorf("reason %s arranca en %v", r, v)
		}
	}
	if len(DenyReasons) != 8 {
		t.Errorf("las etiquetas de reason son las 8 de la tarea: %v", DenyReasons)
	}
	for _, d := range Decisions {
		if v := testutil.ToFloat64(g.decisions.WithLabelValues(d, "warm_ready")); v != 0 {
			t.Errorf("decision %s arranca en %v", d, v)
		}
	}
	if testutil.ToFloat64(g.entries) != 0 || testutil.ToFloat64(g.failures) != 0 {
		t.Error("los contadores de auditoría arrancan en 0")
	}
	if testutil.ToFloat64(g.retention) != 90 || testutil.ToFloat64(g.chainOK) != 1 {
		t.Errorf("retención %v, cadena %v", testutil.ToFloat64(g.retention), testutil.ToFloat64(g.chainOK))
	}
	if n, err := testutil.GatherAndCount(reg, "aqs_gate_denied_total"); err != nil || n != 8 {
		t.Errorf("series de aqs_gate_denied_total: %d %v", n, err)
	}
}

func TestGovLabelsAreBounded(t *testing.T) {
	reg := NewRegistry()
	g := NewGov(reg, 90, nil)
	for i := 0; i < 50; i++ {
		g.GateDecision("deny", authz.State("estado-"+strings.Repeat("x", i)), "motivo-"+strings.Repeat("y", i))
		g.PolicyChange("politica-"+strings.Repeat("z", i), "rejected")
	}
	if n, _ := testutil.GatherAndCount(reg, "aqs_gate_denied_total"); n != 8 {
		t.Errorf("reason fuera del conjunto debía agruparse: %d series", n)
	}
	if v := testutil.ToFloat64(g.denied.WithLabelValues("internal_error")); v != 50 {
		t.Errorf("internal_error = %v", v)
	}
	if v := testutil.ToFloat64(g.decisions.WithLabelValues("deny", "invalid")); v != 50 {
		t.Errorf("to=invalid = %v", v)
	}
	if v := testutil.ToFloat64(g.policies.WithLabelValues("unknown", "rejected")); v != 50 {
		t.Errorf("name=unknown = %v", v)
	}
}

func TestAuditAppendedEmitsLogLineWithoutDetail(t *testing.T) {
	buf := &bytes.Buffer{}
	g := NewGov(NewRegistry(), 90, NewLogger(buf, "svc", slog.LevelInfo))
	now := time.Unix(1_800_000_000, 0)
	g.AuditAppended(audit.Entry{At: now, Actor: "a1", Action: "gate.allow", RunID: "r1",
		Detail: map[string]string{"target_namespace": "aqs-test", "workflow": "checkout"}}, nil)
	out := buf.String()
	if !strings.Contains(out, `"message":"audit"`) || !strings.Contains(out, `"action":"gate.allow"`) ||
		!strings.Contains(out, `"run_id":"r1"`) || !strings.Contains(out, `"actor":"a1"`) {
		t.Errorf("línea de auditoría incompleta: %s", out)
	}
	if strings.Contains(out, "aqs-test") || strings.Contains(out, "checkout") {
		t.Errorf("el detalle de hechos no debe copiarse al log: %s", out)
	}
	if testutil.ToFloat64(g.entries) != 1 || testutil.ToFloat64(g.lastAt) != 1_800_000_000 {
		t.Errorf("entradas %v, última %v", testutil.ToFloat64(g.entries), testutil.ToFloat64(g.lastAt))
	}
	g.AuditAppended(audit.Entry{}, errors.New("disco lleno"))
	if testutil.ToFloat64(g.failures) != 1 || testutil.ToFloat64(g.entries) != 1 {
		t.Error("un fallo cuenta como fallo, no como entrada")
	}
}

func TestAuditChainOKGaugeFollowsMonitor(t *testing.T) {
	g := NewGov(NewRegistry(), 90, nil)
	var verr error
	clock := time.Unix(1000, 0)
	m := NewChainMonitor(func() error { return verr }, g, 10*time.Second, func() time.Time { return clock }, nil)
	if err := m.Verify(); err != nil || testutil.ToFloat64(g.chainOK) != 1 {
		t.Fatalf("cadena íntegra: %v %v", err, testutil.ToFloat64(g.chainOK))
	}
	verr = &audit.VerifyError{Line: 3, Reason: "hash no coincide"}
	if err := m.Check(nil); err != nil {
		t.Fatal("dentro del ttl se reutiliza el resultado anterior")
	}
	clock = clock.Add(11 * time.Second)
	if err := m.Check(nil); err == nil || testutil.ToFloat64(g.chainOK) != 0 {
		t.Fatalf("cadena rota debía bajar el gauge a 0: %v %v", err, testutil.ToFloat64(g.chainOK))
	}
	verr = nil
	_ = m.Verify()
	if testutil.ToFloat64(g.chainOK) != 1 {
		t.Error("al recuperarse el gauge vuelve a 1")
	}
}
