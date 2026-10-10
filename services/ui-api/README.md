# ui-api (U1-T04)

API JSON del inbox. Sin UI HTML ni Kubernetes.

- `GET /notifications[?state=pending|confirmed|rejected]` -> `[]Notification`, mas reciente primero.
- `POST /notifications/{id}/confirm` con `{"flows":[...]}` -> `201 ConfirmationReceipt`; `404`, `409`, `400`, `413`, `415`, `401`.
- `GET /warm` -> `200 WarmState` (estado del warm, solo lectura). Lee `GET {WARM_URL}/warm` de go-warm-manager con el token de **servicio** (`UIAPI_WARM_TOKEN_FILE`); el token de la persona nunca sale hacia el warm. Respuesta inválida contra el esquema, transporte, plazo 2 s, 5xx, `401` del warm o circuito abierto -> `503 warm_unavailable` + `Retry-After`. Sin `WARM_URL` -> `503 warm_unavailable` (el resto de rutas sigue sirviendo).
- `GET /confirmations[?limit=N]` -> `200 []ConfirmationReceipt`, más reciente primero; `limit` 1–100 (por defecto 20; fuera de rango -> `400`). Un `user` ve solo sus recibos (`confirmed_by` = su `Principal.ID`); `admin` ve todos. El rol sale del verificador de tokens, nunca de cabeceras ni parámetros.
- Errores: `{"code","message"}`. Todas las respuestas llevan los cinco security headers.

`run_id` = `run-` + 32 hex (128 bits de `crypto/rand`). `confirmed_by` es el `Principal.ID` del token.

## Configuracion

| Variable | Descripcion |
|---|---|
| `UIAPI_AUTH` | **Obligatoria.** `identity` (go-identity real, valida **cada** petición, sin caché) o `fake` (dev/prueba); sin ella no arranca. |
| `IDENTITY_URL` | Con `identity`: URL `http(s)` de go-identity (`GET /auth/session`). Obligatoria. Timeout 2 s; 5 fallos de transporte seguidos abren el circuito 10 s. Identidad caída/lenta/inválida: `503 identity_unavailable` + `Retry-After` (nunca modo abierto). |
| `UIAPI_ALLOW_FAKE_AUTH` / `UIAPI_ENV` | `fake` exige `UIAPI_ALLOW_FAKE_AUTH=true` y no arranca con `UIAPI_ENV=prod`. |
| `UIAPI_OUTBOX_FILE` | **Obligatoria.** Outbox JSONL donde se publica `run.confirmed` al confirmar (transporte de transición, C-45). Si publicar falla, el `201` se mantiene, el recibo queda pendiente (`aqs_inbox_publish_pending`) y se reintenta al arrancar y cada 5 s; `event_id` es UUIDv5 de `run_id`, sin duplicados. |
| `UIAPI_FAKE_TOKENS` | Con `fake`: `token=id:rol,...` (roles `user`/`admin`). |
| `WARM_URL` | Opcional. URL `http(s)` de go-warm-manager para `GET /warm`. Sin ella, `/warm` responde `503 warm_unavailable`. Con ella es obligatoria `UIAPI_WARM_TOKEN_FILE` (si falta, no arranca). |
| `UIAPI_WARM_TOKEN_FILE` | Archivo con el token de servicio del warm-manager. Se lee en cada llamada; nunca se pasa el valor por variable de entorno ni se registra. |
| `UIAPI_EVENTS_FILE` | Outbox JSONL de go-intake (transicion, C-45). Tail desde el inicio, idempotente por `event_id`; evento invalido = descartado y contado. |
| `UIAPI_DATA_DIR` | Contiene `confirmations.jsonl` (solo-agregar, fsync por recibo). Una linea truncada no impide arrancar. |
| `UIAPI_ALLOWED_ORIGINS` | Origenes CORS separados por coma; vacia = ninguno; `*` se rechaza. |
| `UIAPI_RATE_LIMIT_RPS` / `_BURST` | Token bucket por IP (10 / 20). `429` + `Retry-After`. |
| `UIAPI_TRUST_PROXY` | `true` usa `X-Forwarded-For` (ultimo salto). |
| `LISTEN_ADDR` | Por defecto `:8080`. |
| `LOG_LEVEL` | `debug\|info\|warn\|error` (por defecto `info`). |

Publicar `run.confirmed` y la verificacion real de tokens: U1-T07.

## Observabilidad (U1-T06)

Contrato de `docs/observability.md`; mismo patrón que `go-identity` (`internal/obs`).

- **Log JSON** (una línea por evento, a stdout): `timestamp` (UTC), `level` (minúsculas), `message`, `request_id`, `trace_id`, `service`; el log de acceso añade `route`, `method`, `status`, `duration_ms`. Nunca se registran cuerpo, `Authorization` ni tokens. Los errores internos de `httpapi` emitidos dentro de una petición (500 al confirmar, panic) salen por `slog` con su `request_id` y `trace_id`; las líneas del suscriptor y de arranque no pertenecen a una petición y llevan ambos vacíos. El acceso de una respuesta 5xx sale en `warn`; el resto, en `info`.
- **Identificadores:** `X-Request-Id` se acepta solo si cumple `^[A-Za-z0-9._-]{8,64}$` (si no, se genera) y se devuelve siempre. `trace_id` sale de `traceparent` W3C v00 válido (si no, se genera) y se devuelve en `traceparent`. Sin exportador OTLP (C-48).
- **Sondas** (sin token): `GET /healthz` (shallow, siempre 200), `GET /readyz` (deep; 503 con `{"status":"unavailable","checks":{...}}` si falla `data_dir` —directorio de datos escribible—, `events_file` —archivo de eventos legible; si aún no existe basta con que exista su directorio— o `token_verifier`), `GET /metrics` (Prometheus). Las sondas se registran en el log a nivel `info`; sube `LOG_LEVEL` a `warn` para silenciarlas.
- **Excepción documentada (C-73):** `/healthz`, `/readyz` y `/metrics` se sirven por fuera del limitador por IP, del token y de los security headers de la API (`obs.Wrap` envuelve a `httpapi`). Por eso no llevan CSP/HSTS/etc. (solo `Cache-Control: no-store` los JSON de salud) ni gastan cupo del rate limit. No exponen datos de negocio y no están en el OpenAPI.
- **Métricas:** `aqs_http_requests_total{service,route,method,code}`, `aqs_http_request_duration_seconds{service,route,method}`, `aqs_http_in_flight{service}` (`route` es el patrón: `/notifications`, `/notifications/{id}/confirm` o `unmatched`), `aqs_inbox_confirmations_total` (solo cuenta el `201`) y `aqs_inbox_notifications{state}` (`pending|confirmed|rejected`, siempre las tres series). Los contadores arrancan en 0.
- El chequeo `token_verifier` es un invariante de arranque (`httpapi.New` rechaza un verificador nulo): en el binario no puede fallar; se prueba por inyección.
