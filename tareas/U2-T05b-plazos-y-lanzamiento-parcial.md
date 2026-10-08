# U2-T05b — Runners: plazo por corrida, lanzamiento parcial y pruebas de invariantes (seguimiento de U2-T05)

**Unidad:** U2 — Orquestación & Warm Sandbox Lifecycle
**Historias que implementa:** US-M6 (runners sobre el warm; ningún Job colgado; evidencia fiable)
**Depende de:** U2-T05 **fusionada** (PR #50). Cierra los hallazgos abiertos de `revisiones/U2-T05/ronda-1.md` (F-01, F-02, F-03 NARANJA y los AMARILLOS baratos). **Debe estar fusionada antes de integrar con U3 (U2-T08)**: hoy en modo `real` la fase `run` falla cerrada por falta de `FlowPlan`, por eso los defectos no son alcanzables todavía. **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano. **Versión mínima a propósito.**

---

## Alcance

**Dentro** (una línea, concreta):

> En `services/go-run-controller/runner/` y `runctl/`: que el plazo por corrida no dependa de Jobs que se borran, que un lanzamiento parcial no deje Jobs vivos, y que cada invariante declarada en U2-T05 tenga una prueba que mate su mutación.

Detalle:

- **F-01 — plazo por corrida.** Hoy el origen del plazo es el `created-at` más antiguo entre los Jobs **vivos** y el controlador borra los Jobs vencidos, así que el origen avanza. Arreglo simple: no borrar un Job vencido hasta que su flujo esté resuelto (`settle` con éxito: evidencia escrita y marcador al final), y medir el plazo de la corrida desde un origen que persista (el `created-at` del primer Job de la corrida, que ya no se borra, o una marca escrita una sola vez en el almacén de evidencia: elegir lo más simple y justificarlo). Esto cierra además el AMARILLO «si la escritura de evidencia falla, el Job se borra y el estado vive solo en memoria».
- **F-02 — lanzamiento parcial.** Construir y validar **todos** los Jobs de la ola (`Exec.Plan` + `BuildJob`, ambos puros) **antes** de crear ninguno; y ante un error de `CreateJob`, borrar de forma best-effort los Jobs creados en esa llamada. Un `flow_id` inválido en el 2.º flujo no puede dejar el 1.º vivo.
- **F-03 — pruebas que matan mutaciones** (mínimo exigible), cada una con su mutación repetida y su rojo pegado en la bitácora:
  1. con 2 flujos y uno sin evidencia, **no** se publica `run.done` (mutación: `len(URIs)==0` como única condición);
  2. el error de `Publish` de `run.done` no se traga y se reintenta (mutación: ignorarlo);
  3. publicación única de `run.done` tras un reinicio entre la publicación y la salida de `running` (mutación: quitar la guarda `DonePublish`);
  4. `logs.txt` se escribe **antes** que `result.json`: si `logs.txt` falla no existe `result.json`, incluido el caso «hash distinto», hoy exento (mutación: invertir el orden);
  5. `activeDeadlineSeconds` del Job == timeout del flujo, comprobado a través del `Launcher` (mutación: constante arbitraria);
  6. `result.json` lleva `"status":"passed"` cuando pasa; la cuota se consulta **una** vez por corrida con varias olas; cableado `real`: `Log` y `NS` del lanzador (mutaciones: `ResPassed`→`ResFailed`; consultar en cada ola; omitir `Log`; cambiar `NS`).
- **AMARILLOS baratos.** Marcar `logs_unavailable` en `result.json` si falla `pods/log` (en vez de guardar un `logs.txt` vacío sin explicación); una línea en el README para U3: «`run.done` NO implica corrida exitosa: lee `result.json.status` de cada flujo».
- **Punto de partida opcional:** la rama local `wip/U2-T05b` (en el repositorio local de quien la creó; si no está, ignorar) contiene el trabajo parcial, **sin revisar**, de la ronda 2 interrumpida de U2-T05. Puede reutilizarse tras revisarlo; no se confía en él.

**Fuera** (cortado a propósito):

- `DeleteJob` sin reintento, fallo permanente de lectura de S3 a mitad de corrida (vigilante de corridas atascadas), `ttlSecondsAfterFinished`, circuito del adaptador S3, imágenes `RUNNER_IMAGE`/ensayo, retención del bucket, plazos devueltos por U4: candidatas.
- Cambiar la máquina de estados, los gates, el ensayo (U2-T04) o los otros servicios; `contracts/**`, `deploy/**`, `policy/**`, workflows.
- Comandos contra un clúster o la nube.

---

## Archivos de contexto

- `revisiones/U2-T05/ronda-1.md` (los repros `rev_S1_S4_test.go` y `rev_R1_R4_test.go` del revisor vivían en el scratchpad de la sesión y pueden no existir: reproducir desde la descripción), `bitacoras/U2-T05.md`, `tareas/U2-T05-runners-evidencia.md`
- `services/go-run-controller/runner/`, `runctl/controller.go` (`runnersStep`), `cmd/go-run-controller/real.go`

---

## Criterios de aceptación

Desde la raíz del worktree.

- [ ] **CA-1** — Pruebas en verde con `-race` y lo anterior intacto.
  ```bash
  cd services/go-run-controller && go test -race -count=1 ./... 2>&1 | grep -c FAIL; go test -race -count=1 -v ./... | grep -c -E '^\s*--- PASS'
  ```
  Esperado: `0` y ≥ `175` (los 163 de U2-T05 más ≥ 12).

- [ ] **CA-2** — **El plazo por corrida se respeta aunque venzan Jobs.**
  ```bash
  cd services/go-run-controller && go test -race -count=1 -run 'Runner(RunTimeoutNotStretched|ExpiredJobsKeptUntilSettled)' -v ./runner/ | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS`; con `MaxParallel=1`, `FlowTimeout=1m`, `RunTimeout=3m` y 6 flujos que vencen, **no** se lanzan los 6 (a lo sumo 3); con la escritura de evidencia fallando, el Job vencido **no** se borra. **Rojo primero** con la reproducción del informe (`jobsCreados=6`).

- [ ] **CA-3** — **Un lanzamiento parcial no deja Jobs vivos.**
  ```bash
  cd services/go-run-controller && go test -race -count=1 -run 'Runner(InvalidFlowCreatesNoJobs|PartialLaunchLeavesNoJobs)' -v ./runner/ | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS`; con un `flow_id` inválido en el 2.º flujo se crean **0** Jobs, y con un `CreateJob` que falla en el 2.º quedan **0** Jobs vivos al terminar, también por el camino de reset. **Rojo primero** con la reproducción (`estado final: failed, jobs vivos: 1`).

- [ ] **CA-4** — **Las mutaciones de F-03 mueren** (las 8 del informe), con la mutación aplicada en una copia (`ulimit -v 4000000` y `go test -timeout 60s`) y el rojo pegado en la bitácora.
  ```bash
  cd services/go-run-controller && go test -race -count=1 -run 'RunDone(PartialEvidence|PublishErrorRetried|OncePerRun)|EvidenceMarkerLast|JobDeadlineEqualsFlowTimeout|ResultStatusPassed|QuotaOncePerRun|RealWiring(Log|NS)' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS` en al menos ocho pruebas; la bitácora lista cada mutación con su prueba roja y las supervivientes equivalentes justificadas.

- [ ] **CA-5** — `logs_unavailable` y la nota para U3.
  ```bash
  cd services/go-run-controller && go test -race -count=1 -run 'LogsUnavailable' -v ./... | grep -E '^(--- |ok|FAIL)'; grep -c -i 'no implica' services/go-run-controller/README.md
  ```
  Esperado: `--- PASS` (con `pods/log` fallando, `result.json` marca `logs_unavailable`) y ≥ `1`.

- [ ] **CA-6** — Higiene, imagen y alcance.
  ```bash
  (cd services/go-run-controller && go vet ./... && go vet -tags minio,contract ./... && test -z "$(gofmt -l .)" && echo ok); docker build -q -t aqs-go-run-controller:ci services/go-run-controller >/dev/null && docker inspect aqs-go-run-controller:ci --format '{{.Config.User}}'; git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/go-run-controller/|bitacoras/U2-T05b.md|revisiones/U2-T05b/)' | wc -l
  ```
  Esperado: `ok`, `65532:65532`, `0` y `0`.

---

## Plan de pruebas

- `kubernetes/fake` con Jobs vencidos y reloj inyectable; `Evidence` en memoria con fallos programables; MinIO real para el CA de evidencia heredado (`go test -tags minio -run EvidenceS3` sigue verde).
- **Barrido de clase** (la familia: estado derivado de algo que se borra o que vive en memoria, y pruebas que no matan la invariante declarada): tabla camino→prueba→mutación en la bitácora para Jobs, evidencia, marcador, `DonePublish` y cuota.

**Rojo primero:** registrar `jobsCreados=6` y `jobs vivos: 1` fallando contra `main`, y las mutaciones de F-03 sobreviviendo con la suite actual.

---

## Notas

- **Tope de 3 rondas** y **versión mínima** (decisión del humano). En cada NO-VERDE, barrido de la clase del defecto.
- **Mutaciones con límites:** `ulimit -v 4000000` y `go test -timeout 60s` por mutante (un mutante colgado consumió 17 GB de RAM).
- Go 1.26.8; podman con `:z`; `ajv`/`yq` vía `npx`/imagen (`-p ajv-cli@5.0.0 -p ajv-formats@3.0.1`).
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
