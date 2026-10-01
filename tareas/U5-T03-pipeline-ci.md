# U5-T03 — Pipeline de CI (GitHub Actions)

**Unidad:** U5 — Plataforma & GitOps
**Historias que implementa:** US-M10
**Depende de:** U5-T01 (árbol `.github/workflows/`, `services/`, `agents/`). Ola 2, en paralelo con U5-T02 y U5-T04.

---

## Alcance

**Dentro** (una línea, concreta):

> Crear `.github/workflows/ci.yml` con los jobs `build`, `test`, `vuln`, `sbom` y `publish`, que descubren los servicios bajo `services/*/` y `agents/*/` y terminan en verde cuando todavía no hay ninguno; `publish` empuja imágenes solo en tags `v*`, etiquetadas con la versión del tag y el SHA y nunca `latest`. Además, crear el guard `scripts/ci/check-no-latest.sh <dir>`, que falla si algún `image:` bajo `<dir>` usa `:latest` o no lleva tag ni digest, y engancharlo en un job `no-latest` sobre `deploy/flux/prod`.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- `.github/workflows/contracts.yml` y todo `contracts/`: es **U5-T02**.
- Cualquier archivo bajo `deploy/`: es **U5-T04**. El guard se prueba con fixtures temporales, no creando manifiestos.
- Crear servicios, `go.mod`, `Dockerfile` o código de ejemplo para tener algo que construir.
- Firmado con `cosign`: se propone como tarea candidata. En esta tarea basta con generar el SBOM y adjuntarlo como artefacto del workflow.
- Modificar `README.md` de la raíz.
- Ejecutar el workflow en GitHub o empujar imágenes. La corrida real la verifica el humano en el PR (`gh run list`), por decisión del humano.

---

## Archivos de contexto

Rutas, no contenido pegado:

- `unidades-y-tareas.md`
- `aidlc-docs/inception/application-design/unit-task-plans/U5.md`
- `aidlc-docs/inception/application-design/unit-of-work.md` (layout de `services/` y `agents/`: Go con `cmd/` e `internal/`; agentes en Python o TS)
- `aidlc-docs/inception/application-design/services.md`
- `aidlc-docs/inception/requirements/requirements.md` (NF-SEG, versiones fijadas, rollback version-pinned)

---

## Criterios de aceptación

Cada uno con **su comando**. El revisor los va a correr él mismo, uno por uno, desde la raíz del worktree.

- [ ] **CA-1** — Existen el workflow y el guard.
  ```bash
  test -f .github/workflows/ci.yml && test -x scripts/ci/check-no-latest.sh && echo OK || echo FALTA
  ```
  Esperado: `OK`. Antes de la tarea: `FALTA` (rojo inicial).

- [ ] **CA-2** — `actionlint` no reporta nada sobre `ci.yml` (incluye `shellcheck` de los bloques `run:`).
  ```bash
  docker run --rm --security-opt label=disable -v "$PWD":/repo -w /repo rhysd/actionlint:1.7.7 -color .github/workflows/ci.yml; echo "rc=$?"
  ```
  Esperado: solo `rc=0`.

- [ ] **CA-3** — Están los seis jobs.
  ```bash
  grep -c -E '^  (build|test|vuln|sbom|publish|no-latest):' .github/workflows/ci.yml
  ```
  Esperado: `6`.

- [ ] **CA-4** — Todas las acciones están fijadas por SHA de 40 caracteres y los permisos por defecto son mínimos.
  ```bash
  grep -hE '^\s*-?\s*uses:' .github/workflows/ci.yml | grep -v -E '@[0-9a-f]{40}' | wc -l
  grep -c -E '^permissions:' .github/workflows/ci.yml; grep -A1 -E '^permissions:' .github/workflows/ci.yml | grep -c 'contents: read'
  ```
  Esperado: `0`, `1`, `1`. Los permisos extra (por ejemplo `packages: write`) solo aparecen a nivel del job `publish`.

