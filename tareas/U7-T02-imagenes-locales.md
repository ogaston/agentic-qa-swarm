# U7-T02 — Construir con podman y cargar en kind las 9 imágenes de la plataforma

**Unidad:** U7 — Plataforma en kind local (podman)
**Historias que implementa:** US-M10.
**Depende de:** U7-T01 **fusionada**. **Ola 2**. **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea, concreta):

> Añadir `scripts/kind/build-images.sh` y `scripts/kind/load-images.sh`, que construyen con podman las 7 imágenes de `services/*` y las 2 de `agents/agent-*` con el mismo contexto que la CI y el tag que esperan los manifiestos (`ghcr.io/ogaston/agentic-qa-swarm/<nombre>:0.0.0`), y las cargan en el clúster `aqs` con `kind load image-archive`.

Detalle:

- **Lista de imágenes**: se obtiene de `scripts/ci/list-services.sh` (o de los directorios con `Dockerfile` bajo `services/` y `agents/`); no se escribe a mano en dos sitios. Si la lista no da 9, el script falla.
- **Construcción**: `podman build -t <ref> <dir>` con el directorio del servicio como contexto (igual que `ci.yml`). Sin `latest`. Una imagen que falla corta el script con su nombre.
- **Carga**: `podman save` a un archivo en `mktemp -d` (borrado con `trap`) y `kind load image-archive --name aqs`. Llama a `require_kind_context` (U7-T01) antes de cargar.
- Variable opcional `AQS_IMAGES="ui-api go-identity"` para construir/cargar solo un subconjunto.
- README `deploy/kind/README.md`: sección «Imágenes».

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Publicar en un registro, firmar, SBOM (eso es la CI), registro local en el clúster.
- Cambiar Dockerfiles, servicios o manifiestos. Si una imagen no construye, se registra como candidata y se escala.

---

## Archivos de contexto

- `tareas/U7-T01-cluster-kind.md` y `bitacoras/U7-T01.md`
- `.github/workflows/ci.yml` (contexto de build y tags)
- `scripts/ci/list-services.sh`
- `deploy/flux/base/control-plane.yaml` (referencias de imagen)

---

## Criterios de aceptación

Desde la raíz del worktree, con el clúster creado (`bash scripts/kind/kind-up.sh`).

- [ ] **CA-1** — Scripts válidos.
  ```bash
  bash -n scripts/kind/*.sh && podman run --rm -v "$PWD":/mnt:Z -w /mnt koalaman/shellcheck:v0.10.0 scripts/kind/*.sh 2>&1 | wc -l
  ```
  Esperado: `0`.

- [ ] **CA-2** — Construcción de las 9.
  ```bash
  bash scripts/kind/build-images.sh; echo "rc=$?"
  podman images --format '{{.Repository}}:{{.Tag}}' | grep -c -E '^ghcr\.io/ogaston/agentic-qa-swarm/(ui-api|go-intake|go-run-controller|go-warm-manager|go-reset|go-governance|go-identity|agent-planner|agent-reporter):0\.0\.0$'
  ```
  Esperado: `rc=0` y `9`.

- [ ] **CA-3** — Carga, leída de vuelta dentro del nodo.
  ```bash
  bash scripts/kind/load-images.sh; echo "rc=$?"
  podman exec aqs-control-plane crictl images | grep -c -E 'ghcr\.io/ogaston/agentic-qa-swarm/[a-z-]+ +0\.0\.0'
  ```
  Esperado: `rc=0` y `9`.

- [ ] **CA-4** — Las referencias coinciden con los manifiestos.
  ```bash
  comm -23 <(grep -h -o -E 'ghcr\.io/ogaston/agentic-qa-swarm/[a-z-]+:0\.0\.0' deploy/flux/base/*.yaml | sort -u) <(podman exec aqs-control-plane crictl images -o json | grep -o -E 'ghcr\.io/ogaston/agentic-qa-swarm/[a-z-]+:0\.0\.0' | sort -u) | wc -l
  ```
  Esperado: `0` (toda imagen propia que piden los manifiestos está en el nodo).

- [ ] **CA-5** — Subconjunto y guarda.
  ```bash
  AQS_IMAGES=go-identity bash scripts/kind/build-images.sh 2>&1 | grep -c -E 'agentic-qa-swarm/(ui-api|go-intake)'
  kubectl config set-context aqs-guard-test --cluster=kind-aqs --user=kind-aqs >/dev/null && kubectl config use-context aqs-guard-test >/dev/null
  bash scripts/kind/load-images.sh; echo "rc=$?"
  kubectl config use-context kind-aqs >/dev/null; kubectl config delete-context aqs-guard-test >/dev/null
  ```
  Esperado: `0` y `rc=3`.

- [ ] **CA-6** — Alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(scripts/kind/|deploy/kind/README\.md|bitacoras/U7-T02\.md|revisiones/U7-T02/)' | wc -l
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

- Los criterios leen de vuelta con `podman images` y `crictl` dentro del nodo, no con el código de salida del script.
- **Rojo primero:** pegar la salida de `bash scripts/kind/build-images.sh` (no existe).
- Al terminar: `bash scripts/kind/kind-down.sh`.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
