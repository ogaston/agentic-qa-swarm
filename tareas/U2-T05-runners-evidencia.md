# U2-T05 — Runners `runner-{run}-{flow}`, evidencia a MinIO y `run.done`

**Unidad:** U2 — Orquestación & Warm Sandbox Lifecycle
**Historias que implementa:** US-M6 (ejecución de flujos sobre el warm, evidencia durable, sin LLM y sin egress)
**Depende de:** U2-T04 (**fusionada**: ambas modifican `go-run-controller`), U2-T01 (esquema `EvidenceURIs`), U5-T05 (MinIO).

---

## Alcance

**Dentro** (una línea, concreta):

> Añadir a `services/go-run-controller/` el lanzamiento de un Job `runner-{run}-{flow}` por flujo aprobado (motor enchufable), la recolección de su evidencia a MinIO mediante un puerto `Evidence` con adaptador S3, el cumplimiento de la cuota y el timeout del workflow, y la publicación de `run.done`.

Detalle:

- **Runners.** Solo se lanzan con `ensayo_passed=true` registrado y tras el gate `rehearsing → running` de U4. Un Job por flujo del `FlowPlan`: `runner-<run>-<flow_id>`, con límite de concurrencia (`RUN_MAX_PARALLEL_RUNNERS`, 3 por defecto) y `activeDeadlineSeconds` igual al timeout del workflow. Constructor de Job puro con las **mismas restricciones que el ensayo**: sin credenciales de LLM ni Secrets, sin egress (etiqueta de la NetworkPolicy de aislamiento de `aqs-test`), `automountServiceAccountToken: false`, `serviceAccountName`, no root, `readOnlyRootFilesystem`, tag fijado, `resources` completos. Subcomando `render-runner-job --run <id> --flow <id>`.
- **Motor enchufable.** Puerto `Executor` (`Plan(flow) → JobSpec`) con dos implementaciones: `HTTPStepsExecutor` (ejecuta los pasos del flujo con la imagen `RUNNER_IMAGE`, tag fijado) y `FakeExecutor` para pruebas. Cambiar de motor no cambia la máquina de estados ni el constructor de Jobs de seguridad (el `Executor` solo aporta `command`/`args`/`env` **no sensibles**; el constructor **rechaza** cualquier `env` cuyo nombre case con `LLM|API_KEY|TOKEN|SECRET|PASSWORD`).
- **Evidencia.** Al terminar cada Job, `collectEvidence` copia sus logs (`pods/log`) y el resultado a MinIO bajo `runs/<run_id>/<flow_id>/…` mediante el puerto `Evidence` (adaptador S3 con `github.com/minio/minio-go/v7`, escrito aquí; la configuración llega de `EVIDENCE_ENDPOINT`, `EVIDENCE_BUCKET` y credenciales por archivo, nunca por variable con valor en claro en el repo) y devuelve `EvidenceURIs` (válidas contra `contracts/plans/evidence-uris.schema.json`). Cada objeto se **lee de vuelta** y se compara por hash antes de aceptarlo. Si una escritura falla, el flujo cuenta como fallido (nunca se declara evidencia que no existe).
- **`run.done`.** Cuando todos los runners terminaron y su evidencia está guardada, se publica `run.done` con `evidence_uris` (≥ 1; válido contra su esquema) y la corrida pasa `running → resetting` (el reset verificado es la única salida). Si un runner supera el timeout o la cuota, el flujo se marca fallido, el Job se elimina, y la corrida sigue a `resetting`; **nunca** queda un Job colgado.
- **Cuota y timeout del workflow.** El controlador consulta la política de workflows vía el gate de U4 (el campo `workflow` de `GateRequest`); una denegación por cuota o aprobación pendiente impide lanzar runners. El timeout por flujo y por corrida sale de la política; sin política legible → no se lanza nada (fail-closed).
- **Observabilidad.** Métricas `aqs_runner_jobs_total{result}`, `aqs_evidence_objects_total{result}` y `aqs_run_duration_seconds`; logs JSON con `run_id`, `flow_id`, `trace_id`, sin contenido de evidencia ni secretos.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- El reporte post-mortem y la lectura de evidencia (U3/C6), `report.ready`, y la generación de flujos (U3).
- Cambiar el ensayo, los adaptadores de fase o la máquina de estados salvo lo imprescindible para enchufar los runners; los cambios de comportamiento de T02/T04 se reportan, no se hacen.
- Modificar `contracts/**`, `deploy/**`, `policy/**` o workflows. Los límites de recursos, cuotas y NetworkPolicy de los Jobs son **U2-T07**.
- Construir las imágenes `RUNNER_IMAGE` (k6 u otra): se configuran por etiqueta fijada; construirlas es candidata.
- Credenciales de MinIO en el repo: solo valores de prueba generados en el job o en la prueba.
- Aplicar nada a un clúster real.

