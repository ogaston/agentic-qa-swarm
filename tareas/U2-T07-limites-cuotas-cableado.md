# U2-T07 — Límites, cuotas y cableado de despliegue de U2; circuito en las llamadas externas del controlador

**Unidad:** U2 — Orquestación & Warm Sandbox Lifecycle
**Historias que implementa:** US-M5, US-M6 (dimensionado especial: burst de Jobs por corrida, scale-down del warm en idle)
**Depende de:** U2-T02, U2-T03, U2-T04, U2-T05 y U2-T06, las cinco **fusionadas** (cablea sus variables y modifica `go-run-controller`). Cierra los manifiestos de U2.

---

## Alcance

**Dentro** (una línea, concreta):

> Cablear en `deploy/flux/base/` las variables, volúmenes, sondas y cuotas de `go-run-controller`, `go-warm-manager` y `go-reset`, añadir `ResourceQuota`/`LimitRange` del namespace `aqs-test`, el HPA del controlador y el CronJob `idle-check`, y añadir timeout + circuito a las llamadas HTTP externas del controlador, todo como **manifiestos revisables** que pasan `kubeconform` y `conftest` sin ningún `apply`.

Detalle:

- **Cableado de los tres servicios** (modificar en su sitio los Deployments de `deploy/flux/base/control-plane.yaml`): variables de cada tarea (`RUN_*`, `WARM_*`, `RESET_*`, `GOVERNANCE_URL`, `IDENTITY_URL`, `RUN_PHASES=real`, `WARM_URL`, `RESET_URL`, `EVIDENCE_*`, etc.) tomando tokens y claves de **Secrets por referencia** (`secretKeyRef`; los Secrets en claro están prohibidos y los valores van cifrados con SOPS como en U5-T14), `readinessProbe` en `/readyz` y `livenessProbe` en `/healthz`, `securityContext` no root con `readOnlyRootFilesystem` y volúmenes `emptyDir` para escritura temporal. `go-run-controller` y `go-reset` llevan un PVC para su diario/sesiones (`RUN_DATA_DIR`, `RESET_DATA_DIR`) con `fsGroup: 65532` y `strategy: Recreate` (aprendido en U4-T07, C-70). **No** se define `RUN_PHASES=fake` ni `RUN_ALLOW_FAKE_PHASES` en ningún manifiesto.
- **Cuotas del namespace de prueba.** En `deploy/flux/base/` (`aqs-test`): un `ResourceQuota` (`pods`, `requests.cpu`, `requests.memory`, `limits.cpu`, `limits.memory`, `count/jobs.batch`) dimensionado para el burst de una corrida (ensayo + `RUN_MAX_PARALLEL_RUNNERS` runners + deploy + reset simultáneos, con margen documentado en un comentario de una línea) y un `LimitRange` que fija `default`, `defaultRequest` y `max` por contenedor, de modo que **ningún Job sin límites** pueda crearse en el namespace (el ResourceQuota ya rechaza pods sin límites cuando cubre cpu/memoria).
- **HPA del controlador.** `HorizontalPodAutoscaler` (`autoscaling/v2`) sobre `go-run-controller`. **Decisión de la tarea:** `minReplicas: 1` y `maxReplicas: 1` hasta que se resuelva la persistencia compartida (C-45/C-49): con un diario en un PVC de un solo escritor, más réplicas romperían la integridad. El manifiesto existe para que escalar sea un cambio de un número, con un comentario que lo explique.
- **Scale-down del warm en idle.** CronJob `idle-check` (`go-reset idle-check`, `concurrencyPolicy: Forbid`, cada 10 minutos) junto a `housekeeping` y `rebuild`, con el mismo `serviceAccountName: go-reset`, límites y `securityContext`. Se verifica que `warm-policy` (`idleScaleDownAfter`, `minReplicasIdle`) sigue siendo la única fuente del umbral.
- **Timeouts y circuito en el controlador** (código, en `services/go-run-controller/`): a las llamadas HTTP a `go-governance`, `go-identity`, `go-warm-manager` y `go-reset` se les añade un **circuito** (5 fallos consecutivos de transporte o 5xx abren 10 s; los abortos del contexto del llamante **no** cuentan; sonda medio-abierta), con el mismo comportamiento que el de `services/ui-api/internal/auth/identity.go` (leerlo; se **escribe** aquí, no se importa). Con el circuito abierto, el gate **deniega** (fail-closed) y las fases fallan: nunca se asume éxito. Métricas `aqs_upstream_circuit_open{upstream}` y `aqs_upstream_calls_total{upstream,result}`.
- **Observabilidad.** Reglas de alerta en `deploy/flux/base/observability/` para: `aqs_warm_quarantined > 0`, `aqs_handoff_total` creciente, `aqs_upstream_circuit_open == 1` sostenido y Jobs fallidos, con `promtool test rules` que las prueba (el patrón de U5-T07/T15).

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Aplicar cualquier manifiesto a un clúster (`kubectl apply`, `flux reconcile`): los manifiestos son artefactos revisables en el PR (AUTONOMIA-01).
- Cambiar la lógica de fases, gates, reset o deploy de T02–T06 más allá del circuito descrito.
- Modificar `contracts/**`, los esquemas de `contracts/plans/`, `ci.yml`, `contracts.yml`, `policies.yml` o `scripts/ci/policies.sh`. Si una política existente rechaza algo legítimo, se reporta; se pueden **añadir** reglas y pruebas Rego nuevas en `policy/` solo si esta tarea lo exige explícitamente (aquí no se exige ninguna).
- Egress de `aqs-system` hacia la API de Kubernetes, MinIO y GitHub (C-51: decisión humana por direcciones de clúster).
- Secretos en claro, valores reales de MinIO, tags `latest`.
- El overlay `prod`: los cambios van a `base/` y `dev`; `prod` hereda sin parches nuevos.

