# U2-T03b — `go-warm-manager`: deploy en proceso (opción A), versión mínima

**Unidad:** U2 — Orquestación & Warm Sandbox Lifecycle
**Historias que implementa:** US-M2 (deploy sobre el warm; fail-closed tras 2 reintentos)
**Depende de:** U2-T03 fusionada. Sustituye a la antigua «T03b: candados de prueba» (decisión A del humano, 2026-10-07: sin Jobs sin token). **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano. **Versión mínima a propósito**: el humano pidió la solución fácil; lo que no está aquí se cortó y queda como candidata.

---

## Alcance

**Dentro** (una línea, concreta):

> Reemplazar en `services/go-warm-manager/` el Job `deploy-{run}` por un parche **en proceso** de la imagen del Deployment `warm-app` (con la ServiceAccount del servicio), esperando el rollout completo, con el mismo tope de 3 intentos y fail-closed, y añadir las tres pruebas que mataban mutaciones vivas.

Detalle:

- **Deploy en proceso.** `Deploy(run, artifact)` hace `patch` de la imagen del contenedor de `warm-app` a `artifact.ref` y espera el rollout **completo** (`observedGeneration`, `updatedReplicas`/`availableReplicas`, sin pods extra: reutiliza el criterio de `AppReady` de `go-reset`, copiado, no importado) hasta `WARM_JOB_TIMEOUT` (renombrar `WARM_DEPLOY_TIMEOUT`). Sigue exigiendo warm `ready`+`reset_verified` y lo pasa a `dirty` con CAS **antes** de parchear. Máximo 3 intentos (inicial + 2 reintentos); agotados → `deploy.failed` con `reason` + 1 handoff. Un timeout del rollout **nunca** es éxito.
- **Solo `published-image`.** `build-from-repo` se rechaza con `422` y `deploy.failed`-equivalente síncrono (`reason: build-from-repo no soportado`): fail-closed. Se mantienen la validación de registros permitidos y la prohibición de `latest`.
- **Se elimina el camino de Jobs:** constructor del Job, `render-deploy-job`, `WARM_DEPLOYER_IMAGE`, `ResolveOrphans` y su cableado (ya no hay Jobs que huérfanear). `GET /deploys/{run_id}` sigue respondiendo desde memoria; **tras un reinicio** el warm persistido está `dirty` (CAS en el ConfigMap), así que no se despliega nada hasta un reset verificado (documentarlo en el README).
- **Tres pruebas que antes sobrevivían:** (1) rollout que no completa hasta el timeout → `deploy.failed` + 1 handoff, nunca `deploy.done`; (2) `InferSurface` con deploy `pending` → 409; (3) `StartDeploy` repetido con el mismo `run_id` en memoria devuelve el estado existente sin tocar el warm.
- README: `WARM_DEPLOY_TIMEOUT`, deploy en proceso, `build-from-repo` no soportado, estado de deploy en memoria.

**Fuera** (cortado a propósito):

- Soporte de `build-from-repo`, persistir el estado del deploy en anotaciones, resolver deploys en vuelo tras un reinicio, titular del warm en `WarmState`, validación OCI estricta, presupuesto de reintentos persistido: candidatas.
- RBAC y cableado de despliegue (**U2-T07**), `contracts/**`, `deploy/**`, `policy/**`, workflows.
- Comandos contra un clúster o la nube.

---

## Archivos de contexto

- `services/go-warm-manager/` (`service.go`, `jobspec.go`, `adapters/kube/`, `cmd/go-warm-manager/main.go`, `README.md`), `tareas/U2-T03-warm-manager.md`
- `revisiones/U2-T03/ronda-3.md` (mutaciones X4, X20 y barrido), `services/go-reset/internal/adapters/kube/kube.go` (criterio de rollout completo)

---

## Criterios de aceptación

Desde la raíz del worktree.

- [ ] **CA-1** — Pruebas en verde con `-race`.
  ```bash
  cd services/go-warm-manager && go test -race -count=1 ./... 2>&1 | grep -c FAIL; go test -race -count=1 -v ./... | grep -c -E '^\s*--- PASS'
  ```
  Esperado: `0` y ≥ `100`.

- [ ] **CA-2** — **Exactamente 3 intentos y nunca un Job.** Leído de vuelta del clientset falso: el Deployment recibió 3 parches de imagen, no existe ningún `Job`, hay 1 `deploy.failed` con `reason` y 1 handoff.
  ```bash
  cd services/go-warm-manager && go test -race -count=1 -run 'Deploy(Retry|Exhausted|NeverCreatesJobs|RolloutTimeout)' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS` en al menos cuatro pruebas, incluida la del rollout que nunca completa (nunca `deploy.done`). **Mutación obligatoria** en la bitácora: `waitRollout` devolviendo éxito ante el timeout debe poner roja esa prueba.

- [ ] **CA-3** — Las otras dos pruebas y la valla de `build-from-repo`.
  ```bash
  cd services/go-warm-manager && go test -race -count=1 -run 'SurfaceRequiresFinishedDeploy|StartDeployIdempotent|BuildFromRepoRejected' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS` en al menos tres; `build-from-repo` da `422` y no toca el Deployment.

- [ ] **CA-4** — El camino de Jobs desapareció.
  ```bash
  grep -rn -E 'batchv1|render-deploy-job|WARM_DEPLOYER_IMAGE|ResolveOrphans' services/go-warm-manager --include=*.go --include=Dockerfile --include=README.md | wc -l
  ```
  Esperado: `0`.

- [ ] **CA-5** — Arranque y API siguen fail-closed.
  ```bash
  cd services/go-warm-manager && go test -race -count=1 -run 'API|Config|Fence' -v ./... | grep -c -E '^\s*--- PASS'
  ```
  Esperado: ≥ `20` (los de U2-T03 siguen verdes: 401 antes del enrutado, 409 con warm no listo, vallas del `fake`, configuración inválida).

- [ ] **CA-6** — Higiene, imagen y alcance.
  ```bash
  (cd services/go-warm-manager && go vet ./... && go vet -tags minio ./... && test -z "$(gofmt -l .)" && echo ok); docker build -q -t aqs-go-warm-manager:ci services/go-warm-manager >/dev/null && docker inspect aqs-go-warm-manager:ci --format '{{.Config.User}}'; git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/go-warm-manager/|bitacoras/U2-T03b.md|revisiones/U2-T03b/)' | wc -l
  ```
  Esperado: `ok`, `65532:65532`, `0` y `0`.

---

## Plan de pruebas

- `kubernetes/fake` con un reactor que simula el avance del rollout (con retardo) y otro que nunca lo completa; reloj inyectable.
- Barrido corto de clase (la familia de U2-T03): para el deploy en proceso, tabla camino→prueba en la bitácora (CAS ready→dirty, parche fallido, rollout timeout, handoff, evento, reinicio con warm `dirty`).

**Rojo primero:** registrar que hoy `Deploy` crea Jobs y que un rollout que no completa no está probado.

---

## Notas

- **Tope de 3 rondas** y **versión mínima** (humano). No ampliar el alcance: lo que sobre es candidata.
- Go 1.26.8; podman con `:z`; `ajv`/`yq` vía `npx`/imagen.
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
