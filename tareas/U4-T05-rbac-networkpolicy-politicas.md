# U4-T05 — Aislamiento como artefactos revisables: RBAC mínimo, NetworkPolicy de `aqs-system` y políticas Rego

**Unidad:** U4 — Gobernanza & Identidad
**Historias que implementa:** US-M8.1 (RBAC test-ns-only, sin wildcards), US-M8.2 (NetworkPolicy namespace-only, sin egress a LLM desde los runners)
**Depende de:** U5 completa (esta tarea **endurece** lo que dejaron U5-T06, U5-T09, U5-T10, U5-T12 y U5-T15). No depende de otras tareas de U4: puede correr en paralelo con U4-T01 y U1-T01.
**Origen:** candidatas C-12 (parcial), C-21, C-22 y C-28, y el hueco de que cuatro Deployments del control plane corren hoy con el ServiceAccount `default` y su token montado.

---

## Alcance

**Dentro** (una línea, concreta):

> Dar a `ui-api`, `go-intake`, `go-governance` y `go-identity` un ServiceAccount propio **sin permisos y sin token montado**, añadir a `aqs-system` una NetworkPolicy de **ingress** deny-by-default con permisos explícitos, cerrar con reglas Rego los huecos de RBAC y de aislamiento que quedaron anotados (wildcards, escalada, `exec`, `secrets`, bindings de `aqs-test` en otros namespaces, `hostNetwork`, `automount` a nivel de pod, `serviceAccountName` vacío) y verificar la matriz de permisos esperada **sin clúster** con `scripts/ci/rbac-matrix.sh`.

Detalle (todo bajo `deploy/flux/base/security/`, salvo lo indicado):

1. **ServiceAccounts.** Añadir a `serviceaccounts.yaml` `ui-api`, `go-intake`, `go-governance` y `go-identity` en `aqs-system`, con `automountServiceAccountToken: false`. Asignarlos con `serviceAccountName` (y `automountServiceAccountToken: false` a nivel de pod) en esos cuatro Deployments de `deploy/flux/base/control-plane.yaml`. No se crea ningún Role ni RoleBinding para ellos: **no hablan con la API de Kubernetes**.
2. **NetworkPolicy de `aqs-system`** (nuevo archivo `networkpolicies-system.yaml`, añadido a `kustomization.yaml`), solo **Ingress**:
   - `default-deny-ingress`: `podSelector: {}`, `policyTypes: [Ingress]`.
   - `allow-same-namespace`: ingress desde `podSelector: {}`.
   - `allow-metrics-from-observability`: ingress desde el namespace `aqs-observability` (`kubernetes.io/metadata.name`), puerto `8080/TCP`.
   - `allow-public-http`: para los pods `app.kubernetes.io/name` en `go-intake` y `ui-api`, ingress desde namespaces con la etiqueta `aqs.io/ingress-controller: "true"`, puerto `8080/TCP`. Esa etiqueta la pone el operador sobre el namespace de su controlador de ingress (se documenta); sin ella el tráfico externo **queda bloqueado** (falla cerrado).
   - **Sin ninguna regla de egress en `aqs-system`**: el control plane necesita la API de Kubernetes, MinIO, el LLM fuera del clúster y GitHub, y esas direcciones dependen del clúster. Eso sigue abierto como candidata C-51 y se declara en `docs/seguridad/aislamiento.md`.
