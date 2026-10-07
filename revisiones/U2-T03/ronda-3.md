# Ronda 3 (última) — U2-T03

VEREDICTO: NO-VERDE (residual mínimo: 1 NARANJA, solo pruebas. El código de producción es aceptable para fusionar en desarrollo; ver «Aceptabilidad»)

Worktree `agentic-qa-swarm-wt-U2-T03`, sha 3d6263e. Quedó limpio (`git status --short | wc -l` = 0). Todas mis pruebas y mutaciones las hice en una copia hecha con `git archive` en el scratchpad (`scratchpad/r3`, `old`, `side`, `mut.py`, `mut2.py`).

## Criterios de aceptación, verificados por mí (frescos, `-count=1`)
| # | Comando que corrí | Resultado |
|---|---|---|
| 1 | `go test -race -count=1 -v ./...` | pasa: 157 PASS, 0 FAIL (`TestEventsDump` se salta salvo con `WARM_DUMP_EVENTS_DIR`; suite de segundos) |
| 2 | `go test -count=1 -run 'Deploy(Retry\|Exhausted\|Failed)' -v ./...` | pasa (exactamente 3 Jobs leídos del clientset falso) |
| 3 | build + `render-deploy-job`, kubeconform y conftest por podman con `:z` | pasa: `Valid: 1, Invalid: 0`; `23 passed, 0 failures`; `latest rc=1`; `registro rc=1` |
| 4 | volcado + `npx ajv-cli@5.0.0 ajv-formats@3.0.1 --spec=draft2020 -c ajv-formats` | pasa: los 4 eventos dan `valid` |
| 5 | `up` con `WARM_ALLOW_FAKE_KUBE=true WARM_OBJECT_STORE=file WARM_ALLOW_FILE_STORE=true` | pasa: `401 401 400 409`, `0`, `0`, y `POST /surface` sin deploy da 409 |
| 6 | `go test -count=1 -tags minio -run ObjectStoreS3 -v ./...` | pasa: `--- PASS: TestObjectStoreS3 (0.77s)` con MinIO real |
| 7 | los tres `env ... wm` | `rc=1`, `rc=1`, `rc=1` |
| 8 | `go list -deps`, `docker build`, `list-services.sh` | `192`, `65532:65532`, `1` |
| 9 | `go vet` (con y sin `-tags minio`), `gofmt -l`, `git status`, diff contra el merge-base | `ok`, `0`, `0` |

El dominio no importa `client-go`, `minio` ni `net/http` (comprobado con `go list -f '{{.Imports}}'`).

## Los cuatro NARANJA de la ronda 2: cerrados
- **F-01 (superficie fail-closed).**
  - App inalcanzable: `ErrSurfaceUnreachable`, API 502, sin evento ni objeto, y los errores de sondeo se loguean con `run_id`, `trace_id` y `path`.
  - Sin endpoints: `ErrNoSurface`, API 502.
  - La prueba de paridad `TestValidateSurfaceMatchesSchema` compara contra el esquema real con jsonschema. Los 11 casos coinciden, y el validador de ejecución solo puede ser más estricto.
  - Con el binario:
    - `WARM_APP_URL` rechazada con `warm-app:8080`, `ftp://x`, `http://`, `//x` y `http://u:p@h`.
    - `WARM_OBJECT_STORE` vacío da rc=1.
    - `file` sin `WARM_ALLOW_FILE_STORE=true` da rc=1, y `TRUE` en mayúsculas también.
    - `gcs` da rc=1.
    - `s3` sin credenciales da rc=1.
  - La barrera de `file` contra clúster/prod no se puede aislar con el binario: la barrera de `fake` salta antes y `incluster` exige un clúster real. Queda cubierta por `TestFileStoreFencesAndExplicitStore` y mis mutaciones X8 y X9, que mueren.
- **Decisión de «deploy `done` y warm `dirty`».** Ratificada. Rechaza el warm `ready` sin deploy, `cuarentena` e `idle-escalado`. Mi mutación X3 (quitar la exigencia de `dirty`) muere.
  - Camino con un warm `dirty` por otra causa: ese warm solo se alcanza con un deploy de otra corrida o un reset, y el deploy de la corrida propia debe ser `done`. Queda en AMARILLO (F-03 a).
