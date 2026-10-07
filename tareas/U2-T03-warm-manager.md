# U2-T03 — `go-warm-manager`: entorno warm, deploy por corrida (`deploy-{run}`) e inferencia de superficie

**Unidad:** U2 — Orquestación & Warm Sandbox Lifecycle
**Historias que implementa:** US-M2 (deploy del artefacto sobre el warm; fail-closed tras 2 reintentos)
**Depende de:** U2-T01 (módulo y esquemas de `contracts/plans/`), U5-T04/T05 (manifiestos del warm y MinIO). Puede correr en paralelo con U2-T02 y U2-T06.

---

## Alcance

**Dentro** (una línea, concreta):

> Implementar en `services/go-warm-manager/` la gestión del entorno warm reutilizable (estados `ready`/`dirty`/`cuarentena`/`idle-escalado`), el Job `deploy-{run}` que despliega el artefacto **sobre** el warm con fail-closed tras 2 reintentos, la inferencia de la superficie externa y la publicación de `warm.ready`, `deploy.done|failed` y `surface.ready`, detrás de puertos con fakes y adaptadores reales verificados **sin clúster** (cliente de Kubernetes falso y MinIO local).

Detalle:

- **Estado del warm** (`WarmState` de `contracts/plans/warm-state.schema.json`). Persistido en el ConfigMap `warm-state` del namespace de prueba (puerto `StateStore`; adaptador Kubernetes y fake en memoria). Transiciones legales: `ready→dirty` (una corrida lo usa), `dirty→ready` (solo con `reset_verified=true`), `dirty→cuarentena`, `cuarentena→ready` (solo con acción humana registrada: llega como `reset_verified=true` del propio U2-T06), `ready→idle-escalado`, `idle-escalado→ready`. Cualquier otra se rechaza.
- **`ensureWarmReady`.** Devuelve el `WarmState` actual y exige `state=ready` **y** `reset_verified=true` **y** las sondas de salud (app, DB, Redis: `GET` al Service o `Ready` del pod, puerto `HealthProbe`) en verde. Si el warm está `idle-escalado`, lo escala y espera a `Ready` (timeout configurable, 120 s por defecto). Si está `dirty` o en `cuarentena`, **no** hace el reset: responde que no está listo (el reset es de `go-reset`, U2-T06). Publica `warm.ready` (válido contra su esquema) solo cuando todo está verde.
- **Deploy `deploy-{run}`.** Un Job en el namespace de prueba (`aqs-test`) que aplica el artefacto (`build-from-repo` o `published-image`) sobre el Deployment `warm-app` (puerto `ContainerRuntime` para build/pull; fake y adaptador de Job). El constructor del Job es una función pura y **sin credenciales**: sin `envFrom` de Secrets, `automountServiceAccountToken: false`, `serviceAccountName` explícito, `runAsNonRoot`, `readOnlyRootFilesystem`, sin `hostNetwork`, imagen con tag fijado (nunca `latest`) y `resources` completos. Un artefacto cuya `ref` apunte fuera de una lista de registros permitidos (`WARM_ALLOWED_REGISTRIES`) se rechaza antes de crear nada.
- **Reintentos (V8).** Si el Job falla, se reintenta como máximo 2 veces (3 Jobs en total solo si `attempts` ≤ 2 reintentos; el **tercer reintento nunca se crea**); tras agotarlos, `deploy.failed` con `reason` y se llama a `Alerter.Handoff`. Un deploy exitoso publica `deploy.done`. Ambos eventos validan contra `contracts/events/deploy.schema.json` (`deploy.failed` lleva `reason`; `deploy.done` no).
- **Superficie.** `InferSurface` consulta solo el exterior del Service del warm (`GET /openapi.json` y rutas comunes; si no hay OpenAPI, sondeo de puertos declarados): **no lee código fuente**. Valida el resultado contra `contracts/plans/surface-artifact.schema.json`, lo guarda en el puerto `ObjectStore` (adaptadores: sistema de archivos para pruebas y **S3/MinIO** con `github.com/minio/minio-go/v7`) y publica `surface.ready` con `surface_uri` y `endpoint_count`.
- **API REST servicio a servicio** (bearer `WARM_SERVICE_TOKEN`, comparación en tiempo constante; sin token válido → `401`): `POST /warm/ensure` → `200` con `WarmState` si está listo, `409` con `WarmState` si no; `POST /deploys` `{run_id, artifact}` → `202`; `GET /deploys/{run_id}` → `{run_id, state: pending|done|failed, attempts}`; `POST /surface` `{run_id}` → `200` con `SurfaceArtifact`; `GET /warm` → `WarmState`. Cuerpos desconocidos, campos extra o `run_id` vacío → `400`. Documentada en `services/go-warm-manager/README.md` (candidata: llevarla al OpenAPI).
- **Subcomando `render-deploy-job`** (`go-warm-manager render-deploy-job --run <id> --artifact-kind <k> --artifact-ref <ref>`): imprime el Job como YAML en stdout sin tocar ningún clúster. Existe para que los manifiestos que el servicio genera pasen las mismas políticas que los de `deploy/`.
- **Observabilidad y arranque.** `/healthz`, `/readyz`, `/metrics` (`aqs_warm_state{state}`, `aqs_deploy_attempts_total{result}`, `aqs_handoff_total{phase}`); logs JSON con `run_id` y `trace_id`, sin secretos. Variables: `WARM_NAMESPACE` (`aqs-test`; cualquier otro valor → no arranca), `WARM_SERVICE_TOKEN`, `WARM_OUTBOX_FILE`, `WARM_OBJECT_STORE` (`file`|`s3`), `WARM_ALLOWED_REGISTRIES`, `LISTEN_ADDR`. `Dockerfile` según el patrón de `go-identity`.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Reset, verificación de reset, cuarentena como acción, rebuild/teardown y scale-down automático en idle: **U2-T06**. Aquí `idle-escalado` solo se **lee** y se **sale** de él.
- Ensayo, runners y cualquier Job que no sea `deploy-{run}`: **U2-T04/T05**.
- La máquina de estados de la corrida y los gates (U2-T02): `go-warm-manager` no consulta a `go-governance`; el controlador es quien lo hace.
- Aplicar nada a un clúster real: las pruebas usan `k8s.io/client-go/kubernetes/fake`. La validación en dev la hace el humano.
- Modificar `contracts/**`, `deploy/**`, `policy/**` o workflows. El cableado de manifiestos y cuotas es **U2-T07**.
- Secretos de staging o producción: el warm solo usa valores sintéticos declarados.