3. **Políticas Rego** (`policy/*.rego` con su `*_test.rego`; `import rego.v1`). Denegar:
   - **Wildcards** (`"*"`) en `apiGroups`, `resources` o `verbs` de cualquier `Role`/`ClusterRole`.
   - **Escalada y acceso a secretos** en cualquier `Role`: recurso `secrets`, `pods/exec`, `pods/attach`, `pods/portforward`, `serviceaccounts/token`, y verbos `escalate`, `bind`, `impersonate`.
   - **C-28:** un `RoleBinding` en **cualquier** namespace cuyo sujeto sea un ServiceAccount de `aqs-test` (o `User`/`Group` equivalente, como ya hace `isolation.rego` para `aqs-test`).
   - **C-21:** `serviceAccountName` vacío, nulo o no-string en los Deployments del control plane; ampliar la regla actual a **los siete** Deployments de `aqs-system` (hoy solo comprueba tres) y exigir que los cuatro nuevos tengan `automountServiceAccountToken: false` en el pod.
   - **C-22:** en `aqs-test`, cualquier plantilla de Pod (Deployment, StatefulSet, Job, CronJob) con `automountServiceAccountToken: true`.
   - **`hostNetwork: true`, `hostPID: true` y `hostIPC: true`** en cualquier workload de `aqs-test` (un pod con red del nodo salta las NetworkPolicy y rompe el bloqueo de egress al LLM).
   - Sobre `aqs-system`: una NetworkPolicy con `ipBlock` en `ingress`, o con una regla de ingress vacía o con `from: [{}]` (abre el ingreso a todos).
   - Regla de **conjunto** (`--combine`): debe existir `default-deny-ingress` en `aqs-system` con `podSelector: {}` y `policyTypes` que contenga `Ingress`.
4. **Matriz RBAC sin clúster.** `docs/seguridad/rbac-matriz.csv` (`sujeto,namespace,verbo,recurso,esperado`) con al menos 24 filas, y `scripts/ci/rbac-matrix.sh`, que construye el overlay (`dev` y `prod`), calcula con `yq`/`jq` los permisos efectivos de los `Role` y `RoleBinding` renderizados y compara con el CSV. Sale distinto de 0 si cualquier fila difiere. Filas mínimas: `go-run-controller`/`go-warm-manager`/`go-reset` crean `jobs` en `aqs-test` (`yes`) y en `default` y `aqs-system` (`no`); ninguno lee `secrets` (`no`); ningún ServiceAccount de `aqs-test` ni de los cuatro servicios nuevos puede nada (`no`); nadie tiene `ClusterRole` ni wildcards.
5. **Integración en la validación existente.** Añadir a `scripts/ci/policies.sh` **una** comprobación `rbac-matrix` por overlay (`OK|FALLA rbac-matrix <overlay>`), sin tocar el resto del script.
6. **Documentación** `docs/seguridad/aislamiento.md` (≤ 80 líneas): qué aísla cada capa, la etiqueta `aqs.io/ingress-controller`, la matriz `kubectl auth can-i` **esperada** para que el humano la compruebe en dev (los comandos van escritos, ninguno se ejecuta), y lo que queda abierto (C-51, egress de `aqs-system`; `aqs-observability` sin NetworkPolicy).

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Aplicar nada a un clúster ni ejecutar `kubectl`: son manifiestos revisables. El `kubectl auth can-i` se **documenta**; lo ejecuta el humano.
- Egress de `aqs-system` o cualquier NetworkPolicy en `aqs-observability`: C-51 y el resto de C-12. Si el codificador cree que hace falta, lo reporta como bloqueo.
- Cambiar el Role `aqs-test-operator`, sus RoleBindings, las NetworkPolicy de `aqs-test` o `egress.rego` salvo para **añadir** pruebas. Si una regla nueva falla contra el Role existente (por ejemplo, `pods/log` o `deployments`), se reporta; no se relaja la regla ni se edita el Role.
- Revisar `deployments/scale`, `statefulsets/scale` o PVC del Role (C-29): depende de `go-reset` real, U2.
- Pod Security Admission, `securityContext` endurecido (C-15), Kyverno, OPA Gatekeeper o cualquier controlador de admisión.
- Tocar `ui-api`/servicios Go, `contracts/**`, `.github/workflows/**` o los manifiestos de MinIO, observabilidad y backups.
- Cambios en `control-plane.yaml` distintos de añadir `serviceAccountName` y `automountServiceAccountToken: false` a los cuatro Deployments indicados.

---

## Archivos de contexto

