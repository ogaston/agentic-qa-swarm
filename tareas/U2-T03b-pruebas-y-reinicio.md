# U2-T03b — Candados de prueba y reinicio idempotente de `go-warm-manager` (seguimiento de U2-T03)

**Unidad:** U2 — Orquestación & Warm Sandbox Lifecycle
**Historias que implementa:** US-M2 (deploy sobre el warm; fail-closed tras 2 reintentos)
**Depende de:** U2-T03 **fusionada** (PR #41). Cierra los hallazgos abiertos de `revisiones/U2-T03/ronda-3.md` (F-01 NARANJA de pruebas y F-03 (b), más pequeños). **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea, concreta):

> Añadir las pruebas que matan las mutaciones que seguían vivas en U2-T03 (timeout de Job tratado como éxito, `InferSurface` con deploy `pending`, `ResolveOrphans` sin cablear), hacer que `StartDeploy` consulte los Jobs antes de tomar el warm (un `run_id` ya `done` no emite un `deploy.failed` falso tras un reinicio) y limpiar los defectos menores señalados, todo en `services/go-warm-manager/`.

Detalle:

- **Pruebas (sin tocar producción salvo el refactor mínimo de `run` en `main.go` para poder inyectar un clientset):**
  - **X20.** Un Job que se queda `pending` hasta `JobTimeout` produce `deploy.failed` + 1 handoff + 1 Job; si `waitJob` devolviera `JobSucceeded` ante el timeout, la prueba falla.
  - **X4.** `InferSurface` con el deploy de la corrida `pending` devuelve `ErrNotReady`/409 (no solo «nunca desplegado» y «fallido»).
  - **X14/X15.** `run` llama a `ResolveOrphans` al arrancar y **no ignora su error**: con un clientset que contiene un Job huérfano en vuelo, tras el arranque hay un `deploy.failed` + handoff para ese `run_id`; con `ResolveOrphans` fallando, el servicio no empieza a servir.
  - Menores: idempotencia en memoria de `StartDeploy` (Y25), `ErrNotReady`→409 de `POST /surface` (Y28), `probe.HTTP` no sigue redirecciones (Y27), `WARM_S3_SECURE` seguro por defecto (X13, extrayendo la lógica a una función).
- **`StartDeploy` idempotente de verdad (F-03 b).** Antes de `takeWarm` consulta `DeployState` (memoria **y** Jobs por `aqs.io/run-id`): si la corrida ya tiene deploy `done` o `failed` en los Jobs, devuelve ese estado sin tocar el warm ni emitir eventos. Rojo primero con `TestScratchReplayAfterRestart` del informe (corrida `done`, reset a `ready`, reinicio, mismo `run_id` → hoy emite `deploy.failed` + handoff).
- **Limpieza.** Sustituir `TrackKnownForTest` (método exportado de producción solo para pruebas) por `export_test.go`; quitar `var _ = errors.Is`; el README dice `WARM_JOB_TIMEOUT` 11m (como el código); la función `up` de la bitácora debe ser **literalmente ejecutable** (con `WARM_ALLOW_FAKE_KUBE=true`, `WARM_OBJECT_STORE=file`, `WARM_ALLOW_FILE_STORE=true`), probada tal cual.
- **Trace de huérfanos.** `ResolveOrphans` usa un `trace_id` no nulo (generado una vez por arranque).

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Titular del warm (`run_id`) en `WarmState` (cambia `contracts/plans/`; candidata para que `InferSurface` rechace corridas ajenas), persistir la decisión de huérfano en una anotación del Job, cancelar el Job en vuelo en `ResolveOrphans`, reanudar la observación de Jobs en vuelo, presupuesto de reintentos persistido, validación OCI estricta, volumen para el outbox.
- Imagen del deployer, ServiceAccount y RBAC: **U2-T07** (con su decisión pendiente).
- Modificar `contracts/**`, `deploy/**`, `policy/**` o workflows.
- Comandos contra un clúster o la nube.

---

## Archivos de contexto

- `revisiones/U2-T03/ronda-3.md` (mutaciones X4, X14, X15, X20 y reproducciones `TestScratch*`; el scratchpad puede no existir: reproduce desde la descripción), `ronda-2.md`, `bitacoras/U2-T03.md`
- `services/go-warm-manager/` (`service.go`, `cmd/go-warm-manager/main.go`, `adapters/kube/`, `README.md`)
- `tareas/U2-T03-warm-manager.md`

---

## Criterios de aceptación

Desde la raíz del worktree.

- [ ] **CA-1** — Pruebas en verde con `-race` y lo anterior intacto.
  ```bash
  cd services/go-warm-manager && go test -race -count=1 -v ./... | grep -c -E '^\s*--- PASS'; go test -race -count=1 ./... 2>&1 | grep -c FAIL
  ```
  Esperado: ≥ `165` (los 157 de U2-T03 más ≥ 8) y `0`.

- [ ] **CA-2** — **Las cuatro mutaciones vivas ahora mueren**, con la mutación aplicada en una copia y el rojo pegado en la bitácora (rojo → verde al revertir).
  ```bash
  cd services/go-warm-manager && go test -count=1 -run 'JobTimeout|SurfaceRequiresFinishedDeploy|ResolveOrphansWired|StartupFailsWhenOrphans' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS` en al menos cuatro pruebas (X20, X4, X14, X15); y en la bitácora, cada mutación (`waitJob` devuelve `JobSucceeded` ante el timeout; `d.State != "done"` → `d.State == "failed"`; quitar la llamada a `ResolveOrphans` en `run`; ignorar su error) con su salida roja.

- [ ] **CA-3** — **Reinicio idempotente**: un `run_id` ya terminado no genera eventos falsos.
  ```bash
  cd services/go-warm-manager && go test -race -count=1 -run 'StartDeploy(Idempotent|AfterRestart|ConsultsJobs)' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS`; tras reiniciar, un `POST /deploys` con un `run_id` `done` en los Jobs responde con el estado real, no emite `deploy.failed` ni handoff y no toma el warm; con uno `failed`, devuelve `failed` sin Job nuevo.

- [ ] **CA-4** — La `up` de la bitácora arranca tal cual y el servicio responde.
  ```bash
  # el codificador pega la función `up` literal en la bitácora y la ejecuta sin retoques
  ```
  Esperado: `401 401 400 409`, `0`, `0` (como CA-5 de U2-T03) con **esa** función, sin variables añadidas a mano.

- [ ] **CA-5** — Limpieza y documentación.
  ```bash
  grep -rn 'TrackKnownForTest' services/go-warm-manager --include=*.go | grep -v _test.go | wc -l; grep -rn 'var _ = errors.Is' services/go-warm-manager | wc -l; grep -c '11m' services/go-warm-manager/README.md
  ```
  Esperado: `0`, `0` y ≥ `1`.

- [ ] **CA-6** — Higiene, imagen y alcance.
  ```bash
  (cd services/go-warm-manager && go vet ./... && go vet -tags minio ./... && test -z "$(gofmt -l .)" && echo ok); docker build -q -t aqs-go-warm-manager:ci services/go-warm-manager >/dev/null && docker inspect aqs-go-warm-manager:ci --format '{{.Config.User}}'; git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/go-warm-manager/|bitacoras/U2-T03b.md|revisiones/U2-T03b/)' | wc -l
  ```
  Esperado: `ok`, `65532:65532`, `0` y `0`.

---

## Plan de pruebas

- `kubernetes/fake` con Jobs preexistentes (done, failed, en vuelo) para los reinicios; reloj inyectable para `JobTimeout`.
- **Barrido de clase obligatorio** (la familia de U2-T03: valores por defecto optimistas, operaciones no atómicas, errores tragados, estado solo en memoria, pruebas que no ejercen la invariante): repetir el barrido de mutaciones del informe de la ronda 3 (X1–X20, Y25–Y32) y declarar en la bitácora cuáles siguen vivas y por qué son equivalentes.

**Rojo primero:** registrar `TestScratchReplayAfterRestart` fallando contra `main`, y las cuatro mutaciones sobreviviendo con la suite actual.

---

## Notas

- **Tope de 3 rondas** (decisión del humano). En cada NO-VERDE, barrido de la clase del defecto.
- **Go y Dockerfile.** `go 1.26.8`; el `Dockerfile` ya existe. Podman: montajes con `:z`; `ajv`/`yq` vía `npx`/imagen.
- **Candidatas a registrar, no hacer:** titular del warm en el estado; decisión de huérfano persistida y cancelación del Job; reanudar la observación; presupuesto de reintentos por `run_id`.
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
