# Ronda 1 — U8-T02

VEREDICTO: NO-VERDE

Worktree `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U8-T02`, HEAD 857f819. Lo único que bloquea es F-01 (NARANJA). Los cinco criterios pasan con mi propia ejecución. No tengo `Write`, así que el informe va en este mensaje y el orquestador debe guardarlo en `revisiones/U8-T02/ronda-1.md`.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Pruebas existentes y nuevas en verde | `for s in go-intake ui-api go-run-controller; do (cd services/$s && GOTOOLCHAIN=auto go test -count=1 ./...); done` | pasa, `rc=0` en los tres. Con `-v` no hay ningún `SKIP`. Las pruebas NATS corren con un `nats-server` embebido y tardan entre 0,03 y 3,7 s. |
| 2 | Recorrido por NATS con servicios reales | `bash scripts/test/u8-nats-local.sh` | pasa: `OK webhook-firmado-202`, `OK inbox-recibe-notificacion`, `OK confirm-201`, `OK controlador-recibe-run-confirmed`, `OK sin-duplicados`, `rc=0` (5,9 s). `podman ps -a` queda sin contenedor `aqs-u8`. |
| 2 (arbitraje) | Contrato dice 201 | `grep` de `operationId: confirmRun` en `contracts/openapi/control-plane.yaml` | confirmado: `confirmRun` responde `'201'`. Acepto `confirm-201`. |
| 3 | Al menos una vez tras reinicio | `bash scripts/test/u8-nats-local.sh --redelivery` | pasa (`OK reentrega-tras-reinicio`, `rc=0`). Ver F-01: la prueba no distingue el mecanismo. |
| 4 | Manifiestos y políticas | `kubectl kustomize deploy/flux/kind \| grep -c -E 'name: EVENTS_TRANSPORT'; bash scripts/ci/policies.sh 2>&1 \| grep -c '^FALLA'` | `3` y `0`, como se esperaba. |
| 5 | Alcance | `git status --short \| wc -l` y el `grep -v` del criterio sobre `git diff --name-only $b..HEAD` | `0` y `0`, medido en HEAD 857f819 y no solo en e06ba4a. |

Otras comprobaciones mías:
- `git fetch` + `git merge-tree --write-tree HEAD origin/main`: `rc=0`. `git log HEAD..origin/main` está vacío, así que origin/main no ha avanzado.
- `go mod tidy -diff` en los tres servicios: `rc=0`.
- `go vet ./...` en los tres servicios: sin salida (limpio).
- Dependencias de producción: `go list -deps ./cmd/...` no incluye `nats-server` en ningún servicio. Solo lo importan los `_test.go`.
- Worktree limpio tras mis ejecuciones.

## Comportamiento comprobado leyendo estado de vuelta (sonda propia, no está en el repo)
Copié el script al scratchpad, arranqué los servicios antes que NATS y añadí un reinicio de ui-api con datos vacíos.
- **NATS no listo al arrancar:** los tres servicios siguen vivos. `/readyz` da 503 con `"nats":"fail"` en los tres. Al levantar NATS y el stream pasan a 200 solos, y el recorrido completo (webhook, inbox, confirmación, controlador) funciona.
- **Replay de ui-api sin duplicados (decisión a):** reinicié ui-api con `UIAPI_DATA_DIR` vacío.
  - El inbox tenía 2 notificaciones antes y 2 después, sin duplicar.
  - El durable `ui-api-inbox` quedó con `Unprocessed 0`, `Redelivered 0`, `Outstanding Acks 0`.
  - El durable `go-run-controller-confirmed` quedó igual.
  - El stream conservó los 6 mensajes.
  - Que la confirmación aparezca como `pending` tras mi reinicio es esperado: borré el registro de recibos.
- **Webhook con NATS caído:** `go-intake/internal/intake/handler.go:176-179` devuelve 503 `publish_failed` y deja `PublishPending`, así que el reintento de GitHub republica con el mismo `event_id`. Es el contrato de hoy. No lo ejecuté.

## Hallazgos
### F-01 · NARANJA · `scripts/test/u8-nats-local.sh` (CA-2 y CA-3) · Dos comprobaciones del script no pueden fallar
1. **`sin-duplicados`, mitad del controlador.**
   - CA-2 pide «una sola notificación y una sola corrida».
   - Tras inyectar el `run.confirmed` duplicado, el script solo comprueba `GET /runs/$run == 200`, y eso ya era 200 antes de inyectarlo. Si el controlador creara dos corridas, la prueba seguiría en `OK`.
   - La bitácora lo reconoce («no cuenta filas del diario»), pero es una lectura de vuelta que no lee nada del duplicado.
   - Arreglo propuesto: contar las corridas o los `run_id` en el diario o `RUN_DATA_DIR` del controlador, o comprobar que el estado de la corrida no cambia tras el duplicado.
