# U5-T11 — Namespace explícito en `minio-init` y política contra objetos sin namespace (C-31, C-32)

**Unidad:** U5 — Plataforma & GitOps
**Historias que implementa:** US-M10 (corrige un defecto de U5-T05, ya fusionada en main)
**Depende de:** U5-T05 y U5-T06 (fusionadas). Ola 4, en paralelo con U5-T08 y U5-T09 en retrabajo.
**Origen:** hallazgo M-01 de `revisiones/U5-T08/ronda-1.md`.

---

## Alcance

**Dentro** (una línea, concreta):

> **Primero**, en `deploy/flux/base/minio/kustomization.yaml`: el `configMapGenerator` pasa a llamarse `minio-init-script` con `namespace: aqs-system`. En `job-init.yaml`, el volumen del Job `minio-init` apunta a `minio-init-script`.
>
> Hoy el ConfigMap `minio-init` no tiene namespace. Bajo Flux, sin `targetNamespace`, acabaría en `default`, y el Job de `aqs-system` no podría montarlo, así que el bucket `evidence` no se crearía. Se renombra para que CA-1 de U5-T05, que filtra por nombre `minio-init` en `aqs-system`, siga dando sus tres líneas.
>
> **Después**, añadir `policy/namespace.rego` y `policy/namespace_test.rego`, con una regla que deniegue cualquier objeto sin `metadata.namespace`, o con un valor no-string o vacío, cuyo `kind` no sea de ámbito de clúster. La lista explícita de kinds de clúster es:
> - `Namespace`
> - `ClusterRole`
> - `ClusterRoleBinding`
> - `CustomResourceDefinition`
> - `PersistentVolume`
> - `StorageClass`
> - `PriorityClass`
> - `IngressClass`
> - `ValidatingWebhookConfiguration`
> - `MutatingWebhookConfiguration`
> - `APIService`

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Cualquier otro cambio en `deploy/flux/base/minio/`: imagen, `init-bucket.sh`, StatefulSet, Service, `disableNameSuffixHash` o `securityContext` (candidatas C-14 a C-17).
- `deploy/flux/base/backup/` (U5-T08, en retrabajo; corrige su propio ConfigMap), `scripts/test/minio-local.sh` y el resto de `deploy/`.
- Modificar `policy/security.rego`, `security_test.rego`, `default_deny.rego`, `egress*.rego` o `isolation*.rego` (este último lo toca U5-T09). El posible hueco de `egress: null` en `security.rego` lo está evaluando el revisor de U5-T09 y no se toca aquí.
- Cualquier `kubectl`, `flux` o `apply` contra un clúster.

---

## Archivos de contexto

- `revisiones/U5-T08/ronda-1.md` (M-01: diagnóstico)
- `tareas/candidatas.md` (C-31, C-32)
- `tareas/U5-T05-minio.md` (sus CA-1 y CA-5, que deben seguir pasando)
- `deploy/flux/base/minio/` y `policy/`

---

## Criterios de aceptación

Desde la raíz del worktree. Se usan estos alias:

```bash
K='docker run --rm --security-opt label=disable -v '"$PWD"':/w -w /w registry.k8s.io/kustomize/kustomize:v5.4.3'
Y='docker run --rm -i --security-opt label=disable mikefarah/yq:4.44.3 -N'
C='docker run --rm -i --security-opt label=disable -v '"$PWD"':/project -w /project openpolicyagent/conftest:v0.56.0'
```

- [ ] **CA-1** — El ConfigMap del script de `minio-init` vive en `aqs-system`.
  ```bash
  $K build deploy/flux/prod | $Y 'select(.kind == "ConfigMap" and (.metadata.name | test("^minio-init"))) | .metadata.name + " ns=" + (.metadata.namespace // "NONE")'
  ```
  Esperado: `minio-init-script ns=aqs-system`. Antes de la tarea: `minio-init ns=NONE` (rojo inicial).

- [ ] **CA-2** — En el build no queda ningún objeto sin namespace salvo los Namespaces.
  ```bash
  for e in dev prod; do $K build deploy/flux/$e | $Y 'select((.metadata.namespace // "") == "") | .kind' | sort -u; done
  ```
  Esperado: `Namespace` en cada overlay, y nada más.

- [ ] **CA-3** — El Job monta el ConfigMap renombrado.
  ```bash
  $K build deploy/flux/prod | $Y 'select(.kind == "Job" and .metadata.name == "minio-init") | .metadata.namespace + " " + (.spec.template.spec.volumes[] | select(has("configMap")) | .configMap.name)'
  ```
  Esperado: `aqs-system minio-init-script`.

