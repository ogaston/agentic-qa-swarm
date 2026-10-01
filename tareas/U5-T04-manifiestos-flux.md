# U5-T04 — Manifiestos Flux por entorno (control plane + entorno warm)

**Unidad:** U5 — Plataforma & GitOps
**Historias que implementa:** US-M10
**Depende de:** U5-T01 (árbol `deploy/flux/{base,dev,prod}/`). Ola 2, en paralelo con U5-T02 y U5-T03.

---

## Alcance

**Dentro** (una línea, concreta):

> Escribir los manifiestos Kustomize de `deploy/flux/base` y los overlays `dev` y `prod`. Deben contener dos namespaces: `aqs-system` (control plane) y `aqs-test` (entorno warm). En el control plane van un Deployment y un Service por cada uno de los 7 servicios de `services.md`. El entorno warm lleva `warm-app` (Deployment), `warm-db` (StatefulSet de Postgres), `warm-redis` (Deployment) y los CronJobs `housekeeping` y `rebuild`, más un ConfigMap `warm-policy` con la política de scale-down en idle. Cada overlay lleva su objeto Flux `Kustomization` (`kustomize.toolkit.fluxcd.io/v1`) apuntando a su propio path. Todo debe ser válido con `kustomize build` y `kubeconform`, sin aplicar nada.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- MinIO: es **U5-T05**.
- RBAC y NetworkPolicy: es **U5-T06**. Ni siquiera como placeholder.
- Observabilidad (ServiceMonitor, dashboards, retención): es **U5-T07**.
- Backups: es **U5-T08**.
- La lógica de reset, rebuild o scale-down: es U2 (`go-reset`). Aquí solo van el CronJob con la imagen y los argumentos, y el ConfigMap con la política.
- Secrets reales o credenciales. Si un contenedor necesita una contraseña, se referencia un Secret por nombre (`secretKeyRef`) sin crearlo.
- `.github/workflows/*` y `scripts/ci/*`: es **U5-T03**. `contracts/`: es **U5-T02**.
- Modificar `README.md` de la raíz.
- **Cualquier** `kubectl apply`, `flux bootstrap`, `flux reconcile` o `flux diff` contra un clúster (AUTONOMIA-01). `flux diff kustomization` requiere un clúster, así que lo verifica el humano.

---

## Archivos de contexto

Rutas, no contenido pegado:

- `unidades-y-tareas.md`
- `aidlc-docs/inception/application-design/unit-task-plans/U5.md`
- `aidlc-docs/inception/application-design/unit-of-work.md` (layout de `deploy/flux/`, U2 y U5)
- `aidlc-docs/inception/application-design/services.md` (los 7 servicios del control plane y los CronJobs)
- `aidlc-docs/inception/application-design/components.md` (C3 y C5: estados del warm, reset, rebuild)
- `aidlc-docs/inception/requirements/requirements.md` (M7, M10, NF-RES-09 scale-down en idle, NF-SEG-01)

---

## Criterios de aceptación

Cada uno con **su comando**. El revisor los va a correr él mismo, uno por uno, desde la raíz del worktree. `K` es un alias para no repetir la imagen fijada:

```bash
K='docker run --rm --security-opt label=disable -v '"$PWD"':/w -w /w registry.k8s.io/kustomize/kustomize:v5.4.3'
```

- [ ] **CA-1** — Los dos overlays construyen.
  ```bash
  for e in dev prod; do $K build deploy/flux/$e > /dev/null || { echo "FALLA $e"; exit 1; }; done; echo BUILD OK
  ```
  Esperado: `BUILD OK`. Antes de la tarea: `FALLA dev` (rojo inicial).

- [ ] **CA-2** — `kubeconform` valida todo, incluidos los CRDs de Flux, sin recursos omitidos.
  ```bash
  for e in dev prod; do $K build deploy/flux/$e | docker run --rm --security-opt label=disable -i ghcr.io/yannh/kubeconform:v0.6.7 -strict -summary -schema-location default -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json' -; done
  ```
  Esperado: dos líneas de resumen con `Invalid: 0, Errors: 0, Skipped: 0`.

