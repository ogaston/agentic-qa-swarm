# Ronda 2 — U2-T02b

VEREDICTO: NO-VERDE

Los cinco supervivientes del F-01 de la ronda 1 mueren con mis propias mutaciones, y F-02 a F-05 están cerrados. Lo que bloquea es un NARANJA nuevo: mi barrido de mutaciones sobre `Poll` y el diario reabierto encontró supervivientes que pierden un evento o corrompen el diario. La bitácora afirma «Supervivientes: ninguna», y eso no es cierto para estas ramas. No hay ROJO. El arreglo esperado es solo de pruebas (unas 3, sin tocar código de producción). El diff completo está dentro de alcance.

Worktree: `git status --short | wc -l` = 0 tras todas mis corridas. HEAD `503a2d3`, merge-base `c674965`.

## Criterios de aceptación, verificados por mí (frescos, `-count=1`)
| # | Comando que corrí | Resultado |
|---|---|---|
| CA-1 | literal, con `-race` | pasa: `89` (≥67) y `0`; sin `--- SKIP`; los 7 paquetes `ok` |
| CA-2 | literal | pasa: 9 `--- PASS` (PartialWrite ×3, SyncFailure ×2, TornTail ×3, propiedad) |
| CA-3 | `unshare -rm` + tmpfs 16k, `bb_disk.sh`, servicio real con go-identity y go-governance reales | pasa |
| CA-4 | literal | pasa: 6 `--- PASS`, incluye el lazo real y el lote |
| CA-5 | literal | pasa: `TestReadyzPersistWiring` |
| CA-6 | literal | pasa: `2` y `1` |
| CA-7 | vet con y sin `-tags contract`, gofmt, `docker build` + inspect, status, alcance | pasa: `ok`, `65532:65532`, `0`, `0` |

Detalle de CA-3:
- **Disco lleno:** el diario queda en 13 líneas / 3988 bytes, `aqs_persist_errors_total` sube de 7 a 13, `/readyz` da 503 con `persist:fail`, sin handoff y sin cambios a los 3 s.
- **`kill -9` con cola `{"seq":99,"prev":"ab`:** arranca con 200, log `bytes_descartados=20` y `aqs_journal_tail_discarded_total 1`.
- **Al liberar espacio:** llega a `done` (seq 23), `/readyz` da 200 y el outbox tiene 1 línea.
- **Segundo `kill -9`:** arranca.

## Mutaciones del F-01, repetidas por mí sobre una copia de HEAD
| Mutación | Resultado |
|---|---|
| `break`→`continue` en `tick` | MATADA: `EventBatchPersistErrorDoesNotSkipLaterEvents` |
| `loop` sin `tick` | MATADA: `EventNotLostWhenSaveFailsInRealLoop` |
| Outbox restaurando a 0 | MATADA: `OutboxFailureKeepsPriorLines` |
| Quitar `noteJournalTail` | MATADA: `JournalTornTailWiringOpenJournalLogsAndCounts` |
| `JournalOptions{}` sin `Log` | MATADA: `JournalTornTailWiringOpenJournalLogsAndCounts` |

La extracción de `openJournal` es correcta y comprobable. La declaración honesta del README y la bitácora sobre `run()` es aceptable: solo la caja negra cubre la secuencia completa de arranque, como en F-01 (d).

Mi barrido propio:
- **Alcance:** 100 mutaciones sobre `main.go`, `events.go`, `journal.go`, `fileio.go` y `obs/metrics.go`.
- **Mutantes que no compilaron:** los reescribí hasta compilar.
- **Resultado:** 6 de las 12 supervivientes de abajo no son equivalentes y están en F-01 o en el AMARILLO que sigue. El resto no tiene efecto práctico: `tickEvery` constante, ignorar el error de `Poll` (solo log), `len(valid) > 5` y `|| false`.

## Barrido de caos
- **Qué hace el harness:** 2000 semillas × 120 ticks, con el diario real sobre un escritor con fallos, `FileSource` y `Outbox` reales y `Controller` real. Inyecta `Write` parcial (1, 7, 60, 150, 1000 bytes y `len-1`), `Sync` fallido, `Truncate` fallido y reinicios con y sin cola de `kill -9`. En el outbox añadí `Write` parcial de `len-1`, 30, 100 y 250 bytes con `Truncate` fallido.
- **Resultado:** `PASS`, 0 violaciones: el diario siempre reabre, ninguna corrida retrocede, no se pierde ningún evento, todas llegan a `done` con una sola publicación, ≤3 `Launch` por fase y 0 handoffs. Se cumple en particular la cola JSON completa sin `\n`.
- **Sensibilidad del harness:** mata mi mutante «Outbox sin `completeTail`» (134 de 1500 semillas con run.done duplicado) y mi mutante `size = len(b)` (987 de 1500 semillas: «el diario no vuelve a arrancar»).

## Hallazgos que BLOQUEAN

### F-01 · NARANJA · `adapters/events.go` `Poll` y `adapters/journal.go` `OpenJournalOpts` · mutaciones supervivientes en las ramas nuevas de offset y de reapertura
Es la misma clase del F-01 de la ronda 1. La tabla de «Barrido de clase (ronda 2)» de la bitácora dice «Supervivientes: ninguna», y eso es falso para estas ramas, que no están en la tabla:

1. **Línea parcial que se completa después.**
   - **Mutación:** en `Poll`, cambiar `break // línea parcial` por `off += len(line); break`. Toda la suite queda verde.
   - **Efecto:** el offset salta la parte escrita de la línea y, cuando el productor la completa, el evento se pierde.
   - **Reproducción:** `TestRevPartialLineCompletedLaterIsDelivered` (en `scratchpad/r2/pc/.../adapters/zz_partial_test.go`). Con el código real pasa; con la mutación da «evento perdido tras completarse la línea: []».
   - **Por qué cuenta:** `off` es código reescrito en esta tarea. La única prueba con cola parcial (`FileSourceFullLinesOnlyAndFilters`) nunca la completa.
2. **`size` tras descartar una cola.**
   - **Mutación:** `s.size = len(b)` en vez de `len(valid)` al abrir. Toda la suite queda verde.
   - **Efecto:** tras un reinicio con cola descartada, un `Write` parcial seguido de `Truncate(size)` extiende el archivo con NULs y el diario deja de reabrir.
   - **Cobertura:** solo lo detecta mi caos. Falta una prueba de «reabrir con cola → `Save` con `Write` parcial → reabrir».
3. **Cola de 1 byte.**
   - **Mutación:** `s.discarded > 0` → `> 1`. Sobrevive.
   - **Efecto:** el byte no se trunca y el siguiente `Save` queda pegado a él; el diario no arranca después.
   - **Cobertura:** los tests usan colas de 8 y 19 bytes.

Corrección esperada, solo pruebas: una por cada punto (1 a 3). Mi prueba del punto 1 sirve tal cual.

## AMARILLOS (no bloquean; aceptables para fusionar)
### F-02 · AMARILLO · pruebas con efecto limitado a contadores, errores de apertura y el chequeo `journal`
- **`FileSource.discarded`:** `off >= scanned` en vez de `>`, no resetear `scanned` al reemplazar el archivo, o `Pos = off - len(line)`. Solo cambian el contador `Discarded()`, que ninguna de esas pruebas aserta.
- **Chequeo `journal` de `readyChecks`:** devolver `nil` o intercambiarlo con el de `persist` sobrevive. `TestReadyzPersistWiring` usa `okJournal`, y el chequeo `persist` ya cubre el caso del diario roto.
- **Errores de apertura:** ignorar el error de `Truncate` o de `ReadFile` en `OpenJournalOpts`, y `openJournal` devolviendo `(j, nil)` ante error. No hay prueba que los fuerce.
- **`Healthy()` sin `Sync`:** sobrevive. Es comportamiento previo.
- **`TestEventAckAfterApplyAlreadyAppliedNotAppliedTwice`:** deja una variable `before` sin uso (`_ = before`).

## Hallazgos cerrados, verificados
- **F-02 de la ronda 1 (cola completa sin `\n`):** cerrado. Mi caos con `len-1` no duplica, y la mutación «sin `completeTail`» muere en la prueba unitaria y en el caos.
  - **¿Puede el `dirty` tras doble fallo dar por publicado un evento no durable?** No. Con la línea completa en disco y `Sync` fallido, el reintento ve «publicada y `dirty`», sincroniza y solo entonces responde nil. Las mutaciones B4 a B7 mueren.
  - **¿Puede duplicar?** No. Una cola parcial se aísla con `\n` y no se cuenta como publicada.
- **F-03 (contrato de `PhaseLauncher`):** cerrado. El comentario de `ports.go` y el README dicen «idempotente por (corrida, fase); adoptar el trabajo vivo; `Started` es solo el intento».
- **F-04 (README):** cerrado. Documenta el diario roto (`/readyz` 503, `/healthz` 200, hay que reiniciar) y el handoff con `Save` fallido-pero-persistido.
- **F-05:** cerrado. `appendDurable` devuelve `(intact, err)`; la propiedad usa `s.broken` en vez de `Healthy()`; la prueba de «ya aplicado» pasa por `tick`/`Ack`.
- **Coherencia tras el incidente del script de mutaciones:** sin pérdidas ni cambios a medias. Todo lo de la ronda 1 y la ronda 2 está en el diff contra el merge-base y es coherente con mis pruebas y con las 100 mutaciones. El historial (`a9662e5`, `ee53d6e`, `89a0554`, `503a2d3`) está en orden y el árbol está limpio.

## Tareas candidatas (fuera de alcance)
- `/healthz` fallando con el diario roto, o reabrir el diario en caliente (ya está en el README).
- HMAC y ancla del `seq`, rotación del diario, `halted` visible, timeout de `rehearsal.passed` (las candidatas aceptadas).
- Un evento irrecuperable (`ErrNotFound`) se confirma y se descarta sin reintentar (ya registrado en la bitácora).
- Archivo de eventos rotado a un tamaño mayor que el offset: se lee desde mitad de línea (comportamiento previo).

VEREDICTO: NO-VERDE
NARANJA|services/go-run-controller/adapters/events.go Poll y adapters/journal.go OpenJournalOpts|Mutaciones supervivientes en las ramas nuevas: línea parcial completada después se pierde (reproducido), size tras descartar cola corrompe el diario tras Write parcial, cola de 1 byte sin truncar
AMARILLO|adapters/events.go (Discarded), main.go readyChecks journal, errores de apertura|Supervivientes de bajo impacto (contador Discarded, chequeo journal del cableado, errores de Truncate/ReadFile), variable `before` sin uso
INFORME: revisiones/U2-T02b/ronda-2.md
