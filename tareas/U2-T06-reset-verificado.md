# U2-T06 — `go-reset`: reset verificado, cuarentena, scale-down en idle y rebuild/teardown periódico

**Unidad:** U2 — Orquestación & Warm Sandbox Lifecycle
**Historias que implementa:** US-M7.1, US-M7.2 (KPI: reset verificado 100% + higiene de rebuild)
**Depende de:** U2-T01 (módulo y esquema `WarmState`), U5-T04 (manifiestos del warm y `warm-policy`), U5-T09 (CronJobs). Puede correr en paralelo con U2-T02 y U2-T03.

---

## Alcance

**Dentro** (una línea, concreta):

> Implementar en `services/go-reset/` el reset verificado entre corridas (`reset-{run}`), la cuarentena si falla, el scale-down del warm en idle, la higiene periódica (`housekeeping` con grace de 24 h y `rebuild`/`teardown`) y la persistencia de la sesión "incompleta", detrás de puertos con fakes y adaptadores verificados **sin clúster**.

Detalle:

- **Reset verificado** (`POST /resets` `{run_id}`, servicio a servicio). Un Job `reset-{run}` en `aqs-test` ejecuta, en este orden: (1) restart de los servicios del warm (`warm-app`), (2) limpieza de la DB al **baseline** (puerto `DatabaseCleaner`: adaptador que ejecuta el script de baseline declarado en `RESET_BASELINE_SCRIPT`), (3) `FLUSHALL` del Redis del warm (puerto `CacheFlusher`), (4) **verificación**: lista de comprobaciones (`app-ready`, `db-baseline`, `cache-empty`, mínimo 3) que **leen de vuelta el estado real** (conteo de filas contra el baseline; `DBSIZE` = 0; `Ready` del pod), no el código de salida de los pasos anteriores. Solo si **todas** pasan: `reset_verified=true`, estado `ready` y evento `reset.verified` (válido contra su esquema; `checks[].ok` siempre `true`).
- **Cuarentena.** Si cualquier paso o comprobación falla, tras como máximo 1 reintento: estado `cuarentena`, **ningún** `reset.verified`, `Alerter.Quarantine` (alerta `warm.quarantined`, solo registro y métrica) y `reset_verified=false` persistido. Salir de `cuarentena` solo ocurre con un reset que verifique de nuevo (la acción humana de investigar y reintentar llega por `POST /resets`).
- **Constructor del Job puro y sin credenciales**: igual que `deploy-{run}` en U2-T03 (sin `envFrom` de Secrets, `automountServiceAccountToken: false`, `serviceAccountName` explícito, no root, sin `hostNetwork`, tag fijado, `resources` completos). Subcomando `render-reset-job --run <id>` que lo imprime en YAML, igual que en U2-T03.
- **Scale-down en idle.** `go-reset idle-check` (comando de un solo disparo, pensado para CronJob): si el warm lleva `idleScaleDownAfter` (ConfigMap `warm-policy`, hoy 30 m) sin corrida activa, lo escala a `minReplicasIdle` y pasa a `idle-escalado`. **Nunca** escala un warm `dirty` ni uno con corrida activa.
- **Higiene.** `go-reset housekeeping`: sesiones de corrida abandonadas (sin actividad > `HOUSEKEEPING_GRACE`, 24 h por defecto, configurable) se cierran: el warm se lleva a `ready` (vía reset verificado) o a `cuarentena`, y la sesión se persiste como `incompleta` con el plan y el estado alcanzados (puerto `SessionStore`, adaptador JSONL en `RESET_DATA_DIR`) para poder reanudarla. `go-reset rebuild` y `go-reset teardown`: reconstruyen el warm desde la imagen base o lo destruyen y reprovisionan; terminan con `teardown.verified` (`mode` `rebuild`|`teardown`, `verified: true`) **solo si** la verificación posterior pasó; si no, cuarentena y sin evento.
- **Puertos** (interfaz + fake + adaptador): `KubeAPI` (adaptador `client-go`; pruebas con `kubernetes/fake`), `DatabaseCleaner`, `CacheFlusher`, `StateStore` (ConfigMap `warm-state`, mismo formato que U2-T03), `SessionStore`, `EventPublisher` (outbox JSONL `RESET_OUTBOX_FILE`, `event_id` determinista), `Alerter`, `Clock`.
- **Observabilidad y arranque.** `/healthz`, `/readyz`, `/metrics` (`aqs_reset_total{result}`, `aqs_warm_quarantined`, `aqs_warm_idle_scaled_total`, `aqs_housekeeping_sessions_closed_total`); logs JSON con `run_id`/`trace_id`, sin secretos. Variables: `RESET_NAMESPACE` (`aqs-test`; otro valor → no arranca), `RESET_SERVICE_TOKEN`, `RESET_OUTBOX_FILE`, `RESET_DATA_DIR`, `RESET_BASELINE_SCRIPT`, `HOUSEKEEPING_GRACE`, `LISTEN_ADDR`. `Dockerfile` según `go-identity`; el binario sirve la API y expone los subcomandos `idle-check`, `housekeeping`, `rebuild`, `teardown`.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Deploy del artefacto, superficie y `warm.ready` (**U2-T03**), ensayo y runners (**U2-T04/T05**), la máquina de estados y los gates (**U2-T02**).
- Los manifiestos de los CronJobs `housekeeping`/`rebuild` y sus cuotas: ya existen en `deploy/` (U5-T09) o los cablea **U2-T07**. Aquí no se toca `deploy/**`, `policy/**`, `contracts/**` ni los workflows.
- Un esquema de evento `warm.quarantined`: no existe en `contracts/` (candidata). La alerta es registro + métrica.
- Aplicar nada a un clúster real. Todo el comportamiento se prueba con fakes y con `client-go/kubernetes/fake`; la validación en dev la hace el humano.
- Borrar o resetear **cualquier** cosa fuera de `aqs-test`: si el recurso no está en ese namespace, se rechaza.

