# Ronda 2 — U8-T02

VEREDICTO: VERDE

Worktree `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U8-T02`, HEAD 88ac5aba. Corrí yo mismo los cinco criterios sobre ese HEAD y pasan. F-01, F-02 y F-03 están corregidos y las mutaciones que repetí hacen fallar las pruebas. No queda ROJO ni NARANJA en pie. No tengo `Write`: el informe va aquí y el orquestador debe guardarlo en `revisiones/U8-T02/ronda-2.md`.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Pruebas existentes y nuevas en verde | `for s in go-intake ui-api go-run-controller; do (cd services/$s && GOTOOLCHAIN=auto go test -count=1 ./...); done` | pasa: go-intake rc=0, ui-api rc=0, go-run-controller rc=0 |
| 2 | Recorrido por NATS con servicios reales | `bash scripts/test/u8-nats-local.sh` | pasa: `OK webhook-firmado-202`, `OK inbox-recibe-notificacion`, `OK confirm-201`, `OK controlador-recibe-run-confirmed`, `OK sin-duplicados`, `rc=0`. El 201 coincide con el contrato (arbitraje de la ronda 1). |
| 3 | Reentrega tras reinicio | `bash scripts/test/u8-nats-local.sh --redelivery` | pasa: `OK reentrega-tras-reinicio`, `rc=0` |
| 4 | Manifiestos y políticas | `kubectl kustomize deploy/flux/kind \| grep -c -E 'name: EVENTS_TRANSPORT'; bash scripts/ci/policies.sh 2>&1 \| grep -c '^FALLA'` | `3` y `0` |
| 5 | Alcance | `git status --short \| wc -l` y el `grep -v` del criterio sobre `git diff --name-only $b` | `0` y `0` |

- `git fetch` + `git merge-tree --write-tree HEAD origin/main`: `rc=0` (árbol d521e9d). `git log HEAD..origin/main` está vacío, así que origin/main no ha avanzado.
- Tras mis ejecuciones, el worktree queda limpio (`git status --short` → 0) y no sobra ningún contenedor `aqs-u8`.
- Las mutaciones las hice sobre una copia exportada con `git archive` en el scratchpad. El worktree del codificador no se tocó.

## Verificación de los hallazgos de la ronda 1
- **F-01 (1) `sin-duplicados`, mitad del controlador.**
  - Mutación mía: quité el `return nil` de dedupe en `EvRunConfirmed` (`services/go-run-controller/runctl/controller.go`).
  - Resultado: `FALLA sin-duplicados: ... registros confirmed de la corrida antes=1 después=3 (quiero 1 y 1) ...`, `rc=1`.
  - Sin la mutación pasa. La comprobación ahora lee el diario `runs.jsonl` del controlador, no solo el código HTTP.
- **F-01 (2) `reentrega-tras-reinicio`.**
  - Mutación mía: quité `msg.Ack()` tras `handle` en `services/ui-api/inbox/nats.go`.
  - Resultado: `FALLA reentrega-tras-reinicio: el durable ui-api-inbox no acusó el mensaje (ack_floor=0, secuencia=1) ...`, `rc=1`.
  - Ahora decide el `ack_floor` del durable leído con `consumer info --json`, no la aparición de la notificación, que el replay produce igual. `jq` está disponible en este entorno, y el script lo exige.
- **F-02, caso nuevo `TestNATSSubscriberMismoEventIDDistintaNotificacion`.**
  - Mutación mía: quité el dedupe por `event_id` de `Store.Apply`.
  - Resultado: FAIL con `n-2` y `n-1` aplicados (`quiero solo n-1`).
  - La prueba ya distingue el dedupe por `event_id` del dedupe por notification id.
  - Acepto la justificación de no añadir «Ack antes de aplicar». El `handle` es síncrono y sin error, y la reentrega sin Ack ya la cubre el script.
- **F-03, chequeo `replay` en `/readyz`.**
  - El chequeo `replay` falla hasta que `NATSSubscriber.replayed` (ahora `atomic.Bool`) pasa a true.
  - Mutación mía: `Replayed()` devuelve siempre `true`.
  - Resultado: `TestTransportNATSSinServidorArrancaYNoEstaListo` falla (`falta el chequeo replay o no falla antes del replay`).
  - En la ejecución completa del script los servicios llegan a `ready` tras el replay.
- **F-04, aceptado.** Los tests de transporte escritos después ya se explicaron en la ronda 1, y esta ronda solo añade pruebas que probé con mutación. Las subidas transitivas por resolución de versiones y el cableado de `base` siguen sin objeción.
- **F-05, aceptado como AMARILLO.**
  - `ConnectNATS` y `NATSReady` copiados en cada módulo Go independiente: justificado.
  - El WARN por tick solo ocurre durante `nats-init`.
  - Un consumidor efímero del replay lo limpia NATS por inactividad.
  - Nada de esto rompe un criterio.

## Hallazgos en pie
- Ningún ROJO ni NARANJA.
- **AMARILLO (no bloquea):** F-05 (copias de `ConnectNATS`/`NATSReady`, WARN por tick durante `nats-init`, consumidor efímero del replay sin borrado explícito) y F-04 (diferencia entre el `nats-server` 2.15.1 embebido de las pruebas y la imagen 2.10.24 del despliegue; el script cubre la imagen real).

## Tareas candidatas (defectos reales fuera de alcance)
- **Retención de 7 días vs. inbox en memoria:** tras ese plazo y un reinicio de ui-api, las notificaciones antiguas pendientes desaparecen. Hace falta persistir la proyección o un snapshot.
- **`run.done` solo se deduplica 2 min** por `Nats-Msg-Id`. Habría que deduplicar en el consumidor.
- **`rehearsal.*` y eventos de `go-warm-manager`/`go-reset`** siguen sin transporte NATS.
- **Variables de datos de ui-api y go-intake** (`UIAPI_DATA_DIR`, `INTAKE_DATA_DIR`) ausentes de `control-plane.yaml` (U8-T07).

## Rutas de transcripciones largas
- Copia mutable usada en las mutaciones: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/1c7b66eb-a3e9-4dad-81e8-f56a81f889ed/scratchpad/mut`

```
VEREDICTO: VERDE
AMARILLO|services/*/nats.go (ConnectNATS/NATSReady por módulo), Poll con WARN por tick|F-05 duplicación entre módulos y ruido de log durante nats-init; no bloquea
INFORME: revisiones/U8-T02/ronda-2.md
```