---

## Archivos de contexto

- `unidades-y-tareas.md` (U2) y `aidlc-docs/inception/application-design/unit-task-plans/U2.md`
- `deploy/flux/base/control-plane.yaml`, `warm.yaml` (CronJobs `housekeeping`/`rebuild`, `warm-policy`), `namespaces.yaml`, `security/`, `observability/`, `minio/`, `kustomization.yaml`, `deploy/flux/dev/`
- `policy/*.rego` y sus `*_test.rego`; `scripts/ci/policies.sh` (qué corre y con qué imágenes)
- `tareas/U4-T07-alertas-retencion-dashboard.md` y `tareas/U4-T08-cableado-despliegue.md` (patrón de cableado, `fsGroup`, `Recreate`); `tareas/U5-T14-secrets-sops.md`; `tareas/U5-T15-politicas-en-ci.md`
- `services/ui-api/internal/auth/identity.go` (circuito de referencia) y las tareas U2-T02 a U2-T06 (nombres de variables)
- `tareas/candidatas.md` (C-45, C-49, C-51, C-53, C-70)

---

## Criterios de aceptación

Desde la raíz del worktree. Las políticas y `kubeconform` corren con la CLI `docker` (en este entorno es podman; `scripts/ci/policies.sh` ya funciona).

- [ ] **CA-1** — Los overlays construyen y pasan todas las políticas.
  ```bash
  bash scripts/ci/policies.sh 2>&1 | grep -E '^(OK|FALLA)'
  ```
  Esperado: solo líneas `OK` (kubeconform, conftest-test, conftest-combine, rbac-matrix para `dev` y `prod`; `conftest-verify`; `promtool-rules`; `check-secrets`) y ninguna `FALLA`. Antes de la tarea el resultado es el actual (todo `OK`), así que el rojo inicial se demuestra con CA-2 y CA-3.

- [ ] **CA-2** — El namespace de prueba tiene cuota y límites, y están dimensionados para el burst.
  ```bash
  K='docker run --rm -i --security-opt label=disable registry.k8s.io/kustomize/kustomize:v5.4.3'
  Y='docker run --rm -i --security-opt label=disable mikefarah/yq:4.44.3 -N'
  docker run --rm --security-opt label=disable -v "$PWD":/w:ro,z -w /w registry.k8s.io/kustomize/kustomize:v5.4.3 build deploy/flux/dev > /tmp/dev-build.yaml
  $Y 'select(.kind == "ResourceQuota" and .metadata.namespace == "aqs-test") | .spec.hard | keys | sort | join(",")' < /tmp/dev-build.yaml
  $Y 'select(.kind == "LimitRange" and .metadata.namespace == "aqs-test") | .spec.limits[0] | (.default.cpu != null) and (.defaultRequest.memory != null) and (.max.cpu != null)' < /tmp/dev-build.yaml
  ```
  Esperado: una línea con `count/jobs.batch,limits.cpu,limits.memory,pods,requests.cpu,requests.memory` y `true`. Antes de la tarea: ambas salidas vacías (rojo inicial). El codificador justifica en la bitácora los números con la cuenta del burst (1 ensayo + máx. de runners + deploy + reset + warm base).

