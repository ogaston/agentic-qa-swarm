# Ronda 1 — U1-T07

VEREDICTO: NO-VERDE

Un hallazgo NARANJA en pie (F-01): el circuito se abre por abortos del cliente con go-identity sano. Los 10 criterios de aceptación pasan en mi ejecución. El worktree quedó limpio (`git status --short` vacío) y el desborde de alcance es 0.

## Criterios de aceptación, verificados por mí (comandos de la tarea, frescos, `-count=1`)
| # | Criterio | Resultado |
|---|---|---|
| CA-1 | `--- PASS` / `FAIL` / skip con tag contract | `260`, `0`, `5`. Pasa. |
| CA-2 | e2e con go-identity real | `200 401 401 401`. Pasa. |
| CA-3 | `run.confirmed` | `201`, `1`, `ev.json valid` (ajv), `marta checkout,refund`, `run_id coincide`, session principal `marta`. Pasa. El token aparece 0 veces en el log y 0 veces en `/metrics`. |
| CA-4 | fail-closed | ocho `503`, `Retry-After` = 1, `aqs_identity_circuit_open 1`. Pasa. |
| CA-5 | republicación | `TestRepublishPendingSendsExactlyOnceEvenIfCalledTwice` y `TestPublishPendingKeepsReceiptWhenPublishFails` en PASS. Pasa. |
| CA-6 | fake acotado | cuatro `rc=1`. Con `UIAPI_OUTBOX_FILE` puesto, los motivos en stderr son el correcto (`prohibido con UIAPI_ENV=prod`; `IDENTITY_URL debe ser una URL http(s)`). Pasa. |
| CA-7 | contrato contra go-identity real | 5 PASS, ningún FAIL/SKIP. Pasa. |
| CA-8 | workflow | `actionlint rc=0` (con `:z`), `0`, `0`, sin `ACCION NUEVA`. Pasa. |
| CA-9 | imagen y dependencias | `0`, `65532:65532` (el `docker build` literal funcionó), `0`. Pasa. |
| CA-10 | higiene y alcance | `ok`, `0`, `0`. Pasa. `go mod tidy -diff` sin diferencias. |

Las pruebas unitarias cubren los casos pedidos: 200 válido, 401, cuerpo inválido, rol desconocido, timeout, circuito abierto/cerrado/medio-abierto, fake bloqueado en prod, `ErrUnavailable` a 503 y re-publicador sin duplicados. La evidencia de la bitácora coincide con la mía.

Sobre los puntos que pidió el codificador:
- **Contratos con go-identity propio (TTL 1 s):** aceptable. Está documentado en la cabecera del archivo y `IDENTITY_URL` solo hace de compuerta de omisión.
- **`UIAPI_OUTBOX_FILE` con default en el Dockerfile:** aceptable.
- **`TestAuthIsMandatory`:** su modificación en sitio es legítima por el cambio de comportamiento del fake.
- **`go.mod` y `go.sum`:** consistentes con `go mod tidy`.
- **Semántica del circuito:** ver F-01.

## Hallazgos

### F-01 · NARANJA · `services/ui-api/internal/auth/identity.go` (`call`, `Verify`, `settle`) · Un aborto del cliente cuenta como fallo de identidad y abre el circuito con identidad sana
`call` deriva el contexto de la petición entrante. Si el cliente cancela, `client.Do` devuelve error. `Verify` lo trata como fallo de transporte y llama `settle(probe, false)`. Cinco abortos seguidos abren el circuito 10 s y todos los usuarios reciben `503`, aunque go-identity esté perfectamente sano.

Lo reproduje de caja negra, con un go-identity falso lento (python) en el puerto 18210 y `ui-api` real en el 18211:
```
good before: 200
(5x curl -m 0.3 con token "slow", abortado por el cliente)
good after 5 client aborts: 503
aqs_identity_calls_total{result="error"} 5
aqs_identity_circuit_open 1
```
El token usado en la última petición era válido y la identidad nunca falló. El vector es explotable sin credenciales: basta enviar tokens basura y cerrar la conexión mientras identidad está lenta o cargada. Los abortos de clientes legítimos con identidad lenta producen el mismo efecto.

La tarea define el circuito por «fallos de transporte consecutivos» de identidad. Un aborto del llamante no es un fallo de identidad. Ninguna prueba cubre este caso.

Qué corregir:
- En `Verify`/`call`, si `ctx.Err() != nil` en el contexto padre (no el timeout propio), devolver `ErrUnavailable` sin llamar a `settle(...,false)`. Liberar `probing` si era sonda, sin reabrir el circuito ni contar el fallo.
- Añadir una prueba que cancele el contexto padre ≥5 veces y verifique que el circuito sigue cerrado.

### F-02 · AMARILLO · `cmd/ui-api/main.go` (`verifierFromEnv`) · La valla del fake solo reconoce el literal `prod`
`UIAPI_ENV=production` o `PROD` no bloquea el fake. Sigue haciendo falta `UIAPI_ALLOW_FAKE_AUTH=true`, así que no bloquea. Conviene comparar sin distinguir mayúsculas, o listar los valores aceptados.

### F-03 · AMARILLO · `inbox/publish.go` (`Outbox.Publish`) · Cada publicación relee el outbox completo para deduplicar
Es O(n) por evento y está bajo mutex. Es aceptable como transporte de transición (C-45), pero crecerá linealmente.

### F-04 · AMARILLO · `inbox/store.go` y `published.go` · `PublishPendingCount` es `len(receipt) - len(pubDone)`
Un `published.jsonl` con ids huérfanos, sin recibo, podría dar un gauge negativo. Es un caso de borde improbable.

### F-05 · AMARILLO · Semántica del circuito (decisión del codificador, sin cambio de severidad)
Cuenta como fallo el 5xx, el cuerpo inválido y cualquier código distinto de 200/401. Es coherente con «denegar». Los 200 y 401 reinician la cuenta, y no hay redirecciones (probado en unitaria). No es defecto, pero queda anotado porque la tarea decía «fallos de transporte» a secas.

## Tareas candidatas (defectos reales fuera de alcance)
- `deploy/flux/base/control-plane.yaml` no define `UIAPI_AUTH`, `IDENTITY_URL` ni `UIAPI_OUTBOX_FILE` para `ui-api`, y hoy no podría arrancar. Hay que cablearlo con la integración de despliegue (`deploy/**` está fuera de alcance aquí). No verifiqué el estado previo del manifiesto, solo que ahora no los define.
- Que U2 consuma `run.confirmed`, y el transporte real que reemplace al outbox JSONL: ya están registrados como C-45 y U2.

## Rutas de transcripciones largas
- Reproducción de F-01: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/e739f6e8-0994-4df5-8a11-324f50957ce4/scratchpad/slow.py`. `up.sh` en el mismo directorio es la copia literal del bloque de la tarea.

VEREDICTO: NO-VERDE
NARANJA|services/ui-api/internal/auth/identity.go (call/Verify/settle)|Un aborto del cliente cuenta como fallo de identidad y abre el circuito con identidad sana
