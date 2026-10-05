# go-intake

Recibe `POST /webhooks/github` (U1-T02/T03): verifica la firma HMAC, clasifica el evento, persiste la notificación y publica `notify.created` en el outbox JSONL. No hay otras rutas de negocio.

## Configuración

| Variable | Descripción |
|---|---|
| `GITHUB_WEBHOOK_SECRET` | **Obligatoria.** Secreto HMAC del webhook. |
| `INTAKE_DATA_DIR` | **Obligatoria.** Contiene `notifications.jsonl`. |
| `INTAKE_EVENTS_FILE` | **Obligatoria.** Outbox JSONL de `notify.created`. |
| `ARTIFACT_REGISTRY` | Registro de imágenes para eventos de tag. |
| `LISTEN_ADDR` | Por defecto `:8080`. |
| `LOG_LEVEL` | `debug\|info\|warn\|error` (por defecto `info`). |

## Observabilidad (U1-T06)

Contrato de `docs/observability.md`; mismo patrón que `go-identity` (`internal/obs`).

- **Log JSON** (una línea por evento, a stdout): `timestamp` (UTC), `level` (minúsculas), `message`, `request_id`, `trace_id`, `service`; el log de acceso añade `route`, `method`, `status`, `duration_ms`; los del webhook, `delivery_id`, `code`, `notification_id`, `repo`, `sha`. Nunca se registran cuerpo, `X-Hub-Signature-256`, `Authorization`, el secreto ni el contenido de los payloads.
- **Identificadores:** `X-Request-Id` se acepta solo si cumple `^[A-Za-z0-9._-]{8,64}$` (si no, se genera) y se devuelve siempre. `trace_id` sale de `traceparent` W3C v00 válido (si no, se genera), se devuelve en `traceparent` y es el `trace_id` del evento `notify.created`. Sin exportador OTLP (C-48).
- **Sondas** (sin token): `GET /healthz` (shallow, siempre 200), `GET /readyz` (deep; 503 con `{"status":"unavailable","checks":{...}}` si falla `data_dir` —directorio de datos escribible—, `outbox` —el archivo de eventos se puede abrir para agregar; si aún no existe, su directorio debe ser escribible— o `webhook_secret`), `GET /metrics` (Prometheus). Las sondas se registran en el log a nivel `info`; sube `LOG_LEVEL` a `warn` para silenciarlas.
- **Excepción:** las sondas las sirve `obs.Wrap` antes de la aplicación: no pasan por la verificación de firma ni llevan cabeceras de aplicación (solo `Cache-Control: no-store` los JSON de salud). No exponen datos de negocio y no están en el OpenAPI.
- **Métricas:** `aqs_http_requests_total{service,route,method,code}`, `aqs_http_request_duration_seconds{service,route,method}`, `aqs_http_in_flight{service}` (`route` es el patrón: `/webhooks/github` o `unmatched`), `aqs_intake_notifications_created_total`, `aqs_intake_webhook_rejected_total{reason}` (`invalid_signature|unsupported_event|unresolvable_artifact|bad_request|too_large`; los 503 internos `store_unavailable`, `artifact_unavailable` y `publish_failed` no son rechazos del remitente: se ven como `code=~"5.."`; `publish_failed` además en `aqs_intake_publish_failures_total`). Los contadores arrancan en 0.
- El chequeo `webhook_secret` es un invariante de arranque (sin secreto el servicio no arranca): en el binario no puede fallar; se prueba por inyección.
- Dependencia nueva: `github.com/prometheus/client_golang` v1.24.1 (la misma de `go-identity`/`go-governance`).