2. **`reentrega-tras-reinicio` no prueba la reentrega del durable.**
   - ui-api reinicia con la proyección vacía y el replay (decisión a) relee todo el stream. Mi sonda lo confirma: tras reiniciar, ui-api muestra las notificaciones ya acusadas (`Unprocessed 0`).
   - La notificación aparecería igual aunque el durable no reentregara nada. El criterio prometía probar «al menos una vez», y la prueba lo verifica por un camino distinto.
   - Arreglo propuesto: desactivar el replay en la prueba, o comprobar que el consumidor durable llevó el mensaje de «sin ack» a «con ack» (`consumer info`: `AckFloor` o `Delivered` avanzan por el durable).
   - Para el controlador sí hay prueba de reentrega (`TestNATSSourceAckYReentregaSinAck`). El hueco es solo ui-api.

### F-02 · AMARILLO · `services/ui-api/inbox/nats_test.go:195-241` y `:120-170` · Pruebas con poder de discriminación limitado
- `TestNATSSubscriberDedupePorEventIDEnStore` publica dos mensajes con el mismo `event_id` y el mismo `notification_id` (`n-1`). `Store.Apply` también deduplica por notification id, así que la prueba no demuestra el dedupe por `event_id`. Falta un caso con `event_id` igual y notification id distinto.
- `TestNATSSubscriberAckSoloTrasAplicarYTermInvalido` solo comprueba que no queda nada pendiente. No detectaría un `Ack` antes de aplicar, aunque con `handle` síncrono y sin error es difícil.

### F-03 · AMARILLO · `services/ui-api/cmd/ui-api/main.go` (rama `else`) · `/readyz` no espera al replay
- Con `file`, `Drain` carga el inbox antes de servir. Con `nats`, el replay corre en segundo plano y ui-api ya anuncia `ready` mientras el inbox puede estar incompleto. Un `confirm` en esa ventana corta daría 404.
- Es una diferencia de comportamiento respecto a `file`, no un incumplimiento de criterio.
- Sugerencia: añadir un chequeo `replayed` en `/readyz`.

### F-04 · AMARILLO · decisiones del codificador (b), (c), (d)
- **(b)** Los tests de selección de transporte se escribieron después. Es una irregularidad de proceso, pero leyéndolos cubren comportamiento real (nats sin `NATS_URL` falla, desconocido falla, file sigue exigiendo archivos). El rojo de adaptadores y de CA-2/CA-4 sí está en la bitácora. No bloquea.
- **(c)** Las subidas transitivas son consecuencia de la resolución de versiones al añadir `nats.go` y `nats-server`: `x/sys` 0.47→0.48, `x/text` 0.40→0.42, `klauspost/compress` 1.19.2→1.20.1 y `x/time` 0.15→0.16. Aceptable.
  - Los tests usan `nats-server` v2.15.1 embebido, mientras que despliegue y script usan la imagen 2.10.24. El script cubre la discrepancia.
- **(d)** `EVENTS_TRANSPORT=nats` y `NATS_URL` en `base`: coherente con el alcance, con `nats.aqs-system.svc:4222` y la etiqueta `aqs.io/nats-client` presente en los tres Deployments. Solo hay reglas de ingreso en `aqs-system`, así que no hay egress que abrir. Sin objeción.
  - Consecuencia ya declarada: en clúster, `rehearsal.passed/failed` no viajan por NATS (fuera de alcance).

### F-05 · AMARILLO · duplicación y limpieza (clase repetible, barrida)
- `ConnectNATS` y `NATSReady` están copiados tres veces (`go-intake/internal/intake/nats.go`, `ui-api/inbox/nats.go`, `go-run-controller/adapters/nats.go`) con los mismos parámetros de reconexión. Es esperable con módulos Go independientes, pero habrá que mantener tres copias.
- `NATSSource.Poll` registra un `WARN` cada 500 ms mientras el stream no existe (ruido en logs durante `nats-init`).
- El consumidor efímero del replay no se borra explícitamente. Lo limpia NATS por inactividad. Mi `consumer ls` vio dos efímeros tras dos arranques de ui-api.

## Tareas candidatas (defectos reales fuera de alcance)
- **Retención de 7 días vs. inbox en memoria:** tras ese plazo y un reinicio de ui-api, las notificaciones antiguas pendientes desaparecen. Hace falta persistir la proyección o un snapshot. La bitácora ya lo declara.
- **`run.done` solo se deduplica 2 min** por `Nats-Msg-Id`. Un reenvío posterior no lo detiene el stream; habría que deduplicar en el consumidor.
- **`rehearsal.*` y eventos de `go-warm-manager`/`go-reset`** siguen sin transporte NATS (ya declarado fuera de alcance).
- **Variables de base de ui-api y go-intake** (`UIAPI_DATA_DIR`, `INTAKE_DATA_DIR`, …) ausentes de `control-plane.yaml` (U8-T07).

## Rutas de transcripciones largas
- Sonda propia (arranque sin NATS, replay): `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/1c7b66eb-a3e9-4dad-81e8-f56a81f889ed/scratchpad/probe1.sh`

```
VEREDICTO: NO-VERDE
NARANJA|scripts/test/u8-nats-local.sh (CA-2 sin-duplicados, CA-3 reentrega)|Dos comprobaciones no pueden fallar: la «una sola corrida» solo mira GET /runs=200, y la reentrega la cubre el replay y no el durable
INFORME: revisiones/U8-T02/ronda-1.md
```
