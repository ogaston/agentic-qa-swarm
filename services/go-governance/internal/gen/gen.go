package gen

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"pgregory.net/rapid"

	"github.com/ogaston/agentic-qa-swarm/services/go-governance/authz"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/audit"
	"github.com/ogaston/agentic-qa-swarm/services/go-governance/internal/policy"
)

// TestNS es el namespace de prueba permitido.
const TestNS = "aqs-test"

// Pool son los nombres de workflow que usan los generadores de GateInput y Source.
var Pool = []string{"checkout", "login-flow", "report-export"}

// Edge es una arista del grafo de estados.
type Edge struct{ From, To authz.State }

// LegalEdges es el grafo legal escrito a mano desde la especificación (oráculo
// independiente de authz.LegalTransition).
func LegalEdges() []Edge {
	e := []Edge{
		{authz.StateConfirmed, authz.StateWarmReady}, {authz.StateWarmReady, authz.StateDeploying},
		{authz.StateDeploying, authz.StateInferring}, {authz.StateInferring, authz.StateRehearsing},
		{authz.StateRehearsing, authz.StateRunning}, {authz.StateRunning, authz.StateResetting},
		{authz.StateResetting, authz.StateReporting}, {authz.StateReporting, authz.StateDone},
		{authz.StateDeploying, authz.StateResetting}, {authz.StateInferring, authz.StateResetting},
		{authz.StateRehearsing, authz.StateResetting},
	}
	for _, s := range []authz.State{authz.StateConfirmed, authz.StateWarmReady, authz.StateDeploying,
		authz.StateInferring, authz.StateRehearsing, authz.StateRunning, authz.StateResetting, authz.StateReporting} {
		e = append(e, Edge{s, authz.StateFailed})
	}
	return e
}

// IsLegal consulta el oráculo.
func IsLegal(from, to authz.State) bool {
	for _, e := range LegalEdges() {
		if e.From == from && e.To == to {
			return true
		}
	}
	return false
}

func intBelow(t *rapid.T, label string, n int) int { return rapid.IntRange(0, n-1).Draw(t, label) }

// Fact es un hecho tri-estado uniforme.
func Fact() *rapid.Generator[authz.Fact] {
	return rapid.SampledFrom([]authz.Fact{authz.True, authz.False, authz.Unknown})
}

// FactNotTrue es un hecho False o Unknown.
func FactNotTrue() *rapid.Generator[authz.Fact] {
	return rapid.SampledFrom([]authz.Fact{authz.False, authz.Unknown})
}

// FactBiased favorece True (60%) para que las decisiones Allow sean frecuentes.
func FactBiased() *rapid.Generator[authz.Fact] {
	return rapid.Custom(func(t *rapid.T) authz.Fact {
		switch n := intBelow(t, "fact", 10); {
		case n < 6:
			return authz.True
		case n < 8:
			return authz.False
		}
		return authz.Unknown
	})
}

// Role genera roles válidos o inválidos.
func Role() *rapid.Generator[authz.Role] {
	return rapid.Custom(func(t *rapid.T) authz.Role {
		if intBelow(t, "valid", 3) < 2 {
			return rapid.SampledFrom([]authz.Role{authz.RoleUser, authz.RoleAdmin}).Draw(t, "role")
		}
		return rapid.SampledFrom([]authz.Role{"", "Admin", "USER", "root", "user ", "admin\n", "superadmin"}).Draw(t, "badrole")
	})
}

// Principal genera principals con rol válido o inválido y ID posiblemente vacío.
func Principal() *rapid.Generator[authz.Principal] {
	return rapid.Custom(func(t *rapid.T) authz.Principal {
		return authz.Principal{ID: rapid.SampledFrom([]string{"", "u1", "u2", "admin-1", "ü"}).Draw(t, "id"), Role: Role().Draw(t, "role")}
	})
}

// RunState genera un estado válido.
func RunState() *rapid.Generator[authz.State] { return rapid.SampledFrom(authz.AllStates) }

// RunStateAny genera un estado válido (80%) o inválido.
func RunStateAny() *rapid.Generator[authz.State] {
	return rapid.Custom(func(t *rapid.T) authz.State {
		if intBelow(t, "valid", 5) < 4 {
			return RunState().Draw(t, "state")
		}
		return rapid.SampledFrom([]authz.State{"", "Running", "RUNNING", "warm-ready", "paused", "done "}).Draw(t, "bad")
	})
}