- [ ] **CA-4** — Los criterios de U5-T05 siguen pasando.
  ```bash
  $K build deploy/flux/prod | $Y 'select(.metadata.namespace == "aqs-system" and (.metadata.name == "minio" or .metadata.name == "minio-init")) | .kind + "/" + .metadata.name' | sort
  $K build deploy/flux/prod | $Y 'select(.kind == "ConfigMap" and (.metadata.name | test("^minio-init"))) | .data["init-bucket.sh"]' | sed '$d' | diff - deploy/flux/base/minio/init-bucket.sh && echo IGUAL
  bash scripts/test/minio-local.sh >/dev/null; echo "harness rc=$?"
  ```
  Esperado: `Job/minio-init`, `Service/minio`, `StatefulSet/minio`; luego `IGUAL` y `harness rc=0`.

- [ ] **CA-5** — `kubeconform` estricto y las políticas sobre el build real.
  ```bash
  for e in dev prod; do $K build deploy/flux/$e | docker run --rm -i ghcr.io/yannh/kubeconform:v0.6.7 -strict -summary -schema-location default -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json' -; done
  $C verify --no-color --policy policy; echo "verify rc=$?"
  for e in dev prod; do $K build deploy/flux/$e | $C test --no-color --policy policy --all-namespaces - >/dev/null; echo "$e rc=$?"; done
  ```
  Esperado: dos líneas con `Invalid: 0, Errors: 0, Skipped: 0`; `verify rc=0`, con al menos 4 pruebas nuevas en `namespace_test.rego`; y `dev rc=0`, `prod rc=0`.

- [ ] **CA-6** — La regla nueva deniega de verdad, sobre el build real, y no deniega los objetos de clúster.
  ```bash
  t=$(mktemp -d); $K build deploy/flux/prod > "$t/b.yaml"
  f() { { cat "$t/b.yaml"; printf -- '---\n%s\n' "$1"; } | $C test --no-color --policy policy --all-namespaces - >/dev/null; echo "$2 rc=$?"; }
  f '{apiVersion: v1, kind: ConfigMap, metadata: {name: suelto}}' cm-sin-ns
  f '{apiVersion: v1, kind: ConfigMap, metadata: {name: nulo, namespace: null}}' cm-ns-null
  f '{apiVersion: batch/v1, kind: CronJob, metadata: {name: cj, namespace: ""}, spec: {schedule: "* * * * *", jobTemplate: {spec: {template: {spec: {restartPolicy: Never, containers: [{name: c, image: "x:1"}]}}}}}}' cronjob-ns-vacio
  f '{apiVersion: v1, kind: Namespace, metadata: {name: otro}}' namespace-ok
  rm -rf "$t"
  ```
  Esperado: `cm-sin-ns rc=1`, `cm-ns-null rc=1`, `cronjob-ns-vacio rc=1` y `namespace-ok rc=0`.

- [ ] **CA-7** — Solo cambiaron los archivos permitidos.
  ```bash
  git diff --name-only $(git merge-base HEAD origin/main) | sort
  ```
  Esperado, exactamente: `bitacoras/U5-T11.md`, `deploy/flux/base/minio/job-init.yaml`, `deploy/flux/base/minio/kustomization.yaml`, `policy/namespace.rego` y `policy/namespace_test.rego`.

- [ ] **CA-8** — Árbol limpio tras el commit.
  ```bash
  git status --short | wc -l
  ```
  Esperado: `0`.

---

## Plan de pruebas

- Rojo inicial: las salidas literales de CA-1 y CA-2 sobre la base.
- CA-4 demuestra que el arreglo no rompe U5-T05, incluida la lectura de vuelta real del harness.
- CA-6 es la matriz negativa y positiva de la regla nueva sobre el build real.

**Rojo primero:** el codificador registra en su bitácora la salida de los comandos literales de CA-1 y CA-2 antes de cambiar nada.

---

## Notas

- `configMapGenerator` admite un campo `namespace` por generador. Con `disableNameSuffixHash: true`, kustomize no reescribe la referencia del volumen al cambiar el nombre, así que hay que editar `job-init.yaml`.
- La política usa `import rego.v1`.
- La bitácora pega el **comando literal** de cada criterio y su salida. El CA-8 posterior al último commit va en el informe de vuelta, con una nota en la bitácora que lo diga.
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
