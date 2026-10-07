# Ronda 1 — U2-T02b

VEREDICTO: NO-VERDE

Los seis hallazgos de U2-T02 (F-01 a F-06) están bien cerrados en el comportamiento, y mi barrido propio no encontró ninguna violación. El bloqueo es de cobertura: varias mutaciones sobre el código nuevo sobreviven a toda la suite, y una de ellas pierde `run.confirmed` en un escenario realista (F-01). Un NARANJA, cuatro AMARILLOS, ningún ROJO. Worktree limpio: `git status --short | wc -l` = 0 antes y después de mis corridas. HEAD `ed1487a`, merge-base `c674965`.

## Criterios de aceptación, verificados por mí (frescos, `-count=1`)
| # | Comando que corrí | Resultado |
|---|---|---|
| 1 | CA-1 literal | pasa: `79` (≥67) y `0`. Sin `--- SKIP`/`FAIL`; `-race` limpio |
| 2 | CA-2 literal | pasa: 8 `--- PASS` (PartialWrite ×2, SyncFailure ×2, TornTail ×3, propiedad rapid) |
| 3 | `unshare -rm` + tmpfs 16k, servicio real con go-identity y go-governance reales (`rev2b/bb_disk.sh`, salida en `rev2b/bb_disk.out`) | pasa |
| 4 | CA-4 literal | pasa: 5 `--- PASS` |
| 5 | CA-5 literal + mi M1 | pasa: `TestReadyzPersistWiring`; con `persist` devolviendo `nil` queda roja |
| 6 | CA-6 literal | pasa: `2` y `1` |
| 7 | vet (con y sin `-tags contract`), gofmt, `docker build`+inspect, status, diff fuera de alcance | pasa: `ok`, `65532:65532`, `0`, `0`. Diff: 15 archivos, todos dentro de alcance. Hash de la tarea coincide con el del acuse. |

CA-3 en detalle:
- Disco lleno: el diario se queda en 13 líneas / 3988 bytes (el `Write` parcial se restauró), `aqs_persist_errors_total` sube y `/readyz` da `{"persist":"fail"}` con 503. 3 s después nada cambió, sin fallo ni handoff.
- `kill -9` con cola `{"seq":99,"prev":"ab` añadida a mano: arranca (200), log `bytes_descartados=20`, métrica `aqs_journal_tail_discarded_total 1`.
- Al liberar espacio llega a `done` (seq 23), `/readyz` 200 y el outbox tiene 1 línea. Un segundo `kill -9` y reinicio arranca.

## Barrido de clase propio (efecto externo + fallo del almacén)
Harness `rev2b/zz_chaos_test.go`, 2000 semillas × 120 ticks, con el diario real sobre un escritor con fallos, `FileSource` y `Outbox` reales y `Controller` real. Inyecta:
- `Write` parcial (1, 7, 60, 150, 1000 bytes), `Sync` fallido (1 o 2 veces) y `Truncate` fallido;
- fallos de `Write` y `Sync` en el outbox;
- reinicios aleatorios (el `FileSource` repite desde 0), con y sin cola de `kill -9`;
- reinicio forzado si el diario queda roto.

Resultado: **0 violaciones** de estas invariantes:
- el diario siempre reabre;
- ninguna corrida retrocede de estado tras reabrir;
- ningún evento se pierde;
- todas llegan a `done` y se publican una sola vez;
- ≤3 `Launch` por (corrida, fase);
- 0 handoffs.

Además `rapid -rapid.checks=3000`: OK. El harness es sensible: con mi mutación `Mstale` (no avanzar `size`) detecta «el diario no vuelve a arrancar» y regresiones de estado.

Puntos de la lupa:
- **(1) Descarte de cola.** No descarta una línea confirmada: `Save` solo devuelve nil tras `Write` completo (Go no devuelve escrituras cortas sin error) y `Sync` OK, así que toda línea confirmada termina en `\n`. Si `Sync` y truncado fallan, el diario queda roto y la línea en vuelo puede quedar en disco: «al menos una vez», idempotente. Ver F-04.
- **(2) Ack.** Crash entre `Apply` y `Ack` repite el evento: es idempotente por `Seen` y por `run_id`. Con `ErrPersist` el `break` es esencial, porque `Ack(ev)` salta el offset hasta `ev.Pos`. Ese `break` no tiene prueba: ver F-01.
- **(3) /readyz.** M1 repetida: roja.
- **(5) Pruebas eliminadas o cambiadas.** `TestJournalTornLineRefuses` quedó cubierta con aserción invertida por `TornTailOnlyFileOpensEmpty` más `TornTailWithNewlineStillRefuses`; el ajuste de `FileSourceFullLinesOnlyAndFilters` no debilita nada.
- **(6) Documentación.** Presente y correcta, salvo la observación de F-03.

