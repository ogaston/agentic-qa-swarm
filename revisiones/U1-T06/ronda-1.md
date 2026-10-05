# Ronda 1 — U1-T06

VEREDICTO: NO-VERDE

Dos NARANJA en pie (F-01 y F-02). Los nueve criterios dan lo esperado, con las salvedades arbitradas de CA-2 y CA-7. La CI del PR no terminó (jobs cancelados tras ~15 min en cola sin runner), así que `vuln` queda sin confirmar. Worktree limpio (HEAD `e59109b`); procesos del revisor detenidos. `GOTOOLCHAIN=go1.26.8`, `-count=1`, binarios de HEAD y de `origin/main`.

## Criterios de aceptación, verificados por el revisor
| # | Resultado |
|---|---|
| 1 | `go test -race` en verde: go-intake 112 PASS / 0 FAIL, ui-api 168 / 0; `go vet`, `gofmt`, `go mod tidy -diff` limpios; `go mod verify` ok |
| 2 | literal: `healthz 200`, `readyz 200` (con `chmod 500` como root sigue en 200, esperado); variante: borrar `data_dir` → 503 con `data_dir:"fail"` y `unavailable`, vuelve a 200 al recrearlo (el chequeo hace `CreateTemp`); con `chattr +i` también 503 y se recupera |
| 3 | `true` y `4bf92f3577b34da6a3ce929d0e0e4736` |
| 4 | `0`; con `LOG_LEVEL=debug`, un webhook válido con PII, una cookie y `?token=`: ni el log ni `/metrics` contienen nada de eso |
| 5 | `2`, `0`, `1`, `36` |
| 6 | 6 PASS de ui-api más `TestOpsEndpointsNeedNoTokenAndSkipApp`; en vivo 600 sondas con burst=3 sin ningún 429; el negocio sigue `200, 200, 200, 429` y `/healthz` en 200 con la IP limitada |
| 7 | defecto de especificación comprobado: tres dashboards (`aqs-dashboard`, `aqs-dashboard-u1`, `aqs-dashboard-u4`) en dev y prod; `aqs-dashboard-u1` con título `AQS U1 Ingesta` y 8 paneles; `policies.sh` con shim: 0 `FALLA`; kubeconform dev y prod con 63 válidos |
| 8 | siete métricas, todas publicadas; los 8 `expr` pasan `promtool check rules`; ids de panel únicos |
| 9 | `ok`, `ok`, `0`, `0`, `0`; `kustomization.yaml` solo ganó `- dashboard-u1.yaml` |

Regresión: mismas entradas contra base y HEAD (37 casos en go-intake y 31 en ui-api): 0 diferencias de estado y de cuerpo; solo cambian las cabeceras nuevas `X-Request-Id` y `Traceparent`. Identificadores con el binario real: `X-Request-Id` válido se acepta y uno corto, con espacio, con tilde o de 65 caracteres se regenera; `traceparent` válido se propaga y versión `01`/`ff`, ceros, longitud corta o sufijo extra se regeneran; el `trace_id` del evento `notify.created` es el del `traceparent` enviado y los 7 eventos validan contra el esquema real con ajv. Métricas por caso correctas (firma ausente/mala/doble `Authorization` → `invalid_signature`=4; ping e `issues` → `unsupported_event`=2; fork → `unresolvable_artifact`=1; cuerpo de 1,1 MB → `too_large`=1; tipo de contenido erróneo, falta de delivery, JSON inválido, payload inválido → `bad_request`=4; webhooks válidos → `created`=5; todos arrancan en 0); en ui-api confirmar cambia el gauge (`pending` 3→2, `confirmed` 0→1) y `aqs_inbox_confirmations_total` 0→1, un 409 o un 404 no cuentan; `route` siempre patrón o `unmatched` (métodos raros → `OTHER`; un id de 4000 caracteres no crea series).

## Hallazgos
### F-01 · NARANJA · `services/ui-api/cmd/ui-api/main.go:newHandler` y `internal/httpapi/api.go:98,247` · las líneas de error dentro de una petición salen sin `request_id` ni `trace_id`
Con `UIAPI_DATA_DIR` borrado, un `POST /notifications/{id}/confirm` con `X-Request-Id: req-corr-12345` da 500; el log tiene dos líneas: `level:"warn", message:"confirmando \"n-…\": persistiendo confirmacion: open …/confirmations.jsonl: no such file…"` con `request_id:""` y `trace_id:""`, y la línea de acceso (`status:500`, nivel `info`) con `request_id:"req-corr-12345"` y su `trace_id`. La línea que explica el error, y la de `panic atendiendo` (mismo mecanismo), no se correlaciona con la petición; el acceso de un 5xx tampoco sube de `info`. El codificador dijo que cambiar `Config.Logger` rompería pruebas existentes; no hace falta: basta un campo opcional `Config.Slog` o un `ctx` en esas dos llamadas. Lo que sí cumple: barrido de todas las líneas de ambos servicios en una sesión realista, todas JSON válido con las cinco claves; en go-intake toda línea dentro de una petición lleva ids; quedan vacíos solo los de arranque, los del suscriptor y los de `httpapi`. Exigido: que las líneas de `httpapi` emitidas dentro de una petición lleven `request_id` y `trace_id`, con prueba, y que un 5xx de acceso salga en `warn` o `error` (o que la bitácora diga que se decidió no hacerlo).

