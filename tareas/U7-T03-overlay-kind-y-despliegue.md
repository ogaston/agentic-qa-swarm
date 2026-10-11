# U7-T03 — Overlay `deploy/flux/kind`, Secrets efímeros y despliegue en el clúster local

**Unidad:** U7 — Plataforma en kind local (podman)
**Historias que implementa:** US-M10, sobre US-M8.1 y US-M8.2 (RBAC y NetworkPolicy aplicados de verdad).
**Depende de:** U7-T02 **fusionada**. **Ola 3**. **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea, concreta):

> Añadir el overlay `deploy/flux/kind/` (plataforma sin observabilidad ni backups), `scripts/kind/secrets.sh` (crea en el clúster, nunca en el repo, los Secrets que piden los manifiestos con valores aleatorios) y `scripts/kind/deploy.sh` (aplica el overlay en `kind-aqs` y espera a que todo esté disponible), e incluir el overlay `kind` en `scripts/ci/policies.sh`.

Detalle:

- **Overlay** (`deploy/flux/kind/kustomization.yaml`): referencia **piezas** de `../base` (`namespaces.yaml`, `control-plane.yaml`, `warm.yaml`, `security`, `minio`) en vez de `../base` entero, para dejar fuera `observability` (HelmRelease, HelmRepository, ServiceMonitor y PrometheusRule exigen CRDs de Flux y prometheus-operator que kind no trae) y `backup` (destino S3 externo). Reutiliza los parches de `deploy/flux/dev/kustomization.yaml` (réplicas 1, memoria). Sin `latest`, sin cambiar imágenes. Ningún archivo de `deploy/flux/base/`, `dev/` o `prod/` cambia.
- **Secrets** (`scripts/kind/secrets.sh`): lista los Secrets que referencian los objetos del overlay (`kubectl kustomize deploy/flux/kind` → `secretName`, `secretKeyRef`, `secretRef`) y crea cada uno con `kubectl create secret … --dry-run=client -o yaml | kubectl apply -f -`, con las claves que esos objetos usan. A día de hoy son: `minio-root`, `minio-kms`, `minio-tls` (certificado autofirmado con `openssl`, SAN `minio.<ns>.svc` y su `ca.crt`), `aqs-evidence-s3`, `warm-db-credentials`, `go-governance-service-token`, `go-reset-service-token`, `go-warm-manager-service-token` y `go-identity-users` (usuarios `demo` rol `user` y `admin` rol `admin` hechos con `go-identity hash-password`; sus contraseñas se guardan en `${XDG_RUNTIME_DIR:-/tmp}/aqs-kind/` con permisos `600`, nunca en el repo ni en la salida). Si un Secret referenciado no está en la lista que el script sabe crear, falla con su nombre. Idempotente: si ya existe, no lo regenera.
- **Despliegue** (`scripts/kind/deploy.sh`): `require_kind_context` → `secrets.sh` → `kubectl apply -k deploy/flux/kind` (con `--server-side` si hace falta para CRD-less) → `kubectl rollout status` de cada Deployment y StatefulSet con `--timeout=300s` → espera a que el Job de inicialización de MinIO termine. Imprime `OK|FALLA <objeto>` y sale distinto de 0 si algo falla.
- **Políticas**: `scripts/ci/policies.sh` construye también el overlay `kind` y le pasa las mismas comprobaciones que a `dev` (kubeconform estricto, conftest, regla de conjunto default-deny). El overlay **no** relaja ninguna política (`policy/*.rego` no cambia).
- **Defectos destapados**: si un servicio no arranca en kind por un defecto suyo (no del overlay), se registra como candidata en la bitácora, se documenta la `FALLA` y se escala al orquestador; no se arregla en esta tarea.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Instalar Flux, prometheus-operator, metrics-server o ingress en el clúster; observabilidad y backups en kind (candidatas).
- Cambios en servicios, agentes, `policy/`, `deploy/flux/base|dev|prod/` o SOPS.
- Pruebas de humo de las APIs (U7-T04).

---

## Archivos de contexto

- `tareas/U7-T01-cluster-kind.md`, `tareas/U7-T02-imagenes-locales.md` y sus bitácoras
- `deploy/flux/base/**`, `deploy/flux/dev/kustomization.yaml`
- `scripts/ci/policies.sh`, `policy/*.rego`
- `docs/operaciones/secrets.md`, `scripts/secrets/generate.sh` (qué claves lleva cada Secret)
- `services/go-identity/README.md` (`hash-password`, formato de `users.json`)

---

## Criterios de aceptación

Desde la raíz del worktree, con el clúster creado y las imágenes cargadas (`kind-up.sh`, `build-images.sh`, `load-images.sh`).

- [ ] **CA-1** — Políticas sobre el overlay, sin relajarlas.
  ```bash
  bash scripts/ci/policies.sh 2>&1 | grep -E '^(OK|FALLA)' | grep -c FALLA; bash scripts/ci/policies.sh 2>&1 | grep -c -E '^OK .*kind'
  git diff --name-only $(git merge-base HEAD origin/main) -- policy deploy/flux/base deploy/flux/dev deploy/flux/prod | wc -l
  ```
  Esperado: `0`, al menos `3` y `0`.

- [ ] **CA-2** — El overlay deja fuera lo que kind no soporta.
  ```bash
  kubectl kustomize deploy/flux/kind | grep -c -E '^kind: (HelmRelease|HelmRepository|ServiceMonitor|PrometheusRule)$'
  kubectl kustomize deploy/flux/kind | grep -c -E '^kind: (Deployment|StatefulSet|NetworkPolicy)$'
  ```
  Esperado: `0` y un número mayor que `0`.

