# U4-T08 — Cableado de despliegue de `go-identity` y `go-governance` (candidatas C-53 y C-70, solo U4)

**Unidad:** U4 — Gobernanza & Identidad (cierre de despliegue)
**Historias que implementa:** US-M8.3 y US-M10 (los servicios de U4 deben poder arrancar y pasar `/readyz` cuando Flux los reconcilia)
**Depende de:** U4-T01 a U4-T07 (fusionadas). No depende de U1 ni de U2.
**Origen:** C-53 (variables de entorno, Secrets, sondas) y C-70 (`fsGroup` y `strategy: Recreate`). Hoy `go-governance` **no arranca** en un clúster: sin `GOVERNANCE_SERVICE_TOKEN` y `GOVERNANCE_AUTH` termina con error, y el volumen de auditoría puede no ser escribible por el usuario no root (reproducido por el revisor de U4-T07: `open /data/audit.jsonl: permission denied`). `go-identity` tampoco arranca sin `IDENTITY_USERS_FILE`.

---

## Alcance

**Dentro** (una línea, concreta):

> Cablear en `deploy/flux/base/control-plane.yaml` los Deployments `go-governance` y `go-identity` (variables de entorno, referencias a Secrets **por nombre, nunca valores**, `fsGroup`, `strategy: Recreate` donde hay PVC), añadir una política Rego que prohíba valores literales en variables de entorno sensibles de `aqs-system`, y documentar en `docs/operaciones/secrets.md` cómo un humano crea los dos Secrets nuevos con SOPS.

Detalle:

1. **`go-governance`** (solo su Deployment en `control-plane.yaml`):
   - `spec.strategy.type: Recreate` (un PVC `ReadWriteOnce` con una réplica no admite `RollingUpdate`).
   - `spec.template.spec.securityContext.fsGroup: 65532` (el usuario de la imagen es 65532; el PVC se monta como root por defecto).
   - Variables de entorno (se **añaden** a las existentes `TZ`, `GOVERNANCE_DATA_DIR`, `GOVERNANCE_AUDIT_RETENTION_DAYS`): `GOVERNANCE_AUTH=identity`, `GOVERNANCE_ENV=prod`, `GOVERNANCE_TEST_NAMESPACE=aqs-test`, `IDENTITY_URL=http://go-identity.aqs-system.svc:8080`, y `GOVERNANCE_SERVICE_TOKEN` con `valueFrom.secretKeyRef` hacia el Secret **`go-governance-service-token`**, clave `token`.
   - El overlay `deploy/flux/dev/kustomization.yaml` cambia `GOVERNANCE_ENV` a `dev` con un parche (estratégico por nombre de variable, no por índice).
2. **`go-identity`** (solo su Deployment):
   - `IDENTITY_USERS_FILE=/etc/aqs/identity/users.json` y `IDENTITY_TRUST_PROXY=false` (valor por defecto, explícito para que quede visible).
   - Volumen de tipo `secret` con el Secret **`go-identity-users`** (clave `users.json`), montado de solo lectura en `/etc/aqs/identity`, con `defaultMode: 0440`, y `securityContext.fsGroup: 65532` en el pod para que el usuario 65532 lo lea.
   - Sin `strategy` nueva (no tiene PVC).