### F-02 · NARANJA · `services/go-intake/internal/intake/observability_test.go:150` · la negativa de fugas no cubre la ruta aceptada y varios mutantes sobreviven
Mutante propio: añadir `"body", string(body)` al log `webhook aceptado` deja toda la suite en verde (la fuga más grave posible: los payloads de GitHub traen nombres y correos). Causa: `TestObsLogContractAndNoSecrets` solo pone la marca `CUERPO-SECRETO-XYZ` en la petición con firma mala; la petición aceptada usa el fixture, sin cadena distintiva. Mutantes que también sobreviven (misma clase de cobertura ausente): mínimo de `X-Request-Id` de 8 a 9 (el borde de 8 no se prueba); `traceparent` con hex en mayúsculas aceptado; quitar `inFlight.Inc()` (`aqs_http_in_flight` sin prueba de valor); buckets del histograma cambiados por `{1}` (el p95 del dashboard depende de ellos); quitar `Allow` del 405 de las sondas; cambiar la etiqueta `route` de las sondas de go-intake. Exigido: una petición válida firmada con una marca distintiva en el cuerpo (nombre y correo) y comprobar que ni el log ni `/metrics` la contienen; los demás mutantes son AMARILLO por separado, salvo buckets e `in_flight`, que conviene cubrir en el mismo arreglo.

### F-03 · AMARILLO · CI sin confirmar
Los tres workflows de `e59109b` terminaron cancelados, no fallidos: `ci` (run 37366216129): `no-latest` en success; `discover`, `test`, `vuln`, `build`, `sbom` y `publish` cancelados o saltados sin ningún paso ejecutado; `policies` y `contracts` cancelados. Los jobs estuvieron en cola ~15 minutos sin runner y se autocancelaron (19:52–20:07 UTC). El `vuln` de ambos servicios sigue sin confirmar: relanzar la CI antes de decidir.

### F-04 · AMARILLO · observaciones menores
Ruido de las sondas: hoy los Deployments de go-intake y ui-api no tienen `livenessProbe` ni `readinessProbe` (solo identity y governance), así que es hipotético; a `periodSeconds=10` serían ~20 000 líneas por día y pod; la tarea lo fuerza con CA-3 y el README documenta `LOG_LEVEL=warn`; vía posible: `/metrics` y `/readyz` en `debug` y solo `/healthz` en `info`. `/readyz` hace una escritura en disco por llamada sin autenticación ni rate limit (la tarea lo exige); con un disco colgado cada llamada deja una goroutine bloqueada tras el timeout de 2 s. Un outbox que es un symlink roto da `outbox:"ok"` aunque el append fallaría; en ui-api un archivo de eventos o confirmaciones con basura da `ok` (coherente con el diseño: el parser descarta líneas ilegibles). Mutantes equivalentes sobre `IsRegular` e `IsDir` (`OpenFile` y `Stat` ya fallan antes).

## Arbitraje del orquestador, valorado
- Chequeos que no pueden fallar en el binario (`webhook_secret` y `token_verifier`): honesto y suficiente: `TestReadyChecksEachOneCanFail` cubre secreto vacío, verificador nulo y función nula; los mutantes mueren; el cableado está cubierto; ambos README dicen que no pueden fallar en el binario y que se prueban por inyección. No es hallazgo (a diferencia de U4 F-01, el invariante de arranque está documentado y los demás chequeos fallan de verdad).
- Etiqueta `service` frente al ServiceMonitor: no es hallazgo pero queda una dependencia frágil: el ServiceMonitor no usa `honorLabels` ni relabelings; el operador de Prometheus (no verificado en un clúster) añade `service` con el nombre del Service de Kubernetes y la etiqueta de la aplicación pasaría a `exported_service`; los Services se llaman exactamente `go-intake` y `ui-api`, así que `service=~"go-intake|ui-api"` seguiría dando los mismos valores y el dashboard no queda vacío; `dashboard-u4.yaml` no filtra por `service`. Candidata.

## Lo que sí aguanta
34 mutantes sobre go-intake y 26 sobre ui-api (copia del scratchpad): mueren por una prueba real los de readiness (datadir, outbox, events, secreto, verificador, cableado, timeout), motivos de rechazo, contadores y su cableado, `trace_id` del evento, `route` cruda y con id, regex de `request_id` y de `traceparent`, nivel en mayúsculas, hora no UTC, 200 siempre, sondas detrás de la app, firma, `Authorization` y redacción de claves sensibles. `/readyz` en vivo de ambos servicios: `data_dir` ausente, reemplazado por un archivo o inmutable → 503 y se recupera; outbox o events como directorio → 503. Las sondas viven fuera de token, rate limit y `secure()`; excepción de cabeceras documentada en los README. Fallos de arranque como JSON de nivel `error` con `rc=1`. Sin desborde; no se tocaron pruebas existentes (solo se añadió `TestRoutePatternBoundsCardinality` al final de `api_test.go`).

## Tareas candidatas (fuera de alcance)
- Quitar la dependencia entre el filtro `service` del dashboard y el nombre del Service: `honorLabels: true` en el ServiceMonitor, o no filtrar.
- Sondas, `securityContext` y `fsGroup` para los Deployments de go-intake y ui-api.
- Readiness que detecte el symlink roto del outbox.
- Rate limit o caché de unos segundos para `/readyz` sin autenticación.

VEREDICTO: NO-VERDE
NARANJA|services/ui-api/cmd/ui-api/main.go newHandler + internal/httpapi/api.go:98,247|Las líneas de error emitidas dentro de una petición (500 «confirmando…», panic) salen con request_id y trace_id vacíos, y el 5xx de acceso sale en info; sin correlación en el log
NARANJA|services/go-intake/internal/intake/observability_test.go:150|La negativa de fugas no cubre la ruta aceptada: loguear el cuerpo en «webhook aceptado» deja la suite en verde; sobreviven también mínimo 8 de request_id, hex en mayúsculas, in_flight y buckets del histograma
INFORME: revisiones/U1-T06/ronda-1.md