---

## Archivos de contexto

- `unidades-y-tareas.md` (U2) y `aidlc-docs/inception/application-design/unit-task-plans/U2.md`
- `aidlc-docs/inception/application-design/components.md` (C3), `component-methods.md`, `services.md`
- `contracts/plans/*.schema.json` (U2-T01), `contracts/events/{warm.ready,deploy,surface.ready}.schema.json` y sus ejemplos
- `deploy/flux/base/warm.yaml` (Deployment `warm-app`, Service, DB, Redis, ConfigMap `warm-policy`), `deploy/flux/base/security/rbac.yaml` (permisos de `go-warm-manager`), `deploy/flux/base/minio/`
- `policy/*.rego` (`security`, `workloads`, `isolation`) y `scripts/ci/policies.sh`
- `scripts/test/minio-local.sh` (patrón para levantar MinIO en contenedor)
- `tareas/U2-T01-stubs.md`, `tareas/U2-T02-run-controller.md` (nombres de eventos y de variables)

---

## Criterios de aceptación

Desde la raíz del worktree.

- [ ] **CA-1** — Pruebas unitarias en verde (≥ 25 casos), con `-race`.
  ```bash
  cd services/go-warm-manager && go test -race -v ./... | grep -c -E '^\s*--- PASS'; go test -race ./... 2>&1 | grep -c FAIL
  ```
  Esperado: ≥ `25` y `0`. Cubren: cada transición legal e ilegal del estado del warm; `ensureWarmReady` (listo, `dirty`, `cuarentena`, sin `reset_verified`, sonda caída, `idle-escalado` con escalado y con timeout); constructor del Job; registro no permitido; reintentos 0, 1, 2 y tercera falla (sin tercer Job); `deploy.done|failed` válidos; superficie con y sin OpenAPI.

