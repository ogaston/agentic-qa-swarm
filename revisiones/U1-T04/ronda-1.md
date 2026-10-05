# Ronda 1 — U1-T04

VEREDICTO: NO-VERDE

Los 9 criterios pasan con la ejecución propia del revisor (`GOTOOLCHAIN=go1.26.8`, `-count=1`). Un NARANJA bloquea (F-01). Worktree limpio; procesos del revisor detenidos.

## Criterios de aceptación, verificados por el revisor
| # | Resultado |
|---|---|
| 1 | 69 PASS / 0 FAIL; `-race` ok en 4 paquetes |
| 2 | `n-1 pending` y `n0.json valid` (con `UIAPI_AUTH=fake` en `start()`) |
| 3 | `401 401 401 401` y `401` |
| 4 | `201 409 404`, `u1`, `1`, `1` (con reinicio) |
| 5 | `5` y `3` |
| 6 | `https://app.example`, `0`, `0` |
| 7 | `6×200`, `24×429`, `Retry-After: 1` |
| 8 | `0`, `65532:65532`, `0`, `1` |
| 9 | `ok`, `0`, `0` |

Arbitraje confirmado: (1) con el `start()` literal, sin `UIAPI_AUTH`, el binario sale con `rc=1` y «UIAPI_AUTH es obligatorio» (defecto de especificación). (2) el `docker build` literal falla en `go mod download` por la CA del proxy; con un contexto temporal (`COPY ca.crt` + `ENV SSL_CERT_FILE`) el diff contra el Dockerfile real son esas dos líneas, la imagen corre como `65532`, la CA no queda en la imagen final y `UIAPI_AUTH` no está definido en la imagen. (3) `Retry-After` dio `1`; ver F-03. (4) `go 1.26.8`; CI del PR #33 sobre `83b3241`: `ci` (incluido `vuln (services/ui-api)`), `contracts` y `policies` en success.

## Sondas adversariales (binario real)
- Autenticación: `Bearer ` vacío, doble espacio inicial, espacio interno, token en query o cookie, lista `Bearer a,Bearer b` y `Basic` dan 401; `bearer` en minúscula da 200 (RFC 7235). Mutantes `failopen`, `verifier-err-ignored`, `no-recover`, `panic-logs-value` y `token-in-401-log` muertos por las pruebas; el token no aparece en el log en vivo.
- Confirmación: `confirmed_by` sale del token; 25 POST concurrentes → `1×201` y `24×409` con una sola línea en disco; línea final truncada no impide arrancar; `flows` `[]`, `[""]`, `[" "]`, `[null]`, `{}`, `"x"`, vacío o texto tras el objeto → 400; Content-Type `text/plain`, vacío y `application/jsonx` → 415; cuerpo de 65536 bytes → 404 y de 65537 → 413; ids con `..`, `%2e%2e`, `%2F`, `%00`, vacío o de 100 KB → 404 o 431, sin path traversal.
- Inbox: `go-intake` real con su secreto y tres webhooks firmados más un reenvío: `ui-api` leyó el outbox real, 3 `pending`, más reciente primero y dedup correcto; línea rota y evento inválido real descartados con log; duplicado añadido en vivo no crea entradas; línea parcial sin salto no se consume; `state=bogus`, `state=` y `state=PENDING` → 400.
- Rate limiting: con `X-Forwarded-For` variable y sin `UIAPI_TRUST_PROXY`, `5×200` y `15×429`; con `UIAPI_TRUST_PROXY=true` una IP agota su ráfaga y otra no; se usa el último salto del XFF; el refill tras 1,3 s da 200.
- Cabeceras de seguridad: los cinco headers en 200, 201, 204, 400, 401, 404, 405, 409, 413, 415 y 429 (el 500 por panic lo cubre `TestVerifierPanicIs500NoLeak`).
- CORS: solo el origen exacto; orígenes casi iguales y `null` sin cabeceras; sin `*` ni credenciales.
- Contrato: sin `k8s.io`; la notificación valida con ajv; códigos de estado del OpenAPI.
- Mutantes que mueren: `confirmed_by` del cuerpo, CORS `*`, header de seguridad ausente, filtro `state`, orden, doble confirmación, truncado, XFF siempre confiable, rate limit global, Content-Type, `flows` vacío, texto tras el objeto, `UIAPI_AUTH` por defecto, línea parcial. Sobreviven los de F-03, F-04 y F-05.

## Hallazgos
### F-01 · NARANJA · `services/ui-api/inbox/event.go:42-83` · El validador acepta eventos que el esquema `notify.created` rechaza
`ParseNotifyCreated` usa `encoding/json` con `DisallowUnknownFields`, que casa los nombres de campo sin distinguir mayúsculas. El esquema real tiene `additionalProperties: false` y los campos son `required`. Diferencial contra `contracts/events/notify.created.schema.json` (jsonschema/v6 con `AssertFormat`): `EVENT_ID` en vez de `event_id`, `Type` y `DATA` → schema=false, parser=true (DIVERGE). Un evento inválido contra el contrato crea una notificación `pending` visible en el inbox, aunque la tarea promete que uno inválido se descarta. `TestParseAgreesWithSchema` pasa porque no incluye mutaciones de capitalización de claves. Divergencias en sentido inverso (más estrictas que el esquema, no abren nada): `"version": 1.0` o `1e0`; fecha con `t` o `z` en minúscula.

### F-02 · AMARILLO · `internal/httpapi/api.go:~125` y `auth.go` · doble cabecera `Authorization` y `state` repetido: gana la primera
`Authorization: Bearer nope` + `Authorization: Bearer tok-user` → 401; con el orden inverso → 200; `?state=pending&state=bogus` → 200. Determinista, sin bypass, pero un proxy delante podría validar un valor distinto del que usa `ui-api`. Lo seguro es rechazar con 401/400 si hay más de uno.

### F-03 · AMARILLO · `internal/httpapi/api_test.go:249` · el valor de `Retry-After` no se verifica
El mutante que fija `Retry-After` en la constante `1` sobrevive: la prueba solo exige que no esté vacío, aunque tiene reloj inyectable.

### F-04 · AMARILLO · `internal/httpapi/api_test.go` · el límite de 64 KiB no está fijado por literal
Cambiar `MaxBodyBytes` a 6400 KiB sobrevive (la prueba usa la constante). El borde se verificó en vivo (65536 → 404, 65537 → 413). Tampoco se prueba que el offset del subscriber se reinicie al truncarse el archivo (`offset-no-reset` sobrevive).

### F-05 · AMARILLO · `inbox/subscriber.go` · línea sin límite de tamaño
`ReadBytes('\n')` no limita el tamaño de línea del outbox. El archivo es de confianza (lo escribe `go-intake`): riesgo bajo.

## Tareas candidatas (fuera de alcance)
- Validar contra el esquema real o con claves sensibles a mayúsculas hasta que C-45 decida el transporte.
- Corregir `start()` en `tareas/U1-T04-inbox-ui-api.md` añadiendo `UIAPI_AUTH=fake` (defecto de especificación arbitrado).

VEREDICTO: NO-VERDE
NARANJA|services/ui-api/inbox/event.go:42-83|El validador de notify.created acepta claves en otra capitalización que el esquema real rechaza (EVENT_ID, Type, DATA) y crea notificaciones pending
INFORME: revisiones/U1-T04/ronda-1.md
