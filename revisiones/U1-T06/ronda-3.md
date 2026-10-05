# Ronda 3 — U1-T06

VEREDICTO: NO-VERDE

Los nueve CA dan lo esperado, y el arbitraje de ronda 2 queda cumplido para PII y para la semántica de métricas del código. Sobreviven mutantes plausibles de cableado y de nivel de log, así que no puedo darlo por VERDE. Hay un NARANJA y varios AMARILLO.

Trabajé sobre `git archive HEAD` en `/tmp/claude-0/-home-user/4ca8050b-f1f3-5eab-a78d-10b43e5bda2f/scratchpad/rev-u1t06-r3/`. No toqué el worktree, que sigue limpio (0 cambios) y en HEAD `cb2c74d`. No quedan procesos vivos.

## Criterios de aceptación (`GOTOOLCHAIN=go1.26.8`, `-count=1`)
| CA | Resultado |
|---|---|
| 1 | go-intake 151 PASS / 0 FAIL, ui-api 212 / 0. La suma supera el mínimo de 10. |
| 2 | `healthz 200`, `readyz 200`. Con `chmod 500` sigue en 200 porque corro como root. Variante arbitrada, borrar `data_dir`: `"data_dir":"fail"`, `unavailable`, 503. |
| 3 | `true` y `4bf92f3577b34da6a3ce929d0e0e4736`. |
| 4 | `0`. |
| 5 | `2`, `0`, `1`, `36`. |
| 6 | 10 pruebas `TestOpsEndpoints*` y `TestOpsEndpointsNeedNoTokenAndSkipApp` en PASS, incluida la de las 18 rutas. |
| 7 | Con docker: el build da `aqs-dashboard`, `aqs-dashboard-u1` y `aqs-dashboard-u4`. El tercero ya está en la base (U4-T07), no es desborde. El título es `AQS U1 Ingesta` y hay 8 paneles. `policies.sh` da 2 `FALLA kubeconform` (dev y prod) por falta de red hacia `raw.githubusercontent.com`; no es un fallo del contenido. El job `policies` de CI en GitHub pasó en `cb2c74d`. Conftest pasa 1449 de 1449. |
| 8 | Solo nombres publicados: `aqs_http_request_duration_seconds_bucket`, `aqs_http_requests_total`, `aqs_inbox_confirmations_total`, `aqs_inbox_notifications`, `aqs_intake_notifications_created_total`, `aqs_intake_publish_failures_total`, `aqs_intake_webhook_rejected_total`. |
| 9 | `go-intake ok`, `ui-api ok`, `0` archivos desbordados (excluyendo `revisiones/`, como indica la tarea), `0` líneas extra en `kustomization.yaml`. |

## Arbitraje de ronda 2: PII y métricas
Enumeré los sitios de log del código de producción. En go-intake: el `reject` con sus 10 llamadas, los dos `Put`, la publicación, la aceptación, `store.go:63`, `main.go` y `readyz`. En ui-api: `logError`, el panic, el suscriptor, `main.go` y el log de acceso.

- **Cubiertos:** todos los de handler y obs, y los rechazos del webhook por cada causa (17 rutas en go-intake, 18 más el panic y 13 líneas inválidas en ui-api).
- **Fuga corregida:** `ParseNotifyCreated` ya no mete nombres de clave ni el `err` de json en «evento descartado». Lo verifiqué con mutantes que restauran el contenido y todos mueren.
- **Sin cubrir pero inocuo:** `store.go:63` de go-intake solo imprime el número de línea. Meter la línea del registro en el log sobrevive; esas líneas son del propio servicio y no llevan payload de GitHub.
- **Panic en go-intake:** no hay recuperador. Es el comportamiento de la base (net/http lo registra), ya registrado como candidata.
- **405 de `GET /webhooks/github`:** sin prueba. En vivo confirmé que no filtra la ruta ni lleva PII en el log, y la etiqueta sale como `/webhooks/github`.

La semántica de métricas quedó bien fijada en el código: `code`, unidad de latencia, `reason`, `publish_failures`, buckets, etiquetas y `in_flight` mueren. Con ellas mueren también los míos de `traceparent`, readyz, redacción y `Connection: close` solo en 413.

