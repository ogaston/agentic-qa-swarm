# U8-T03 — Imagen `aqs-runner`: ensayo y ejecución de flujos dentro de los Jobs

**Unidad:** U8 — Producto demostrable (ciclo completo en kind)
**Historias que implementa:** US-M5 (ensayo bloqueante), US-M6 (ejecución con evidencia); candidatas C-100 y C-87 (parte).
**Depende de:** U7 cerrada. **Ola 1**. **Tope: 3 rondas**.

---

## Alcance

**Dentro** (una línea, concreta):

> Crear `services/aqs-runner/` (Go, con Dockerfile) que implemente exactamente la interfaz que ya usan los Jobs de `go-run-controller`: `aqs-runner rehearse --run <id> --flow <id> --target <url>` (lee `REHEARSAL_STEPS` y `REHEARSAL_INVARIANT`) y `aqs-runner http-steps --flow <id> --target <url>` (los argumentos y variables que genera `runner.HTTPStepsExecutor`); ejecuta los pasos HTTP contra el warm, sale `0` si todos cumplen su `expect_status` y distinto de `0` si no, y escribe en stdout un resumen JSON por paso; y apuntar `REHEARSAL_IMAGE` y `RUNNER_IMAGE` de `deploy/flux/base/control-plane.yaml` a `ghcr.io/ogaston/agentic-qa-swarm/aqs-runner:0.0.0`.

Detalle:

- La interfaz se toma de `services/go-run-controller/rehearsal/job.go` (`BuildJob`) y `services/go-run-controller/runner/executor.go` (`HTTPStepsExecutor.Plan`). **No** se cambia el controlador: si la interfaz es ambigua, el codificador lo registra y escala.
- Sin LLM, sin credenciales y sin egress más allá del destino: el binario solo habla con `--target`. Rechaza `--target` que no sea `http(s)://` y rutas que no empiecen por `/`. Timeout por paso (5 s) y total (el plazo lo pone el Job).
- Imagen mínima no root compatible con el `securityContext` de los Jobs (`readOnlyRootFilesystem`, `runAsNonRoot`, uid 65532).
- Al ser una imagen nueva bajo `services/`, `scripts/kind/build-images.sh` y `lib.sh` pasan a esperar el número que da `scripts/ci/list-services.sh` en lugar del `9` fijo (cierra F-02 de U7-T02) y la CI la construye sola.

**Fuera**:

- Motor k6 (C-87 sigue abierta para cuando el runner ejecute el artefacto k6 de U3).
- Cambios en `go-run-controller`, en el formato de `FlowPlan` o en la evidencia (la sube el controlador).
- La app de referencia (U8-T04).

---

## Archivos de contexto

- `services/go-run-controller/rehearsal/job.go`, `services/go-run-controller/runner/executor.go`, `services/go-run-controller/runner/job.go`, `services/go-run-controller/plan/`
- `services/go-run-controller/cmd/go-run-controller/real.go` (`render-rehearsal-job`, `render-runner-job`)
- `contracts/plans/flow-plan.schema.json`
- `scripts/ci/list-services.sh`, `scripts/kind/build-images.sh`, `scripts/kind/lib.sh`, `.github/workflows/ci.yml`
- Un Dockerfile existente de referencia: `services/go-identity/Dockerfile`

---

## Criterios de aceptación

- [ ] **CA-1** — Pruebas del binario.
  ```bash
  (cd services/aqs-runner && go test ./... ; echo "rc=$?")
  ```
  Esperado: `rc=0`, con pruebas de: todos los pasos cumplen → `0`; un paso con estado distinto → `≠0`; destino caído → `≠0`; `--target` inválido → `≠0` sin hacer peticiones.

- [ ] **CA-2** — Interfaz idéntica a la que generan los Jobs (leída de los Jobs renderizados, no copiada a mano).
  ```bash
  bash scripts/test/u8-runner-contract.sh; echo "rc=$?"
  ```
  Esperado: `OK rehearse-args`, `OK http-steps-args`, `OK paso-fallido-rc`, `rc=0`. El script (nuevo) toma `args` y `env` de `go run ./cmd/go-run-controller render-rehearsal-job …` y `render-runner-job …`, ejecuta la imagen con `podman run` contra un servidor HTTP local de prueba y comprueba el código de salida.

- [ ] **CA-3** — Imagen construida, cargada y referenciada.
  ```bash
  bash scripts/kind/build-images.sh >/dev/null && podman image exists ghcr.io/ogaston/agentic-qa-swarm/aqs-runner:0.0.0 && echo imagen
  kubectl kustomize deploy/flux/kind | grep -A1 -E 'name: (REHEARSAL_IMAGE|RUNNER_IMAGE)$' | grep -c 'aqs-runner:0.0.0'
  podman run --rm --user 65532 --read-only ghcr.io/ogaston/agentic-qa-swarm/aqs-runner:0.0.0 --help >/dev/null 2>&1; echo "rc=$?"
  ```
  Esperado: `imagen`, `2` y `rc=0`.

- [ ] **CA-4** — Políticas y lista de servicios coherentes.
  ```bash
  bash scripts/ci/policies.sh 2>&1 | grep -c '^FALLA'; grep -c -E '\b9\b' scripts/kind/build-images.sh scripts/kind/lib.sh | awk -F: '{s+=$2} END {print s}'
  ```
  Esperado: `0` y `0`.

- [ ] **CA-5** — Alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/aqs-runner/|deploy/flux/base/control-plane\.yaml|scripts/kind/|scripts/test/u8-runner-contract\.sh|deploy/kind/README\.md|bitacoras/U8-T03\.md|revisiones/U8-T03/)' | wc -l
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

- **Rojo primero:** `podman run … aqs-runner:0.0.0` no existe; pegar el error.
- Pruebas negativas antes que positivas.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
