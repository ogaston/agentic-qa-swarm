# go-run-controller

Controlador de corridas (C9): máquina de estados persistida en un diario JSONL con hash encadenado,
con un gate de go-governance (U4) en cada transición, `GET /runs/{id}`, `/healthz`, `/readyz` y `/metrics`.
Los adaptadores reales de fase (Jobs de Kubernetes) llegan con U2-T03 a T06; hoy solo `RUN_PHASES=fake`.

## Persistencia y fallos de disco

- **Diario** (`RUN_DATA_DIR/runs.jsonl`). Un `Save` que falla en `Write` o `Sync` trunca el archivo al
  tamaño previo y no avanza `seq`/`last`. Si no puede garantizarlo, el diario queda **roto**: todo
  `Save` posterior devuelve error, el controlador no avanza ni lanza nada y `/readyz` da `persist: fail`.
- **Arranque.** Una cola sin `\n` final (escritura interrumpida por `kill -9`; nunca tuvo un `Save`
  exitoso detrás) se descarta, con una línea de log (`bytes_descartados`) y `aqs_journal_tail_discarded_total`.
  Una línea completa (con `\n`) que no encadena sigue impidiendo el arranque (corrupción real).
- **Eventos de entrada** (`RUN_EVENTS_FILE`). El offset se confirma (`Ack`) solo cuando `Apply` sale
  bien. Con `ErrPersist` el evento se entrega de nuevo en el siguiente tick; la idempotencia por
  `Seen` y por `run_id` evita aplicarlo dos veces.
- **Eventos de salida** (`RUN_OUTBOX_FILE`, `run.done`). Idempotente por `event_id`; solo cuentan las
  líneas completas, y un fallo de escritura deja el archivo como estaba.

## Garantías y límites

- `PhaseLauncher.Launch` es **idempotente** por (corrida, fase): ante un reintento el lanzador debe
  **adoptar el trabajo vivo** de esa (corrida, fase) en vez de crear otro. `Started[fase]` es solo el
  número de intento (tope 3), no una clave de deduplicación. Un crash entre `Launch` y el guardado de
  `Launched` repite el lanzamiento (hasta 3 veces por fase).
- El **handoff es «a lo sumo una vez»** (at-most-once) por fase: `HandedOff[fase]` se guarda antes de
  alertar, así que un crash entre ese guardado y el aviso lo pierde; también puede quedar en disco sin
  aviso si un `Save` falla pero persiste (diario roto). Un `halted` (sin salida segura) tampoco es
  visible en `GET /runs`: el único rastro es el log y `aqs_handoff_total`.
- Un **diario roto** (`ErrJournalBroken`) no se recupera solo: requiere reiniciar el proceso
  (`/readyz` da 503 y `/healthz` sigue en 200, así que el kubelet no lo reinicia). Candidata: hacer
  fallar `/healthz` o reabrir el diario en caliente.
- El cableado de `run()` está cubierto por pruebas vía `openJournal` (log y métrica de la cola
  descartada) y `readyChecks`; la secuencia completa de `run()` solo la cubre la caja negra manual (CA-3).
- Fuera de alcance (candidatas): HMAC y ancla del último `seq` del diario, rotación/compactación,
  `halted` visible en la API, tiempo máximo de espera de `rehearsal.passed`.