- `unidades-y-tareas.md` (sección U4)
- `aidlc-docs/inception/application-design/unit-task-plans/U4.md`
- `aidlc-docs/inception/requirements/requirements.md` (M8, NF-SEG-06 «sin wildcards», NetworkPolicy namespace-only)
- `aidlc-docs/inception/application-design/components.md` (quién crea Jobs y rollouts: C2-C5, C9)
- `deploy/flux/base/security/` y `deploy/flux/base/control-plane.yaml`
- `policy/` (todas las reglas y pruebas actuales) y `scripts/ci/policies.sh`
- `tareas/U5-T06-rbac-networkpolicy.md`, `tareas/U5-T09-cronjobs-al-control-plane.md`, `tareas/U5-T10-politica-egress.md`, `tareas/U5-T12-ipblock-null.md`, `tareas/candidatas.md` (C-12, C-21, C-22, C-28, C-29)

---

## Criterios de aceptación

Desde la raíz del worktree. Alias:

```bash
K='docker run --rm --security-opt label=disable -v '"$PWD"':/w -w /w registry.k8s.io/kustomize/kustomize:v5.4.3'
Y='docker run --rm -i --security-opt label=disable mikefarah/yq:4.44.3 -N'
C='docker run --rm -i --security-opt label=disable -v '"$PWD"':/project -w /project openpolicyagent/conftest:v0.56.0'
```

- [ ] **CA-1** — Los cuatro ServiceAccounts existen sin token montado; los tres del control plane con API siguen como estaban.
  ```bash
  $K build deploy/flux/prod | $Y 'select(.kind=="ServiceAccount" and .metadata.namespace=="aqs-system") | .metadata.name + "=" + ((.automountServiceAccountToken // "sin-definir")|tostring)' | sort
  ```
  Esperado: `go-governance=false`, `go-identity=false`, `go-intake=false`, `go-reset=sin-definir`, `go-run-controller=sin-definir`, `go-warm-manager=sin-definir`, `ui-api=false` (y las demás que ya existieran, p. ej. del backup, sin cambios). Antes de la tarea: faltan las cuatro primeras y `ui-api` (rojo inicial).

- [ ] **CA-2** — Cada Deployment usa su ServiceAccount y los cuatro nuevos no montan token a nivel de pod.
  ```bash
  $K build deploy/flux/prod | $Y 'select(.kind=="Deployment" and .metadata.namespace=="aqs-system") | .metadata.name + "=" + (.spec.template.spec.serviceAccountName // "-") + "/" + ((.spec.template.spec.automountServiceAccountToken // "sin-definir")|tostring)' | sort
  b=$(git merge-base HEAD origin/main); git diff -U0 $b -- deploy/flux/base/control-plane.yaml | grep -E '^[+-][^+-]' | grep -v -E '^\+\s+(serviceAccountName: (ui-api|go-intake|go-governance|go-identity)|automountServiceAccountToken: false)$' | wc -l
  ```
  Esperado: siete líneas `go-governance=go-governance/false`, `go-identity=go-identity/false`, `go-intake=go-intake/false`, `go-reset=go-reset/sin-definir`, `go-run-controller=go-run-controller/sin-definir`, `go-warm-manager=go-warm-manager/sin-definir`, `ui-api=ui-api/false`; y `0`.

- [ ] **CA-3** — NetworkPolicy de `aqs-system`: las cuatro, con la forma exacta, y ninguna de egress.
  ```bash
  $K build deploy/flux/prod | $Y 'select(.kind=="NetworkPolicy" and .metadata.namespace=="aqs-system") | .metadata.name + " " + (.spec.policyTypes|join(","))' | sort
  $K build deploy/flux/prod | $Y 'select(.kind=="NetworkPolicy" and .metadata.namespace=="aqs-system") | select(.spec.egress != null or (.spec.policyTypes | contains(["Egress"]))) | .metadata.name' | wc -l
  $K build deploy/flux/prod | $Y 'select(.kind=="NetworkPolicy" and .metadata.name=="allow-metrics-from-observability") | .spec.ingress[0] | [.from[0].namespaceSelector.matchLabels."kubernetes.io/metadata.name", (.ports[0].port|tostring), .ports[0].protocol] | join(" ")'
  $K build deploy/flux/prod | $Y 'select(.kind=="NetworkPolicy" and .metadata.name=="allow-public-http") | [(.spec.podSelector.matchExpressions[0].values|sort|join(",")), (.spec.ingress[0].from[0].namespaceSelector.matchLabels."aqs.io/ingress-controller")] | join(" ")'
  ```
  Esperado: `allow-metrics-from-observability Ingress`, `allow-public-http Ingress`, `allow-same-namespace Ingress`, `default-deny-ingress Ingress`; `0`; `aqs-observability 8080 TCP`; `go-intake,ui-api true`. Antes de la tarea: ninguna línea (rojo inicial). (Las NetworkPolicy de `aqs-test` no aparecen porque el filtro es `aqs-system`.)