## Hallazgos
### F-01 · NARANJA · cableado de `main.newHandler` de go-intake sin prueba de semántica
Esta es la clase F-02 de ronda 2: la semántica de las métricas de la que depende el dashboard. En ui-api esos mutantes mueren (`cmd/ui-api` prueba la cadena real). En go-intake sobreviven:
- `const serviceName = "go-intake"` cambiado a otro valor. Todas las series y los logs pierden `service="go-intake"`, que es lo que filtra el dashboard (`service=~"go-intake|ui-api"`). El dashboard quedaría vacío con la suite en verde.
- `Config{Service: ...}` con otro valor en `obs.Wrap`.
- `Route: nil`, que deja el webhook como `route="unmatched"`.
- `NewHTTPMetrics(reg, <otro>)`, que cambia la etiqueta `service` de `aqs_http_in_flight`. Esto sobrevive en ui-api también.

Se corrige con un test en `cmd/go-intake` que, sobre `newHandler`, aserte `service`, `route="/webhooks/github"` y `aqs_http_in_flight{service=...}` (y lo mismo para `in_flight` en ui-api).

### F-02 · AMARILLO · cableado de `main()` y niveles de log sin prueba
- `obs.ParseLevel(os.Getenv("LOG_LEVEL"))` cambiado a `ParseLevel("")` sobrevive en ambos servicios. Solo se prueba `ParseLevel`, no el uso del entorno.
- Quitar `slog.SetDefault(log)` sobrevive en ambos. Los `log.Printf` residuales saldrían como texto plano, rompiendo el contrato JSON.
- ui-api: cambiar `slog.NewLogLogger(..., LevelInfo)` del suscriptor, o el `LevelWarn` de `cfg.Logger`, a `LevelDebug` sobrevive. Con el nivel por defecto, «evento descartado» desaparecería.
- go-intake: bajar `WarnContext` de «webhook rechazado» a Info, bajar `ErrorContext` de la publicación a Warn, o bajar el `ErrorContext` del primer «no se pudo persistir» a Info, sobreviven. Los niveles de los logs de dominio no están asertados.

### F-03 · AMARILLO · mutantes de log que sobreviven
- Quitar `notification_id` del log «webhook aceptado» sobrevive. La tarea pide registrar ese identificador.
- Fijar `"status", 200` en el log de rechazo sobrevive.
- Añadir una serie extra `reason="other"` inicializada en 0 sobrevive.
- Loguear la cabecera `Cookie` en el log de acceso sobrevive en ambos servicios. Loguear `X-Hub-Signature-256` sobrevive en ui-api, donde la cabecera no tiene sentido.
- En go-intake, las marcas de PII por ruta solo van en el cuerpo, no en cabeceras. Sobreviven mutantes que loguean `X-Forwarded-For`, `Content-Type`, `User-Agent`, `RemoteAddr`, `Origin`, `Host`, el `X-Request-Id` crudo y el `traceparent` crudo de entrada. En ui-api, `X-Forwarded-For` y `Content-Type` sí mueren.

### F-04 · AMARILLO · semántica del dashboard
Solo se prueba que los nombres de métrica existan, que es lo que pide CA-8. Sobreviven `code=~"5.."` cambiado a `"4.."`, `0.95` a `0.50`, quitar `le` del `sum by`, `sum by (reason)` a `sum by (service)`, y cambiar el filtro `service=~` a otro valor. Solo mueren los mutantes que cambian el nombre de la métrica. Aceptable según el CA, pero es la misma clase de fallo silencioso que F-01.

### F-05 · AMARILLO · `Connection: close` fuera del 413
Poner `Connection: close` en todo `c >= 400` sobrevive en ambos. La prueba comprueba que el 413 cierra y que un 200 no, pero no un 400 ni un 404.

### F-06 · AMARILLO · CI de PR #35
Sin confirmar del todo, y HEAD ya es `cb2c74d`. Sobre `cb2c74d`:
- **En success:** `test` y `vuln` de go-intake y ui-api, `policies`, `contracts` (run de PR), `sbom` y `no-latest`.
- **Sin runner (cancelados a los ~15 min, sin ningún paso ejecutado):** `build` de go-intake, ui-api y go-governance, y `test` de go-identity. Por eso el run `ci` queda en `failure` global y `build` de U1-T06 no está confirmado.
- **Aparte:** el run de `contracts` por push (`validate`) también quedó cancelado.

