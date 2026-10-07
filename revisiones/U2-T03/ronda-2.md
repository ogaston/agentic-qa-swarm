# Ronda 2 — U2-T03

VEREDICTO: NO-VERDE

Worktree `agentic-qa-swarm-wt-U2-T03`, sha 8228f38. Los cinco NARANJA de la ronda 1 quedaron corregidos de verdad y los CA-1..CA-9 pasan corridos por mí. Mi barrido de la familia (valores por defecto optimistas, operaciones no atómicas, errores tragados, pruebas que no ejercen la invariante) encontró 4 NARANJA nuevos que la ronda 1 no cubrió y que la bitácora declara cubiertos. El worktree quedó limpio (`git status --short | wc -l` = 0). Todas mis pruebas y mutaciones las hice en una copia en el scratchpad, hecha con `git archive`.

## Criterios de aceptación, verificados por mí (frescos, `-count=1`)
| # | Comando que corrí | Resultado |
|---|---|---|
| 1 | `go test -race -count=1 -v ./...` | pasa: 118 PASS, 0 FAIL; `TestEventsDump` se salta salvo con `WARM_DUMP_EVENTS_DIR`; el tiempo de la suite es de segundos, no de milisegundos |
| 2 | `go test -count=1 -run 'Deploy(Retry\|Exhausted\|Failed)' -v ./...` | pasa; el test de kube lee la lista de Jobs del clientset (exactamente 3) |
| 3 | build + `render-deploy-job`, kubeconform y conftest por podman con `:z` | pasa: `Valid: 1, Invalid: 0`; `23 passed, 0 failures`; `latest rc=1`; `registro rc=1` |
| 4 | volcado con `WARM_DUMP_EVENTS_DIR` + `npx ajv-cli@5.0.0 ajv-formats@3.0.1 --spec=draft2020 -c ajv-formats` | pasa: `warm.ready`, `deploy.done`, `deploy.failed` y `surface.ready` dan `valid` |
| 5 | función `up` de la bitácora, más `WARM_ALLOW_FAKE_KUBE=true` | pasa: `401 401 400 409`, `0`, `0` |
| 6 | `go test -count=1 -tags minio -run ObjectStoreS3 -v ./...` | pasa: `--- PASS: TestObjectStoreS3 (0.80s)` con MinIO real |
| 7 | los tres `env ... wm` de la tarea | pasa: `rc=1`, `rc=1`, `rc=1` |
| 8 | `go list -deps`, `docker build`, `list-services.sh` | pasa: `192`, `65532:65532`, `1` |
| 9 | `go vet` (con y sin `-tags minio`), `gofmt -l`, `git status`, diff contra el merge-base | pasa: `ok`, `0`, `0` |

## Hallazgos de la ronda 1 que verifiqué como cerrados
- **F-01 de la ronda 1.** La semilla sin ConfigMap es `dirty` con `reset_verified=false`.
  - Con el binario: `WARM_KUBE=fake` sin `WARM_ALLOW_FAKE_KUBE=true` da rc=1. `TRUE` en mayúsculas también da rc=1.
  - `KUBERNETES_SERVICE_HOST` da rc=1. `WARM_ENV=PROD` y ` Production ` dan rc=1. `staging` arranca.
  - Las mutaciones N3, N4, N5 y M5 mueren.
  - Sobrevive solo N6: `WARM_FAKE_STATE` con `reset_verified=true` forzado para cualquier estado. Solo afecta al fake y es AMARILLO.
- **F-02 de la ronda 1.** `takeWarm` usa mutex más CAS.
  - Mis dos reproducciones dan 1 Job. Cinco `StartDeploy` concurrentes dan 1 `deploy.done` y 4 `deploy.failed`.
  - Un `Status` que falla una vez da 1 Job y `deploy.done`.
  - Un `Status` pendiente para siempre da `deploy.failed` con 1 Job y handoff, sin Job extra.
  - Con CAS perdido, o con `Create` fallido, no hay segundo Job. El CAS ocurre antes del `Create`, así que no existe un CAS que pierda después de crear el Job.
