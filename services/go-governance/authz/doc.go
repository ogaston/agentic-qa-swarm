// Package authz contiene el modelo de dominio de los gates y un evaluador
// falso de authorizeTransition (stub de U4-T01, consumido por U2).
//
// Firmas:
//
//	type Evaluator interface {
//		AuthorizeTransition(ctx context.Context, in GateInput) (Decision, error)
//	}
//
//	type GateInput struct {
//		RunID           string
//		From, To        State // enum de Run.state del OpenAPI
//		Confirmed       Fact
//		ResetVerified   Fact
//		EnsayoPassed    Fact
//		TargetNamespace string
//		WorkflowAllowed Fact
//		Workflow         string // opcional
//		ApprovalRecorded Fact
//	}
//
//	type Fact int // Unknown (valor cero) | True | False; IsTrue() solo para True
//
//	type Decision struct {
//		Allow    bool
//		Reason   string // no vacío siempre que Allow=false
//		AuditRef string
//	}
//
// Reglas: un error nunca equivale a permitir; Allow=false siempre lleva Reason;
// un hecho Unknown se lee como False (Fact.IsTrue).
//
// FakeEvaluator: NO evalúa hechos; resuelve solo por (From, To). Orden de
// resolución: (from,to) exacta, luego (from,""), luego ("",to), luego ("","");
// sin regla, Deny. Una entrada inválida (RunID vacío, estado fuera del enum) o
// un contexto cancelado también deniegan. Registra cada llamada (Calls) y es
// seguro ante concurrencia.
//
// RuleEvaluator (rules.go) es el evaluador real (U4-T04): evalúa los hechos
// (Unknown cuenta como False), el namespace de prueba exacto, la legalidad de la
// transición y, si GateInput.Workflow no está vacío, la política de workflows
// (WorkflowSource).
//
// Política de workflows: si GateInput.Workflow no está vacío, su EXISTENCIA en la
// política `workflows` se exige en todo destino salvo resetting, reporting, done
// y failed (el reset y el cierre siempre deben poder intentarse). La aprobación
// (requires_approval) y la cuota diaria solo se evalúan en running. Con workflow
// vacío cuenta únicamente el hecho workflow_allowed reportado.
//
// workflow_allowed en warm_ready es autodeclarado: no se contrasta con ninguna
// política (todavía no hay selección de workflow). U2-T02 deberá reportarlo antes
// de que exista esa selección (el diseño pasa el workflow a deployToWarm, que es
// posterior a warm_ready). La matriz testdata/authorize_matrix.json se reproduce contra
// él en TestMatrixReal, sin programarlo con la verdad de cada fila; la
// reproducción con el fake (TestMatrixReplay) solo valida su cableado.
package authz
