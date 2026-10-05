# Ronda 2 — U1-T02

VEREDICTO: VERDE

Ronda 2 (retrabajo por decisión del humano: `ping` responde 200). Re-corrí CA-1 a CA-8 con SHA `2374bea` y todos pasan. No queda ningún ROJO ni NARANJA.

## Criterios de aceptación, corridos por el revisor
| # | Resultado |
|---|---|
| 1 | 29 PASS y 0 FAIL. La tabla `TestWebhook` tiene 13 subcasos y cubre los 9 exigidos; `TestWebhookPing` añade 2 subpruebas. |
| 2 | `202`, 1 línea de evento, `ev.json valid`, estado `pending`. |
| 3 | `401`, 0 archivos en el directorio de datos, `sin eventos`. |
| 4 | PASS en `TestWebhookIdempotentDelivery`, `TestWebhookRestartKeepsDeliveryIndex` y `TestStoreRestartIgnoresBrokenLine`. |
| 5 | `0` y `0`. |
| 6 | `rc=1` y `0`. |
| 7 | La imagen construye con el contexto temporal que añade el ca-bundle. Corre con UID `65532` y `Config.User` es `65532:65532`. Ningún `FROM` sin tag o con `latest`; `list-services` da `1` y `detect-lang` da `go`. |
| 8 | `go vet` y `gofmt` dan `ok`, y el worktree está limpio. El diff fuera de `services/go-intake/` y la bitácora son 3 archivos del loop (ver F-01). |

## Ping
- El cambio de la ronda es mínimo: 6 líneas en `handler.go` y los cambios de `handler_test.go`. El Dockerfile no cambió.
- El bloque `ping` va después de la verificación HMAC y del `Content-Type`.
- Con el binario real: ping firmado → `200 {"status":"pong"}`; firma `sha256=00` → `401`; sin firma → `401`; después de los pings no se creó el archivo de eventos ni hay archivos en el directorio de datos.
- `issues` y la acción `closed` siguen en `400 unsupported_event`, cubiertos en `TestWebhook`.
- `TestWebhookPing` es una prueba real: 4 mutaciones en una copia del servicio (ping vuelve a `400`; se desactiva la rama de ping; ping persiste y publica; ping se mueve antes de la verificación de firma) fallan todas en `TestWebhookPing`.

## Docker
El build literal en el daemon del revisor falló en un paso distinto al que cita el codificador (un `RUN` sin red, por `--bridge=none`), así que el error de CA del proxy en `go mod download` no se reprodujo. El build con contexto temporal (`COPY ca.crt` + `ENV SSL_CERT_FILE`) sí construye y cumple CA-7.

## Hallazgos
- **F-01 · AMARILLO · CA-8 literal:** da 3 archivos fuera de alcance: `tareas/U1-T02-webhook-intake.md`, `tareas/candidatas.md` y `revisiones/U1-T02/ronda-1.md`. Son archivos del loop y del humano, no del codificador; la nota de la tarea excluye `revisiones/`.
- **F-02 · AMARILLO · `handler.go`:** un ping con firma válida y `Content-Type` correcto responde `200` aunque falte `X-GitHub-Delivery`. Inocuo y coherente con la especificación.

## Tareas candidatas
Las que ve el revisor (C-76, C-77, C-78) ya están en `tareas/candidatas.md`.

VEREDICTO: VERDE
AMARILLO|CA-8 literal|El diff contra la base incluye tareas/, candidatas.md y revisiones/ (archivos del loop, no del codificador)
INFORME: revisiones/U1-T02/ronda-2.md