- [ ] **CA-3** — Los tres servicios están cableados, sin Secrets en claro ni fakes.
  ```bash
  for s in go-run-controller go-warm-manager go-reset; do
    echo "== $s"
    $Y 'select(.kind == "Deployment" and .metadata.name == "'$s'") | .spec.template.spec.containers[0] | [(.readinessProbe.httpGet.path), (.livenessProbe.httpGet.path), (.securityContext.readOnlyRootFilesystem), (.env | map(select(.valueFrom.secretKeyRef != null)) | length > 0)] | join(" ")' < /tmp/dev-build.yaml
  done
  $Y 'select(.kind == "Deployment") | .spec.template.spec.containers[].env[]? | select(.name == "RUN_PHASES" or .name == "RUN_ALLOW_FAKE_PHASES") | .name + "=" + (.value // "")' < /tmp/dev-build.yaml
  $Y 'select(.kind == "Deployment" and (.metadata.name == "go-run-controller" or .metadata.name == "go-reset")) | .metadata.name + " " + (.spec.strategy.type) + " " + (.spec.template.spec.securityContext.fsGroup | tostring)' < /tmp/dev-build.yaml
  ```
  Esperado: por servicio `/readyz /healthz true true`; luego `RUN_PHASES=real` (y ninguna línea `RUN_ALLOW_FAKE_PHASES`); y `go-run-controller Recreate 65532`, `go-reset Recreate 65532`.

- [ ] **CA-4** — HPA con `maxReplicas: 1` documentado y el CronJob `idle-check`.
  ```bash
  $Y 'select(.kind == "HorizontalPodAutoscaler") | .metadata.name + " " + (.spec.minReplicas|tostring) + " " + (.spec.maxReplicas|tostring) + " " + .spec.scaleTargetRef.name' < /tmp/dev-build.yaml
  $Y 'select(.kind == "CronJob") | .metadata.name + " " + .spec.concurrencyPolicy + " " + (.spec.jobTemplate.spec.template.spec.containers[0].args | join(","))' < /tmp/dev-build.yaml | sort
  ```
  Esperado: `go-run-controller 1 1 go-run-controller` (el nombre del HPA puede variar; el codificador lo cita) y entre los CronJobs `housekeeping Forbid housekeeping`, `idle-check Forbid idle-check` y `rebuild Forbid rebuild`.

- [ ] **CA-5** — Sin tags `latest` ni imágenes sin digest/tag, y los Secrets siguen cifrados.
  ```bash
  bash scripts/ci/check-no-latest.sh; echo "no-latest rc=$?"
  bash scripts/ci/check-secrets.sh 2>&1 | grep -v 'Emulate'; echo "check-secrets rc=${PIPESTATUS[0]}"
  ```
  Esperado: `no-latest rc=0` y `check-secrets rc=0`.

- [ ] **CA-6** — Las alertas nuevas existen y su prueba pasa.
  ```bash
  bash scripts/ci/policies.sh 2>&1 | grep -E 'promtool-rules'
  docker run --rm --security-opt label=disable -v "$PWD":/w:ro,z -w /w --entrypoint promtool prom/prometheus:v2.55.1 check rules deploy/flux/base/observability/*.rules.yaml 2>&1 | tail -n 4
  grep -c -E 'aqs_warm_quarantined|aqs_upstream_circuit_open|aqs_handoff_total' deploy/flux/base/observability/*
  ```
  Esperado: `OK promtool-rules`, `SUCCESS` en `check rules` y un conteo ≥ `3`. (El codificador ajusta el glob al nombre real de los archivos de reglas; los cita.)

- [ ] **CA-7** — El circuito del controlador: se abre con 5 fallos, no cuenta abortos del llamante y el gate deniega con el circuito abierto.
  ```bash
  cd services/go-run-controller && go test -race -run 'Circuit' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS` en al menos cinco pruebas (abre con 5 fallos; 5 abortos del contexto padre **no** abren, como en U1-T07 F-01; sonda medio-abierta que cierra; sonda abortada que libera; gate con circuito abierto → deniega y no avanza la corrida), sin `FAIL`.

- [ ] **CA-8** — Pruebas del servicio en verde y alcance.
  ```bash
  (cd services/go-run-controller && go test -race ./... 2>&1 | grep -c FAIL && go vet ./... && test -z "$(gofmt -l .)" && echo ok); git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(deploy/flux/(base|dev)/|services/go-run-controller/|bitacoras/U2-T07.md|revisiones/U2-T07/)' | wc -l
  ```
  Esperado: `0` (ningún FAIL), `ok`, `0` y `0`.

