# Ronda 1 — U4-T01

VEREDICTO: NO-VERDE

Los 7 criterios de aceptación pasan con la ejecución propia del revisor. Queda un hallazgo NARANJA: la prueba `UnknownFactIsFalse` no ejercita el evaluador. Worktree limpio tras la revisión.

## Criterios de aceptación, verificados por el revisor
| # | Criterio | Resultado |
|---|---|---|
| 1 | Pruebas de ambos módulos con `-race` (con `-count=1`) | pasa: `ok .../authz`, `ok .../principal`, sin `FAIL` |
| 2 | `FakeEvaluator` fail-closed y programable | pasa: 7 `PASS` (DefaultDeny, ErrorIsNotAllow, UnknownFactIsFalse, RecordsCalls, Concurrent, AllowDenyAndSpecificOverWildcard, InvalidInputDenied) |
| 3 | Matriz Allow/Deny y reproducción | pasa: 17 filas, `allow,deny`, 14 deny, 17 subpruebas `PASS` más `Shape`, `ShapeNegative` y `Replay` |
| 4 | Ningún `allow` con hecho `unknown` | pasa: `true` |
| 5 | Principales y roles | pasa: `admin,user`; 3 inválidos |
| 6 | Higiene de módulos | pasa con `test -e go.sum` (go.sum vacío: módulos sin dependencias; la tarea lo permite si se documenta, y la bitácora lo documenta) |
| 7 | Árbol limpio y sin desborde | pasa: `0` y `0` |

## Hallazgos
### F-01 · NARANJA · `services/go-governance/authz/fake_test.go:49` y `fake.go:79` · «Unknown es False» no se comprueba a través del evaluador
`TestFakeEvaluatorUnknownFactIsFalse` solo comprueba `Fact.IsTrue()` y que el valor cero sea `Unknown`; nunca llama a `AuthorizeTransition`, que no lee ningún hecho. Sonda del revisor (copia fuera del worktree): un fake con `Allow("","")` y `GateInput{RunID:"r", From: done, To: running}` con todos los hechos `Unknown` devuelve `{Allow:true Reason:permitido}`. La prueba exigida por nombre en CA-2 está en verde sin respaldar la propiedad que su nombre afirma.
Opciones: (a) documentar en `doc.go` y en la prueba que el fake no evalúa hechos (solo la regla `(From, To)`) y reencuadrar la prueba; (b) hacer que una regla `Allow` degrade a Deny cuando algún hecho sea `Unknown`/`False`, y probarlo.

### F-02 · AMARILLO · `services/go-governance/authz/matrix_test.go:70-91` · La reproducción de la matriz es circular
`TestMatrixReplay` programa `Deny(from, to, r.ReasonContains)` y comprueba que `Reason` contenga `ReasonContains`: se cumple por construcción. La tarea pide ese diseño; solo valida el cableado. Anotar que la fuerza real de la matriz llega con U4-T04.

### F-03 · AMARILLO · `bitacoras/U4-T01.md:21-25` · Evidencia resumida, no literal
La sección «Verde» pega solo la salida del primer comando; CA-2 a CA-6 van como prosa y CA-7 no aparece.

### F-04 · AMARILLO · `services/go-governance/authz/matrix_test.go:56` · Desviación menor del plan de pruebas
El plan pedía una copia temporal de la matriz con un `allow` y un hecho `unknown`; se usó una matriz en memoria (misma función `checkShape`). Equivalente.

### F-05 · AMARILLO · `services/go-governance/authz/doc.go` · Firmas parciales
La tarea pide documentar las firmas del modelo en `doc.go`; solo figura `Evaluator`. Faltan `GateInput`, `Decision`, `Fact` y las reglas del fake (orden de resolución específica > `(from,"")` > `("",to)` > `("","")`).

## Tareas candidatas (defectos reales fuera de alcance)
- En U4-T04, reproducir la matriz contra el evaluador real sin programarlo con la verdad de cada fila (ver F-02).
- La CI no corre los módulos sin `Dockerfile` (limitación conocida de la tarea).

VEREDICTO: NO-VERDE
NARANJA|services/go-governance/authz/fake_test.go:49, fake.go:79|«Unknown es False» no se comprueba a través del evaluador
INFORME: revisiones/U4-T01/ronda-1.md