3. **Nueva política Rego** `policy/secretrefs.rego` con su `policy/secretrefs_test.rego` (`import rego.v1`): en `aqs-system`, ninguna variable de entorno de un contenedor de `Deployment`, `StatefulSet`, `Job` o `CronJob` cuyo nombre termine en `TOKEN`, `SECRET`, `PASSWORD`, `PASSWD` o `SECRET_KEY` (o sea `*_TOKEN`, `*_SECRET`, …) puede llevar `value`: debe usar `valueFrom`. Pruebas: una que pasa y una que falla por cada sufijo, `value: ""` (también se deniega), `valueFrom.secretKeyRef` (pasa), nombre parecido que no termina en el sufijo (pasa). Si algún manifiesto existente de U5 la incumple, **no** se relaja la regla: se reporta como bloqueo.
4. **Documentación** en `docs/operaciones/secrets.md` (secciones nuevas, sin cambiar el resto): cómo crear y cifrar con SOPS los dos Secrets que **no** se generan con `generate.sh`:
   - `go-governance-service-token` (`token`): `openssl rand -hex 32`, mínimo 32 caracteres.
   - `go-identity-users` (`users.json`): cómo producir cada `password_hash` con `go-identity hash-password` (la contraseña por stdin), un `mfa_secret` base32 de al menos 160 bits para cada `admin`, el formato `[{username, password_hash, role, mfa_secret?}]`, roles `user`/`admin`.
   - Para ambos: el comando `sops --encrypt` con el mismo patrón que `scripts/secrets/generate.sh` (`stringData` cifrado, destinatario age pasado como argumento), la ruta de destino `deploy/flux/<env>/secrets/<nombre>.sops.yaml` y la línea que hay que añadir a `secrets/kustomization.yaml`, y la advertencia de que **los pods de `go-governance` y `go-identity` no pasan `/readyz` hasta que existan los dos Secrets en el clúster**. Actualizar la lista «Secrets cubiertos» indicando que estos dos se crean con este procedimiento.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- **Crear valores reales de Secrets** ni commitear ningún `*.sops.yaml`: lo hace un humano con su clave age. Esta tarea no añade ningún Secret al build (ni cifrado ni en claro).
- Modificar `scripts/secrets/generate.sh` o `scripts/test/secrets-e2e.sh`: el entorno del loop no puede ejecutarlos (la imagen de SOPS de `ghcr.io` está bloqueada por el proxy). Extender `generate.sh` con los dos Secrets queda como candidata.
- Cablear `ui-api`, `go-intake` o cualquier servicio de U1/U2/U3: sus variables dependen de decisiones abiertas (C-45).
- Endurecimiento general de `securityContext` (`readOnlyRootFilesystem`, `capabilities`, `seccompProfile`, `runAsNonRoot` en todos los workloads): C-15. Solo `fsGroup` aquí.
- NetworkPolicy de egress de `aqs-system` (C-51) o cualquier cambio de `policy/` distinto de añadir `secretrefs.rego` y su prueba.
- `deploy/flux/base/observability/**`, `backup/**`, `minio/**`, `security/**`.
- Cambios en los otros cinco Deployments de `control-plane.yaml`, en los Services o en el PVC `go-governance-data`.
- Cambios de código Go o de contratos.
- Aplicar nada a un clúster. La comprobación real (pods en `Ready`, el volumen escribible) la hace el humano en dev, con los dos Secrets creados; queda documentada, no ejecutada.

---

## Archivos de contexto

- `unidades-y-tareas.md` (sección U4)
- `deploy/flux/base/control-plane.yaml`, `deploy/flux/dev/kustomization.yaml`, `deploy/flux/prod/kustomization.yaml`
- `services/go-governance/README.md`, `services/go-identity/README.md` (variables y arranque)
- `tareas/U4-T07-alertas-retencion-dashboard.md` y `revisiones/U4-T07/ronda-1.md` (F-02: `fsGroup` y `Recreate`)
- `docs/operaciones/secrets.md`, `scripts/secrets/generate.sh` (patrón SOPS), `scripts/ci/check-secrets.sh`
- `policy/` (reglas actuales y pruebas), `scripts/ci/policies.sh`
- `tareas/candidatas.md` (C-53, C-70, C-15, C-45)

---

## Criterios de aceptación

Desde la raíz del worktree. Alias (en este entorno `registry.k8s.io` y `ghcr.io` están bloqueados: kustomize y kubeconform están re-etiquetados localmente con los nombres de abajo; kubeconform en contenedor necesita `--network host`):

```bash
K='docker run --rm --security-opt label=disable -v '"$PWD"':/w -w /w registry.k8s.io/kustomize/kustomize:v5.4.3'
Y='docker run --rm -i --security-opt label=disable mikefarah/yq:4.44.3 -N'
C='docker run --rm -i --security-opt label=disable -v '"$PWD"':/project -w /project openpolicyagent/conftest:v0.56.0'
```

