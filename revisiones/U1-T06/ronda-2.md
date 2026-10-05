# Ronda 2 — U1-T06

VEREDICTO: NO-VERDE

Los nueve criterios dan lo esperado (con las salvedades arbitradas de CA-2 y CA-7). F-01 de la ronda 1 (correlación) queda corregido y comprobado con el binario real. F-02 de la ronda 1 queda corregido solo en parte: sobreviven mutantes de fuga de PII y de semántica de métricas. Dos NARANJA. La ronda 2 no tocó `deploy/` (CA-7 y CA-8 no se repiten). Worktree limpio, HEAD `b534fdb`, sin procesos vivos. Todo del revisor en `scratchpad/rev-u1t06-r2/`.

## Criterios de aceptación, verificados por el revisor (`GOTOOLCHAIN=go1.26.8`, `-count=1`, binarios de HEAD y de `origin/main` desde `git archive`)
| # | Resultado |
|---|---|
| 1 | go-intake 120 PASS / 0 FAIL, ui-api 176 / 0, rc=0; `go vet` y `gofmt` limpios |
| 2 | `healthz 200`, `readyz 200`; con `chmod 500` sigue en 200 (root); variante arbitrada: borrar `data_dir` → `"data_dir":"fail"`, `unavailable`, 503; al recrearlo vuelve a 200 |
| 3 | `true` y `4bf92f3577b34da6a3ce929d0e0e4736` |
| 4 | `0` |
| 5 | `2`, `0`, `1`, `36` |
| 6 | 8 pruebas `TestOpsEndpoints*` más `TestOpsEndpointsNeedNoTokenAndSkipApp` PASS; en vivo con burst 3 y XFF: `200 200 200 429 429 429`, otra IP intacta, `/healthz` 200 con la IP limitada |
| 7, 8 | no repetidos (el diff no toca `deploy/`) |
| 9 | `ok`, `ok`, `0`, `0`, `0` |

CI del PR #35: sin confirmar. No existe ejecución sobre `90b0a5d`; sobre `b534fdb` los jobs terminaron `cancelled` y `skipped` sin pasos (solo `no-latest` en success); `vuln` sigue sin confirmar.

## F-01 de la ronda 1 (correlación): corregido
Con el binario real, `UIAPI_DATA_DIR` borrado y `X-Request-Id: req-corr-12345`, el 500 da dos líneas `warn` (error y acceso) con el mismo `request_id` y `trace_id` (igual al `Traceparent` de la respuesta); sin cabecera, ids generados e iguales. Un 503 de go-intake y los de `/readyz` salen en `warn` con ids; las sondas sanas en `info`. `Config.Slog` opcional; `Config.Logger` no cambió; 0 líneas eliminadas en `_test.go`. Barrido de una sesión realista a nivel debug en ambos servicios (165 líneas): todas JSON válido con las cinco claves; toda línea dentro de una petición lleva `request_id` y `trace_id`; sin ids solo `go-intake escuchando`, `ui-api escuchando`, las del suscriptor y la advertencia de arranque (aceptable). Con PII, `s3cret`, tokens, `Bearer` y `sha256=` el log y `/metrics` dan `0` coincidencias. Mutantes de correlación muertos: sin `cfg.Slog`, contexto vacío, ids intercambiados, 5xx en `info`, llamadas sin ctx de «webhook aceptado» y «webhook rechazado».

## Regresión frente a `origin/main`
Mismas entradas contra base y HEAD: 0 diferencias de estado y de cuerpo (incluidos los 500 de confirmación con el directorio borrado; el panic de ui-api solo por pruebas). 25 confirmaciones concurrentes → 1 `201` y 24 `409`, `aqs_inbox_confirmations_total` en 1. Webhook firmado real de go-intake leído por ui-api: aparece en `/notifications` con el `trace_id` de la petición. Parser estricto idéntico en ambos binarios. `LOG_LEVEL` funciona en vivo aunque sin prueba (F-04).

## Hallazgos
### F-01 · NARANJA · `services/go-intake/internal/intake/handler.go` y `observability_test.go:225` · F-02 de la ronda 1 incompleto: mutantes de fuga de PII que la suite deja pasar
`TestObsAcceptedPayloadLeaksNothing` mata el mutante `"body", string(body)` en «webhook aceptado» y los de firma y de acceso. Pero el cuerpo firmado (con nombre y correo) se puede loguear en estas rutas y la suite sigue en verde (6 mutantes sobreviven): `unsupported_event`, `invalid_payload`, `invalid_json` (con el `err`), `unresolvable_artifact`, `unsupported_media_type` y `publish_failed` (`ErrorContext`). Son rutas donde llega un payload ya autenticado (p. ej. un `pull_request` con acción ignorada trae `sender` y `head_commit.author`). Es justo la clase que pidió la ronda 1: la prueba de fugas cubre una sola ruta. Exigido: que la prueba de PII recorra TODAS las rutas del handler con el payload distintivo (o un helper que repita la negativa en cada rechazo y en el fallo de publicación).