- **F-02 (ensure idle→ready).** 8 `ensure` concurrentes dan 1 `ScaleUp` y 0 errores. Con DOS `Service` sobre un mismo store, 12 `ensure` dan 0 malos y 2 `ScaleUp` (uno por instancia, idempotente). Repetido 30 veces con `-race`, sin fallos.
- **F-03 (cierre y deploys huérfanos).**
  - El cierre espera a `Shutdown`.
  - `GET /deploys` deriva el estado de los Jobs.
  - `ResolveOrphans` se llama al arrancar, antes de servir.
  - **SIGTERM, sí demostrado con el binario.** Hice una petición `POST /deploys` con el cuerpo a medias y envié `kill -TERM`.
    - Binario nuevo: tras la señal le entrego el resto del cuerpo, responde `409` completo, el proceso sigue vivo y sale con rc=0.
    - Binario de 8ece5c2: respuesta vacía (cortada).
    - Esta prueba, más `TestServeWaitsForInFlightRequestOnShutdown` con `http.Server` real, es suficiente.
- **F-04 (mutaciones vivas).** MA, M6 (dos `Service` sobre un mismo store), MD, MD3 (otro error de CAS), N8, P1, P2, P3 y P4 mueren. Mis 21 mutaciones nuevas más las del barrido mueren salvo las listadas en F-01.

## Hallazgos

### F-01 · NARANJA · `service.go` (`waitJob`, `InferSurface`), `cmd/go-warm-manager/main.go` (`run`) · Mutaciones vivas sobre fail-closed nombrados (solo pruebas)
Misma clase que F-04 de la ronda 2. La bitácora declara cubierto el barrido y siguen vivas estas mutaciones, con un `go test ./...` completo en verde. Mi propio comportamiento esperado sí se cumple en el código (lo comprobé con pruebas en `scratch2_test.go`). Falta el candado.
- **X20 · timeout de Job tratado como éxito.**
  - Mutación: `waitJob` devuelve `JobSucceeded, nil` en vez de `timeout esperando al Job`. Resultado: `deploy.done` para un Job que nunca terminó.
  - Ninguna prueba hace que un Job se quede `pending` hasta `JobTimeout`. `grep JobPending` solo aparece en las pruebas de `DeployState` y `ResolveOrphans`.
  - Es el análogo exacto de N8 (timeout → 200), que yo califiqué NARANJA.
  - Mi prueba que lo fija, con el código real: `Outcome=pending` siempre da `deploy.failed`, 1 Job y 1 handoff.
- **X4 · `InferSurface` acepta un deploy `pending`.**
  - Mutación: `d.State != "done"` → `d.State == "failed"`. Sobrevive.
  - `TestSurfaceRequiresFinishedDeployAndTakenWarm` prueba «nunca desplegado» y «deploy fallido», no «en curso».
  - Con un deploy `pending` el warm ya está `dirty`, así que esa condición es la única guarda.
- **X14 y X15 · `ResolveOrphans` en el arranque.**
  - Quitar la llamada en `run`, o ignorar su error, deja todo en verde.
  - La corrección de F-03 (b) solo está probada a nivel de dominio. El cableado no, y el binario en modo fake no tiene Jobs previos para demostrarlo.
  - Arreglo esperado: refactor mínimo de `run` para inyectar un clientset con un Job huérfano, o prueba equivalente.
- **Arreglo esperado (3 pruebas, sin tocar producción):** una prueba de `Pending` perpetuo contra `JobTimeout` con `deploy.failed`, una de `InferSurface` con deploy `pending` y una de cableado de `ResolveOrphans` en `run`.

### F-02 · AMARILLO · varios · Mutaciones vivas menores (no bloquean)
- **Y25.** La idempotencia en memoria de `StartDeploy` (devolver el estado existente por `run_id`) no tiene prueba.
- **Y28.** El mapeo `ErrNotReady` → 409 de `POST /surface` no tiene prueba.
- **Y27.** `probe.HTTP` no tiene prueba de que no sigue redirecciones.
- **Y32.** `outbox.Sync` no se puede verificar en pruebas.
- **X13.** `WARM_S3_SECURE` seguro por defecto: el README y la respuesta lo afirman, pero no hay prueba, porque la lógica está inline en `run`.

### F-03 · AMARILLO (candidatas; defectos reales en producción) · Aristas de reinicio y reintento, fail-closed o de datos acotados
No las llamo NARANJA porque cada una exige un segundo evento anormal además del reinicio.
- **(a) `InferSurface` no distingue quién tiene el warm.** `WarmState` no lleva titular (el esquema está fuera de alcance).
  - Reproducido (`TestScratchStaleRun`): run-a `done`, reset, run-b desplegada con otro artefacto; `InferSurface(run-a)` devuelve 200 y publica `surface.ready` con la superficie de la app de run-b.
  - Requiere que el único llamante autenticado, el controlador, pida la superficie de una corrida ya terminada y reseteada.
  - Arreglo: exigir que sea el último deploy exitoso (recordarlo y derivarlo de los Jobs por fecha) o llevar el titular del warm a `WarmState`.