## Mutaciones (copia en `rev2b/mm_*`, pruebas rojas)
| Mutación | Resultado |
|---|---|
| M1 persist siempre `nil` | roja: `ReadyzPersistWiring` |
| M7 sin `break`/Ack con `ErrPersist` | roja: 4 pruebas |
| M9 ignorar error de `Sync` | roja: 3 |
| M3 diario sin descartar cola | roja: 4 |
| M2 sin restaurar | roja: 6 |
| M4 sin truncar la cola | roja: 1 |
| M6 dedupe por `bytes.Contains` | roja: 1 |
| M12 ignorar error de `Publish` | roja: 1 |
| `size` obsoleto, `Truncate(0)`, sin chequeo de roto, `Healthy` sin roto | rojas |
| Ack sin posición, offset sin prefijo, recontar inválidas | rojas |
| **`break` → `continue` en `tick`** | **SOBREVIVE** |
| **`Outbox`: restaurar a tamaño 0 en vez de `len(b)`** | **SOBREVIVE** |
| **`loop` sin llamar a `tick` (cuerpo antiguo, Ack tras `Apply`)** | **SOBREVIVE** |
| **quitar `noteJournalTail(...)` de `run()`** | **SOBREVIVE** |
| **`JournalOptions{}` sin `Log` en `run()`** | **SOBREVIVE** |
| `Ack` sin máximo (`committed = ev.Pos`) | sobrevive (equivalente en la práctica: Ack siempre en orden) |

## Hallazgos

### F-01 · NARANJA · `cmd/go-run-controller/main.go:161-190` (`loop`/`tick`/`run`), `adapters/events.go` (Outbox) · mutaciones supervivientes en el núcleo del arreglo
Es la misma clase del F-03 de la ronda 3 de U2-T02 (cableado sin prueba), ahora en el código nuevo. Se agrupan:
1. **`break` de `tick`.** Cambiarlo por `continue` deja toda la suite verde, y es un escenario realista. Lote `[run.confirmed, rehearsal.passed]` con el disco caído:
   - `run.confirmed` da `ErrPersist` y no se confirma.
   - `rehearsal.passed` da `ErrNotFound` (sin `Save`), se confirma, y `Ack` salta el offset hasta su posición.
   - `run.confirmed` queda perdido.
   - Prueba mía `TestRevBatchOrderingNotLost` (`rev2b/zz_batch_test.go`, usa `rig`/`ticks` de `loop_test.go`): pasa con el código real y da «REPRODUCIDO: run.confirmed perdido» con `continue`. Es el defecto de F-02 de U2-T02, que reaparece con un cambio de una línea sin que ninguna prueba lo vea.
2. **`loop` → `tick`.** Reponer el cuerpo antiguo en `loop` (Ack tras `Apply` sin mirar `ErrPersist`) deja la suite verde: las pruebas llaman a `tick`, no a `loop`.
3. **`Outbox` restaura a `int64(len(b))`.** Con `0` toda la suite pasa. Con contenido previo, un `Publish` fallido borraría los `run.done` anteriores. Todas las pruebas de fallo del outbox parten de un archivo vacío (mi `TestRevOutboxFailureKeepsPriorLines` pasa con el código real, así que se puede añadir tal cual).
4. **Cableado en `run()`.** Quitar `noteJournalTail(store, metrics)` o `Log: log` deja la suite verde. La métrica y el log solo se comprueban en la caja negra manual.

Corrección esperada:
- (a) una prueba de lote con un evento intermedio que falla por `ErrPersist` y otro posterior que no llama a `Save`;
- (b) una prueba que ejecute `loop` real con contexto cancelable;
- (c) una prueba del `Outbox` con una línea previa;
- (d) para el cableado de `run()`, o bien probarlo extrayéndolo, o bien registrar explícitamente que solo lo cubre la caja negra.