- [ ] **CA-4** — `kubeconform` estricto en ambos overlays.
  ```bash
  for e in dev prod; do $K build deploy/flux/$e | docker run --rm -i ghcr.io/yannh/kubeconform:v0.6.7 -strict -summary -schema-location default -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json' -; done
  ```
  Esperado: dos líneas con `Invalid: 0, Errors: 0, Skipped: 0`.

- [ ] **CA-5** — Las pruebas unitarias de las políticas pasan y hay al menos 14 pruebas nuevas.
  ```bash
  $C verify --no-color --policy policy; echo "rc=$?"
  base=$(for f in $(git ls-tree --name-only origin/main policy/ | grep '_test\.rego$'); do git show origin/main:$f; done | grep -c '^test_'); now=$(cat policy/*_test.rego | grep -c '^test_'); echo "nuevas=$((now-base))"
  ```
  Esperado: `rc=0` sin fallos y `nuevas=` ≥ `14`, con al menos una prueba que pasa y otra que falla por cada regla de la lista del «Dentro» (3).

- [ ] **CA-6** — El build real de ambos overlays cumple todas las políticas, incluida la de conjunto.
  ```bash
  for e in dev prod; do $K build deploy/flux/$e | $C test --no-color --policy policy --all-namespaces -; echo "test $e rc=$?"; $K build deploy/flux/$e | $C test --no-color --policy policy --all-namespaces --combine -; echo "combine $e rc=$?"; done
  ```
  Esperado: cuatro `rc=0`, con `0 failures`. Si una regla nueva falla contra manifiestos de U5 (p. ej., el Role `aqs-test-operator`), el codificador **no** relaja la regla: lo reporta como bloqueo.