## Supervivientes equivalentes o de bajo valor
- Mutantes de obs, ambos servicios:
  - `duration_ms` como entero en vez de float.
  - Timestamp RFC3339 sin fracción.
  - Redacción solo a nivel raíz, porque no se usan grupos.
  - `Route(r)` evaluado antes de servir.
  - `/metrics` servido sin `statusWriter`.
  - Estado 204 por defecto cuando no se escribe nada.
  - Registrar el método crudo en el log de acceso.
  - Contexto del chequeo de `readyz` desligado de la petición.
- ui-api: quitar `"method"` del log del panic; `verifierSet` siempre verdadero (invariante de arranque); loguear `line` en el descarte por «línea demasiado larga» (`line` va vacío en esa rama).
- go-intake: `ready-nosecret` es equivalente porque `NewHandler` ya exige secreto.

## Mutantes
Unos 182 propios, todos distintos de los del codificador, en copias con `contracts` y `deploy` enlazados. Un mutante no-op confirmó la base verde antes de contar nada. Moldes:
- obs, ambos servicios: 86.
- Dominio y cableado: 64.
- Dashboard: 6.
- Fuga en el log de acceso: 26.

Cuatro no compilaron y no cuentan. Los que sobreviven y no son equivalentes son los de F-01 a F-05.

## Alcance y pruebas
- **Alcance:** el diff contra el merge-base `dfff676` solo toca `services/{go-intake,ui-api}`, `deploy/flux/base/observability/{dashboard-u1.yaml,kustomization.yaml}`, la bitácora y `revisiones/`.
- **Pruebas existentes:** no se debilitó ninguna. `api_test.go` solo añade (36 líneas nuevas, 2 eliminadas, que son refactor de `newEnv`). `TestObsAcceptedPayloadLeaksNothing` se sustituyó por una versión más amplia. La prueba del panic se endureció con una marca de PII.
- **Secretos y PII:** no hay en la bitácora, los informes ni el diff; los correos son `falso.test`.

## Para pasar a VERDE
1. Pruebas de `newHandler` en `cmd/go-intake` (y `in_flight` en ambos) con `service`, `route` y `in_flight{service}` reales (F-01).
2. Pruebas de `LOG_LEVEL`, de `SetDefault` y de los niveles de los logs de dominio (F-02).
3. Aserciones de `notification_id` en el log de aceptación y de `Cookie` y cabeceras no sensibles con marcas por ruta en go-intake (F-03).

Cuando la CI de GitHub corra `build` y `test` de go-identity sin cancelarse, F-06 queda resuelto.

VEREDICTO: NO-VERDE
NARANJA|services/go-intake/cmd/go-intake/main.go (newHandler)|Cableado sin prueba de semántica: `serviceName`, `Wrap.Service`, `Route` y la etiqueta `service` de `aqs_http_in_flight` (también en ui-api) pueden cambiar con la suite en verde; el dashboard filtra por `service`
AMARILLO|main.go de ambos y handler.go de go-intake|LOG_LEVEL, `slog.SetDefault`, nivel de los puentes de ui-api y niveles de los logs de dominio sin prueba
AMARILLO|handler.go y obs.go|`notification_id` del log de aceptación, `Cookie` y cabeceras no sensibles con marcas por ruta en go-intake, y la serie `reason="other"` sobreviven
AMARILLO|dashboard-u1.yaml|Solo se prueban los nombres de métrica, no la semántica de las expresiones
AMARILLO|obs.go de ambos|`Connection: close` en todo 4xx sobrevive
AMARILLO|CI PR #35|`test` y `vuln` de go-intake y ui-api en success; `build` de go-intake y ui-api sin runner (cancelados), `ci` global en failure
INFORME: este mensaje (el revisor no escribió `revisiones/U1-T06/ronda-3.md`; el orquestador decide si lo vuelca)