- [ ] **CA-3** — Despliegue, leído de vuelta.
  ```bash
  bash scripts/kind/deploy.sh 2>&1 | grep -E '^(OK|FALLA)'; echo "rc=${PIPESTATUS[0]}"
  kubectl get deploy -A -l aqs.io/tier=control-plane -o jsonpath='{range .items[*]}{.metadata.name}={.status.availableReplicas}{"\n"}{end}'
  kubectl get pods -n aqs-system --no-headers | grep -v -c -E 'Running|Completed'
  ```
  Esperado: ninguna `FALLA`, `rc=0`, cada Deployment del control plane con `=1`, y `0` pods en otro estado.

- [ ] **CA-4** — Secrets fuera del repo y sin fugas.
  ```bash
  bash scripts/kind/deploy.sh > "${TMPDIR:-/tmp}/u7t03.out" 2>&1; grep -c -E 'password_hash|\$argon2id\$|BEGIN (RSA |EC )?PRIVATE KEY' "${TMPDIR:-/tmp}/u7t03.out"
  git grep -n -E 'kind: Secret' -- deploy/flux/kind | wc -l
  stat -c '%a' "${XDG_RUNTIME_DIR:-/tmp}/aqs-kind"/* | sort -u
  ```
  Esperado: `0`, `0` y solo `600`.

- [ ] **CA-5** — Idempotencia y guarda.
  ```bash
  a=$(kubectl get secret go-identity-users -n aqs-system -o jsonpath='{.metadata.uid}'); bash scripts/kind/deploy.sh >/dev/null 2>&1; echo "rc=$?"; b=$(kubectl get secret go-identity-users -n aqs-system -o jsonpath='{.metadata.uid}'); [ "$a" = "$b" ] && echo mismo
  kubectl config set-context aqs-guard-test --cluster=kind-aqs --user=kind-aqs >/dev/null && kubectl config use-context aqs-guard-test >/dev/null; bash scripts/kind/deploy.sh; echo "rc=$?"; kubectl config use-context kind-aqs >/dev/null; kubectl config delete-context aqs-guard-test >/dev/null
  ```
  Esperado: `rc=0`, `mismo` y `rc=3`.

- [ ] **CA-6** — Alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(deploy/flux/kind/|scripts/kind/|scripts/ci/policies\.sh|deploy/kind/README\.md|bitacoras/U7-T03\.md|revisiones/U7-T03/)' | wc -l
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

- El despliegue se lee de vuelta con `kubectl get` (disponibilidad, estado de pods, uid de Secrets), no con el código de salida del script.
- **Rojo primero:** pegar la salida de `kubectl kustomize deploy/flux/kind` y de `bash scripts/kind/deploy.sh` (no existen).
- Al terminar: `bash scripts/kind/kind-down.sh`.

---

## Errata (2026-10-10, ronda 1, decidida por el humano)

La ronda 1 destapó dos problemas de la especificación. El humano decidió:

- **E-1 · Forma del overlay.** El cargador por defecto de kustomize no admite archivos sueltos de `../base` fuera del directorio del overlay. Se acepta que el overlay use `../base` **entero** y quite `observability` y `backup` con parches `$patch: delete`. CA-2 sigue siendo la comprobación del resultado. No se usa `--load-restrictor`.
- **E-2 · Huecos de configuración que impiden arrancar (solo en kind).** Tres Deployments del control plane no arrancan por huecos que `base`, `dev` y `prod` también tienen. En kind se cubren **solo** desde `deploy/flux/kind/` y `scripts/kind/`. `base`, `dev` y `prod` no cambian.
  - `go-intake` exige `GITHUB_WEBHOOK_SECRET` (`services/go-intake/cmd/go-intake/main.go:39`). El overlay la añade con `secretKeyRef` a un Secret nuevo `go-intake-webhook` (clave `secret`). `secrets.sh` crea ese Secret con un valor aleatorio, igual que los demás.
  - `ui-api` exige `UIAPI_AUTH` (`services/ui-api/cmd/ui-api/main.go:57`). El overlay pone `UIAPI_AUTH=identity` e `IDENTITY_URL` apuntando al Service de `go-identity` dentro del clúster. Primero se mira si las NetworkPolicy de `base` ya permiten el tráfico `ui-api` → `go-identity`. Si no lo permiten, el overlay añade **una** NetworkPolicy de permiso acotada a ese par de pods y a ese puerto, y debe seguir pasando `policies.sh`. Ninguna regla de `policy/*.rego` cambia.
  - `go-reset` monta el ConfigMap `go-reset-baseline`, que `base` declara como aportado por un humano; en el repo no existe ningún `baseline.sh`. `secrets.sh` crea en el clúster un `baseline.sh` **de relleno**, solo para kind, con el contrato `clean|verify|version`:
    - `clean` sale con `0`;
    - `verify` imprime `0`;
    - `version` imprime `kind-stub`.
    El ConfigMap lleva la etiqueta `aqs.io/kind-stub: "true"`. Ese relleno **no** verifica nada: U7-T04 debe declarar el reset verificado como `PENDIENTE` en kind.
- Los criterios de aceptación y CA-6 no cambian. Candidatas: C-103 a C-106.

## Notas

- El HPA de `go-run-controller` queda sin métricas (no hay metrics-server); con `maxReplicas: 1` no cambia nada.
- Que todos los pods estén `Running` no implica que el ciclo de corrida funcione: los eventos van por archivos en el disco de cada pod (sin transporte entre pods, P1) y los agentes no tienen Deployment (P2). Eso lo deja explícito U7-T04.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