- [ ] **CA-1** — `go-governance`: estrategia, `fsGroup` y variables.
  ```bash
  $K build deploy/flux/prod | $Y 'select(.kind=="Deployment" and .metadata.name=="go-governance") | .spec.strategy.type + " " + (.spec.template.spec.securityContext.fsGroup | tostring)'
  $K build deploy/flux/prod | $Y 'select(.kind=="Deployment" and .metadata.name=="go-governance") | .spec.template.spec.containers[0].env[] | select(.value != null) | .name + "=" + .value' | grep -v '^$' | sort
  $K build deploy/flux/prod | $Y 'select(.kind=="Deployment" and .metadata.name=="go-governance") | .spec.template.spec.containers[0].env[] | select(.name=="GOVERNANCE_SERVICE_TOKEN") | (.valueFrom.secretKeyRef.name + "/" + .valueFrom.secretKeyRef.key + " valor=" + (.value | tostring))' | grep -v '^$'
  ```
  Esperado: `Recreate 65532`; las líneas `GOVERNANCE_AUDIT_RETENTION_DAYS=90`, `GOVERNANCE_AUTH=identity`, `GOVERNANCE_DATA_DIR=/data`, `GOVERNANCE_ENV=prod`, `GOVERNANCE_TEST_NAMESPACE=aqs-test`, `IDENTITY_URL=http://go-identity.aqs-system.svc:8080` y `TZ=UTC`, en ese orden alfabético; y `go-governance-service-token/token valor=null`. Antes de la tarea: `RollingUpdate null` y faltan las variables (rojo inicial).

- [ ] **CA-2** — `go-identity`: archivo de usuarios desde un Secret, de solo lectura, legible por el usuario no root.
  ```bash
  $K build deploy/flux/prod | $Y 'select(.kind=="Deployment" and .metadata.name=="go-identity") | .spec.template.spec.containers[0].env[] | select(.value != null) | .name + "=" + .value' | grep -v '^$' | sort
  $K build deploy/flux/prod | $Y 'select(.kind=="Deployment" and .metadata.name=="go-identity") | .spec.template.spec.volumes[0] | [.secret.secretName, .secret.items[0].key, .secret.items[0].path, (.secret.defaultMode | tostring)] | join(" ")' | grep -v '^$'
  $K build deploy/flux/prod | $Y 'select(.kind=="Deployment" and .metadata.name=="go-identity") | .spec.template.spec.containers[0].volumeMounts[0] | [.mountPath, (.readOnly | tostring)] | join(" ")' | grep -v '^$'
  $K build deploy/flux/prod | $Y 'select(.kind=="Deployment" and .metadata.name=="go-identity") | ((.spec.template.spec.securityContext.fsGroup | tostring) + " " + (.spec.strategy.type // "RollingUpdate"))' | grep -v '^$'
  ```
  Esperado: `IDENTITY_TRUST_PROXY=false`, `IDENTITY_USERS_FILE=/etc/aqs/identity/users.json` y `TZ=UTC`; `go-identity-users users.json users.json 288` (`0440` en decimal); `/etc/aqs/identity true`; y `65532 RollingUpdate`.

- [ ] **CA-3** — El overlay `dev` cambia solo `GOVERNANCE_ENV`.
  ```bash
  for e in dev prod; do echo -n "$e: "; $K build deploy/flux/$e | $Y 'select(.kind=="Deployment" and .metadata.name=="go-governance") | .spec.template.spec.containers[0].env[] | select(.name=="GOVERNANCE_ENV") | .value' | grep -v '^$'; done
  diff <($K build deploy/flux/dev | $Y 'select(.kind=="Deployment" and .metadata.name=="go-governance") | .spec.template.spec.containers[0].env[] | select(.name != "GOVERNANCE_ENV") | .name' | grep -v '^$') <($K build deploy/flux/prod | $Y 'select(.kind=="Deployment" and .metadata.name=="go-governance") | .spec.template.spec.containers[0].env[] | select(.name != "GOVERNANCE_ENV") | .name' | grep -v '^$') && echo "mismas variables"
  ```
  Esperado: `dev: dev`, `prod: prod` y `mismas variables`.

