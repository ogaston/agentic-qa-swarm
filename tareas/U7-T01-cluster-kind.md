# U7-T01 — Clúster kind local sobre podman (`kind-up` / `kind-down`)

**Unidad:** U7 — Plataforma en kind local (podman)
**Historias que implementa:** US-M10 (GitOps del producto: los manifiestos se ejercitan en un clúster real), base de P5.
**Depende de:** U5 terminada y **decisión C-96 aprobada por el humano**. **Ola 1**. **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea, concreta):

> Añadir `deploy/kind/cluster.yaml` y `scripts/kind/kind-up.sh` / `kind-down.sh`, que crean y destruyen un clúster kind efímero llamado `aqs` sobre podman (`KIND_EXPERIMENTAL_PROVIDER=podman`), con imagen de nodo fijada por digest, y una guarda común que impide operar sobre cualquier contexto de `kubectl` distinto de `kind-aqs`.

Detalle:

- **Configuración** (`deploy/kind/cluster.yaml`): un nodo `control-plane`, `networking.disableDefaultCNI: false` (kindnet, que aplica NetworkPolicy), sin `extraPortMappings` (el acceso es por `kubectl port-forward` en loopback). La imagen de nodo es la que las notas de la versión de kind 0.33.0 declaran por defecto, **con `@sha256:`**; prohibido un tag sin digest.
- **Guarda** (`scripts/kind/lib.sh`): función `require_kind_context` que sale con código `3` y el mensaje `contexto <x> no es kind-aqs` si `kubectl config current-context` no es exactamente `kind-aqs`. Todo script de `scripts/kind/` que hable con el clúster la llama antes de cualquier `kubectl`. Los scripts nunca cambian el contexto actual salvo `kind-up.sh` (que lo deja en `kind-aqs`, como hace kind).
- **`kind-up.sh`**: comprueba requisitos (`kind`, `podman`, `kubectl`; cgroup v2 con delegación para podman sin root, o podman con root) y, si falta algo, sale con un mensaje accionable sin crear nada. Es idempotente: si `aqs` ya existe, no lo recrea y sale `0`. Espera a que el nodo esté `Ready` (`kubectl wait --for=condition=Ready node --all --timeout=180s`).
- **`kind-down.sh`**: `kind delete cluster --name aqs`; idempotente (sale `0` si no existe). No toca otros clústeres ni contenedores de podman.
- **README** `deploy/kind/README.md`: requisitos en Fedora (podman sin root vs. con root), uso, y advertencia de que el clúster es **efímero y local**: nunca se apunta a dev/prod.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Construir o cargar imágenes (U7-T02), aplicar manifiestos (U7-T03), ingress, registro local, varios nodos, Flux en el clúster.
- Cambios en `deploy/flux/**`, en servicios o en agentes.

---

## Archivos de contexto

- `deploy/flux/README.md` (si existe) y `deploy/flux/base/kustomization.yaml` (qué se desplegará después)
- `docs/operaciones/` (convenciones de runbooks)
- `scripts/test/u3-up.sh` (estilo de scripts: `set -euo pipefail`, mensajes `OK|FALLA`)
- `tareas/candidatas.md` (C-96)

---

## Criterios de aceptación

Desde la raíz del worktree, con kind 0.33, podman y kubectl instalados.

- [ ] **CA-1** — Imagen de nodo fijada.
  ```bash
  grep -c -E 'image: kindest/node:v[0-9.]+@sha256:[0-9a-f]{64}' deploy/kind/cluster.yaml
  bash -n scripts/kind/*.sh && shellcheck scripts/kind/*.sh 2>&1 | wc -l
  ```
  Esperado: `1` y `0` (si `shellcheck` no está instalado, correrlo con `podman run --rm -v "$PWD":/mnt:Z koalaman/shellcheck:v0.10.0 scripts/kind/*.sh`).

- [ ] **CA-2** — Creación y lectura de vuelta.
  ```bash
  bash scripts/kind/kind-up.sh && kubectl config current-context && kubectl get nodes --no-headers | awk '{print $2}' && kubectl get storageclass --no-headers | grep -c '(default)'
  ```
  Esperado: `kind-aqs`, `Ready` y `1`.

- [ ] **CA-3** — Idempotencia.
  ```bash
  bash scripts/kind/kind-up.sh; echo "rc=$?"; kind get clusters | grep -c '^aqs$'
  ```
  Esperado: `rc=0` y `1` (no se recrea: la edad del nodo en `kubectl get nodes` no vuelve a cero).

- [ ] **CA-4** — La guarda rechaza otro contexto.
  ```bash
  kubectl config set-context aqs-guard-test --cluster=kind-aqs --user=kind-aqs >/dev/null
  kubectl config use-context aqs-guard-test >/dev/null
  bash -c 'source scripts/kind/lib.sh; require_kind_context'; echo "rc=$?"
  kubectl config use-context kind-aqs >/dev/null; kubectl config delete-context aqs-guard-test >/dev/null
  ```
  Esperado: el mensaje `contexto aqs-guard-test no es kind-aqs` y `rc=3`.

- [ ] **CA-5** — Destrucción.
  ```bash
  bash scripts/kind/kind-down.sh; echo "rc=$?"; kind get clusters | grep -c '^aqs$'; podman ps -a --format '{{.Names}}' | grep -c '^aqs-control-plane$'
  bash scripts/kind/kind-down.sh; echo "rc=$?"
  ```
  Esperado: `rc=0`, `0`, `0` y otra vez `rc=0`.

- [ ] **CA-6** — Alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(deploy/kind/|scripts/kind/|bitacoras/U7-T01\.md|revisiones/U7-T01/)' | wc -l
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

- Los criterios son la prueba: crean, leen de vuelta con `kubectl`/`kind`/`podman` y destruyen.
- **Rojo primero:** pegar la salida de `bash scripts/kind/kind-up.sh` (no existe) antes de implementar.
- Dejar el clúster destruido al terminar la ronda (CA-5 último).

---

## Notas

- Las operaciones contra el clúster solo están permitidas en el contexto `kind-aqs` y por la autorización de C-96. Nada de esto se aplica a dev/prod.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