- [ ] **CA-2** — **Fail-closed tras 2 reintentos**, comprobado en el cliente de Kubernetes falso: deploy roto → handoff y sin tercer reintento.
  ```bash
  cd services/go-warm-manager && go test -run 'Deploy(Retry|Exhausted|Failed)' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS`; la prueba lee de vuelta, del clientset falso, la lista de Jobs: con un deploy que siempre falla hay **exactamente 3** Jobs `deploy-<run>-*` (intento inicial + 2 reintentos), ningún cuarto, el último evento es `deploy.failed` con `reason` y `Alerter` recibió 1 handoff.

- [ ] **CA-3** — El Job que genera el servicio pasa las mismas políticas que los manifiestos de `deploy/`.
  ```bash
  t=$(mktemp -d); (cd services/go-warm-manager && go build -o "$t/wm" ./cmd/go-warm-manager)
  "$t/wm" render-deploy-job --run r-1 --artifact-kind published-image --artifact-ref ghcr.io/ogaston/demo:1.2.3 > "$t/job.yaml"
  docker run --rm -i --security-opt label=disable ghcr.io/yannh/kubeconform:v0.6.7 -strict -summary - < "$t/job.yaml"
  docker run --rm --security-opt label=disable -v "$PWD":/project:z -v "$t":/in:z -w /project openpolicyagent/conftest:v0.56.0 test /in/job.yaml --policy policy --all-namespaces
  "$t/wm" render-deploy-job --run r-1 --artifact-kind published-image --artifact-ref ghcr.io/ogaston/demo:latest >/dev/null 2>&1; echo "latest rc=$?"
  "$t/wm" render-deploy-job --run r-1 --artifact-kind published-image --artifact-ref docker.io/evil/x:1 >/dev/null 2>&1; echo "registro no permitido rc=$?"; rm -rf "$t"
  ```
  Esperado: `Valid: 1, Invalid: 0`, `0 failures` de conftest, y los dos últimos `rc=` distintos de `0`. Si una política de `policy/` no cubre `Job` y por eso no rechaza un Job inseguro, el codificador lo reporta con la salida (candidata); no edita `policy/`.

- [ ] **CA-4** — Los eventos que publica validan contra sus esquemas con `ajv`.
  ```bash
  cd services/go-warm-manager && go test -run 'Outbox|Events' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS`; además el codificador, **con la herramienta real**, vuelca a archivo un `warm.ready`, un `deploy.done`, un `deploy.failed` y un `surface.ready` producidos por las pruebas y los valida con `ajv` (`--spec=draft2020 -c ajv-formats`) contra `contracts/events/*.schema.json`, y pega la salida `valid` de los cuatro en la bitácora.

- [ ] **CA-5** — La API: `401` sin token, `400` ante cuerpos inválidos, `409` si el warm no está listo, y el token no se filtra.
  ```bash
  up
  for p in /warm /warm/ensure; do curl -s -o /dev/null -w '%{http_code} ' http://127.0.0.1:18310$p; done
  curl -s -o /dev/null -w '%{http_code} ' -X POST -H "Authorization: Bearer $wtok" -H 'Content-Type: application/json' -d '{"run_id":"","extra":1}' http://127.0.0.1:18310/deploys
  curl -s -o /dev/null -w '%{http_code}\n' -X POST -H "Authorization: Bearer $wtok" http://127.0.0.1:18310/warm/ensure
  grep -c -F "$wtok" "$t/wm.log"; curl -s http://127.0.0.1:18310/metrics | grep -c -F "$wtok"
  down
  ```
  Esperado: `401 401 400 409` (el servicio arranca con el fake de Kubernetes y el warm sembrado en `dirty`; el codificador documenta la función `up` en la bitácora) y luego `0` y `0`.

- [ ] **CA-6** — La superficie se sube a MinIO real y el URI es el del evento.
  ```bash
  cd services/go-warm-manager && go test -tags minio -run 'ObjectStoreS3' -v ./... | grep -E '^\s*--- (PASS|FAIL|SKIP)|^(ok|FAIL)'
  ```
  Esperado: `--- PASS` (la prueba levanta MinIO con la imagen fijada por digest de `scripts/test/minio-local.sh`, sube la superficie, la lee de vuelta byte a byte y valida el contenido contra `surface-artifact.schema.json`) y ningún `FAIL`/`SKIP`. Sin `-tags minio` la prueba no existe y `go test ./...` no necesita contenedores.

