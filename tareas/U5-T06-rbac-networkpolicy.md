# U5-T06 — RBAC base y NetworkPolicy base (test-ns-only)

**Unidad:** U5 — Plataforma & GitOps
**Historias que implementa:** US-M10 (base de US-M8.1 RBAC test-ns-only y US-M8.2 NetworkPolicy sin egress/LLM)
**Depende de:** U5-T04 (namespaces `aqs-system` y `aqs-test`, Deployments y CronJobs). Ola 3, en paralelo con U5-T05 y U5-T07.

---

## Alcance

**Dentro** (una línea, concreta):

> Crear `deploy/flux/base/security/` con su propio `kustomization.yaml`, que contenga:
> - **ServiceAccounts:**
>   - En `aqs-system`: `go-run-controller`, `go-warm-manager` y `go-reset`.
>   - En `aqs-test`: `aqs-reset` (para los CronJobs) y `aqs-runner` (para ensayo y runners), con `automountServiceAccountToken: false`.
> - **Un Role `aqs-test-operator` en `aqs-test`** (Jobs, Pods, Pods/log, Deployments, StatefulSets, Services, ConfigMaps), con RoleBindings para los tres ServiceAccounts del control plane y para `aqs-reset`.
> - **NetworkPolicies en `aqs-test`:**
>   - `default-deny` (Ingress y Egress para todos los pods).
>   - `allow-same-namespace` (ingress y egress entre pods de `aqs-test`).
>   - `allow-dns` (egress a `kube-system` por el puerto 53 TCP y UDP).
>   - `allow-from-control-plane` (ingress desde `aqs-system`).
>
> Asignar los ServiceAccounts añadiendo `serviceAccountName` en `control-plane.yaml` (los 3 Deployments) y en `warm.yaml` (los 2 CronJobs, con `aqs-reset`). Crear las políticas `policy/*.rego` con sus pruebas `policy/*_test.rego`, que hacen cumplir estas reglas sobre el build. En `deploy/flux/base/kustomization.yaml` añadir `- security` como **última** entrada de `resources`.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- NetworkPolicies para `aqs-system` u otros namespaces: es la candidata C-12.
- Cualquier `ClusterRole` o `ClusterRoleBinding`.
- Cualquier `ipBlock` o egress fuera del clúster desde `aqs-test` (bloquear el LLM es el objetivo).
- MinIO (**U5-T05**), observabilidad (**U5-T07**) y backups (**U5-T08**).
- En `control-plane.yaml` y `warm.yaml`, cualquier cambio distinto de añadir `serviceAccountName`.
- En `deploy/flux/base/kustomization.yaml`, cualquier cambio distinto de añadir `- security` como última entrada.
- `kubectl auth can-i` o cualquier comando contra un clúster. El resultado esperado de `can-i` se **documenta** en la bitácora; lo verifica el humano.

---

## Archivos de contexto

- `unidades-y-tareas.md`
- `aidlc-docs/inception/application-design/unit-task-plans/U5.md`
- `aidlc-docs/inception/requirements/requirements.md` (M8, AUTONOMIA, "NetworkPolicy namespace-only que bloquea LLM a runners")
- `aidlc-docs/inception/application-design/components.md` (C2, C3, C4, C5 y C9: quién crea Jobs y rollouts en el namespace de prueba)
- `deploy/flux/base/` (lo que dejó U5-T04)
- `tareas/candidatas.md`

---

## Criterios de aceptación

Desde la raíz del worktree. Se usan estos alias:

```bash
K='docker run --rm --security-opt label=disable -v '"$PWD"':/w -w /w registry.k8s.io/kustomize/kustomize:v5.4.3'
Y='docker run --rm -i --security-opt label=disable mikefarah/yq:4.44.3 -N'
C='docker run --rm -i --security-opt label=disable -v '"$PWD"':/project -w /project openpolicyagent/conftest:v0.56.0'
```

- [ ] **CA-1** — Las NetworkPolicies de `aqs-test` están en el build.
  ```bash
  $K build deploy/flux/prod | $Y 'select(.kind == "NetworkPolicy" and .metadata.namespace == "aqs-test") | .metadata.name' | sort
  ```
  Esperado: `allow-dns`, `allow-from-control-plane`, `allow-same-namespace` y `default-deny`. Antes de la tarea: ninguna línea (rojo inicial).

- [ ] **CA-2** — `kubeconform` estricto en ambos overlays.
  ```bash
  for e in dev prod; do $K build deploy/flux/$e | docker run --rm -i ghcr.io/yannh/kubeconform:v0.6.7 -strict -summary -schema-location default -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json' -; done
  ```
  Esperado: dos líneas con `Invalid: 0, Errors: 0, Skipped: 0`.

- [ ] **CA-3** — Las pruebas unitarias de las políticas pasan.
  ```bash
  $C verify --no-color --policy policy; echo "rc=$?"
  ```
  Esperado: `rc=0`, con al menos 8 pruebas y 0 fallos. Hay al menos una prueba que pasa y otra que falla por cada regla de la lista de abajo.

