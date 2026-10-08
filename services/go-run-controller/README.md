# go-run-controller

Controlador de corridas (C9): máquina de estados persistida en un diario JSONL con hash encadenado,
con un gate de go-governance (U4) en cada transición, `GET /runs/{id}`, `/healthz`, `/readyz` y `/metrics`.
`RUN_PHASES=real` usa los adaptadores reales (U2-T04); `fake` sigue exigiendo `RUN_ALLOW_FAKE_PHASES=true` y se rechaza con `RUN_ENV=prod`.

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

## Ensayo y adaptadores reales (U2-T04)

- `RUN_PHASES=real` exige `WARM_URL`, `WARM_SERVICE_TOKEN`, `RESET_URL`, `RESET_SERVICE_TOKEN`, `RUN_ARTIFACT_REF`
  (imagen publicada con tag fijado; hoy un solo artefacto para todas las corridas), `REHEARSAL_IMAGE` (tag fijado, nunca
  `latest`), `REHEARSAL_TARGET_URL` y, opcional, `REHEARSAL_SERVICE_ACCOUNT` (`aqs-runner`). Solo URL http(s); cliente
  con timeout de 5 s, sin redirecciones; todo error, `409`, cuerpo inválido o timeout es fallo de la fase.
- Fases reales: `deploy` (idempotente por corrida: adopta el deploy existente, si no `POST /warm/ensure` + `POST /deploys`
  y espera `GET /deploys/{run_id}` hasta `done`), `infer` (`POST /surface`), `rehearse` (Job `rehearsal-<run>-<n>`),
  `reset` (`POST /resets`, exige `reset_verified=true`). `run` y `report` fallan cerrado hasta U2-T05.
- Ensayo: `Launch` valida el FlowPlan (un plan roto no crea Jobs), adopta el Job vivo por etiquetas
  (`aqs.io/run-id`, `aqs.io/phase`) y solo si no hay uno crea el siguiente (tope 3 en el clúster). El resultado se lee del
  estado del Job; `rehearsing -> running` exige `ensayo_passed=true` registrado en el almacén y nada lo salta.
- `go-run-controller render-rehearsal-job --run <id> --flow <id>` imprime el Job sin tocar un clúster.
- Límites conocidos (candidatas): sin fuente real de `FlowPlan` (U3): en real falla cerrado y el ensayo no pasa; el deploy
  espera dentro de `Launch` (bloquea el lazo hasta 12 min).

## Runners y evidencia (U2-T05 / U2-T05b)

- Fase `run`: un Job `runner-<run>-<flujo>` por flujo (tope `RUN_MAX_PARALLEL_RUNNERS`), evidencia en MinIO leída de vuelta
  y comparada por hash: `logs.txt` primero y `result.json` (marcador) al final. Si `pods/log` falla, `result.json` lleva
  `logs_unavailable: true`.
- **Para U3: `run.done` NO implica corrida exitosa.** Significa que la evidencia de todos los flujos está guardada; el
  resultado de cada flujo (`passed`, `failed` o `timeout`) está en `result.json.status`: léelo siempre.
- Plazos: el del flujo es el `activeDeadlineSeconds` del Job; el de la corrida se mide desde `runs/<run>/started-at`
  (escrito una sola vez en el almacén antes del primer Job, no depende de Jobs que se borran). Un Job vencido no se borra
  hasta que su evidencia queda escrita. La ola de Jobs se valida entera antes de crear ninguno; si una creación falla, se
  borran los creados en esa llamada.