// NamespaceNear genera namespaces distintos de aqs-test, incluidos los casi iguales.
func NamespaceNear() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		switch intBelow(t, "kind", 10) {
		case 0:
			return "AQS-TEST"
		case 1:
			return "aqs-test "
		case 2:
			return ""
		case 3:
			return rapid.SampledFrom([]string{"staging", "prod", "default", "kube-system", "aqs-system"}).Draw(t, "env")
		case 4:
			return "aqs-test-" + rapid.SampledFrom([]string{"prod", "2", "", "x"}).Draw(t, "suf") // prefijo de aqs-test
		case 5:
			return rapid.SampledFrom([]string{"aqs-tes", "aqs-tesт", "aqs‐test", "аqs-test", "aqs-test​", "aqs_test"}).Draw(t, "conf") // Unicode confundible
		case 6:
			return " aqs-test"
		case 7:
			return TestNS + rapid.StringN(1, 6, -1).Draw(t, "ext")
		case 8:
			return rapid.SampledFrom([]string{"aqs-tes\u0442", "aqs\u2010test", "\u0430qs-test"}).Draw(t, "conf2")
		}
		s := rapid.String().Draw(t, "any")
		if s == TestNS {
			s += "x"
		}
		return s
	})
}

// Namespace genera aqs-test (55%) o un namespace distinto, con sesgo a los casi iguales.
func Namespace() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		if intBelow(t, "is-test", 20) < 11 {
			return TestNS
		}
		return NamespaceNear().Draw(t, "near")
	})
}

// workflowAlphabet es el alfabeto de los nombres de workflow válidos.
const workflowAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789-"

// WorkflowName genera nombres válidos (^[a-z][a-z0-9-]{0,62}$), con sesgo a las longitudes 1 y 63.
func WorkflowName() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		n := rapid.IntRange(1, 63).Draw(t, "len")
		switch intBelow(t, "edge", 5) {
		case 0:
			n = 1
		case 1:
			n = 63
		}
		var b strings.Builder
		b.WriteByte(rapid.SampledFrom([]byte("abcdefghijklmnopqrstuvwxyz")).Draw(t, "first"))
		for i := 1; i < n; i++ {
			b.WriteByte(workflowAlphabet[intBelow(t, "ch", len(workflowAlphabet))])
		}
		return b.String()
	})
}

// predecessor devuelve un origen legal de to (cualquier estado si to no tiene ninguno).
func predecessor(t *rapid.T, to authz.State) authz.State {
	var preds []authz.State
	for _, e := range LegalEdges() {
		if e.To == to {
			preds = append(preds, e.From)
		}
	}
	if len(preds) == 0 {
		return RunState().Draw(t, "from")
	}
	return rapid.SampledFrom(preds).Draw(t, "from")
}

// GateInput genera entradas de gate sesgadas a casos fronterizos. Con dests, el
// destino sale de esa lista y el origen es un predecesor legal el 85% de las veces.
func GateInput(dests ...authz.State) *rapid.Generator[authz.GateInput] {
	return rapid.Custom(func(t *rapid.T) authz.GateInput {
		var in authz.GateInput
		in.RunID = "run-1"
		if len(dests) > 0 {
			in.To = rapid.SampledFrom(dests).Draw(t, "to")
			if intBelow(t, "legal", 20) < 17 {
				in.From = predecessor(t, in.To)
			} else {
				in.From = RunState().Draw(t, "from")
			}
		} else {
			switch n := intBelow(t, "mode", 20); {
			case n < 13:
				in.To = RunState().Draw(t, "to")
				in.From = predecessor(t, in.To)
			case n < 19:
				in.From, in.To = RunState().Draw(t, "from"), RunState().Draw(t, "to")
			default:
				in.From, in.To = RunStateAny().Draw(t, "from"), RunStateAny().Draw(t, "to")
			}
			if intBelow(t, "runid", 50) == 0 {
				in.RunID = ""
			}
		}
		in.Confirmed = FactBiased().Draw(t, "confirmed")
		in.ResetVerified = FactBiased().Draw(t, "reset_verified")
		in.EnsayoPassed = FactBiased().Draw(t, "ensayo_passed")
		in.WorkflowAllowed = FactBiased().Draw(t, "workflow_allowed")
		in.ApprovalRecorded = FactBiased().Draw(t, "approval_recorded")
		in.TargetNamespace = Namespace().Draw(t, "ns")
		if intBelow(t, "happy", 5) == 0 { // caso «todo en regla»: las decisiones Allow son frecuentes
			in.Confirmed, in.ResetVerified, in.EnsayoPassed = authz.True, authz.True, authz.True
			in.WorkflowAllowed, in.ApprovalRecorded, in.TargetNamespace = authz.True, authz.True, TestNS
		}
		if intBelow(t, "wf", 2) == 1 {
			in.Workflow = rapid.SampledFrom(append(append([]string{}, Pool...), "desconocido")).Draw(t, "workflow")
		}
		return in
	})
}