- **F-03 de la ronda 1.** No hay `_ =` en los caminos revisados y el trace es uno por petición. El 409 es síncrono.
- **F-04 de la ronda 1.** Pasan 0, `-1`, `-5s`, `abc`, `0s`, `1x` y `30` sin unidad, para las tres variables: todos dan rc=1.
- **F-05 de la ronda 1.** Repetí mis mutaciones y mueren: Deploy sin `reset_verified` (M1), registro por sufijo (M2) y token vacío (M3). Ver F-04 de esta ronda para las que siguen vivas.

## Hallazgos

### F-01 · NARANJA · `service.go` (`InferSurface`), `cmd/go-warm-manager/main.go` (`WARM_APP_URL`, `WARM_OBJECT_STORE`) · Salidas optimistas en superficie y almacén
- **App inalcanzable.**
  - Con la app inalcanzable (todos los `Get` fallan), `InferSurface` devuelve `Endpoints: []` con `source: "probe"`, publica `surface.ready` con `endpoint_count: 0` y la API responde `200`.
  - No deja ninguna línea de log: `continue` traga el error en todos los `Get`.
  - Con el binario (`WARM_KUBE=fake`, la app no resuelve) obtuve `200`, un `surface.ready` en el outbox y nada en `wm.log` aparte de «escuchando».
  - Una app caída es indistinguible de una app sin superficie. Aguas abajo se generarán flujos contra nada.
- **Estado del warm.** `InferSurface` tampoco mira el estado del warm ni el del deploy de la corrida (probé con el warm `dirty`).
- **Esquema.** La tarea pide validar contra `surface-artifact.schema.json`. No hay validación en tiempo de ejecución, solo en pruebas.
  - `WARM_APP_URL=warm-app:8080` arranca sin queja (rc=124 con timeout, es decir, vivo).
  - Con esa URL el servicio publica un `base_url` que viola el `pattern ^https?://` del esquema. Es una configuración inválida que no impide el arranque.
- **Almacén por defecto.** `WARM_OBJECT_STORE` vacío cae en `file` (`/tmp/warm-objects`) y `WARM_S3_SECURE` por defecto es `false`.
  - En un pod real, olvidar la variable publica `surface.ready` con un URI `file://` que ningún otro servicio puede leer.
  - Es la misma clase que `WARM_KUBE=fake` de la ronda 1: un adaptador de pruebas como valor por defecto sin barrera.
- **Arreglo esperado:** fallar con error (502) si ningún sondeo responde, loguear los errores de sondeo, validar `base_url` y endpoints antes de guardar, validar `WARM_APP_URL` en el arranque, y exigir `WARM_OBJECT_STORE` explícito o rechazar `file` dentro de un clúster. Añadir pruebas de cada uno.

### F-02 · NARANJA · `service.go` (`EnsureWarmReady`, rama `idle-escalado`) · `ensure` concurrente desde `idle-escalado` no es atómico
- Repro (`TestRV_EnsureIdleConcurrent`, 4 `ensure` concurrentes, store con CAS comparando): 1 gana, 3 devuelven `el estado del warm cambió concurrentemente`, y hubo 4 `ScaleUp`.
  - La API mapea ese error a `503 ensure_failed`, aunque el warm ya quedó `ready`.
  - Un cliente cuyo timeout sea menor que la espera de ensure (hasta 120 s) reintenta y provoca la misma carrera.
- La bitácora afirma «idle→ready en ensure ahora con CAS» como barrido cubierto. El CAS existe, pero su conflicto se expone como error en vez de releer el estado.
- La mutación MD (ignorar el error del CAS en esa rama) sobrevive. Ninguna prueba cubre la concurrencia de `ensure`.
- **Arreglo esperado:** ante `ErrStateConflict`, releer el estado y continuar la evaluación (o serializar con mutex como `takeMu`). Añadir una prueba concurrente.