### F-02 · AMARILLO · `adapters/events.go` `published`/`Publish` · una línea JSON completa sin `\n` produce un evento duplicado
Si el `kill -9` ocurre tras escribir todo menos el `\n` final (o `Write` parcial de `len-1` más truncado fallido), `published` la ignora y `Publish` antepone `\n`: la línea queda completa y se añade una segunda con el mismo `event_id`. Reproducido con `TestRevOutboxCompleteJSONNoNewlineDuplicates` (`rev2b/zz_rev_test.go`): «2 líneas completas con el mismo `event_id`». La ventana es de un byte y el `event_id` es UUIDv5 determinista, así que el consumidor puede deduplicar. Corrección esperada: si la cola parsea como objeto completo con ese `event_id`, solo añadir `\n`. Aparte, `err, _ = appendDurable(...)` ignora `intact`: tras `Sync`+truncado fallidos la línea se da por publicada en el siguiente intento sin haber sido durable (doble fallo; no bloquea).

### F-03 · AMARILLO · `runctl/ports.go` (`PhaseLauncher`) y README · la clave de idempotencia documentada nunca se repite
`Launch` es «idempotente por (corrida, fase, `Started[fase]`)», pero `Drive` incrementa `Started` en cada intento. Tras un crash entre `Launch` y `Save(Launched)`, el reintento llega con `Started` = n+1 (otra clave) y el adaptador no puede deduplicar con esa clave. Un adaptador que nombre el Job por (corrida, fase, `Started`) crearía hasta 3 Jobs. La redacción vino de la tarea y el codificador la cumplió; se corrige en la especificación de U2-T04/T05: dedupe por (corrida, fase) «adoptar el Job vivo», con `Started` solo como número de intento. El README es honesto sobre el «tope 3».

### F-04 · AMARILLO · `adapters/journal.go` + README · un diario roto no se recupera sin reinicio y la documentación no lo dice
Tras `ErrJournalBroken` el proceso sigue vivo, `/healthz` da 200 (el kubelet no reinicia) y `/readyz` da 503 para siempre. Hay que reiniciarlo a mano; el README no lo indica. Además, el README describe el handoff «a lo sumo una vez» solo como crash entre guardar y avisar: no menciona que un `Save` fallido-pero-persistido (diario roto) también puede dejar `HandedOff` en disco sin aviso. Basta una línea en README o una candidata.

### F-05 · AMARILLO · `adapters/journal_disk_test.go` · detalles menores de las pruebas
- La propiedad detecta «roto» con `s.Healthy()`, que en un diario sano llama a `Sync` y consume `syncFails` inyectados.
- `appendDurable` devuelve `(err error, intact bool)`, con el `error` primero, lo que contradice la convención de Go.
- `TestEventAckAfterApplyAlreadyAppliedNotAppliedTwice` llama a `Apply` directamente y no ejercita `tick`/`Ack`.

## Tareas candidatas (fuera de alcance, no exigidas)
- Aclarar el contrato de `PhaseLauncher` (F-03) en las tareas U2-T04/T05.
- Reabrir y revalidar el diario roto sin reiniciar el proceso, o hacer fallar `/healthz`.
- Archivo de eventos rotado a un tamaño mayor que el offset: lee desde mitad de línea (comportamiento previo, no introducido aquí).
- Un evento irrecuperable (`ErrNotFound`) se confirma y se descarta (la bitácora ya lo registra).

VEREDICTO: NO-VERDE
NARANJA|services/go-run-controller/cmd/go-run-controller/main.go:161-190 (loop/tick/run) y adapters/events.go (Outbox)|Mutaciones supervivientes en el núcleo del arreglo: `break`→`continue` en tick (run.confirmed perdido, reproducido), loop sin tick, Outbox restaurando a 0, cableado de noteJournalTail/Log en run()
AMARILLO|adapters/events.go published/Publish|Línea JSON completa sin \n produce evento duplicado (ventana de 1 byte; event_id determinista)
AMARILLO|runctl/ports.go PhaseLauncher|La clave de idempotencia (corrida, fase, Started) nunca se repite porque Started se incrementa en cada intento
AMARILLO|adapters/journal.go + README|Diario roto sin recuperación hasta reiniciar y no documentado; /healthz sigue en 200
AMARILLO|adapters/journal_disk_test.go|Detalles menores de pruebas (Healthy consume fallos inyectados, firma (err, intact), Apply sin tick)
INFORME: revisiones/U2-T02b/ronda-1.md