// Source es un authz.WorkflowSource determinista para las propiedades.
type Source struct {
	Rules map[string]authz.WorkflowRule
	Used  int
}

// Workflow implementa authz.WorkflowSource.
func (s *Source) Workflow(_ context.Context, name string) (authz.WorkflowRule, bool, error) {
	r, ok := s.Rules[name]
	return r, ok, nil
}

// RunsAllowed implementa authz.WorkflowSource.
func (s *Source) RunsAllowed(_ context.Context, _ string, _ time.Time) (int, error) {
	return s.Used, nil
}

// Policy devuelve la política `workflows` equivalente.
func (s *Source) Policy() policy.WorkflowsValue {
	out := policy.WorkflowsValue{Workflows: []policy.WorkflowItem{}}
	for _, n := range Pool {
		if r, ok := s.Rules[n]; ok {
			out.Workflows = append(out.Workflows, policy.WorkflowItem{Name: n, Complexity: "low",
				MaxRunsPerDay: r.MaxRunsPerDay, TimeoutSeconds: 60, RequiresApproval: r.RequiresApproval})
		}
	}
	return out
}

// WorkflowSource genera una política de workflows y un consumo diario.
func WorkflowSource() *rapid.Generator[*Source] {
	return rapid.Custom(func(t *rapid.T) *Source {
		s := &Source{Rules: map[string]authz.WorkflowRule{}}
		if intBelow(t, "fresh", 2) == 1 {
			s.Used = intBelow(t, "used", 7)
		}
		for _, n := range Pool {
			if intBelow(t, "present", 5) < 4 {
				s.Rules[n] = authz.WorkflowRule{Name: n, MaxRunsPerDay: rapid.IntRange(1, 5).Draw(t, "max"),
					RequiresApproval: rapid.Bool().Draw(t, "approval"), Version: 1}
			}
		}
		return s
	})
}

// EventsPolicy genera una política `events` válida (subconjunto ordenado de los tres eventos).
func EventsPolicy() *rapid.Generator[policy.EventsValue] {
	return rapid.Custom(func(t *rapid.T) policy.EventsValue {
		all := []string{"commit", "pull_request", "tag"}
		out := []string{}
		for _, i := range rapid.SliceOfNDistinct(rapid.IntRange(0, 2), 0, 3, func(i int) int { return i }).Draw(t, "idx") {
			out = append(out, all[i])
		}
		return policy.EventsValue{EnabledEvents: out}
	})
}

// ConfirmRequiredPolicy genera la única política válida (required=true).
func ConfirmRequiredPolicy() *rapid.Generator[policy.ConfirmRequiredValue] {
	return rapid.Just(policy.ConfirmRequiredValue{Required: true})
}

func edgeInt(t *rapid.T, label string, lo, hi int) int {
	switch intBelow(t, label+"-edge", 6) {
	case 0:
		return lo
	case 1:
		return hi
	}
	return rapid.IntRange(lo, hi).Draw(t, label)
}