---

## Archivos de contexto

- `unidades-y-tareas.md` (U2) y `aidlc-docs/inception/application-design/unit-task-plans/U2.md`
- `aidlc-docs/inception/application-design/components.md` (C4), `component-methods.md` (`executeFlows`, `collectEvidence`)
- `contracts/events/run.done.schema.json` y ejemplos; `contracts/plans/evidence-uris.schema.json`, `flow-plan.schema.json`
- `services/go-run-controller/` (U2-T02, U2-T04), `tareas/U2-T04-ensayo.md`
- `deploy/flux/base/minio/` y `scripts/test/minio-local.sh` (MinIO local fijado por digest)
- `services/go-governance/authz/doc.go` (política de workflows: existencia, cuota y aprobación solo en `running`)
- `policy/*.rego`

---

## Criterios de aceptación

Desde la raíz del worktree.

- [ ] **CA-1** — Pruebas unitarias en verde (≥ 20 casos nuevos), con `-race`.
  ```bash
  cd services/go-run-controller && go test -race -v ./... | grep -c -E '^\s*--- PASS'; go test -race ./... 2>&1 | grep -c FAIL
  ```
  Esperado: ≥ `65` acumulado con U2-T02/T04 y `0`. Cubren: constructor de Job y rechazo de `env` sensible; concurrencia máxima; timeout; cuota denegada; política ilegible; escritura de evidencia fallida; verificación por hash; `run.done` válido.

- [ ] **CA-2** — **Los runners no llevan LLM ni egress**: lo que el controlador genera lo demuestra.
  ```bash
  t=$(mktemp -d); (cd services/go-run-controller && go build -o "$t/rc" ./cmd/go-run-controller)
  "$t/rc" render-runner-job --run r-1 --flow checkout > "$t/job.yaml"
  grep -v automountServiceAccountToken "$t/job.yaml" | grep -c -i -E 'LLM|API_KEY|TOKEN|SECRET|PASSWORD|secretKeyRef|envFrom'
  yq -N '.spec.template.metadata.labels' "$t/job.yaml" 2>/dev/null || docker run --rm -i mikefarah/yq:4.44.3 -N '.spec.template.metadata.labels' < "$t/job.yaml"
  docker run --rm -i --security-opt label=disable ghcr.io/yannh/kubeconform:v0.6.7 -strict -summary - < "$t/job.yaml"
  docker run --rm --security-opt label=disable -v "$PWD":/project:z -v "$t":/in:z -w /project openpolicyagent/conftest:v0.56.0 test /in/job.yaml --policy policy --all-namespaces
  rm -rf "$t"
  ```
  Esperado: `0`; las etiquetas muestran la que selecciona la NetworkPolicy de aislamiento de `aqs-test` (el codificador cita su nombre en la bitácora, tomado de `deploy/flux/base/security/`); `Valid: 1, Invalid: 0`; `0 failures`. Complemento en Go: `go test -run 'RunnerNoSensitiveEnv' -v ./...` en `--- PASS`, incluida la prueba que intenta inyectar `OPENAI_API_KEY` por el `Executor` y comprueba que el constructor lo **rechaza**.

- [ ] **CA-3** — Sin `ensayo_passed` no hay runners, y un fallo siempre termina en reset.
  ```bash
  cd services/go-run-controller && go test -run 'Runner(NeedsEnsayo|Timeout|QuotaDenied|FailureToReset)' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS`; las pruebas leen de vuelta del clientset falso: sin `ensayo_passed`, **0** Jobs `runner-*`; con cuota denegada, 0 Jobs; con timeout, el Job se elimina y la corrida llega a `resetting`; ningún Job queda sin dueño.

- [ ] **CA-4** — La evidencia llega a MinIO real, se lee de vuelta y coincide.
  ```bash
  cd services/go-run-controller && go test -tags minio -run 'EvidenceS3' -v ./... | grep -E '^\s*--- (PASS|FAIL|SKIP)|^(ok|FAIL)'
  ```
  Esperado: `--- PASS` (la prueba levanta MinIO con la imagen fijada por digest de `scripts/test/minio-local.sh`, sube evidencia para dos flujos, la lee de vuelta, compara hash, y valida las `EvidenceURIs` devueltas contra su esquema; una escritura a un bucket inexistente deja el flujo como fallido) y ningún `FAIL`/`SKIP`. Sin `-tags minio` la prueba no existe.

