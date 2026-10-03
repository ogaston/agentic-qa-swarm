# Ronda 2 — U4-T04

VEREDICTO: VERDE

Los 13 criterios pasan con la ejecución propia del revisor (`GOTOOLCHAIN=go1.26.8`, `-count=1`). F-01 y F-06 corregidos y probados con el binario real y con mutantes. Sin ROJO ni NARANJA. Worktree limpio; sin push ni nada contra un clúster.

## Criterios de aceptación, verificados por el revisor
| # | Resultado |
|---|---|
| 1 | 281 PASS, 0 FAIL con `-race -count=1`; `Matrix` 20 PASS frente a `jq length`=17 (3 pruebas de nivel superior + 17 subpruebas, arbitraje vigente); 7 paquetes `ok` |
| 2 | `ok_warm=[true]`, `sin_confirm`/`sin_reset`/`sin_ensayo` `false` con la razón esperada, `con_ensayo=[true]`, `unknown=[false]` |
| 3 | `false` ×6, `[false]` ×3 |
| 4 | `sin=401 persona=401 roto=400`, `sin-allow` |
| 5 | `user=403 sin=401 admin=200 v=1`, `igual=200 v=1 cambia=200 v=2`, `conf_false=422 conf_true=200 evento_malo=422 cuota_malo=422 desconocida=404`, `lee_user=200 sin_politica=404` |
| 6 | `true true false / false / true / false` |
| 7 | con `workflow_allowed:"true"` en la segunda llamada: `["gate.deny","gate.allow"]`, `delete=405 put=405 sin_token=401`, `integra rc=0`; la manipulación se comprobó mutando líneas reales |
| 8 | `[false]` y 6 pruebas `FailClosed*` en PASS |
| 9 | `sin=401 user=403 admin=200`, `identidad_caida=503` |
| 10 | cuatro `rc=1` |
| 11 | redocly válido, 0 líneas eliminadas, `serviceToken \| get,put \| GateDecision,GateRequest` |
| 12 | `docker build` literal construyó; `65532:65532`; `FROM` con tag fijo; `list-services`=1; `go`; `go 1.26.8`; sin `k8s.io` |
| 13 | `vet-ok`, gitleaks sin hallazgos, 0, 0 |

## Verificaciones pedidas
- **F-01 con el binario real.** Con `workflow:"nada"` inexistente: `allow=false` en `confirmed→warm_ready`, `warm_ready→deploying`, `deploying→inferring`, `inferring→rehearsing`, `rehearsing→running`; siguen permitidos `running→resetting`, `deploying→resetting`, `inferring→resetting`, `resetting→reporting`, `reporting→done` y los 8 `→failed`. Con `checkout` (existente) las 19 transiciones dan `true`. Sin política `workflows`, `nada` da `warm=false` y reset/fail/done/rep dan `true`. Aprobación y cuota solo actúan en `running`. Política corrupta en disco impide arrancar. La matriz de 17 filas pasa contra `RuleEvaluator` y `authorize_matrix.json` no cambió. Mutante (quitar `gatedWorkflow = true` de `warm_ready` e `inferring`): `TestWorkflowExistenceByDestination` falla.
- **F-06.** `TestAppendSyncsEveryEntry` y `TestSyncFailureFailsAppendAndPoisons` pasan; el mutante sin `Sync` las hace fallar y el que ignora el error falla la segunda; la interfaz `logFile` no cambia el comportamiento. Mismas manipulaciones de auditoría que en la ronda 1 → `verify-audit` rc=1 con el número de línea (alterar acción, borrar, reordenar, alterar `run_id`/`prev_hash`, truncar). Límite conocido sin cambio: borrar la cola no se detecta.
- **F-02.** `authz/doc.go` documenta que `workflow_allowed` en `warm_ready` es autodeclarado y que U2-T02 debe reportarlo antes de la selección de workflow, y la regla de existencia/cuota/aprobación por destino.
- **Regresiones:** ninguna. 40 gates con tope 5 → exactamente 5; 25 `PUT` → versiones 1..25, cadena íntegra con 66 entradas; el token de servicio no aparece en log ni `data/`.
- **CI de GitHub:** sobre `1f3767b` (contiene el código `1469daf` y solo añade documentación) pasan `ci` (`test`, `build`, `sbom`, `vuln (services/go-governance)`), `contracts` y `policies`.

## Hallazgos
### F-07 · AMARILLO · `authz/doc.go` · Párrafo pegado en el comentario
El párrafo sobre `workflow_allowed` en `warm_ready` quedó sin salto de párrafo con la frase sobre la matriz. Solo legibilidad.

## Valoración de F-03, F-04 y F-05
Ninguno es un defecto dentro de alcance que deba bloquear: F-03 (hash del contenido de la política) no está pedido; la tarea exige «política y versión usadas» y el código lo cumple. F-04 (decodificación estricta) no está pedida; el llamante es un servicio de confianza. F-05 es literal a la tarea. Los tres son candidatas legítimas.

## Tareas candidatas (fuera de alcance)
- Las de la ronda 1: omitir `workflow` y declarar `workflow_allowed=true` salta cuota y aprobación; cadena sin clave ni ancla externa (cola truncada no detectada); disco lleno deja el servicio sin arrancar; un `user` puede crecer la auditoría con `policy.rejected`; `from` lo declara el llamante; estado por réplica.
- Hash del valor de la política en `policy.set`, decodificación estricta y listas blancas de `GOVERNANCE_ENV` y `GOVERNANCE_TEST_NAMESPACE`.

VEREDICTO: VERDE
AMARILLO|authz/doc.go|Línea larga en el comentario de workflow_allowed en warm_ready (no bloquea)
INFORME: revisiones/U4-T04/ronda-2.md