- [ ] **CA-7** — Pruebas negativas sobre el build real: cada fixture concatenado al build debe ser rechazado.
  ```bash
  t=$(mktemp -d); $K build deploy/flux/prod > "$t/b.yaml"
  neg() { { cat "$t/b.yaml"; printf -- '---\n%s\n' "$2"; } | $C test --no-color --policy policy --all-namespaces - >/dev/null 2>&1; echo "$1 rc=$?"; }
  neg wildcard 'apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: {name: x, namespace: aqs-test}
rules: [{apiGroups: ["*"], resources: ["pods"], verbs: ["get"]}]'
  neg secrets 'apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: {name: x, namespace: aqs-test}
rules: [{apiGroups: [""], resources: ["secrets"], verbs: ["get"]}]'
  neg exec 'apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: {name: x, namespace: aqs-test}
rules: [{apiGroups: [""], resources: ["pods/exec"], verbs: ["create"]}]'
  neg escalate 'apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: {name: x, namespace: aqs-test}
rules: [{apiGroups: ["rbac.authorization.k8s.io"], resources: ["roles"], verbs: ["escalate","bind"]}]'
  neg binding-ajeno 'apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata: {name: x, namespace: aqs-system}
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: Role, name: y}
subjects: [{kind: ServiceAccount, name: aqs-runner, namespace: aqs-test}]'
  neg sa-vacio 'apiVersion: apps/v1
kind: Deployment
metadata: {name: ui-api, namespace: aqs-system}
spec: {selector: {matchLabels: {a: b}}, template: {metadata: {labels: {a: b}}, spec: {serviceAccountName: "", containers: [{name: c, image: "x:1"}]}}}'
  neg hostnet 'apiVersion: apps/v1
kind: Deployment
metadata: {name: x, namespace: aqs-test}
spec: {selector: {matchLabels: {a: b}}, template: {metadata: {labels: {a: b}}, spec: {hostNetwork: true, serviceAccountName: aqs-runner, containers: [{name: c, image: "x:1"}]}}}'
  neg automount 'apiVersion: apps/v1
kind: Deployment
metadata: {name: x, namespace: aqs-test}
spec: {selector: {matchLabels: {a: b}}, template: {metadata: {labels: {a: b}}, spec: {automountServiceAccountToken: true, serviceAccountName: aqs-runner, containers: [{name: c, image: "x:1"}]}}}'
  neg ingress-abierto 'apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: x, namespace: aqs-system}
spec: {podSelector: {}, policyTypes: [Ingress], ingress: [{from: [{ipBlock: {cidr: 0.0.0.0/0}}]}]}'
  neg ingress-vacio 'apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: x, namespace: aqs-system}
spec: {podSelector: {}, policyTypes: [Ingress], ingress: [{}]}'
  sed '/name: default-deny-ingress/,/^---/d' "$t/b.yaml" | $C test --no-color --policy policy --all-namespaces --combine - >/dev/null 2>&1; echo "sin-default-deny-ingress rc=$?"
  rm -rf "$t"
  ```
  Esperado: las diez primeras líneas con `rc=1`, y `sin-default-deny-ingress rc=1`. (El codificador puede ajustar el `sed` si el formato de salida de `kustomize` lo exige y lo documenta; lo que se demuestra es que quitar `default-deny-ingress` del conjunto rompe la regla de `--combine`.)

- [ ] **CA-8** — Matriz RBAC: coincide con el CSV y falla si un Role crece.
  ```bash
  wc -l < docs/seguridad/rbac-matriz.csv
  bash scripts/ci/rbac-matrix.sh; echo "rc=$?"
  b=$(git rev-parse --show-toplevel); cp deploy/flux/base/security/rbac.yaml "$b/.rbac.bak" && sed -i '0,/verbs: \["get", "list", "watch", "create", "update", "patch", "delete"\]/s//verbs: ["get", "list", "watch", "create", "update", "patch", "delete", "deletecollection"]/' deploy/flux/base/security/rbac.yaml; git diff --stat -- deploy/flux/base/security/rbac.yaml | tail -n1
  bash scripts/ci/rbac-matrix.sh >/dev/null 2>&1; echo "mutado rc=$?"; mv "$b/.rbac.bak" deploy/flux/base/security/rbac.yaml; git diff --stat -- deploy/flux/base/security/rbac.yaml | wc -l
  ```
  Esperado: un número ≥ `25` (cabecera + ≥ 24 filas); `rc=0` con líneas `OK rbac-matrix dev` y `OK rbac-matrix prod`; una línea de `git diff --stat` que muestra el cambio; `mutado rc=1` (el CSV incluye una fila `go-run-controller,aqs-test,deletecollection,jobs,no`); y `0` (el archivo quedó restaurado). **Si la mutación no produce una diferencia con tu CSV, el CSV está incompleto.**

- [ ] **CA-9** — La comprobación entra en la validación común y todo sigue en verde.
  ```bash
  bash scripts/ci/policies.sh 2>&1 | grep -E '^(OK|FALLA) rbac-matrix'; bash scripts/ci/policies.sh >/dev/null 2>&1; echo "rc=$?"
  b=$(git merge-base HEAD origin/main); git diff -U0 $b -- scripts/ci/policies.sh | grep -E '^[+-][^+-]' | grep -c -E '^-'
  ```
  Esperado: `OK rbac-matrix dev` y `OK rbac-matrix prod`; `rc=0`; `0` líneas eliminadas de `policies.sh`.