- [ ] **CA-5** — `run.done` válido con la herramienta real.
  ```bash
  cd services/go-run-controller && go test -run 'RunDone' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS`; además el codificador vuelca un `run.done` producido por las pruebas y lo valida con `ajv` (`--spec=draft2020 -c ajv-formats -s contracts/events/run.done.schema.json`), pegando la salida `valid` en la bitácora; un `run.done` con `evidence_uris` vacío **no** se publica (la prueba lo comprueba).

- [ ] **CA-6** — Recorrido completo con fakes: `rehearsing → running → resetting` con evidencia y evento.
  ```bash
  cd services/go-run-controller && go test -run 'Flow(ToRunDone|MultiFlow)' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS`; una corrida de 3 flujos con `RUN_MAX_PARALLEL_RUNNERS=2` nunca tiene más de 2 Jobs activos a la vez (leído de vuelta del clientset) y termina con 1 `run.done` con 3 grupos de URIs.

- [ ] **CA-7** — Los secretos de evidencia no se filtran.
  ```bash
  cd services/go-run-controller && go test -run 'EvidenceSecretsNotLogged' -v ./... | grep -E '^(--- |ok|FAIL)'
  t=$(mktemp -d); (go build -C services/go-run-controller -o "$t/rc" ./cmd/go-run-controller); env RUN_PHASES=fake RUN_ALLOW_FAKE_PHASES=true RUN_DATA_DIR="$t/d" EVIDENCE_ENDPOINT=ftp://x "$t/rc" >/dev/null 2>&1; echo "endpoint no http rc=$?"; rm -rf "$t"
  ```
  Esperado: `--- PASS` (ni logs ni `/metrics` contienen la clave de acceso ni el contenido de la evidencia) y `rc=` distinto de `0`.

- [ ] **CA-8** — Imagen, dependencias y alcance.
  ```bash
  go list -C services/go-run-controller -deps ./... | grep -c -E 'minio-go'
  docker build -q -t aqs-go-run-controller:ci services/go-run-controller >/dev/null && docker inspect aqs-go-run-controller:ci --format '{{.Config.User}}'
  (cd services/go-run-controller && go vet ./... && go vet -tags minio,contract ./... && test -z "$(gofmt -l .)" && echo ok); git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/go-run-controller/|bitacoras/U2-T05.md|revisiones/U2-T05/)' | wc -l
  ```
  Esperado: ≥ `1`, un usuario no vacío distinto de `root`/`0`, `ok`, `0` y `0`.

---

## Plan de pruebas

- Unitarias con `kubernetes/fake`, `FakeExecutor`, `Evidence` en memoria y reloj inyectable.
- Propiedad (`rapid`): para toda secuencia de resultados de runners y de escrituras de evidencia, `run.done` se publica solo si cada flujo aceptado tiene al menos un objeto leído de vuelta; nunca se lanza un `runner-*` sin `ensayo_passed`; nunca hay más Jobs activos que el máximo.
- Contrato de MinIO (CA-4) y de política (CA-2).
- Negativa: intentar inyectar `OPENAI_API_KEY`, `API_TOKEN` o `envFrom` por el `Executor` debe ser rechazado.

**Rojo primero:** registrar que `render-runner-job` no existe y que la corrida no sale de `running` sin runners.

---

## Notas

- **Heredado de U2-T02 (rondas 3, F-05, y U2-T02b, F-03):** `Launch` es idempotente por (corrida, fase, flujo), no por intento: `Started[fase]` es solo el número de intento (tope 3). Antes de crear `runner-<run>-<flow>-<Started>`, el lanzador busca por etiquetas el Job vivo de esa (corrida, flujo) y lo **adopta** si existe; `AlreadyExists` cuenta como éxito del lanzamiento. Misma prueba que en U2-T04.

- **Go y Dockerfile.** Igual que U2-T02/T04; `Dockerfile` en su sitio. Podman: montajes con `:z`.
- **Orden de fusión.** U2-T04 debe estar fusionada antes de despachar esta tarea.
- **Candidatas a registrar:** imágenes del runner y del ensayo (k6 u otro motor); retención y ciclo de vida del bucket de evidencia; circuito en el adaptador S3.
- **Informes del loop.** El diff de `revisiones/<tarea>/` no cuenta como desborde.
- Ningún comando contra un clúster ni la nube; el `kubectl get jobs` real y la comprobación de egress desde un pod los hace el humano en dev (U2-T08).

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
