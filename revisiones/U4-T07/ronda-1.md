# Ronda 1 — U4-T07

VEREDICTO: NO-VERDE

Un NARANJA en pie (F-01). Los 14 criterios dan lo esperado con las salvedades arbitradas. Worktree limpio (HEAD 0ea2af5). Sondas y mutantes del revisor en el scratchpad.

## Criterios de aceptación, verificados por el revisor
| # | Resultado |
|---|---|
| 1 | identity 192 PASS / 0 FAIL, governance 316 / 0 con `-race` (base de `origin/main` medida: 172 y 281) |
| 2 | `200 200 200`, `200 200 503`, `unavailable` |
| 3 | 3 PASS (cadena rota → `readyz` 503 y `aqs_audit_chain_ok 0`) |
| 4 | métricas de identity: `6`, `1`, `5`, `1`, `1` |
| 5 | con `workflow_allowed:"true"` en el allow: `2`, allow 1, `namespace_not_test` 1, `not_confirmed` 1, accepted 1, rejected 1, entries 6, chain_ok 1, retention 90 |
| 6 | cardinalidad: `0`, `15`, `0` |
| 7 | log y secretos: `true`, `0`, `gate.allow`, `0` |
| 8 | retención: 0, 30, 89 → `rc=1`; `abc`, `-5`, `90.5` también; `091` y vacío arrancan; sin usos de Remove/Truncate/Rename/O_TRUNC/WriteFile en producción |
| 9 | PVC `90 5Gi ReadWriteOnce sin-clase`; Deployment `90 /healthz /readyz 90 go-governance-data`; identity `/healthz /readyz`; kubeconform dev y prod `Valid: 62, Invalid: 0, Errors: 0, Skipped: 0` (con `--network host` y el CA del proxy) |
| 10 | 7 alertas con severidad, `0`, `SUCCESS: 7 rules found`, `test rc=0` |
| 11 | `7`, `7`, sin `NO PUBLICADA`, `fin` |
| 12 | título correcto, 12 paneles, sin `NO PUBLICADA`; las 12 expresiones parsean |
| 13 | `policies.sh`: 0 `FALLA`, 11 `OK` (con shim de red) |
| 14 | `65532:65532` en ambas imágenes, 0 dependencias k8s, `go 1.26.8` y `client_golang v1.24.1` intactos, `go mod tidy -diff` y `go mod verify` limpios; `0`, `0`, `0` |

CI del PR #25 sobre `0ea2af5`: `ci`, `policies` y `contracts` en success (incluidos los `vuln` de ambos servicios).

## Arbitraje del orquestador, valorado
- `TestNoDeleteAPI`: sigue siendo una lista blanca exacta; los mutantes que añaden `Purge`, `Truncate` o `Rename` fallan todos. Solo lo burla un `VerifyNow` que trunque (limitación previa; el `grep` de CA-8 lo cubre). No debilita la garantía.
- `TestAuthZ_UnregisteredRoute404`: cambiar `/healthz` por `/health` no debilita nada; los endpoints operativos están fuera de `Routes()`.
- `Decision.Code`: sin cambios de decisión. Comparación del binario de `origin/main` con el de HEAD sobre 1518 entradas (17 de la matriz, 1500 aleatorias, una malformada): 0 diferencias en estado HTTP y JSON sin `audit_ref`; `sum(aqs_gate_denied_total)` = 1448 = denegaciones reales. Mapeo honesto.
- `Service.Forbidden`: coherente con la tarea (que no define `rejected`); ver F-04.
- fsGroup y `strategy`: ver F-02 (no NARANJA).

## Hallazgos
### F-01 · NARANJA · `cmd/go-governance/main.go` `policyCheck` · el chequeo «política legible» de `/readyz` no puede fallar y no tiene prueba negativa
`FileStore.Get` solo consulta un mapa en memoria y devuelve error únicamente si el contexto se cancela. Con el servicio en marcha, corromper `policies.jsonl` dejó `/readyz` en `200 {"policies":"ok"}`. Mutante: dejar `policyCheck` sin condición de error (`if _, _, err := ps.Get(ctx, n); false {`) y toda la suite sigue en verde; `TestReadyChecks` solo prueba el caso positivo. La tarea pide que `/readyz` falle con política ilegible y que CA-1 cubra «readiness con cada fallo». Es la única de las cuatro comprobaciones de governance sin prueba de fallo. El almacén tampoco expone su estado `poison` a `/readyz`. Exigido: un chequeo que pueda fallar de verdad (p. ej. el almacén informa de un `poison` o de un archivo ilegible) con caso negativo; o, si el diseño en memoria lo hace imposible tras el arranque, decirlo en la bitácora y dejar de afirmar que «lee las 4 políticas» (lo decide el humano).

