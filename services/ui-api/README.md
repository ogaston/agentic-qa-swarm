# ui-api (U1-T04)

API JSON del inbox. Sin UI HTML ni Kubernetes.

- `GET /notifications[?state=pending|confirmed|rejected]` -> `[]Notification`, mas reciente primero.
- `POST /notifications/{id}/confirm` con `{"flows":[...]}` -> `201 ConfirmationReceipt`; `404`, `409`, `400`, `413`, `415`, `401`.
- Errores: `{"code","message"}`. Todas las respuestas llevan los cinco security headers.

`run_id` = `run-` + 32 hex (128 bits de `crypto/rand`). `confirmed_by` es el `Principal.ID` del token.

## Configuracion

| Variable | Descripcion |
|---|---|
| `UIAPI_AUTH` | **Obligatoria.** Solo `fake` (dev/prueba); sin ella no arranca. La real es U1-T07. |
| `UIAPI_FAKE_TOKENS` | Con `fake`: `token=id:rol,...` (roles `user`/`admin`). |
| `UIAPI_EVENTS_FILE` | Outbox JSONL de go-intake (transicion, C-45). Tail desde el inicio, idempotente por `event_id`; evento invalido = descartado y contado. |
| `UIAPI_DATA_DIR` | Contiene `confirmations.jsonl` (solo-agregar, fsync por recibo). Una linea truncada no impide arrancar. |
| `UIAPI_ALLOWED_ORIGINS` | Origenes CORS separados por coma; vacia = ninguno; `*` se rechaza. |
| `UIAPI_RATE_LIMIT_RPS` / `_BURST` | Token bucket por IP (10 / 20). `429` + `Retry-After`. |
| `UIAPI_TRUST_PROXY` | `true` usa `X-Forwarded-For` (ultimo salto). |
| `LISTEN_ADDR` | Por defecto `:8080`. |

Publicar `run.confirmed` y la verificacion real de tokens: U1-T07.
