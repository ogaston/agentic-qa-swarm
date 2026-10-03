# U5-T15 — Políticas y validación de manifiestos en CI (candidata C-20)

**Unidad:** U5 — Plataforma & GitOps (endurecimiento previo al despliegue)
**Historias que implementa:** US-M10 (refuerza US-M8: las políticas de red y RBAC se comprueban en cada PR)
**Depende de:** U5 completa. Ola 5, en paralelo con U5-T13.
**Origen:** C-20. Hoy `conftest`, `kubeconform` y las pruebas de reglas solo corren si alguien las lanza a mano, y un PR puede romper una política sin que GitHub Actions lo detecte.

---

## Alcance

**Dentro** (una línea, concreta):

> Crear `scripts/ci/policies.sh`: un único script, que corre igual en local y en CI, que construye los overlays `dev` y `prod` con kustomize y luego ejecuta:
> - (1) `kubeconform -strict` sin omisiones;
> - (2) `conftest verify`;
> - (3) `conftest test --all-namespaces`;
> - (4) `conftest test --combine` (la regla de conjunto `default-deny`);
> - (5) `promtool test rules` de `deploy/flux/base/backup/rules_test.yaml`.
>
> Debe salir con un código distinto de 0 si cualquiera falla. Además, crear `.github/workflows/policies.yml`, que lo ejecuta en cada `push` y `pull_request`.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Modificar `ci.yml`, `contracts.yml` o cualquier regla de `policy/` o `deploy/`. Si una comprobación falla sobre main, se reporta como bloqueo; no se "arregla" el manifiesto ni se relaja la política.
- Validar `deploy/flux/clusters/` (lo crea U5-T13 en paralelo). Se puede dejar preparado, pero solo con un `if [ -d ... ]`, y documentado.
- Instalar herramientas en el runner con `curl | sh`: se usan las mismas imágenes docker fijadas que en las tareas de U5.
- Ejecutar nada en GitHub. La corrida real la verifica el humano en el PR.

---

## Archivos de contexto

- `tareas/candidatas.md` (C-20; C-33 sobre `promtool test rules`)
- `policy/` (en especial `default_deny.rego`, que solo actúa con `--combine`)
- `.github/workflows/ci.yml` y `contracts.yml` (convenciones: acciones fijadas por SHA de commit y `permissions` mínimos)
- `scripts/ci/` (estilo de los scripts existentes y su `shellcheck`)
- `tareas/U5-T06-rbac-networkpolicy.md`, `U5-T08-backups.md` y `U5-T11-namespace-explicito.md` (los comandos que hoy se corren a mano)

---

## Criterios de aceptación

Desde la raíz del worktree.

- [ ] **CA-1** — El script existe y pasa sobre el árbol actual.
  ```bash
  test -x scripts/ci/policies.sh && bash scripts/ci/policies.sh > /dev/null 2>&1; echo "rc=$?"
  ```
  Esperado: `rc=0`. Antes de la tarea: `rc=1`, porque el archivo no existe (rojo inicial).

- [ ] **CA-2** — El script ejecuta las 5 comprobaciones en ambos overlays, y su salida las identifica.
  ```bash
  bash scripts/ci/policies.sh 2>&1 | grep -c -E '^(OK|FALLA) (kubeconform|conftest-verify|conftest-test|conftest-combine|promtool-rules)( (dev|prod))?$'
  ```
  Esperado: `8` (`kubeconform`, `conftest-test` y `conftest-combine` por cada overlay, más `conftest-verify` y `promtool-rules` una vez), todas `OK`. *(Enmienda aprobada por el humano tras la ronda 1: antes decía `9` por un error de suma; 3 × 2 + 1 + 1 = 8.)*