### F-02 · AMARILLO · `deploy/flux/base/control-plane.yaml` · volumen no escribible sin `fsGroup` y rollout con RWO
Con `/data` root:root 0755 (PVC típico) y la imagen como 65532: `open /data/audit.jsonl: permission denied`; con el directorio en `65532:65532` arranca. Riesgo real, pero no NARANJA: la tarea limita los cambios de esos Deployments a PVC, volumen, env, anotaciones y sondas (CA-9/CA-14 acotan el diff); el Deployment ni arranca hoy sin `GOVERNANCE_SERVICE_TOKEN` (C-53); ningún criterio comprueba la escritura en `/data`. Candidata: ampliar C-53 con `fsGroup: 65532` y `strategy: Recreate`, y enmendar el texto de la tarea.

### F-03 · AMARILLO · cobertura de pruebas
`AqsAuthFailuresHigh`: pasar `for: 5m` a `2m` deja `promtool test rules` en `rc=0`. Quitar la inicialización en 0 de `aqs_policy_changes_total` no rompe ninguna prueba. Quitar de `usersCheck` de identity el rechazo de «archivo sin usuarios» no rompe ninguna prueba.

### F-04 · AMARILLO · `internal/service/service.go` `Forbidden` · afirmación inexacta y asimetría con la auditoría
La bitácora dice que es «la única forma de que CA-5 dé `rejected=1`»; no es cierto (contar solo el 422 o solo el 403 da 1; contar ambos da 2). Se eligió una de dos opciones válidas. La auditoría registra el 403 como `policy.rejected` y la métrica no lo cuenta.

### F-05 · AMARILLO · varios menores
`AuditAppended` emite la línea `message:"audit"` sin `request_id` ni `trace_id`. `ChainMonitor.Check` y `Log.VerifyNow` verifican el archivo entero con el cerrojo de `Append` tomado y TTL fijo de 10 s (~10 µs por entrada: ~200 ms con 20 000 entradas; ~10 s con un millón). En go-identity, `/healthz`, `/readyz` y `/metrics` salen sin `Content-Security-Policy`, `Strict-Transport-Security` ni `X-Content-Type-Options` (`obs.Wrap` envuelve por fuera de `secure()`), mientras el README afirma «cabeceras de seguridad en todas las respuestas».

## Lo que sí aguanta
Sin fugas de contraseña, token, OTP, campo inventado ni hash en logs (nivel `debug`) ni en `/metrics`; `route` siempre patrón o `unmatched` (métodos raros → `OTHER`; `to` inválido → `invalid`; política desconocida → `unknown`); mutantes sobre producción detectados (redacción, regex de `request_id`, ruta cruda, TTL de la cadena, gauge de cadena, contadores de escalada y bloqueo, mínimo de 90, prioridad de motivos, `audit_failed`, `accepted`, detalle del log de auditoría); `/readyz` de governance en vivo: cadena alterada → 503 y `aqs_audit_chain_ok 0` en pocos segundos y se recupera al restaurar; auditoría borrada → 503 en `data_dir`; identidad inalcanzable → 503; `/readyz` de identity: 503 con archivo de usuarios roto, vacío o ausente, sin filtrar hashes.

## Tareas candidatas (fuera de alcance)
- Ampliar C-53 con `fsGroup: 65532` y `strategy: Recreate` para go-governance (F-02).
- Verificación incremental de la cadena de auditoría sin retener el cerrojo (F-05).
- Cabeceras de seguridad en los endpoints operativos de go-identity, o documentar la excepción (F-05).
- Cableado de `promtool test rules` de seguridad en CI (ya es C-33).

VEREDICTO: NO-VERDE
NARANJA|services/go-governance/cmd/go-governance/main.go policyCheck (readiness «política legible», CA-1)|El chequeo de política de /readyz no puede fallar con el almacén real y no tiene prueba negativa: el mutante sobrevive y con policies.jsonl corrupto /readyz sigue en 200
INFORME: revisiones/U4-T07/ronda-1.md