### F-03 · NARANJA · `cmd/go-warm-manager/main.go` (cierre), `service.go` (`deploys` en memoria) · Un deploy en vuelo queda huérfano sin rastro al reiniciar o recibir SIGTERM
- **Estado del reinicio.**
  - El estado `dirty` sí persiste en el ConfigMap, así que tras el reinicio no se crea ningún Job nuevo (lo reproduje: 409 y 1 Job). El fail-closed se sostiene.
  - Pero la tabla `deploys` es solo memoria: tras el reinicio, `GET /deploys/{run}` da `404` y nadie emitirá jamás `deploy.done` ni `deploy.failed`.
  - El Job en vuelo sigue ejecutándose sin que nadie lo observe. El controlador espera un evento que no llegará.
- **Cierre.** `ListenAndServe` vuelve de inmediato al llamar a `Shutdown`, `run` retorna y el proceso sale mientras `Shutdown` espera.
  - Repro con el binario: `POST /surface` contra una app lenta, `kill -TERM` al segundo, y `curl` obtiene `rc=52` (respuesta vacía). La ventana de 5 s nunca se usa.
- **Arreglo esperado (mínimo):**
  - Esperar a que `Shutdown` termine antes de salir.
  - Hacer que `GET /deploys/{run}` derive el estado de los Jobs por la etiqueta `aqs.io/run-id` cuando no esté en memoria, o emitir `deploy.failed` más handoff para los deploys huérfanos al arrancar.
  - Documentar en el README que el estado es en memoria.

### F-04 · NARANJA · pruebas (`kube_test.go`, `service_test.go`, `api_test.go`) · Siguen vivas mutaciones sobre invariantes que la tarea nombra
Mutaciones que corrí en la copia (verde con la suite actual):
- **MA.** El CAS del adaptador kube sin `resourceVersion` (`cm.ResourceVersion = ""` antes del `Update`).
  - Es el mecanismo que protege entre réplicas. `TestStateStoreCompareAndSwapConflictFromAPIServer` usa un reactor que siempre devuelve conflicto y no comprueba que el `Update` lleve el `resourceVersion` leído.
- **M6 aislada** (CAS del `MemState` sin comparar). Su justificación de «defensa en profundidad equivalente» es falsa.
  - El mutex `takeMu` solo serializa dentro de una instancia de `Service`; el CAS es lo que protege entre instancias (réplicas, reinicio) y en el `ensure` idle→ready, que no toma el mutex. La mutación MC (quitar el mutex dejando CAS) sí muere en kube; el contrario, no.
  - No hay ninguna prueba con dos `Service` sobre un mismo almacén.
- **MD.** Ignorar el error de CAS en `ensure` idle→ready (ver F-02).
- **N8.** `ErrWarmTimeout` mapeado a `200` en vez de `409`.
  - Es un fail-open: el controlador creería listo un warm que no llegó a Ready. La API no tiene prueba de ese camino.
- **P1–P4.** Ignorar el error de `Publish` en `ensure`, el de `ScaleUp`, el de `Objects.Put` en `InferSurface` y el de `Publish` de `surface.ready`. Todo eso queda en verde: son los «errores tragados» de la ronda 1 sin prueba que los fije.
- **Arreglo esperado:** pruebas que maten MA (reactor que valide el `resourceVersion` del Get), M6 (dos servicios sobre un mismo store), MD, N8 y P1–P4. Pegar el rojo en la bitácora.
- **M4 (sin comprobar namespace en `Jobs.Create`).** Sobrevive, y su justificación es cierta: el clientset falso y el apiserver real rechazan el namespace ajeno, y el namespace vacío cae igualmente en `aqs-test`. Lo acepto como equivalente.

### F-05 · AMARILLO · varios · No bloquean
- **`run_id` pegajoso.**
  - Un run que perdió la carrera de `takeWarm` queda `failed` para siempre. Un nuevo POST con el mismo `run_id` devuelve `202 failed` sin Job, aun con el warm `ready` otra vez (repro `TestRV_StickyFailed`).
  - Es coherente con «failed es terminal», pero documentarlo. La tabla `deploys` crece sin límite.