// WarmQuotasPolicy genera una política `warm_quotas` válida con sesgo a los límites.
func WarmQuotasPolicy() *rapid.Generator[policy.WarmQuotasValue] {
	return rapid.Custom(func(t *rapid.T) policy.WarmQuotasValue {
		return policy.WarmQuotasValue{
			MaxRunsPerDay:          edgeInt(t, "max_runs_per_day", 1, 1000),
			RebuildCadenceHours:    edgeInt(t, "rebuild_cadence_hours", 1, 720),
			IdleScaleDownMinutes:   edgeInt(t, "idle_scale_down_minutes", 1, 1440),
			HousekeepingGraceHours: edgeInt(t, "housekeeping_grace_hours", 1, 168),
		}
	})
}

// WorkflowItem genera un workflow válido.
func WorkflowItem() *rapid.Generator[policy.WorkflowItem] {
	return rapid.Custom(func(t *rapid.T) policy.WorkflowItem {
		return policy.WorkflowItem{
			Name:             WorkflowName().Draw(t, "name"),
			Complexity:       rapid.SampledFrom([]string{"low", "medium", "high"}).Draw(t, "complexity"),
			MaxRunsPerDay:    edgeInt(t, "max_runs_per_day", 1, 1000),
			TimeoutSeconds:   edgeInt(t, "timeout_seconds", 1, 3600),
			RequiresApproval: rapid.Bool().Draw(t, "requires_approval"),
		}
	})
}

// WorkflowsPolicy genera una política `workflows` válida con nombres únicos (0 a 6 elementos).
func WorkflowsPolicy() *rapid.Generator[policy.WorkflowsValue] {
	return rapid.Custom(func(t *rapid.T) policy.WorkflowsValue {
		items := rapid.SliceOfNDistinct(WorkflowItem(), 0, 6, func(w policy.WorkflowItem) string { return w.Name }).Draw(t, "items")
		return policy.WorkflowsValue{Workflows: append([]policy.WorkflowItem{}, items...)}
	})
}

// NamedPolicy es una política válida con su nombre y su JSON canónico.
type NamedPolicy struct {
	Name  string
	Value any
}

// ValidPolicy genera cualquiera de las cuatro políticas válidas.
func ValidPolicy() *rapid.Generator[NamedPolicy] {
	return rapid.Custom(func(t *rapid.T) NamedPolicy {
		switch intBelow(t, "which", 4) {
		case 0:
			return NamedPolicy{policy.Events, EventsPolicy().Draw(t, "p")}
		case 1:
			return NamedPolicy{policy.ConfirmRequired, ConfirmRequiredPolicy().Draw(t, "p")}
		case 2:
			return NamedPolicy{policy.WarmQuotas, WarmQuotasPolicy().Draw(t, "p")}
		}
		return NamedPolicy{policy.Workflows, WorkflowsPolicy().Draw(t, "p")}
	})
}

// InvalidPolicy es un valor crudo que rompe exactamente un campo de una política válida.
type InvalidPolicy struct {
	Name string
	Raw  json.RawMessage
	Why  string
}

