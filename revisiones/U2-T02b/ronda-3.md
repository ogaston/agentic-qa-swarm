# Ronda 3 — U2-T02b

VEREDICTO: VERDE

El NARANJA de la ronda 2 (F-01) está cerrado de verdad. La ronda 3 es solo de pruebas, como declaró el codificador, y no queda ningún ROJO ni NARANJA. Lo que sobrevive son AMARILLOS aceptados. Es aceptable fusionar en desarrollo; el humano fusiona.

Worktree limpio: `git status --short | wc -l` = 0 antes y después de mis corridas. HEAD `4858fec`; la ronda 2 era `503a2d3`.

## Verificaciones pedidas
1. **El diff no toca producción.** `git diff 503a2d3..4858fec` toca 6 archivos:
   - `adapters/adapters_test.go`, `adapters/journal_disk_test.go` y `cmd/go-run-controller/loop_test.go` (`*_test.go`);
   - `bitacoras/U2-T02b.md`;
   - `revisiones/U2-T02b/ronda-2.md` y `revisiones/U2-T02b/ronda-2-respuesta.md`.

   El listado de archivos que no son `*_test.go` dentro de `services/go-run-controller` está vacío. En `loop_test.go` solo se quitó la variable `before`, que no se usaba.
2. **Las tres pruebas nuevas matan sus mutaciones**, repetidas por mí en una copia con `ulimit -v 4000000` y `go test -timeout 60s`. Ninguna es vacua.

   | Mutación | Resultado |
   |---|---|
   | `off += int64(len(line)); break` en `Poll` | MATADA: `TestFileSourcePartialLineCompletedLaterIsDelivered` |
   | `s.size = int64(len(b))` | MATADA: `TestJournalTornTailThenPartialWriteStillReopens` |
   | `s.discarded > 1` | MATADA: `TestJournalTornTailOneByteIsTruncated` |
3. **La bitácora corrigió con honestidad la tabla de la ronda 2.** Dice en negrita: «CORRECCIÓN (ronda 3): la frase "Supervivientes: ninguna" de la ronda 2 era falsa». Nombra los tres supervivientes reales y enumera los AMARILLOS que quedan, con la misma lista de mi informe. Es fiel a lo que encontré.
4. **CA-1..CA-7 frescos, `-count=1`.**

   | # | Resultado |
   |---|---|
   | CA-1 | pasa: `92` (≥67) y `0` FAIL; sin `--- SKIP`; los 7 paquetes `ok` |
   | CA-2 | pasa: 11 `--- PASS` |
   | CA-3 | pasa (`bb_disk.sh`, `unshare -rm` + tmpfs 16k, servicio real con go-identity y go-governance reales) |
   | CA-4 | pasa: 6 `--- PASS` |
   | CA-5 | pasa: `TestReadyzPersistWiring` |
   | CA-6 | pasa: `2` y `1` |
   | CA-7 | pasa: vet con y sin `-tags contract` y gofmt dan `ok`; imagen `65532:65532`; status `0`; fuera de alcance `0` |

   Detalle de CA-3:
   - **Disco lleno:** el diario queda en 13 líneas / 3988 bytes, `aqs_persist_errors_total` sube de 7 a 13, `/readyz` da 503 con `persist:fail` y no hay cambios a los 3 s.
   - **`kill -9` con cola de 20 bytes:** arranca con 200, log `bytes_descartados=20` y `aqs_journal_tail_discarded_total 1`.
   - **Al liberar espacio:** llega a `done` (seq 23), `/readyz` da 200 y el outbox tiene 1 línea.
   - **Segundo `kill -9`:** arranca.
5. **Barrido corto y caos.**
   - **Mutaciones:** 14 mutantes nuevos sobre `events.go`, `journal.go`, `fileio.go` y `main.go`, sin colgados ni excesos de memoria. 11 muertos; ver F-02 para los 3 supervivientes.
   - **Caos:** `TestRevChaos`, 2000 semillas × 120 ticks, diario real sobre un escritor con fallos, `FileSource` y `Outbox` reales y `Controller` real. Inyecta `Write` parcial (también `len-1`), `Sync` y `Truncate` fallidos y reinicios con y sin cola de `kill -9`. Resultado: `PASS`, 0 violaciones (88 s): el diario siempre reabre, ninguna corrida retrocede, no se pierde ningún evento, todas llegan a `done` con una sola publicación, ≤3 `Launch` por fase y 0 handoffs.

## Hallazgos que BLOQUEAN
Ninguno.

## Hallazgos aceptables para fusionar
### F-01 · AMARILLO · Los supervivientes AMARILLOS de la ronda 2 siguen en pie, aceptados
- **`FileSource.Discarded()`** (`off >= scanned`, no resetear `scanned`, `Pos = off - len(line)`): solo cambian el contador.
- **Chequeo `journal` de `readyChecks`:** el chequeo `persist` ya cubre el diario roto.
- **Errores de `Truncate`/`ReadFile` al abrir** y `openJournal` devolviendo `(j, nil)` ante error: no hay prueba que los fuerce.
- **`Healthy()` sin `Sync`:** comportamiento previo.

### F-02 · AMARILLO · Supervivientes nuevos de mi barrido corto
- **S3:** en `Poll`, `continue`→`break` ante una línea en blanco. Un renglón vacío en el archivo de eventos retrasaría las líneas posteriores hasta que se confirmen las anteriores. Es la lógica previa (`continue`), sin prueba, y un productor normal no escribe líneas en blanco.
- **S4** (`Ack` limitado a `Pos <= scanned`) y **S12** (`err = nil` cuando ya es nil en `appendDurable`) son equivalentes sin efecto.
- **S14** (omitir el log de «evento no aplicado») solo afecta al log.

### Tareas candidatas (fuera de alcance, no exigidas)
- `/healthz` fallando con el diario roto, o reabrir el diario en caliente.
- Una prueba que fuerce los errores de `Truncate` y `ReadFile` al abrir el diario, y que `openJournal` propague el error.
- Una prueba de línea en blanco en el archivo de eventos.
- HMAC y ancla del `seq`, rotación del diario, `halted` visible, timeout de `rehearsal.passed` (las candidatas aceptadas desde la ronda 1).
- Un evento irrecuperable (`ErrNotFound`) se confirma y se descarta sin reintentar (ya registrado en la bitácora).

VEREDICTO: VERDE
AMARILLO|adapters/events.go Poll (línea en blanco), Discarded(), readyChecks journal, errores de apertura del diario|Supervivientes de bajo impacto aceptados; candidatas de pruebas fuera de esta tarea
INFORME: revisiones/U2-T02b/ronda-3.md