---

## Archivos de contexto

- `unidades-y-tareas.md` (U2) y `aidlc-docs/inception/application-design/unit-task-plans/U2.md`
- `aidlc-docs/inception/application-design/components.md` (C5), `component-methods.md`, `services.md` (CronJobs `housekeeping`, `rebuild`, `teardown`)
- `contracts/events/reset.verified.schema.json`, `teardown.verified.schema.json` y sus ejemplos; `contracts/plans/warm-state.schema.json` (U2-T01)
- `deploy/flux/base/warm.yaml` (`warm-policy`, `warm-app`, `warm-db`, Redis) y los CronJobs de U5-T09 (`deploy/flux/base/`), `security/rbac.yaml`
- `tareas/U2-T03-warm-manager.md` (formato de `WarmState` y del constructor de Job, que aquí se repite, sin importarlo)
- `tareas/U5-T09-cronjobs-al-control-plane.md`

---

## Criterios de aceptación

Desde la raíz del worktree.

- [ ] **CA-1** — Pruebas unitarias en verde (≥ 25 casos), con `-race`.
  ```bash
  cd services/go-reset && go test -race -v ./... | grep -c -E '^\s*--- PASS'; go test -race ./... 2>&1 | grep -c FAIL
  ```
  Esperado: ≥ `25` y `0`. Cubren: cada paso del reset y de la verificación; reintento único; cuarentena; idle-check (escala, no escala si `dirty` o si hay corrida activa); housekeeping antes y después del grace con reloj simulado; rebuild y teardown (con y sin verificación posterior); sesión `incompleta` persistida y releída.

- [ ] **CA-2** — **Sin `reset_verified=true` no hay evento ni warm listo**; un reset fallido va a cuarentena con aviso.
  ```bash
  cd services/go-reset && go test -run 'Reset(Fails|Quarantine|NoEventWithoutVerification)' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS`; las pruebas **leen de vuelta** el `StateStore` y el outbox: con un paso que falla, el estado es `cuarentena`, `reset_verified=false`, el outbox **no contiene** `reset.verified` y el `Alerter` registró 1 cuarentena.

- [ ] **CA-3** — La verificación mide el estado real y no el código de salida: una DB sucia o un Redis con claves **no pasan**, aunque los pasos "terminen bien".
  ```bash
  cd services/go-reset && go test -run 'Verify(DirtyDB|DirtyCache|PodNotReady)' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS` en al menos tres pruebas, cada una con un `DatabaseCleaner`/`CacheFlusher`/`KubeAPI` que devuelve éxito pero deja el estado sucio, y el resultado es cuarentena (rojo si la verificación solo mirara el éxito de los pasos).

- [ ] **CA-4** — Tras fin o abandono (+ grace simulado) el warm queda `ready` o en `cuarentena`, nunca `dirty`; la sesión queda `incompleta`.
  ```bash
  cd services/go-reset && go test -run 'Housekeeping|IncompleteSession' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS`; con el reloj a 23 h 59 no cierra nada; a 24 h 01 cierra, el warm no queda `dirty` y la sesión leída de `SessionStore` tiene `incompleta` con el plan y el estado alcanzados.

- [ ] **CA-4b** — Rebuild/teardown periódico.
  ```bash
  cd services/go-reset && go test -run 'Rebuild|Teardown' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS`; `teardown.verified` solo se publica tras una verificación posterior en verde, con `mode` correcto; con verificación fallida, cuarentena y sin evento.