- [ ] **CA-4** — El build de ambos overlays cumple las políticas.
  ```bash
  for e in dev prod; do $K build deploy/flux/$e | $C test --no-color --policy policy --all-namespaces -; echo "rc=$?"; done
  ```
  Esperado: `rc=0` dos veces, con `0 failures`.

  Las reglas que el conjunto `policy/` debe denegar:
  - `ClusterRoleBinding` o `ClusterRole`, de cualquier tipo.
  - Un `RoleBinding` en `aqs-test` cuyo sujeto sea `aqs-runner`.
  - Un `ServiceAccount` `aqs-runner` sin `automountServiceAccountToken: false`.
  - Una NetworkPolicy en `aqs-test` con `ipBlock`.
  - Un Job o CronJob en `aqs-test` sin `serviceAccountName`, o los Deployments `go-run-controller`, `go-warm-manager` o `go-reset` sin `serviceAccountName`. El resto de workloads (los otros servicios del control plane, el warm, MinIO de U5-T05 y la observabilidad de U5-T07) queda fuera de esta regla, para que la regla no dependa de lo que traigan las otras tareas de la ola.
  - La ausencia de una NetworkPolicy `default-deny` en `aqs-test` con `podSelector: {}` y `policyTypes` Ingress y Egress. Es una regla sobre el conjunto, con `--combine` o equivalente; vale implementarla como un test separado si se documenta.

- [ ] **CA-5** — Pruebas negativas sobre el build, con fixtures temporales concatenados al build real.
  ```bash
  t=$(mktemp -d); $K build deploy/flux/prod > "$t/b.yaml"
  { cat "$t/b.yaml"; printf -- '---\napiVersion: rbac.authorization.k8s.io/v1\nkind: ClusterRoleBinding\nmetadata: {name: x}\nroleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: admin}\nsubjects: [{kind: ServiceAccount, name: aqs-runner, namespace: aqs-test}]\n'; } | $C test --no-color --policy policy --all-namespaces - >/dev/null; echo "crb rc=$?"
  { cat "$t/b.yaml"; printf -- '---\napiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\nmetadata: {name: llm, namespace: aqs-test}\nspec: {podSelector: {}, policyTypes: [Egress], egress: [{to: [{ipBlock: {cidr: 0.0.0.0/0}}]}]}\n'; } | $C test --no-color --policy policy --all-namespaces - >/dev/null; echo "ipblock rc=$?"
  rm -rf "$t"
  ```
  Esperado: `crb rc=1` e `ipblock rc=1`.

- [ ] **CA-6** — Los ServiceAccounts quedan asignados donde corresponde.
  ```bash
  $K build deploy/flux/prod | $Y 'select(.kind == "Deployment" or .kind == "CronJob") | .metadata.namespace + "/" + .metadata.name + "=" + ((.spec.template.spec.serviceAccountName // .spec.jobTemplate.spec.template.spec.serviceAccountName) // "-")' | grep -E 'go-run-controller|go-warm-manager|go-reset|housekeeping|rebuild' | sort
  ```
  Esperado:
  ```
  aqs-system/go-reset=go-reset
  aqs-system/go-run-controller=go-run-controller
  aqs-system/go-warm-manager=go-warm-manager
  aqs-test/housekeeping=aqs-reset
  aqs-test/rebuild=aqs-reset
  ```

- [ ] **CA-7** — Solo se tocaron las líneas permitidas de los archivos de U5-T04.
  ```bash
  b=$(git merge-base HEAD origin/main); git diff --numstat $b -- deploy/flux/base/kustomization.yaml; git diff -U0 $b -- deploy/flux/base/control-plane.yaml deploy/flux/base/warm.yaml | grep -E '^[+-][^+-]' | grep -v -E '^\+\s+serviceAccountName: (go-run-controller|go-warm-manager|go-reset|aqs-reset)$' | wc -l; $Y '.resources[-1]' < deploy/flux/base/kustomization.yaml
  ```
  Esperado: `1\t0\tdeploy/flux/base/kustomization.yaml`, `0` y `security`.

- [ ] **CA-8** — Árbol limpio tras el commit.
  ```bash
  git status --short | wc -l
  ```
  Esperado: `0`.

---

## Plan de pruebas

- Rojo inicial: el comando literal de CA-1 sobre la base no lista ninguna NetworkPolicy.
- CA-3: cada regla tiene sus pruebas en `policy/*_test.rego`, una que pasa y otra que falla.
- CA-5: pruebas negativas sobre el build real.
- En la bitácora, una tabla con el `kubectl auth can-i` esperado: para `aqs-runner` (`no` en todo), para `go-run-controller` (`create jobs -n aqs-test`: `yes`; `create jobs -n default`: `no`) y para `aqs-reset`. El humano lo verifica en un clúster.

**Rojo primero:** el codificador registra en su bitácora la salida del comando literal de CA-1 antes de crear nada.

---

## Notas

- Supuesto de diseño: la evidencia de las corridas la recoge el control plane (C4, desde `aqs-system`). Los pods de `aqs-test` no tienen egress hacia `aqs-system` ni hacia MinIO. Si el codificador ve que algún componente lo necesita, lo reporta como bloqueo; no abre egress por su cuenta.
- Las políticas Rego usan `import rego.v1` (conftest v0.56).
- La bitácora pega el **comando literal** de cada criterio y su salida. El CA-8 posterior al último commit va en el informe de vuelta, con una nota en la bitácora que lo diga.
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