- [ ] **CA-3** — El script falla cuando debe. Pruebas negativas sobre copias del repo, sin tocar el worktree.
  ```bash
  neg() { t=$(mktemp -d); git archive HEAD | tar -x -C "$t"; (cd "$t" && eval "$1" && bash scripts/ci/policies.sh > /dev/null 2>&1); echo "$2 rc=$?"; rm -rf "$t"; }
  neg 'printf "%s\n" "---" "{apiVersion: rbac.authorization.k8s.io/v1, kind: ClusterRoleBinding, metadata: {name: x}, roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: admin}, subjects: []}" >> deploy/flux/base/security/rbac.yaml' politica
  neg 'sed -i "s/^  name: default-deny$/  name: otra/" deploy/flux/base/security/networkpolicies.yaml' default-deny
  neg 'sed -i "s/^  replicas: /  replicaz: /" deploy/flux/base/control-plane.yaml' kubeconform
  neg 'sed -i "0,/exp_alerts:/s//exp_alerts_roto:/" deploy/flux/base/backup/rules_test.yaml' promtool
  ```
  Esperado: `rc` distinto de 0 en los 4 casos.

- [ ] **CA-4** — El workflow es válido, corre en `push` y `pull_request`, invoca el script y sigue las convenciones del repo.
  ```bash
  docker run --rm --security-opt label=disable -v "$PWD":/repo -w /repo rhysd/actionlint:1.7.7 -color .github/workflows/policies.yml; echo "rc=$?"
  grep -c -E '^  (push|pull_request):' .github/workflows/policies.yml
  grep -c 'scripts/ci/policies.sh' .github/workflows/policies.yml
  grep -hE '^\s*-?\s*uses:' .github/workflows/policies.yml | grep -v -E '@[0-9a-f]{40}' | wc -l
  grep -A1 -E '^permissions:' .github/workflows/policies.yml | grep -c 'contents: read'
  ```
  Esperado: solo `rc=0`, luego `2`, `1`, `0` y `1`.

- [ ] **CA-5** — `shellcheck`, alcance y árbol limpio.
  ```bash
  docker run --rm --security-opt label=disable -v "$PWD":/mnt koalaman/shellcheck:v0.10.0 scripts/ci/policies.sh; echo "rc=$?"
  git diff --name-only $(git merge-base HEAD origin/main) | sort
  git status --short | wc -l
  ```
  Esperado: `rc=0`; exactamente `.github/workflows/policies.yml`, `bitacoras/U5-T15.md` y `scripts/ci/policies.sh`; y `0`.

---

## Plan de pruebas

- Rojo inicial: la salida literal de CA-1 sobre la base.
- CA-3 es la matriz negativa: una por cada tipo de comprobación.
- El script debe ejecutar **todas** las comprobaciones aunque una falle, y salir distinto de 0 al final, para que el log de CI muestre todos los fallos de una vez.

**Rojo primero:** el codificador registra en su bitácora la salida del comando literal de CA-1 antes de crear nada.

---

## Notas

- Usa las mismas imágenes fijadas que las tareas de U5:
  - `registry.k8s.io/kustomize/kustomize:v5.4.3`
  - `ghcr.io/yannh/kubeconform:v0.6.7`
  - `openpolicyagent/conftest:v0.56.0`
  - `prom/prometheus:v2.55.1` (para `promtool`)

  En el runner de GitHub (Ubuntu) no hay SELinux, pero `--security-opt label=disable` es inofensivo y mantiene el script idéntico en los dos entornos.
- `promtool test rules` necesita leer `rules_test.yaml` y el archivo de reglas que referencia. Cuidado con los permisos del directorio montado; el `mktemp -d` de U5-T07 necesitó `chmod 755`.
- El esquema de kubeconform de la CRD catalog se descarga por red: el runner la tiene.
- Archivos temporales: siempre en tu propio `mktemp -d`. Cada salida pegada en la bitácora empieza con `pwd`.
- La bitácora pega el **comando literal** de cada criterio y su salida. El CA-5 posterior al último commit va en el informe de vuelta, con una nota en la bitácora que lo diga.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
