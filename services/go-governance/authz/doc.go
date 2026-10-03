// Package authz contiene el modelo de dominio de los gates y un evaluador
// falso de authorizeTransition (stub de U4-T01, consumido por U2).
//
// Firmas:
//
//	type Evaluator interface {
//		AuthorizeTransition(ctx context.Context, in GateInput) (Decision, error)
//	}
//
// Reglas: un error nunca equivale a permitir; Allow=false siempre lleva Reason;
// un hecho Unknown se trata como False. La tabla real de gates es de U4-T04.
package authz