- [ ] **CA-4** — Ningún Secret entra al build y la guardia de secretos sigue en verde.
  ```bash
  for e in dev prod; do $K build deploy/flux/$e | $Y 'select(.kind=="Secret" and (.metadata.name=="go-governance-service-token" or .metadata.name=="go-identity-users")) | .metadata.name' | grep -c -v '^$'; done
  b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -c -E '\.sops\.yaml$|/secrets/'
  bash scripts/ci/check-secrets.sh 2>&1 | grep -E '^(OK|FALLA)'
  ```
  Esperado: `0`, `0`, `0` y `OK check-secrets`.

- [ ] **CA-5** — Los demás Deployments y los Services no cambiaron.
  ```bash
  for n in ui-api go-intake go-run-controller go-warm-manager go-reset; do a=$(git show origin/main:deploy/flux/base/control-plane.yaml | $Y "select(.kind==\"Deployment\" and .metadata.name==\"$n\")" | md5sum); b=$($K build deploy/flux/base | $Y "select(.kind==\"Deployment\" and .metadata.name==\"$n\")" | md5sum); echo "$n $([ "$a" = "$b" ] && echo igual || echo DIFIERE)"; done
  ```
  Esperado: cinco líneas `<nombre> igual`. (Si el `md5` del build base difiere por el orden de campos de kustomize, el codificador compara contra el build de `origin/main` construido con el mismo comando, y lo documenta; lo que se exige es que no cambie ningún campo de esos cinco Deployments.)

- [ ] **CA-6** — `kubeconform` estricto en ambos overlays.
  ```bash
  for e in dev prod; do $K build deploy/flux/$e | docker run --rm -i --network host ghcr.io/yannh/kubeconform:v0.6.7 -strict -summary -schema-location default -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json' -; done
  ```
  Esperado: dos líneas con `Invalid: 0, Errors: 0, Skipped: 0`. (En un entorno con acceso directo a `ghcr.io` el `--network host` no hace falta.)

- [ ] **CA-7** — La política nueva: pruebas unitarias, el build real la cumple, y los negativos fallan.
  ```bash
  $C verify --no-color --policy policy; echo "rc=$?"
  for e in dev prod; do $K build deploy/flux/$e | $C test --no-color --policy policy --all-namespaces -; echo "test $e rc=$?"; done
  t=$(mktemp -d); $K build deploy/flux/prod > "$t/b.yaml"
  neg() { { cat "$t/b.yaml"; printf -- '---\n%s\n' "$2"; } | $C test --no-color --policy policy --all-namespaces - >/dev/null 2>&1; echo "$1 rc=$?"; }
  for suf in TOKEN SECRET PASSWORD PASSWD SECRET_KEY; do neg "literal-$suf" "apiVersion: apps/v1
kind: Deployment
metadata: {name: x, namespace: aqs-system}
spec: {selector: {matchLabels: {a: b}}, template: {metadata: {labels: {a: b}}, spec: {serviceAccountName: x, automountServiceAccountToken: false, containers: [{name: c, image: \"x:1\", env: [{name: FOO_$suf, value: \"abc\"}]}]}}}"; done
  neg vacio "apiVersion: apps/v1
kind: Deployment
metadata: {name: x, namespace: aqs-system}
spec: {selector: {matchLabels: {a: b}}, template: {metadata: {labels: {a: b}}, spec: {serviceAccountName: x, automountServiceAccountToken: false, containers: [{name: c, image: \"x:1\", env: [{name: FOO_TOKEN, value: \"\"}]}]}}}"
  rm -rf "$t"
  ```
  Esperado: `rc=0` de `verify` (sin fallos); `test dev rc=0` y `test prod rc=0`; y los seis negativos con `rc=1`. El número de pruebas `test_` nuevas es ≥ 10: `base=$(for f in $(git ls-tree --name-only origin/main policy/ | grep '_test\.rego$'); do git show origin/main:$f; done | grep -c '^test_'); now=$(cat policy/*_test.rego | grep -c '^test_'); echo $((now-base))`.