- **Acoplamiento de timeouts.** `WARM_JOB_TIMEOUT` (10 min) es igual a `activeDeadlineSeconds` (600 s fijo).
  - Un Job atascado en Pending no se clasifica nunca como `failed`, sino como «indeterminado», y no usa los reintentos. El `reason` dice «no se pudo leer el Job: timeout esperando…», que es engañoso.
  - Con `WARM_JOB_TIMEOUT` < 600 s, el Job sigue vivo después del handoff. Validar JobTimeout > deadline, o derivar el deadline.
- **Duraciones sin cota.** `WARM_READY_TIMEOUT=999999h` y `WARM_POLL_INTERVAL=1ns` arrancan. Una espera cercana al máximo de `time.Duration` desbordaría `+30s` en `WriteTimeout`.
- **`probe.HTTP` trunca en 2 MiB en silencio.** Un OpenAPI mayor cae al sondeo sin aviso.
- **`warm.ready` por llamada.** El `event_id` de `ensure` incluye `UnixNano`, así que cada llamada publica un `warm.ready` nuevo (3 ensures → 3 ids) y la idempotencia del outbox no aplica.
- **Sondas caídas.** `ensure` no deja rastro cuando la sonda está caída (409 sin `reason` ni log).
- **Deployer sin configurar.** `WARM_DEPLOYER_IMAGE` sin configurar usa la imagen placeholder, que no existe. El fallo es ruidoso (handoff) y está marcado como PLACEHOLDER en `jobspec.go` y el README, pero conviene exigirla fuera de pruebas.
- **Bitácora.** La función `up` de CA-5 «copia literal» ya no es literal (le falta `WARM_ALLOW_FAKE_KUBE=true`).

## Juicios pedidos
- **Placeholder `aqs-warm-deployer:0.1.0`:** aceptable. Está marcado en código y README, pasa las políticas y se puede configurar por entorno. Falla ruidosamente, no en silencio.
- **Eliminación del puerto `ContainerRuntime`:** aceptable como código muerto, y está declarado en `ports.go` y en la bitácora. Pero era un puerto de la tarea (alcance y plan de pruebas), así que el orquestador debe ratificar el cambio en `DECISIONES-LOOP.md` o en la tarea.
- **`internal/fakes`:** correcto. No entra al binario (verificado con `go list -deps`). El binario sí incluye `client-go/kubernetes/fake`, acotado por las barreras de entorno; es necesario para CA-5 y lo acepto.

## Tareas candidatas (fuera de alcance)
- T07: `aqs-warm-deployer`, la SA `warm-deployer` y el RBAC de `go-warm-manager`. `kube.StateStore` necesita get/update/create sobre configmaps, además de jobs, pods y scale de deployments y statefulsets.
- Persistir el presupuesto de reintentos y el estado de deploy por `run_id` (anotaciones de los Jobs o un ConfigMap).
- Política Rego para Jobs generados y OpenAPI de la API REST. Ya están en la bitácora.

VEREDICTO: NO-VERDE
NARANJA|service.go InferSurface + main.go WARM_APP_URL/WARM_OBJECT_STORE|Superficie fail-open: app inalcanzable da 200 + surface.ready con 0 endpoints sin log; sin validación de esquema ni de WARM_APP_URL; almacén `file` por defecto
NARANJA|service.go EnsureWarmReady (idle-escalado)|ensure concurrente no atómico: el conflicto de CAS sale como 503 y hay N ScaleUp aunque el warm quedó ready
NARANJA|main.go cierre + service.go deploys en memoria|SIGTERM corta peticiones en vuelo y un deploy en vuelo queda huérfano (404 y ningún evento terminal) tras reiniciar
NARANJA|kube_test.go / service_test.go / api_test.go|Mutaciones vivas sobre invariantes con nombre (CAS sin resourceVersion, M6 con justificación falsa, ensure idle sin CAS, timeout→200, errores de Publish/ScaleUp/Put sin prueba)
INFORME: revisiones/U2-T03/ronda-2.md