- **(b) El `StartDeploy` idempotente solo mira la memoria; `GET` ya mira los Jobs.**
  - Reproducido con el clientset falso real (`TestScratchReplayAfterRestart`): run `done`, reset a `ready`, reinicio, `POST /deploys` con el mismo `run_id`.
  - Responde 202 `pending`, toma el warm (`dirty`), el Job ya existe (`AlreadyExists`), y emite `deploy.failed` más 1 handoff para una corrida que terminó bien.
  - Con el warm `dirty` devuelve un 409 engañoso. Es fail-closed, no despliega nada.
  - Arreglo de 3 líneas: consultar `DeployState` (memoria + Jobs) antes de `takeWarm`.
- **(c) Un huérfano en vuelo se da por `failed` y puede terminar bien.**
  - Evaluación pedida: **es fail-closed y coherente**, pero puede ser un falso `deploy.failed` (el Job sigue y termina bien). El warm queda `dirty` esperando reset, y no hay riesgo de dato ni de desplegar encima.
  - Dos matices reproducidos (`TestScratchOrphanFlipFlop`):
    - Tras un SEGUNDO reinicio, el estado derivado de los Jobs es `done`. Se emite `deploy.done` después de `deploy.failed`, y `InferSurface` pasa. La decisión de huérfano no se persiste (candidata: anotación en el Job).
    - `ResolveOrphans` no cancela el Job en vuelo. Si `go-reset` actúa antes de que termine, el Job puede parchear `warm-app` después del reset.
- **(d) Otros.**
  - Huérfano con Jobs fallidos y reintentos sin agotar: emite `deploy.failed` sin handoff.
  - Un reinicio entre `takeWarm` y `Create` deja el warm `dirty` sin Job ni evento. La ventana es de milisegundos.
  - `ResolveOrphans` usa `trace_id` todo ceros.
  - `HTTP://x` pasa `validateAppURL` pero luego `ValidateSurface` lo rechaza en cada llamada, que es fail-closed.
  - Con `Deployment` de 1 réplica y `RollingUpdate`, durante un despliegue conviven dos pods.
- **Cosas pequeñas.**
  - `TrackKnownForTest` es un método exportado de producción solo para pruebas.
  - `var _ = errors.Is` quedó suelto.
  - El README dice `WARM_JOB_TIMEOUT` 10m y el código usa 11m.
  - El bloque `up` de la bitácora (línea 54, «copia literal») NO arranca: lo corrí tal cual y da rc=1 por `WARM_ALLOW_FAKE_KUBE`. La versión exacta que funciona no aparece en ninguna parte; solo hay una nota en prosa (líneas 207 y 238).

## Aceptabilidad
Lo que queda es **aceptable para fusionar en un entorno de desarrollo** si el humano acepta la deuda de pruebas de F-01.
- No encontré ningún camino fail-open alcanzable con un solo evento normal.
- Todo lo de F-03 es fail-closed o exige un llamante fuera de orden o dos fallos apilados.
- El README documenta las limitaciones principales (estado en memoria más Jobs, tabla sin límite, truncado de 2 MiB).

Para pasar a VERDE basta cerrar F-01 con 3 pruebas, sin tocar producción. Recomiendo además F-03 (b) (3 líneas) y registrar F-03 (a) y (c) como candidatas de U2-T02, U2-T06 y U2-T07. La imagen del deployer, la SA y el RBAC siguen siendo de U2-T07; el RBAC existente ya concede jobs, configmaps y pods a `go-warm-manager`.

## Tareas candidatas (fuera de alcance)
- Titular del warm (`run_id`) en `WarmState` o anotación en el ConfigMap, para que `InferSurface` rechace corridas ajenas (contracts, U2-T06).
- Persistir la decisión de huérfano (anotación del Job) y cancelar el Job en vuelo en `ResolveOrphans` (U2-T07).
- Reanudar la observación de Jobs en vuelo en vez de darlos por indeterminados.
- Presupuesto de reintentos persistido por `run_id`.
- Política Rego de Jobs generados y OpenAPI de la API.

VEREDICTO: NO-VERDE
NARANJA|service.go waitJob/InferSurface + main.go run|Mutaciones vivas sobre fail-closed nombrados: timeout de Job tratado como éxito (X20), InferSurface con deploy pending (X4) y ResolveOrphans no cableado/ignorado en run (X14/X15), solo pruebas
INFORME: revisiones/U2-T03/ronda-3.md