- [ ] **CA-8** — La validación común y la matriz RBAC siguen en verde.
  ```bash
  bash scripts/ci/policies.sh 2>&1 | grep -E '^(OK|FALLA)' | sort | uniq -c
  ```
  Esperado: ninguna línea `FALLA` (si `kubeconform` falla solo por el proxy del entorno, con un shim de `docker run --network host` pasa; el codificador lo documenta). Aparecen `OK rbac-matrix dev|prod` y `OK check-secrets`.

- [ ] **CA-9** — La documentación cubre los dos Secrets y la advertencia.
  ```bash
  for k in go-governance-service-token go-identity-users 'hash-password' 'mfa_secret' 'sops --encrypt' 'secrets/kustomization.yaml' 'readyz'; do echo "$k=$(grep -c -F -e "$k" docs/operaciones/secrets.md)"; done
  b=$(git merge-base HEAD origin/main); git diff -U0 $b -- docs/operaciones/secrets.md | grep -E '^-[^-]' | wc -l
  ```
  Esperado: cada contador ≥ `1`, y las líneas eliminadas ≤ `3` (solo la lista «Secrets cubiertos» se reescribe; el resto del documento queda igual).

- [ ] **CA-10** — Higiene y alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(deploy/flux/base/control-plane\.yaml|deploy/flux/dev/kustomization\.yaml|policy/secretrefs(_test)?\.rego|docs/operaciones/secrets\.md|bitacoras/U4-T08\.md|revisiones/U4-T08/)' | wc -l
  git diff -U0 $b -- deploy/flux/base/control-plane.yaml | grep -E '^[+-][^+-]' | wc -l
  ```
  Esperado: `0`, `0` y un número > `0` (solo se tocó `control-plane.yaml`, y el diff de CA-5 demuestra que solo en los dos Deployments de U4). `scripts/ci/`, `services/` y `contracts/` quedan sin tocar.

---

## Plan de pruebas

- Política Rego: una prueba que pasa y una que falla por cada sufijo, `value: ""`, `valueFrom`, nombre parecido sin sufijo, un contenedor con varias variables (basta una violación), un `Job`/`CronJob`/`StatefulSet` además del `Deployment`, y un workload fuera de `aqs-system` (no se evalúa).
- CA-7: fixtures negativos concatenados al build real.
- Sin comandos contra un clúster. La comprobación real en dev (pods `Ready`, el volumen escribible por el usuario 65532, el archivo de usuarios legible) queda como pasos documentados para el humano en `docs/operaciones/secrets.md`.

**Rojo primero:** el codificador registra en su bitácora la salida literal de CA-1 y CA-2 sobre la base (`RollingUpdate null`, sin variables, sin volumen) y el resultado de CA-7 con las reglas actuales (los fixtures con valor literal salen con `rc=0`).

---

## Notas

- Archivos que se **modifican en su sitio**: `deploy/flux/base/control-plane.yaml` (solo los Deployments `go-governance` y `go-identity`), `deploy/flux/dev/kustomization.yaml` (un parche), `docs/operaciones/secrets.md`. Nada de duplicados con sufijo.
- El nombre del Service de identidad es `go-identity` en `aqs-system`, puerto `http` = 8080 (`control-plane.yaml`). Las NetworkPolicy de `aqs-system` permiten el tráfico dentro del namespace (`allow-same-namespace`), así que `go-governance` → `go-identity` ya está permitido.
- Los pods no pasarán `/readyz` hasta que un humano cree los dos Secrets. Ese es el comportamiento correcto (fail-closed), no un defecto.
- Verificado en el entorno del loop: kustomize, kubeconform, conftest, yq y promtool. **No** verificable aquí: SOPS (imagen bloqueada) y cualquier clúster.
- Candidatas que esta tarea deja abiertas: extender `generate.sh` con los dos Secrets; endurecimiento de `securityContext` (C-15); cableado de `ui-api` y `go-intake` (con C-45).
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