---

## Plan de pruebas

- Los criterios CA-2 a CA-6 son la prueba: leen del build real de los overlays con `yq`, no del texto de los archivos.
- Circuito: reloj inyectable y `httptest` (como U1-T07); casos de borde del contexto cancelado.
- Negativas: añadir a un manifiesto `RUN_ALLOW_FAKE_PHASES=true`, un Secret en claro o `image: x:latest` debe hacer fallar `policies.sh` (el codificador lo prueba en una copia temporal, lo registra y lo revierte).

**Rojo primero:** CA-2 y CA-4 devuelven vacío antes de la tarea; el codificador pega esa salida literal.

---

## Notas

- **DECISIÓN DEL HUMANO (2026-10-07): opción A.** Los Jobs `deploy-{run}` y `reset-{run}` **no están en el camino de ejecución**: `go-warm-manager` aplica el artefacto sobre `warm-app` **en proceso** (U2-T03b) y `go-reset` ejecuta el reset en proceso (U2-T06), ambos con la ServiceAccount del servicio en `aqs-system` y el Role `aqs-test-operator` que ya existe (`deploy/flux/base/security/rbac.yaml`). No se relaja `policy/isolation.rego`, no se crea ServiceAccount en `aqs-test` y no se construye imagen de deployer. Consecuencias para esta tarea: (1) **verificar el RBAC real**: el Role actual concede `deployments`/`statefulsets` (patch/update), `configmaps`, `pods` y `jobs`; si el escalado de los adaptadores usa el subrecurso `scale` (`GetScale`/`UpdateScale`), añadir `deployments/scale` y `statefulsets/scale`, con su prueba de la matriz RBAC (`scripts/ci/rbac-matrix.sh`) y sin ampliar nada más; (2) el Role es compartido por las tres ServiceAccounts: se **documenta** (comentario de una línea) y la separación por servicio queda como candidata, no se hace aquí; (3) `render-deploy-job`/`render-reset-job` ya no se usan en tiempo de ejecución: no se cablean.
- **Heredado de la ola 2 (U2-T02/T03/T06), a cablear aquí:** variables y Secrets de los tres servicios; RBAC de `go-warm-manager`/`go-reset` sobre `configmaps` (`warm-state`: get/create/update), `pods` y escalado de `deployments`/`statefulsets` en `aqs-test` (ver la decisión A); volumen para el outbox (`/tmp` no sobrevive a un reinicio y la idempotencia del outbox se pierde); los CronJobs `housekeeping`/`rebuild`/`idle-check` lanzan `go-reset` **sin** variables ni volumen compartido de `RESET_DATA_DIR`/`RESET_OUTBOX_FILE`, así que no ven las sesiones de la API ni sus eventos llegan a nadie; montar o empaquetar `RESET_BASELINE_SCRIPT` y fijar `RESET_BASELINE_VERSION`/`script version`; exclusión entre procesos de `rebuild` y `housekeeping` (ambos arrancan a las 03:00 del domingo; Lease o desfasar los horarios); que `go-run-controller` llame a `PUT /sessions` de `go-reset` (heartbeat) para que `housekeeping` no resetee una corrida en curso.

- **`yq` (aprendido en U4).** `keys` no ordena (`keys | sort`); `x // "y"` trata `false` como ausente; `if/then` de jq no parsea en yq; `yq -N` sobre un build imprime líneas en blanco (filtra con `grep -v '^$'`). Si un criterio no puede dar el esperado por esa causa, el codificador lo reporta con comando y salida; no rellena a ciegas.
- **Podman.** La CLI `docker` local es podman; en volúmenes se usa `:z` (o `:ro,z`). La nota «Emulate Docker CLI using podman» deja de aparecer si existe `/etc/containers/nodocker`; si no existe, los criterios que capturan `2>&1` de `docker` se filtran con `grep -v Emulate`.
- **Decisión de la tarea sobre el HPA.** Se explica en el propio manifiesto; subir `maxReplicas` queda para después de C-45/C-49. Es candidata.
- **Candidatas a registrar:** egress de `aqs-system` (C-51); persistencia y transporte compartidos para escalar el controlador (C-45/C-49); `ResourceQuota` por unidad de corrida (no por namespace) si se permiten corridas concurrentes; política Rego que exija `ResourceQuota` y `LimitRange` en `aqs-test`.
- **Informes del loop.** El diff de `revisiones/<tarea>/` no cuenta como desborde.
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