- [ ] **CA-5** — `publish` solo corre en tags `v*` y nunca etiqueta `latest`.
  ```bash
  grep -n -E "startsWith\(github\.ref, 'refs/tags/v'\)|refs/tags/v\*" .github/workflows/ci.yml | wc -l
  grep -n -i 'latest' .github/workflows/ci.yml | grep -v -E '^[0-9]+:\s*#' | grep -v -E 'no-latest|latest=false' | wc -l
  grep -q 'docker/metadata-action' .github/workflows/ci.yml && grep -c 'latest=false' .github/workflows/ci.yml || echo N/A
  ```
  Esperado: `≥1`, luego `0` (`latest` solo aparece en comentarios, en el identificador `no-latest` del job y del guard, o en `latest=false`), y por último `≥1` si se usa `docker/metadata-action` (que por defecto añade `latest` con `flavor: latest=auto`) o `N/A` si no se usa. Enmienda aprobada por el humano tras la ronda 1: la versión anterior contaba `no-latest` y `check-no-latest.sh`, que exigen CA-1 y CA-3.

- [ ] **CA-6** — El guard pasa sobre el árbol actual y falla con `:latest` o sin tag (fixtures temporales).
  ```bash
  scripts/ci/check-no-latest.sh deploy/flux/prod; echo "rc=$?"
  t=$(mktemp -d); printf 'spec:\n  image: ghcr.io/x/y:latest\n' > "$t/a.yaml"; scripts/ci/check-no-latest.sh "$t"; echo "rc=$?"; rm -rf "$t"
  t=$(mktemp -d); printf 'spec:\n  image: ghcr.io/x/y\n' > "$t/a.yaml"; scripts/ci/check-no-latest.sh "$t"; echo "rc=$?"; rm -rf "$t"
  t=$(mktemp -d); printf 'spec:\n  image: ghcr.io/x/y:1.2.3\n  other: ghcr.io/x/z@sha256:%064d\n' 0 > "$t/a.yaml"; scripts/ci/check-no-latest.sh "$t"; echo "rc=$?"; rm -rf "$t"
  ```
  Esperado: `rc=0`, `rc=1` (nombra `a.yaml`), `rc=1` (nombra `a.yaml`), `rc=0`.

- [ ] **CA-7** — El descubrimiento de servicios produce una lista vacía válida cuando no hay ninguno. Se prueba el mismo script que usa el workflow.
  ```bash
  bash scripts/ci/list-services.sh; echo "rc=$?"
  t=$(mktemp -d); mkdir -p "$t/services/go-a" "$t/agents/planner"; touch "$t/services/go-a/Dockerfile" "$t/agents/planner/Dockerfile"; (cd "$t" && bash "$OLDPWD/scripts/ci/list-services.sh"); rm -rf "$t"
  ```
  Esperado: primero `[]` y `rc=0`; luego un arreglo JSON con `services/go-a` y `agents/planner`.

- [ ] **CA-8** — Árbol limpio tras el commit y sin `.gitkeep` sobrante en `.github/workflows/`.
  ```bash
  git status --short | wc -l; git ls-files .github/workflows | grep -c '\.gitkeep$'
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

Qué pruebas se escriben **en esta tarea**, no después:

- Rojo inicial de CA-1 (`FALTA`) registrado en la bitácora.
- CA-6 es la prueba falla-pasa del guard. Incluye un caso con digest para demostrar que no rechaza imágenes válidas.
- CA-7 prueba el descubrimiento sin depender de una corrida en GitHub.
- `shellcheck` de `scripts/ci/*.sh`: `docker run --rm --security-opt label=disable -v "$PWD":/mnt koalaman/shellcheck:v0.10.0 scripts/ci/*.sh` sin salida.

**Rojo primero:** el codificador reproduce `FALTA` con el comando literal de CA-1 y lo registra en su bitácora antes de crear nada.

---

## Notas

- El matrix de `build`, `test`, `vuln` y `sbom` sale de `list-services.sh`. Con lista vacía, los jobs se saltan o terminan en verde, nunca en rojo.
- `vuln`: `govulncheck` para Go y el escáner equivalente del lenguaje de cada agente. `sbom`: SBOM SPDX o CycloneDX adjunto con `actions/upload-artifact`.
- Las acciones de terceros se fijan por SHA, con la versión en un comentario (`uses: actions/checkout@<sha> # v4.x.y`).
- La bitácora pega el **comando literal** de cada criterio y su salida, no abreviaturas (hallazgos F-02 y F-03 de U5-T01).
- Esta tarea no ejecuta ningún comando contra un clúster, la nube ni GitHub.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