### F-02 · NARANJA · `internal/obs/obs.go` de ambos servicios y `internal/intake/handler.go:204` · semántica de métricas sin prueba, de la que depende el dashboard
Sobreviven: `code` hardcodeado (`fmt.Sprint(sw.status)` → `"200"` sobrevive en go-intake y muere en ui-api; las métricas de errores `code=~"5.."` quedarían en 0); unidad de latencia (`dur.Seconds()` → `Milliseconds()` sobrevive en ambos; el p95 del dashboard saldría 1000 veces mayor y las pruebas de buckets solo ven su forma); `reason` (quitar `invalid_payload` o `unsupported_media_type` del mapa `bad_request` en `rejectReason`: ese rechazo deja de contarse en `aqs_intake_webhook_rejected_total`); `publish_failures` (contarlo también en el fallo de `store.Put` sobrevive: no hay aserción de que siga en 0 en el 503 de almacén).

### F-03 · AMARILLO · correlación sin protección en rutas poco frecuentes
Sobreviven: `readyz: chequeo fallido` con `context.Background()` (ambos), el segundo `no se pudo persistir` de go-intake con `Error` sin ctx, quitar el campo `error` del log del 500 en ui-api, quitar el log de `readyz` (ambos) y el respaldo `h.log.Printf` de `logError`.

### F-04 · AMARILLO · otros mutantes plausibles que sobreviven
`ParseLevel` / `LOG_LEVEL` sin prueba alguna (el README documenta `LOG_LEVEL=warn`); `GET /healthz` de ui-api sin aserción sobre el cuerpo (en go-intake sí); `traceparent` con solo el trace-id en mayúsculas; flags de `traceparent` con 2 a 4 hex y flags de la respuesta `-00`; acceso 4xx a `warn` (`>=400`); el 500 en `info` (`> 500`) y `>= 502` sobreviven en go-intake; `HEAD` en las sondas devolviendo 405; `Cache-Control: no-store` quitado de las sondas; timeout de checks ×10; redacción de `cookie`; `registry-no-go`. `ts-local` es equivalente (el entorno corre en UTC).

### F-05 · AMARILLO · pérdida de `Connection: close` en el 413
`statusWriter` no propaga `requestTooLarge`: el 413 por cuerpo de 1,2 MB ya no lleva `Connection: close` como en la base (estado y cuerpo idénticos). Un panic dentro de una petición de go-intake sigue sin `request_id` (net/http lo registra por `log.Printf` con el stack; la base ya se comportaba así).

### F-06 · AMARILLO · CI sin confirmar
`vuln` y `test` no han corrido sobre `90b0a5d` ni sobre HEAD.

## Lo que sí aguanta
~185 mutantes en copias del scratchpad: mueren los 21 del codificador (o equivalentes) y los ≥6 propios del revisor sobre readiness, dominio y redacción (`DirWritable` sin borrar el temporal, `AppendableFile` con archivo ausente que pasa, secreto invertido, chequeos con nombre intercambiado, confirmación contada antes del `switch` o en un 409, gauge con el estado equivocado o sin `rejected`, `Counts` sin la vista, `readyz` con 200 en fallo, redacción de `token`, `authorization`, `secret`, `password`, `otp` y `value-leak`).

## Tareas candidatas (fuera de alcance)
- Recuperador de panic en el middleware de go-intake con log correlacionado.
- Propagar `requestTooLarge` desde el `statusWriter`, o añadirle `Unwrap` para `ResponseController`.
- Ruido de las sondas en `info` y `/readyz` sin caché ni rate limit (candidatas de la ronda 1).

VEREDICTO: NO-VERDE
NARANJA|services/go-intake/internal/intake/handler.go + observability_test.go:225|F-02 incompleto: loguear el cuerpo firmado en unsupported_event, invalid_payload, invalid_json, unresolvable_artifact, unsupported_media_type y publish_failed deja la suite en verde; la negativa de PII solo cubre la ruta aceptada
NARANJA|services/*/internal/obs/obs.go + go-intake/internal/intake/handler.go:204|Semántica de métricas sin prueba: `code` fijo en "200" (go-intake), latencia en milisegundos en vez de segundos (ambos), invalid_payload/unsupported_media_type sin contar como bad_request y fallo de almacén contado como publish_failure
INFORME: revisiones/U1-T06/ronda-2.md
