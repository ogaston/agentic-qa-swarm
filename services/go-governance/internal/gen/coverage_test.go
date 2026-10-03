package gen_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/authz"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/gen"
)

// TestMain fija y registra el seed de rapid (PBT-08).
func TestMain(m *testing.M) { gen.Main(m) }

// En 500 sorteos aparece cada estado como origen y como destino, y cada destino
// con Allow y con Deny. Excepción: `confirmed` no tiene transición legal de
// entrada (es el estado inicial), así que solo puede darse con Deny.
func TestPBT_GeneratorCoverage(t *testing.T) {
	gen.AtLeastChecks(t, 500)
	from, to := map[authz.State]bool{}, map[authz.State]bool{}
	allow, deny := map[authz.State]bool{}, map[authz.State]bool{}
	var roleOK, roleBad, nsTest, nsEmpty, nsUpper, nsTrail, nsPrefix, nsUni, nsOther, wfEmpty, wfSet, wfUnknown, unk, tru, fal int
	var invalidState int
	n := 0
	rapid.Check(t, func(t *rapid.T) {
		n++
		in := gen.GateInput().Draw(t, "in")
		src := gen.WorkflowSource().Draw(t, "src")
		d, _ := (&authz.RuleEvaluator{Workflows: src, Now: func() time.Time { return time.Unix(0, 0) }}).AuthorizeTransition(context.Background(), in)
		from[in.From], to[in.To] = true, true
		if in.From.Valid() && in.To.Valid() {
			if d.Allow {
				allow[in.To] = true
			} else {
				deny[in.To] = true
			}
		} else {
			invalidState++
		}
		for _, f := range []authz.Fact{in.Confirmed, in.ResetVerified, in.EnsayoPassed, in.WorkflowAllowed, in.ApprovalRecorded} {
			switch f {
			case authz.True:
				tru++
			case authz.False:
				fal++
			default:
				unk++
			}
		}
		switch ns := in.TargetNamespace; {
		case ns == gen.TestNS:
			nsTest++
		case ns == "":
			nsEmpty++
		case ns == "AQS-TEST":
			nsUpper++
		case ns == "aqs-test ":
			nsTrail++
		case strings.HasPrefix(ns, gen.TestNS):
			nsPrefix++
		case strings.ContainsAny(ns, "ат‐​"):
			nsUni++
		default:
			nsOther++
		}
		switch w := in.Workflow; {
		case w == "":
			wfEmpty++
		case w == "desconocido":
			wfUnknown++
		default:
			wfSet++
		}
		if gen.Role().Draw(t, "role").Valid() {
			roleOK++
		} else {
			roleBad++
		}
	})
	for _, s := range authz.AllStates {
		if !from[s] {
			t.Errorf("estado %s nunca fue origen", s)
		}
		if !to[s] {
			t.Errorf("estado %s nunca fue destino", s)
		}
		if !deny[s] {
			t.Errorf("destino %s nunca dio Deny", s)
		}
		if !allow[s] && s != authz.StateConfirmed {
			t.Errorf("destino %s nunca dio Allow", s)
		}
	}
	if allow[authz.StateConfirmed] {
		t.Error("confirmed no puede ser destino permitido")
	}
	for name, c := range map[string]int{"rol válido": roleOK, "rol inválido": roleBad, "aqs-test": nsTest, "vacío": nsEmpty, "AQS-TEST": nsUpper,
		"aqs-test␠": nsTrail, "prefijo": nsPrefix, "Unicode": nsUni, "otro namespace": nsOther, "workflow vacío": wfEmpty,
		"workflow de la política": wfSet, "workflow desconocido": wfUnknown, "unknown": unk, "true": tru, "false": fal, "estado inválido": invalidState} {
		if c == 0 {
			t.Errorf("clase %q nunca generada en %d sorteos", name, n)
		}
	}
}

// Cada generador de política e entrada de auditoría produce sus casos fronterizos.
func TestPBT_GeneratorCoverage_Policies(t *testing.T) {
	gen.AtLeastChecks(t, 500)
	var len1, len63, empty, many int
	bad := map[string]bool{}
	var weird bool
	rapid.Check(t, func(t *rapid.T) {
		for _, w := range gen.WorkflowsPolicy().Draw(t, "w").Workflows {
			switch len(w.Name) {
			case 1:
				len1++
			case 63:
				len63++
			}
		}
		if p := gen.WorkflowsPolicy().Draw(t, "w2"); len(p.Workflows) == 0 {
			empty++
		} else if len(p.Workflows) >= 5 {
			many++
		}
		bad[gen.InvalidPolicyValue().Draw(t, "bad").Name] = true
		e := gen.AuditEntry().Draw(t, "e")
		if strings.ContainsAny(e.Actor, "<& \"\\\n\x00") {
			weird = true
		}
	})
	if len1 == 0 || len63 == 0 || empty == 0 || many == 0 || len(bad) != 4 || !weird {
		t.Errorf("cobertura insuficiente: len1=%d len63=%d vacía=%d muchas=%d inválidas=%v raros=%v", len1, len63, empty, many, bad, weird)
	}
}
