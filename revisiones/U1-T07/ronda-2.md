# Ronda 2 — U1-T07

VEREDICTO: VERDE

Verifiqué yo mismo que F-01 (el aborto del cliente ya no abre el circuito), F-02 y F-04 están corregidos de verdad. Corrí CA-1..CA-10 frescos (`-count=1`, podman con `:z`) y todos pasan. No queda ningún ROJO ni NARANJA. El worktree quedó limpio (`git status --short` da 0).

## Criterios de aceptación, verificados por mí
| # | Resultado |
|---|---|
| CA-1 | `262` PASS, `0` FAIL, `5` SKIP con tag `contract` sin `IDENTITY_URL`. Pasa. `go test -race ./...` también en verde. |
| CA-2 | `200 401 401 401`. Pasa. |
| CA-3 | `201`, `1`, `ev.json valid` (ajv), `marta checkout,refund`, `run_id coincide`, principal `marta`. El token aparece 0 veces en el log y 0 en `/metrics`. Pasa. |
| CA-4 | Ocho `503`, `Retry-After` = 1, `aqs_identity_circuit_open 1`. Pasa. |
| CA-5 | `TestRepublishPendingSendsExactlyOnceEvenIfCalledTwice`, `TestPublishPendingKeepsReceiptWhenPublishFails` y `TestPublishPendingCountIgnoresOrphanPublishedIDs` en PASS. Pasa. |
| CA-6 | Cuatro `rc=1`. Pasa. |
| CA-7 | 5 PASS de contrato contra `go-identity` real, sin FAIL ni SKIP. Pasa. |
| CA-8 | `actionlint rc=0`, `0`, `0`, sin líneas `ACCION NUEVA`. Pasa. |
| CA-9 | `0`, `65532:65532`, `0`. Pasa. |
| CA-10 | `ok`, `0`, `0`. Pasa. |

## Verificación de las correcciones de la ronda 1
- **F-01 corregido de caja negra.** Usé el `slow.py` de la ronda 1 como identidad falsa y `ui-api` real en `:18211`.
  - Una petición válida antes de los abortos da `200`.
  - Ocho abortos del cliente (`curl -m 0.3` con token `slow`) no abren el circuito.
  - Una petición válida después de los abortos da `200`, y las métricas muestran `circuit_open 0`, `error 0`, `ok 2` y `aqs_identity_circuit_open 0`. En la ronda 1 daba `503` con `error 5` y circuito abierto.
- **Timeout propio sigue contando.** Con una identidad falsa que tarda 3 s (`hang.py`) y clientes que no abortan, cinco peticiones dan `503`. Las métricas muestran `error 5` y `aqs_identity_circuit_open 1`. En `Verify`, `ctx` es el contexto padre; el timeout de 2 s vive en el contexto derivado dentro de `call`, así que `ctx.Err()` es nil cuando vence el timeout propio y no se confunde con la cancelación del padre.
- **Sonda medio-abierta abortada.** `abort(probe)` libera `probing` sin tocar `fails` ni `openUntil`. La prueba nueva comprueba que `probing` queda libre y que la siguiente sonda cierra el circuito. Lo leí en el código y en la prueba. No lo reproduje de caja negra: solo el aborto con el circuito cerrado.
- **Sin timeout de contexto padre en la ruta de petición.** `ReadTimeout` y `WriteTimeout` de `http.Server` no cancelan el contexto. No hay `TimeoutHandler` ni `WithTimeout` sobre la petición. Por eso no hay riesgo de que un deadline del padre enmascare una identidad lenta.
- **F-02 corregido.** La valla compara `prod` y `production` sin distinguir mayúsculas ni espacios, y la prueba cubre `PROD`, `Production` y ` prod `.
- **F-04 corregido.** `PublishPendingCount` cuenta recibos sin marca, así que no puede dar negativo. Lo cubre `TestPublishPendingCountIgnoresOrphanPublishedIDs`.
- **F-03 (candidata) y F-05 (no es defecto):** las respuestas son correctas. El O(n) del outbox JSONL es transitorio hasta C-45 y F-05 queda como decisión documentada.

## Hallazgos
### F-01 · AMARILLO · `internal/auth/identity.go` (`Verify`, rama `ctx.Err() != nil`) · Un 5xx de identidad que llega justo con el padre ya cancelado no cuenta como fallo
Es una carrera mínima e inocua: no abre el circuito de más ni deja de abrirlo en un fallo sostenido. No bloquea.

### F-02 · AMARILLO · `internal/auth/identity.go` · Los abortos del cliente no se registran en `aqs_identity_calls_total`
Solo hay cuatro valores de `result`, y el aborto no es ninguno de ellos. Es coherente con la tarea. Si se quiere observabilidad de abortos, sería un valor nuevo del contador, fuera de esta tarea.

### F-03 · AMARILLO · `cmd/ui-api/main.go` (`verifierFromEnv`) · La llamada a `UIAPI_ENV` se repite dos veces en una sola línea larga
Es solo de legibilidad: se puede sacar a una variable local.

## Tareas candidatas (defectos reales fuera de alcance)
- Mantener la candidata de la ronda 1: `deploy/flux/base/control-plane.yaml` no define `UIAPI_AUTH`, `IDENTITY_URL` ni `UIAPI_OUTBOX_FILE` para `ui-api`. Se cablea con la integración de despliegue.
- Mantener la propuesta del codificador sobre F-03 de la ronda 1 (O(n) del outbox hasta que llegue el transporte real, C-45).
- Pendiente de la CI de GitHub (no verificable aquí): `govulncheck` (job `vuln`) y la ejecución real del workflow.

VEREDICTO: VERDE
AMARILLO|services/ui-api/internal/auth/identity.go (Verify)|Un 5xx con el padre ya cancelado no cuenta como fallo (carrera inocua)
AMARILLO|services/ui-api/internal/auth/identity.go|Los abortos del cliente no se registran en `aqs_identity_calls_total`
AMARILLO|services/ui-api/cmd/ui-api/main.go (verifierFromEnv)|La consulta de `UIAPI_ENV` se repite en una línea larga
