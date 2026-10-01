# U5-T09 — Mover los CronJobs `housekeeping` y `rebuild` al control plane (candidata C-13)

**Unidad:** U5 — Plataforma & GitOps
**Historias que implementa:** US-M10 (refuerza US-M8.1 y US-M8.2: `aqs-test` sin permisos de API ni egress)
**Depende de:** U5-T04 y U5-T06 (fusionadas). Ola 4, en paralelo con U5-T08 y U5-T10.
**Origen:** bloqueo reportado en U5-T06, ronda 1. El humano aprobó la opción A.

---

## Alcance

**Dentro** (una línea, concreta):

> Mover los CronJobs `housekeeping` y `rebuild` de `aqs-test` a `aqs-system`, con `serviceAccountName: go-reset`. `go-reset` ya tiene permisos en `aqs-test` por el Role `aqs-test-operator`.
>
> Eliminar el ServiceAccount `aqs-reset` y su RoleBinding, que quedan sin uso.
>
> Añadir `policy/isolation.rego` y `policy/isolation_test.rego`, con una regla que deniegue cualquier `RoleBinding` en `aqs-test` cuyo sujeto sea un ServiceAccount del propio `aqs-test`. Así ningún pod del namespace de prueba puede tener permisos sobre la API.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Quitar los `env: TZ=UTC` (candidata C-09), cambiar imagen, argumentos o schedules de los CronJobs, o cualquier otra línea de `warm.yaml` distinta de `namespace` y `serviceAccountName` de esos dos CronJobs.
- Modificar `policy/security.rego`, `policy/security_test.rego` o `policy/default_deny.rego`. La regla nueva va en archivos nuevos. U5-T10 también añade archivos nuevos en `policy/`.
- Modificar `networkpolicies.yaml`, el Role `aqs-test-operator` o los bindings de los SA del control plane.
- `deploy/flux/base/backup/` y `base/kustomization.yaml` (U5-T08), y `observability/` (U5-T07).
- Cualquier `kubectl` (incluido `auth can-i`), `flux` o `apply` contra un clúster.

---

## Archivos de contexto

- `tareas/candidatas.md` (C-13)
- `revisiones/U5-T06/ronda-1.md` (diagnóstico del bloqueo)
- `bitacoras/U5-T06.md` (tabla de `can-i` esperado, que esta tarea actualiza)
- `deploy/flux/base/warm.yaml`, `deploy/flux/base/security/` y `policy/`
- `aidlc-docs/inception/application-design/components.md` (C5: `go-reset`, CronJobs `housekeeping` y `rebuild`)

---

## Criterios de aceptación

Desde la raíz del worktree. Se usan estos alias:

```bash
K='docker run --rm --security-opt label=disable -v '"$PWD"':/w -w /w registry.k8s.io/kustomize/kustomize:v5.4.3'
Y='docker run --rm -i --security-opt label=disable mikefarah/yq:4.44.3 -N'
C='docker run --rm -i --security-opt label=disable -v '"$PWD"':/project -w /project openpolicyagent/conftest:v0.56.0'
```

- [ ] **CA-1** — Los CronJobs viven en `aqs-system` con el SA `go-reset`.
  ```bash
  $K build deploy/flux/prod | $Y 'select(.kind == "CronJob" and (.metadata.name == "housekeeping" or .metadata.name == "rebuild")) | .metadata.namespace + "/" + .metadata.name + "=" + .spec.jobTemplate.spec.template.spec.serviceAccountName' | sort
  ```
  Esperado: `aqs-system/housekeeping=go-reset` y `aqs-system/rebuild=go-reset`. Antes de la tarea: `aqs-test/housekeeping=aqs-reset` y `aqs-test/rebuild=aqs-reset` (rojo inicial).

- [ ] **CA-2** — `kubeconform` estricto en ambos overlays.
  ```bash
  for e in dev prod; do $K build deploy/flux/$e | docker run --rm -i ghcr.io/yannh/kubeconform:v0.6.7 -strict -summary -schema-location default -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json' -; done
  ```
  Esperado: dos líneas con `Invalid: 0, Errors: 0, Skipped: 0`.