- [ ] **CA-10** — La documentación existe y cubre lo comprometido.
  ```bash
  wc -l < docs/seguridad/aislamiento.md; grep -c -E 'aqs.io/ingress-controller' docs/seguridad/aislamiento.md; grep -c -E 'kubectl auth can-i' docs/seguridad/aislamiento.md; grep -c -E 'C-51' docs/seguridad/aislamiento.md; grep -c -i -E 'egress' docs/seguridad/aislamiento.md
  ```
  Esperado: un número ≤ `80`, y cuatro números ≥ `1`.

- [ ] **CA-11** — Higiene y alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(deploy/flux/base/security/|deploy/flux/base/control-plane\.yaml|policy/|scripts/ci/(rbac-matrix|policies)\.sh|docs/seguridad/|bitacoras/U4-T05\.md)' | wc -l
  git diff --name-only $b -- deploy/flux/base/security/rbac.yaml policy/egress.rego policy/default_deny.rego | wc -l
  ```
  Esperado: `0`, `0` y `0` (el Role existente, `egress.rego` y la regla `default-deny` de `aqs-test` no se tocan; las reglas nuevas viven en archivos nuevos o en `security.rego`/`isolation.rego` con **adiciones**, nunca eliminaciones).

---

## Plan de pruebas

- Una prueba que pasa y una que falla por cada regla nueva de Rego, incluidas las variantes de evasión conocidas: `"*"` solo en `verbs`, en `resources` o en `apiGroups`; `secrets` junto a otros recursos; `pods/exec` con verbo `get`; `serviceAccountName` ausente, `""`, `null`, número; `hostNetwork: "true"` (string) frente a `true`; binding con `namespace` ausente (el authorizer usa el del RoleBinding), con `User` `system:serviceaccount:aqs-test:…` y con `Group` `system:serviceaccounts`.
- NetworkPolicy de `aqs-system`: ingress con `from` mezclado (un peer válido y un `ipBlock`), `ingress: null`, `from: null`, `ports` ausentes.
- La regla de conjunto con `--combine`: presente, ausente, presente con `policyTypes: [Egress]`, presente en otro namespace.
- `rbac-matrix.sh`: un `Role` con `verbs: ["*"]`, uno con un verbo más, un `RoleBinding` extra para un ServiceAccount de `aqs-test`, y un `ClusterRole` → cada uno hace fallar el script.
- Sin ninguna llamada a un clúster ni a la nube.

**Rojo primero:** el codificador registra en su bitácora la salida literal de CA-1 y de CA-3 sobre la base (faltan los ServiceAccounts y las NetworkPolicy de `aqs-system`) y de CA-7 con las reglas actuales (varios fixtures salen con `rc=0`: ese es el hueco que la tarea cierra).

---

## Notas

- Archivos que se **modifican en su sitio**: `deploy/flux/base/security/serviceaccounts.yaml` y `kustomization.yaml` (adiciones), `deploy/flux/base/control-plane.yaml` (solo las líneas permitidas), `policy/security.rego` e `isolation.rego` (solo adiciones, o archivos `.rego` nuevos), `scripts/ci/policies.sh` (una comprobación). Nada de duplicados con sufijo.
- `scripts/ci/policies.sh` ya ejecuta `conftest verify`, `conftest test --all-namespaces` y `--combine`: las reglas nuevas se aplican sin más cableado. El codificador lo comprueba en CA-6.
- Si el codificador encuentra que MinIO, el Job `minio-init` o el backup incumplen una regla nueva (p. ej. `serviceAccountName` vacío), la regla se limita a los Deployments indicados y el hallazgo se reporta como candidata; no se editan los manifiestos de U5-T05/T08 aquí.
- Orden de fusión: esta tarea toca `control-plane.yaml`; U1 y las demás de U4 no tocan `deploy/` salvo U1-T06 y U4-T07 (observabilidad), que no entran en conflicto con estas líneas.
- Ningún comando contra un clúster ni la nube. El humano comprueba la matriz `can-i` en dev.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
