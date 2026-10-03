package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/authz"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/policy"
)

type decision struct {
	decision string
	to       authz.State
	reason   string
}

type recorder struct {
	decisions []decision
	policies  [][2]string
}

func (r *recorder) GateDecision(d string, to authz.State, reason string) {
	r.decisions = append(r.decisions, decision{d, to, reason})
}
func (r *recorder) PolicyChange(n, res string) { r.policies = append(r.policies, [2]string{n, res}) }

// TestGateReasonMapping: cada motivo real del evaluador se mapea a una etiqueta acotada.
func TestGateReasonMapping(t *testing.T) {
	mod := func(f func(in *authz.GateInput)) authz.GateInput { in := okInput(); f(&in); return in }
	cases := []struct {
		name  string
		in    authz.GateInput
		setup func(e *env)
		want  decision
	}{
		{"allow", okInput(), nil, decision{"allow", authz.StateWarmReady, ""}},
		{"not_confirmed", mod(func(in *authz.GateInput) { in.Confirmed = authz.False }), nil, decision{"deny", authz.StateWarmReady, "not_confirmed"}},
		{"confirmed unknown", mod(func(in *authz.GateInput) { in.Confirmed = authz.Unknown }), nil, decision{"deny", authz.StateWarmReady, "not_confirmed"}},
		{"reset_not_verified", mod(func(in *authz.GateInput) { in.ResetVerified = authz.False }), nil, decision{"deny", authz.StateWarmReady, "reset_not_verified"}},
		{"ensayo_not_passed", func() authz.GateInput { in := wfInput("", "r1"); in.EnsayoPassed = authz.False; return in }(), nil, decision{"deny", authz.StateRunning, "ensayo_not_passed"}},
		{"namespace_not_test", mod(func(in *authz.GateInput) { in.TargetNamespace = "prod" }), nil, decision{"deny", authz.StateWarmReady, "namespace_not_test"}},
		{"workflow_allowed false", mod(func(in *authz.GateInput) { in.WorkflowAllowed = authz.False }), nil, decision{"deny", authz.StateWarmReady, "workflow_not_allowed"}},
		{"workflow fuera de la política", func() authz.GateInput { in := wfInput("desconocido", "r1"); return in }(),
			func(e *env) {
				_, _ = e.svc.SetPolicy(context.Background(), "a1", policy.Workflows, json.RawMessage(wfPolicy))
			},
			decision{"deny", authz.StateRunning, "workflow_not_allowed"}},
		{"illegal_transition", mod(func(in *authz.GateInput) { in.To = authz.StateRunning }), nil, decision{"deny", authz.StateRunning, "illegal_transition"}},
		{"estado inventado", mod(func(in *authz.GateInput) { in.To = "inventado" }), nil, decision{"deny", "inventado", "illegal_transition"}},
		{"el límite de autonomía manda", mod(func(in *authz.GateInput) {
			in.TargetNamespace = "prod"
			in.Confirmed = authz.False
			in.ResetVerified = authz.False
		}), nil,
			decision{"deny", authz.StateWarmReady, "namespace_not_test"}},
		{"internal_error", wfInput("checkout", "r1"), func(e *env) { e.store.getErr = errBoom }, decision{"error", authz.StateRunning, "internal_error"}},
		{"audit_failed", okInput(), func(e *env) { e.fa.fail = true }, decision{"error", authz.StateWarmReady, "audit_failed"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			rec := &recorder{}
			e.svc.obs = rec
			if c.setup != nil {
				c.setup(e)
				rec.policies, rec.decisions = nil, nil
			}
			d, _ := e.svc.Authorize(context.Background(), "svc", c.in)
			if len(rec.decisions) != 1 || rec.decisions[0] != c.want {
				t.Fatalf("decisiones = %+v, esperada %+v", rec.decisions, c.want)
			}
			if c.want.decision != "allow" && d.Allow {
				t.Fatal("una denegación no puede permitir")
			}
			if d.AuditRef != "" && c.want.reason == "audit_failed" {
				t.Fatal("sin auditoría no hay audit_ref")
			}
		})
	}
}

func TestPolicyChangeMetrics(t *testing.T) {
	e := newEnv(t)
	rec := &recorder{}
	e.svc.obs = rec
	ctx := context.Background()
	_, _ = e.svc.SetPolicy(ctx, "a1", policy.Events, json.RawMessage(`{"enabled_events":["tag"]}`))
	_, _ = e.svc.SetPolicy(ctx, "a1", policy.Events, json.RawMessage(`{"enabled_events":["no-existe"]}`))
	_ = e.svc.Rejected("a1", "events", "cuerpo ilegible (400)")
	_ = e.svc.Forbidden("u1", "events", "rol insuficiente (403)") // se audita, no cuenta como cambio
	want := [][2]string{{"events", "accepted"}, {"events", "rejected"}, {"events", "rejected"}}
	if len(rec.policies) != len(want) {
		t.Fatalf("cambios = %v", rec.policies)
	}
	for i := range want {
		if rec.policies[i] != want[i] {
			t.Fatalf("cambios = %v, esperados %v", rec.policies, want)
		}
	}
	if es := e.log.Query("", 10); len(es) != 4 {
		t.Fatalf("las 4 acciones deben auditarse: %d", len(es))
	}
}