- [ ] **CA-3** — `aqs-test` ya no tiene ningún ServiceAccount con permisos, y `go-reset` conserva los suyos.
  ```bash
  $K build deploy/flux/prod | $Y 'select(.kind == "ServiceAccount" and .metadata.name == "aqs-reset") | .metadata.name' | wc -l
  $K build deploy/flux/prod | $Y 'select(.kind == "RoleBinding" and .metadata.namespace == "aqs-test") | .subjects[] | select(.namespace == "aqs-test") | .name' | wc -l
  $K build deploy/flux/prod | $Y 'select(.kind == "RoleBinding" and .metadata.namespace == "aqs-test") | .subjects[] | select(.name == "go-reset") | .namespace'
  ```
  Esperado: `0`, `0` y `aqs-system`.

- [ ] **CA-4** — Todas las políticas, viejas y nueva, pasan sus pruebas y el build.
  ```bash
  $C verify --no-color --policy policy; echo "rc=$?"
  for e in dev prod; do $K build deploy/flux/$e | $C test --no-color --policy policy --all-namespaces -; echo "rc=$?"; $K build deploy/flux/$e | $C test --no-color --policy policy --all-namespaces --combine - >/dev/null; echo "combine rc=$?"; done
  ```
  Esperado: `rc=0` en `verify`, con al menos 2 pruebas más que en main (una que pasa y una que falla de la regla nueva); `rc=0` y `combine rc=0` en dev y prod.

- [ ] **CA-5** — La regla nueva deniega de verdad, con un fixture concatenado al build real.
  ```bash
  t=$(mktemp -d); $K build deploy/flux/prod > "$t/b.yaml"
  { cat "$t/b.yaml"; printf -- '---\napiVersion: rbac.authorization.k8s.io/v1\nkind: RoleBinding\nmetadata: {name: x, namespace: aqs-test}\nroleRef: {apiGroup: rbac.authorization.k8s.io, kind: Role, name: aqs-test-operator}\nsubjects: [{kind: ServiceAccount, name: cualquiera, namespace: aqs-test}]\n'; } | $C test --no-color --policy policy --all-namespaces - >/dev/null; echo "sa-local rc=$?"
  { cat "$t/b.yaml"; printf -- '---\napiVersion: rbac.authorization.k8s.io/v1\nkind: RoleBinding\nmetadata: {name: y, namespace: aqs-test}\nroleRef: {apiGroup: rbac.authorization.k8s.io, kind: Role, name: aqs-test-operator}\nsubjects: [{kind: ServiceAccount, name: go-intake, namespace: aqs-system}]\n'; } | $C test --no-color --policy policy --all-namespaces - >/dev/null; echo "sa-control-plane rc=$?"
  rm -rf "$t"
  ```
  Esperado: `sa-local rc=1` y `sa-control-plane rc=0`. La regla apunta a los SA de `aqs-test` y no a cualquier binding.

- [ ] **CA-6** — Solo cambiaron los archivos y las líneas permitidas.
  ```bash
  b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(deploy/flux/base/warm\.yaml|deploy/flux/base/security/(serviceaccounts|rbac)\.yaml|policy/isolation(_test)?\.rego|bitacoras/U5-T09\.md)$' | wc -l
  git diff -U0 $b -- deploy/flux/base/warm.yaml | grep -E '^[+-][^+-]' | grep -v -E '^[+-]\s+(namespace: (aqs-test|aqs-system)|serviceAccountName: (aqs-reset|go-reset))$' | wc -l
  ```
  Esperado: `0` y `0`.

- [ ] **CA-7** — Árbol limpio tras el commit.
  ```bash
  git status --short | wc -l
  ```
  Esperado: `0`.

---

## Plan de pruebas

- Rojo inicial: la salida literal de CA-1 sobre la base.
- CA-5 es la prueba negativa y la positiva de la regla nueva sobre el build real.
- En la bitácora, la tabla de `kubectl auth can-i` esperado, actualizada respecto de `bitacoras/U5-T06.md`: `aqs-reset` desaparece, y `go-reset` puede `create jobs -n aqs-test` y no `-n default`. La verifica el humano en un clúster.

**Rojo primero:** el codificador registra en su bitácora la salida del comando literal de CA-1 antes de cambiar nada.

---

## Notas

- La política nueva usa `import rego.v1`, como las existentes.
- La bitácora pega el **comando literal** de cada criterio y su salida. El CA-7 posterior al último commit va en el informe de vuelta, con una nota en la bitácora que lo diga.
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
