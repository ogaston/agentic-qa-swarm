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
// Nota: la matriz testdata/authorize_matrix.json es el contrato de U4-T04. Su
// reproducción con el fake es circular (se programa con la verdad de cada
// fila) y solo valida el cableado; la fuerza real llega cuando U4-T04 la
// ejecute contra el evaluador real, que sí evaluará los hechos.
package authz
