# Ronda 2 — U4-T01

VEREDICTO: VERDE

Corrí CA-1..CA-7 yo mismo, con `-count=1` donde aplica, y todos pasan. Las respuestas a F-01..F-05 se sostienen contra el código real. No queda ningún ROJO ni NARANJA. El worktree quedó limpio tras la revisión.

## Criterios de aceptación, verificados por el revisor
| # | Criterio | Resultado |
|---|---|---|
| 1 | Pruebas de ambos módulos con `-race` | pasa: `ok .../authz`, `ok .../principal`, sin `FAIL` |
| 2 | `FakeEvaluator` fail-closed y programable | pasa: 8 `PASS` (DefaultDeny, ErrorIsNotAllow, UnknownFactIsFalse, RecordsCalls, Concurrent, AllowDenyAndSpecificOverWildcard, DoesNotEvaluateFacts, InvalidInputDenied) |
| 3 | Matriz y reproducción | pasa: 17 filas, `allow,deny`, 14 deny, 20 líneas `PASS` distintas, 0 `FAIL` |
| 4 | Ningún `allow` con hecho `unknown` | pasa: `true` |
| 5 | Principales y roles | pasa: `admin,user`; 3 inválidos |
| 6 | Higiene de módulos | pasa con `test -e go.sum` (go.sum vacío: sin dependencias; permitido y documentado) |
| 7 | Árbol limpio y sin desborde | `0` en el árbol; el diff da `2` por `revisiones/U4-T01/` (informes del bucle, no código). Con esa exclusión `0`. Exclusión razonable. |

## Verificación de las respuestas de ronda 1
- F-01 (era NARANJA): resuelto con la opción (a). `doc.go` dice que el fake NO evalúa hechos y documenta el orden de resolución; `TestFakeEvaluatorUnknownFactIsFalse` ejerce `Fact.IsTrue()`, el valor cero de `GateInput`, `UnmarshalJSON("unknown")` y el rechazo de `"quizas"`; `TestFakeEvaluatorDoesNotEvaluateFacts` fija el comportamiento. La aplicación sobre decisiones queda diferida a U4-T04 y `doc.go` lo dice.
- F-02: `doc.go` anota que la reproducción es circular y que la fuerza real llega con U4-T04. Aceptado.
- F-03: la bitácora pega comandos literales y salida para CA-1..CA-6 (ver F-06).
- F-04: equivalente (misma función `checkShape`). Aceptado.
- F-05: `doc.go` documenta `Evaluator`, `GateInput`, `Fact`, `Decision` y el orden de resolución.

## Hallazgos
### F-06 · AMARILLO · `bitacoras/U4-T01.md:87-88` · Evidencia de CA-7 no literal
La bitácora dice «0 y 0 (verificado tras el commit, ver abajo)» sin pegar la salida; el comando literal da `2` por los archivos de `revisiones/`. No bloquea.

### F-07 · AMARILLO · `services/go-governance/authz/fake_test.go:49` · El nombre de la prueba promete más de lo que ejerce
Respalda «Unknown es False» solo a nivel de `Fact`, no de decisión. Está documentado. Cuando U4-T04 implemente el evaluador real debe existir una prueba equivalente a través de `AuthorizeTransition`.

## Tareas candidatas (defectos reales fuera de alcance)
- U4-T04: reproducir la matriz contra el evaluador real sin programarlo con la verdad de cada fila, e incluir una prueba de que un hecho `Unknown` deniega a través de `AuthorizeTransition`.
- La CI no corre los módulos sin `Dockerfile` (limitación conocida).
- La redacción de CA-7 debería contemplar `revisiones/<tarea>/` en el patrón de exclusión.

VEREDICTO: VERDE
AMARILLO|bitacoras/U4-T01.md:87-88|Evidencia de CA-7 no literal (el comando literal da 2 por revisiones/; exclusión razonable)
INFORME: revisiones/U4-T01/ronda-2.md