- [ ] **CA-3** — El entorno warm está completo en `aqs-test` en ambos overlays.
  ```bash
  for e in dev prod; do $K build deploy/flux/$e | awk '/^kind:/{k=$2} /^  name:/{n=$2} /^  namespace:/{print k"/"n"@"$2}' | grep -E '@aqs-test$' | sort | grep -c -E '^(Deployment/warm-app|StatefulSet/warm-db|Deployment/warm-redis|CronJob/housekeeping|CronJob/rebuild|ConfigMap/warm-policy)@'; done
  ```
  Esperado: `6` y `6`.

- [ ] **CA-4** — Los 7 servicios del control plane están en `aqs-system` (Deployment y Service cada uno).
  ```bash
  $K build deploy/flux/prod | awk '/^kind:/{k=$2} /^  name:/{n=$2} /^  namespace:/{print k"/"n"@"$2}' | grep -c -E '^(Deployment|Service)/(ui-api|go-intake|go-run-controller|go-warm-manager|go-reset|go-governance|go-identity)@aqs-system$'
  ```
  Esperado: `14`.

- [ ] **CA-5** — En prod ninguna imagen usa `latest` ni va sin tag o digest.
  ```bash
  $K build deploy/flux/prod | grep -E '^\s*image:' | grep -v -E ':[A-Za-z0-9._-]+$|@sha256:[0-9a-f]{64}$' | wc -l
  $K build deploy/flux/prod | grep -c -E 'image:.*:latest'
  ```
  Esperado: `0` y `0`.

- [ ] **CA-6** — Cada overlay tiene su `Kustomization` de Flux apuntando a su propio path, con `prune` activado.
  ```bash
  for e in dev prod; do $K build deploy/flux/$e | grep -A12 -E '^kind: Kustomization' | grep -E "path: \./deploy/flux/$e$|prune: true" | wc -l; done
  ```
  Esperado: `2` y `2`.

- [ ] **CA-7** — Todos los contenedores del warm y del control plane declaran `resources.limits`, y el warm declara probes.
  ```bash
  $K build deploy/flux/prod | grep -c -E '^\s+image:'; $K build deploy/flux/prod | grep -c -E '^\s+limits:'
  $K build deploy/flux/prod | grep -c -E '^\s+(readinessProbe|livenessProbe):'
  ```
  Esperado: los dos primeros números son iguales; el tercero es `≥ 6` (readiness y liveness en `warm-app`, `warm-db` y `warm-redis`).

- [ ] **CA-8** — Árbol limpio tras el commit y sin `.gitkeep` sobrante en `deploy/flux/`.
  ```bash
  git status --short | wc -l; git ls-files deploy/flux | grep -c '\.gitkeep$'
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

Qué pruebas se escriben **en esta tarea**, no después:

- Rojo inicial de CA-1 (`FALLA dev`) registrado en la bitácora.
- Prueba negativa de CA-5 sobre una copia temporal (`git archive HEAD`): cambiar una imagen de prod a `:latest` y comprobar que el primer conteo o el segundo dejan de ser `0`.
- Prueba negativa de CA-2 sobre una copia temporal: introducir un campo inexistente (por ejemplo `spec.replicaz`) y comprobar que `kubeconform -strict` reporta `Invalid` distinto de 0.

**Rojo primero:** el codificador reproduce `FALLA dev` con el comando literal de CA-1 y lo registra en su bitácora antes de crear nada.

---

## Notas

- Imágenes del control plane: `ghcr.io/ogaston/agentic-qa-swarm/<servicio>:0.0.0` como tag fijado inicial. Imágenes del warm: Postgres y Redis oficiales con tag de versión exacta (nunca `latest`). `warm-app` usa una imagen placeholder fijada; U2 la reemplaza por corrida.
- `warm-policy` documenta `idleScaleDownAfter` y `minReplicasIdle`; quien la aplica es `go-reset` (U2).
- `dev` y `prod` difieren por patches de Kustomize (réplicas, recursos). No se duplica `base`.
- Los nombres de namespace `aqs-system` y `aqs-test` los fija esta tarea; U5-T05..T08 los reutilizan.
- La bitácora pega el **comando literal** de cada criterio y su salida, no abreviaturas (hallazgos F-02 y F-03 de U5-T01).
- Esta tarea no ejecuta ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
