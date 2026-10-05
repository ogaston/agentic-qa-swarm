# Ronda 2 — U1-T04

VEREDICTO: NO-VERDE

Queda un NARANJA en F-01: las claves duplicadas con un `null` final siguen colándose. F-02 y F-03 resueltos; F-04 y F-05 resueltos en lo pedido, con huecos menores de cobertura (AMARILLO). Worktree limpio; sin procesos del revisor vivos.

## Criterios de aceptación, verificados por el revisor (`GOTOOLCHAIN=go1.26.8`, `-count=1`)
| # | Resultado |
|---|---|
| 1 | 4 paquetes ok, 101 PASS, 0 FAIL con `-race` |
| 2 | `n-1 pending` (binario real; no se repitió la validación ajv) |
| 3 | `401 401 401 401` y `401` |
| 4 | `201 409 404`, `u1`, `1`, `1` |
| 5 | `5` y `3` |
| 6 | `https://app.example`, `0`, `0` |
| 7 | 6×`200` y 24×`429`; `Retry-After: 1` |
| 8 | `0`, `1`, `0`; `git diff 33fcd46..HEAD -- services/ui-api/Dockerfile` vacío (no se repitió `docker build`) |
| 9 | `ok`, `0`; fuera de alcance solo `revisiones/U1-T04/*` (arbitrado) |

CI del PR #33: `vuln (services/ui-api)` en success solo para `83b3241`; no hay run de `ci` para `0f8f804` ni para HEAD `fc72c01` (solo `contracts` y `policies`). `go.mod` y `go.sum` no cambian entre `83b3241` y HEAD, así que el resultado de `vuln` sigue valiendo; queda anotado como no confirmado sobre el commit de código de la ronda.

## Hallazgos
### F-01 · NARANJA · `services/ui-api/inbox/event.go:62-84` · claves duplicadas con `null` final: el parser acepta lo que el esquema rechaza
`exactKeys` resuelve bien la capitalización, las claves ausentes, las extra y los tipos. Pero `encoding/json` ignora un `null` sobre un campo `string` o `int` y conserva el valor anterior; el esquema (`jsonschema/v6`) usa el último valor y ve `null`. Casos: `"repo":"acme/shop","repo":null`, `"type":"notify.created","type":null`, `"version":1,"version":null`, `"data":{…},"data":{claves:null}`, `"artifact":{…},"artifact":{"kind":null,"ref":null}`: 22 escalares (11 claves × 2 formatos) y 2 de objeto, todos `schema=false parser=true`. Con el binario real, un evento en el outbox con `"repo":"acme/shop","repo":null` aparece en `GET /notifications` (id `dupnull`) sin log de descarte. `TestParseAgreesWithSchema` no tiene casos de claves duplicadas. Arreglo posible: rechazar cualquier clave duplicada con un tokenizador, o validar contra el `map` ya resuelto.
Diferencial del revisor: 21.937 casos × 2 formatos (capitalización de todas las claves, ausentes, extra, todos los tipos, `version` 1/1.0/1e0/"1", `sha` de 39/40/41 y mayúsculas, ocho formatos de `occurred_at`, `kind` desconocido, `ref` vacío, 20.000 mutaciones aleatorias): 530 casos donde el parser es más estricto que el esquema (divergencias declaradas y duplicados con primer valor inválido y último válido: inocuo). Los únicos casos en que el parser acepta lo que el esquema rechaza son los de F-01.
Resto de F-01 OK: `EVENT_ID`, `Type` y `DATA` no aparecen en `GET /notifications` y dejan log de descarte; 5 webhooks firmados con el `go-intake` real → 202, 5 líneas en el outbox y 5 notificaciones `pending`.

### F-02 · resuelto
Dos cabeceras `Authorization` → 401 en los dos órdenes, también con ambas válidas y distinta capitalización, en GET y POST; una sola → 200. `state` repetido → 400 (`pending&pending`, `bogus&pending`); `state=pending&State=confirmed` → 200 (no cuenta como repetido); `state=` → 400. El rechazo de doble cabecera ocurre después del rate limit (5 dobles → `401 401 401 429 429`), coherente con el orden del handler. Sin token en los logs.

### F-03 · resuelto
`TestRetryAfterExactAndDecreasing` fija `4,3,2` con reloj inyectable a 0.25 rps; los mutantes `wait*0+1` y `wait*0+2` fallan. Binario real: 429 con `Retry-After: 1` a 5 rps.

### F-04 · resuelto
`TestBodyLimitLiteral64KiB` fija 65536 → 404 y 65537 → 413; fallan los mutantes `body-6400KiB`, `body-65537`, `body-65535`; `offset-no-reset` falla `TestFileSubscriberResetsOffsetOnTruncate`.

### F-05 · resuelto en lo pedido
Línea > 1 MiB descartada con log y el subscriber sigue (50 MB con salto: las dos siguientes visibles). 50 MB sin salto al final: no cuelga el proceso (HTTP 200) y pico de memoria 14,9 MB (`VmHWM`); al llegar el salto, esa línea se descarta y la posterior se lee. El mutante sin tope falla `TestFileSubscriberSkipsOversizedLine`.

### F-06 · AMARILLO · `inbox/subscriber_test.go:77` y `internal/httpapi/ratelimit.go` · mutantes de borde que sobreviven
`MaxLineBytes` con +1, −1 o 2 MiB sobreviven (el test usa `MaxLineBytes+10`); quitar el log de descarte sobrevive (el test no asigna `Logger`); `math.Floor` en lugar de `math.Ceil` en `Retry-After` sobrevive. El tope cuenta el `\n`: una línea con 1.048.575 bytes de contenido o menos se acepta y con 1.048.576 exactos se descarta; defendible, no documentado ni testeado.

### F-07 · AMARILLO · `inbox/subscriber.go` (`Drain`)
Con una línea parcial pendiente de 50 MB, cada pasada de `Drain` la relee desde el offset: 14 ticks de CPU en 5 s (despreciable); sin consumo de memoria.

## Sondas de regresión de la ronda 1
25 confirmaciones concurrentes → un solo `201` y 24 `409`; `X-Forwarded-For` ignorado sin `TRUST_PROXY` (`200 200 429 429`) y con `TRUST_PROXY=true` (`200 200 200 200`); las 5 cabeceras de seguridad en 200, 401, 404, 405, 415, 400, 400 de `state` inválido y 204 de OPTIONS.

## Tareas candidatas
- Rechazar claves JSON duplicadas de forma explícita en el outbox (arreglo de F-01; aplica a cualquier otro parser de eventos del repo).
- Documentar si el tope de línea incluye el `\n`.

VEREDICTO: NO-VERDE
NARANJA|services/ui-api/inbox/event.go:62-84 (F-01)|Claves duplicadas con null final: el parser acepta lo que el esquema rechaza (aparece en GET /notifications)
INFORME: revisiones/U1-T04/ronda-2.md