- [ ] **CA-5** — Los Jobs y eventos que genera pasan las políticas y esquemas.
  ```bash
  t=$(mktemp -d); (cd services/go-reset && go build -o "$t/gr" ./cmd/go-reset)
  "$t/gr" render-reset-job --run r-1 > "$t/job.yaml"
  docker run --rm -i --security-opt label=disable ghcr.io/yannh/kubeconform:v0.6.7 -strict -summary - < "$t/job.yaml"
  docker run --rm --security-opt label=disable -v "$PWD":/project:z -v "$t":/in:z -w /project openpolicyagent/conftest:v0.56.0 test /in/job.yaml --policy policy --all-namespaces
  rm -rf "$t"
  cd services/go-reset && go test -run 'Outbox|Events' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `Valid: 1, Invalid: 0`, `0 failures` y `--- PASS`; además el codificador vuelca un `reset.verified` y un `teardown.verified` producidos por las pruebas y los valida con `ajv` (`--spec=draft2020 -c ajv-formats`) contra `contracts/events/*.schema.json`, y pega las dos salidas `valid` en la bitácora.

- [ ] **CA-6** — API: `401` sin token, `400` ante cuerpos inválidos, y un reset no puede salir de `aqs-test`.
  ```bash
  up
  curl -s -o /dev/null -w '%{http_code} ' -X POST http://127.0.0.1:18320/resets
  curl -s -o /dev/null -w '%{http_code} ' -X POST -H "Authorization: Bearer $rtok" -H 'Content-Type: application/json' -d '{"run_id":""}' http://127.0.0.1:18320/resets
  curl -s -o /dev/null -w '%{http_code}\n' -X POST -H "Authorization: Bearer $rtok" -H 'Content-Type: application/json' -d '{"run_id":"r-1","namespace":"aqs-system"}' http://127.0.0.1:18320/resets
  grep -c -F "$rtok" "$t/gr.log"; curl -s http://127.0.0.1:18320/metrics | grep -c -F "$rtok"
  down
  ```
  Esperado: `401 400 400` (el campo `namespace` no existe en el cuerpo y se rechaza como campo extra), `0` y `0`. El codificador documenta `up` (servicio con fakes) en la bitácora.

- [ ] **CA-7** — Arranque fail-closed.
  ```bash
  t=$(mktemp -d); (cd services/go-reset && go build -o "$t/gr" ./cmd/go-reset)
  env -i PATH="$PATH" "$t/gr" >/dev/null 2>&1; echo "sin config rc=$?"
  env RESET_NAMESPACE=aqs-prod RESET_SERVICE_TOKEN=x RESET_OUTBOX_FILE="$t/o" RESET_DATA_DIR="$t/d" "$t/gr" >/dev/null 2>&1; echo "ns ajeno rc=$?"
  env RESET_NAMESPACE=aqs-test RESET_SERVICE_TOKEN=x RESET_OUTBOX_FILE="$t/o" RESET_DATA_DIR="$t/d" HOUSEKEEPING_GRACE=abc "$t/gr" >/dev/null 2>&1; echo "grace invalido rc=$?"; rm -rf "$t"
  ```
  Esperado: tres `rc=` distintos de `0`.

- [ ] **CA-8** — Imagen y dependencias.
  ```bash
  go list -C services/go-reset -deps ./... | grep -c -E 'k8s.io/client-go'
  docker build -q -t aqs-go-reset:ci services/go-reset >/dev/null && docker inspect aqs-go-reset:ci --format '{{.Config.User}}'
  bash scripts/ci/list-services.sh | grep -c 'go-reset'
  ```
  Esperado: ≥ `1`, un usuario no vacío distinto de `root`/`0`, y `1`.

- [ ] **CA-9** — Higiene y alcance.
  ```bash
  (cd services/go-reset && go vet ./... && test -z "$(gofmt -l .)" && echo ok); git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/go-reset/|bitacoras/U2-T06.md|revisiones/U2-T06/)' | wc -l
  ```
  Esperado: `ok`, `0` y `0`.

---

## Plan de pruebas

- Unitarias con fakes de todos los puertos y reloj inyectable; `client-go/kubernetes/fake` para `KubeAPI`.
- Propiedad (`rapid`): para toda secuencia de resultados de pasos y comprobaciones, `reset.verified` se publica **si y solo si** todas las comprobaciones leyeron el estado limpio; el warm nunca queda `ready` con `reset_verified=false`; el reset siempre termina (sin bucles).
- Negativas: pasos que devuelven éxito con estado sucio (CA-3); namespace ajeno; reloj en el borde del grace.
- Mutación manual registrada en la bitácora: cambiar la verificación a "todos los pasos devolvieron éxito" debe hacer fallar CA-3 (rojo → verde).

**Rojo primero:** el codificador registra que `go build ./cmd/go-reset` y `render-reset-job` no existen antes de empezar.

---

## Notas

- **Go y Dockerfile.** `go 1.26.8`, `golang:1.26.8-alpine3.24` + `alpine:3.24`, usuario 65532 (patrón `go-identity`); contexto temporal si la CA del proxy rompe `docker build`. Podman: montajes con `:z`.
- **Riesgo #1 del proyecto: contaminación entre corridas.** Por eso la verificación **lee de vuelta**; es el criterio más importante de la tarea y el revisor lo comprobará con una DB y un Redis deliberadamente sucios.
- **Validación en dev, a cargo del humano:** con aprobación, ejecutar un reset real sobre el warm, ensuciar la DB a mano y comprobar que cuarentena. Se anota en la bitácora.
- **Candidatas a registrar:** esquema de evento `warm.quarantined`; llevar la API REST de `go-reset` al OpenAPI; `deploy/**` con variables y RBAC del servicio (U2-T07).
- **Informes del loop.** El diff de `revisiones/<tarea>/` no cuenta como desborde.
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