func toMap(t *rapid.T, v any) map[string]any {
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func marshal(t *rapid.T, m any) json.RawMessage {
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// outOfRange devuelve un valor fuera de [lo,hi] o un valor de tipo erróneo.
func outOfRange(t *rapid.T, lo, hi int) any {
	switch intBelow(t, "oor", 6) {
	case 0:
		return lo - 1
	case 1:
		return hi + 1
	case 2:
		return -rapid.IntRange(1, 1<<20).Draw(t, "neg")
	case 3:
		return 1 << 40
	case 4:
		return "7"
	}
	return 1.5
}

// BadWorkflowName genera nombres inválidos.
func BadWorkflowName() *rapid.Generator[string] {
	return rapid.SampledFrom([]string{"", "A", "Checkout", "1abc", "-abc", "a_b", "a b", "á", strings.Repeat("a", 64), strings.Repeat("a", 200), "a.b", "a/b"})
}

// InvalidPolicyValue genera un valor fuera de rango, de nombre o de enumerado inválido.
func InvalidPolicyValue() *rapid.Generator[InvalidPolicy] {
	return rapid.Custom(func(t *rapid.T) InvalidPolicy {
		switch intBelow(t, "which", 4) {
		case 0:
			m := toMap(t, EventsPolicy().Draw(t, "p"))
			evs, _ := m["enabled_events"].([]any)
			switch intBelow(t, "how", 4) {
			case 0:
				m["enabled_events"] = append(evs, rapid.SampledFrom([]string{"push", "", "COMMIT", "release", "commit "}).Draw(t, "ev"))
			case 1:
				m["enabled_events"] = []any{"tag", "tag"}
			case 2:
				delete(m, "enabled_events")
			default:
				m["extra"] = 1
			}
			return InvalidPolicy{policy.Events, marshal(t, m), "events"}
		case 1:
			m := map[string]any{"required": false}
			switch intBelow(t, "how", 3) {
			case 1:
				m = map[string]any{}
			case 2:
				m = map[string]any{"required": "true"}
			}
			return InvalidPolicy{policy.ConfirmRequired, marshal(t, m), "confirm_required"}
		case 2:
			m := toMap(t, WarmQuotasPolicy().Draw(t, "p"))
			f := rapid.SampledFrom([]struct {
				k      string
				lo, hi int
			}{{"max_runs_per_day", 1, 1000}, {"rebuild_cadence_hours", 1, 720}, {"idle_scale_down_minutes", 1, 1440}, {"housekeeping_grace_hours", 1, 168}}).Draw(t, "field")
			if intBelow(t, "missing", 8) == 0 {
				delete(m, f.k)
			} else {
				m[f.k] = outOfRange(t, f.lo, f.hi)
			}
			return InvalidPolicy{policy.WarmQuotas, marshal(t, m), "warm_quotas." + f.k}
		}
		base := policy.WorkflowsValue{Workflows: []policy.WorkflowItem{WorkflowItem().Draw(t, "w1"), WorkflowItem().Draw(t, "w2")}}
		if base.Workflows[0].Name == base.Workflows[1].Name {
			base.Workflows[1].Name += "x"
			if len(base.Workflows[1].Name) > 63 {
				base.Workflows[1].Name = "b"
			}
		}
		m := toMap(t, base)
		items := m["workflows"].([]any)
		it := items[intBelow(t, "item", 2)].(map[string]any)
		why := ""
		switch intBelow(t, "how", 6) {
		case 0:
			it["name"], why = BadWorkflowName().Draw(t, "badname"), "name"
		case 1:
			it["complexity"], why = rapid.SampledFrom([]string{"", "LOW", "extreme", "medium "}).Draw(t, "cx"), "complexity"
		case 2:
			it["max_runs_per_day"], why = outOfRange(t, 1, 1000), "max_runs_per_day"
		case 3:
			it["timeout_seconds"], why = outOfRange(t, 1, 3600), "timeout_seconds"
		case 4:
			delete(it, "requires_approval")
			why = "requires_approval"
		default:
			items[1].(map[string]any)["name"], why = items[0].(map[string]any)["name"], "duplicado"
		}
		return InvalidPolicy{policy.Workflows, marshal(t, m), "workflows." + why}
	})
}

// weird compone cadenas con caracteres que el JSON escapa o que suelen romper cadenas de hash.
func weird() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		parts := rapid.SliceOfN(rapid.OneOf(
			rapid.String(),
			rapid.SampledFrom([]string{"<", ">", "&", " ", " ", "\"", "\\", "\n", "\x00", "\t", " ", "é", "日本", "\U0001F600", "�"}),
		), 0, 4).Draw(t, "parts")
		return strings.Join(parts, "")
	})
}

// AuditEntry genera el contenido de una entrada (At, PrevHash y Hash los fija el log).
func AuditEntry() *rapid.Generator[audit.Entry] {
	return rapid.Custom(func(t *rapid.T) audit.Entry {
		e := audit.Entry{
			Actor:  weird().Draw(t, "actor"),
			Action: rapid.SampledFrom([]string{"gate.allow", "gate.deny", "gate.error", "policy.set:events@v1", "policy.rejected"}).Draw(t, "action"),
		}
		if intBelow(t, "run", 3) > 0 {
			e.RunID = weird().Draw(t, "run")
		}
		if n := intBelow(t, "n", 4); n > 0 {
			e.Detail = map[string]string{}
			for i := 0; i < n; i++ {
				e.Detail[weird().Draw(t, "k")] = weird().Draw(t, "v")
			}
		}
		return e
	})
}