- [ ] **CA-7** — Arranque fail-closed.
  ```bash
  t=$(mktemp -d); (cd services/go-warm-manager && go build -o "$t/wm" ./cmd/go-warm-manager)
  env -i PATH="$PATH" "$t/wm" >/dev/null 2>&1; echo "sin config rc=$?"
  env WARM_NAMESPACE=aqs-prod WARM_SERVICE_TOKEN=x WARM_OUTBOX_FILE="$t/o" "$t/wm" >/dev/null 2>&1; echo "ns ajeno rc=$?"
  env WARM_NAMESPACE=aqs-test WARM_SERVICE_TOKEN= WARM_OUTBOX_FILE="$t/o" "$t/wm" >/dev/null 2>&1; echo "token vacio rc=$?"; rm -rf "$t"
  ```
  Esperado: tres `rc=` distintos de `0`.

- [ ] **CA-8** — Imagen y dependencias.
  ```bash
  go list -C services/go-warm-manager -deps ./... | grep -c -E 'k8s.io/client-go'
  docker build -q -t aqs-go-warm-manager:ci services/go-warm-manager >/dev/null && docker inspect aqs-go-warm-manager:ci --format '{{.Config.User}}'
  bash scripts/ci/list-services.sh | grep -c 'go-warm-manager'
  ```
  Esperado: ≥ `1` (aquí sí se permite `client-go`), un usuario no vacío distinto de `root`/`0`, y `1`.

- [ ] **CA-9** — Higiene y alcance.
  ```bash
  (cd services/go-warm-manager && go vet ./... && go vet -tags minio ./... && test -z "$(gofmt -l .)" && echo ok); git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/go-warm-manager/|bitacoras/U2-T03.md|revisiones/U2-T03/)' | wc -l
  ```
  Esperado: `ok`, `0` y `0`.

---

## Plan de pruebas

- Unitarias con fakes de todos los puertos (`StateStore`, `HealthProbe`, `ContainerRuntime`, `ObjectStore`, `Alerter`, `KubeAPI` con `client-go/kubernetes/fake`); reloj inyectable para timeouts.
- Propiedad (`rapid`): para toda secuencia de eventos de Job, nunca existen más de 3 Jobs por corrida y `reset_verified` nunca pasa a `true` por una transición que no sea la del reset.
- Negativas: registro no permitido; artefacto `latest`; `WARM_NAMESPACE` distinto de `aqs-test`; Job con `envFrom` de un Secret (la prueba del constructor lo verifica ausente).
- Contrato de eventos y de MinIO: CA-4 y CA-6.

**Rojo primero:** el codificador registra que `render-deploy-job` y `go build ./cmd/go-warm-manager` no existen antes de empezar.

---

## Notas

- **Go y Dockerfile.** `go 1.26.8`, `golang:1.26.8-alpine3.24` + `alpine:3.24`, usuario 65532 (patrón `go-identity`). Si el `docker build` falla por la CA del proxy, contexto temporal fuera del repo (ver `tareas/U1-T07-*.md`). Podman: montajes con `:z`.
- **Validación en dev, a cargo del humano** (no es un criterio de esta tarea): con aprobación, desplegar la imagen y comprobar con `kubectl get pods -n aqs-test` que los pods de `warm-app`/DB/Redis siguen `Running` tras un deploy, y que un artefacto roto deja 3 Jobs y un handoff. Se anota en la bitácora.
- **Candidatas a registrar:** llevar la API REST de `go-warm-manager` al OpenAPI de `contracts/`; incluir `contracts/plans/` en `contracts/validate.sh`; política que cubra explícitamente Jobs generados en tiempo de ejecución.
- **Hexagonal.** El dominio (estados, reintentos, constructor del Job) no importa `client-go`, `minio-go` ni `net/http`.
- **Informes del loop.** El diff de `revisiones/<tarea>/` no cuenta como desborde.
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
