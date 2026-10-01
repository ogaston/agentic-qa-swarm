# U5-T08 — Backup & Restore de evidencias, runbook de restore y procesos ligeros (cambios, IR/COE)

**Unidad:** U5 — Plataforma & GitOps
**Historias que implementa:** US-M10 (requisitos AR1, AR2, AR7, NF-RES-02, NF-RES-07, NF-RES-11, NF-RES-12 y NF-RES-15)
**Depende de:** U5-T05 (MinIO y bucket `evidence`, fusionado en main). Ola 4.

---

## Alcance

**Dentro** (una línea, concreta):

> Crear `deploy/flux/base/backup/` con su propio `kustomization.yaml`, que contenga:
> - Un CronJob `evidence-backup` en `aqs-system`. Imagen `amazon/aws-cli:2.18.0`, schedule `0 */6 * * *` (RPO ≤ 6 h) y `concurrencyPolicy: Forbid`. Ejecuta `backup.sh`, montado por `configMapGenerator`, que:
>   - copia `s3://evidence` al destino `s3://<bucket>/<YYYY-MM-DDTHH>/` con `--sse AES256`;
>   - borra los prefijos de más de 30 días.
> - El destino se lee del Secret referenciado y no creado `backup-target` (endpoint, bucket y credenciales).
> - Un `PrometheusRule` `aqs-backup-rules` con dos alertas:
>   - `AqsBackupFailed`: un Job del CronJob falló.
>   - `AqsBackupStale`: más de 8 h sin backup exitoso.
>
> Además:
> - Crear `restore.sh`, que restaura un prefijo elegido al bucket `evidence`.
> - Crear `scripts/test/backup-local.sh`, que en un MinIO local prueba backup, cifrado, poda y restore, leyendo de vuelta con la herramienta real.
> - Escribir tres documentos:
>   - `docs/operaciones/runbook-restore.md`: procedimiento, RTO y RPO, validación, failover y failback.
>   - `docs/operaciones/gestion-de-cambios.md`: registro de cambio, aprobación y nota de rollback.
>   - `docs/operaciones/respuesta-a-incidentes.md`: alerta, runbook y post-mortem/COE, con plantilla.
> - Crear el índice `docs/operaciones/README.md`, que enlace los tres.
>
> En `deploy/flux/base/kustomization.yaml`, añadir `- backup` justo **después** de `- namespaces.yaml`.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Crear el Secret `backup-target` o cualquier credencial real. El destino real debe estar **fuera del clúster**; quién lo provee queda en la candidata C-08.
- Backups de una base de datos de plataforma: todavía no existe en los manifiestos. Se documenta en el runbook como pendiente de U2/U4 y se registra como candidata.
- Backups del entorno warm (`warm-db`): es desechable, porque se resetea y se reconstruye (M7).
- Modificar `minio/`, `security/`, `observability/` (U5-T07, en revisión), `control-plane.yaml`, `warm.yaml` o los overlays.
- NetworkPolicy para `aqs-system` (candidata C-12).
- En `deploy/flux/base/kustomization.yaml`, cualquier cambio distinto de añadir `- backup` después de `- namespaces.yaml`.
- Modificar `README.md` de la raíz.
- Cualquier `kubectl`, `flux` o `apply` contra un clúster.

---

## Archivos de contexto

- `unidades-y-tareas.md`
- `aidlc-docs/inception/application-design/unit-task-plans/U5.md`
- `aidlc-docs/inception/requirements/requirements.md` (AR1, AR2, AR7, NF-RES-02, NF-RES-07, NF-RES-11, NF-RES-12, NF-RES-15)
- `deploy/flux/base/minio/` (sobre todo `init-bucket.sh` y `job-init.yaml`, cuyo patrón de envs, CA bundle y `command` se reutiliza) y `scripts/test/minio-local.sh` (patrón del harness)
- `bitacoras/U5-T05.md` (claves de los Secrets de MinIO)
- `tareas/candidatas.md`

---

## Criterios de aceptación

Desde la raíz del worktree. Se usan estos alias:

```bash
K='docker run --rm --security-opt label=disable -v '"$PWD"':/w -w /w registry.k8s.io/kustomize/kustomize:v5.4.3'
Y='docker run --rm -i --security-opt label=disable mikefarah/yq:4.44.3 -N'
```

- [ ] **CA-1** — El CronJob y la regla están en el build.
  ```bash
  $K build deploy/flux/prod | $Y 'select(.metadata.namespace == "aqs-system" and (.metadata.name == "evidence-backup" or .metadata.name == "aqs-backup-rules")) | .kind + "/" + .metadata.name' | sort
  ```
  Esperado: `CronJob/evidence-backup` y `PrometheusRule/aqs-backup-rules`. Antes de la tarea: ninguna línea (rojo inicial).

- [ ] **CA-2** — `kubeconform` estricto, sin recursos omitidos.
  ```bash
  for e in dev prod; do $K build deploy/flux/$e | docker run --rm -i ghcr.io/yannh/kubeconform:v0.6.7 -strict -summary -schema-location default -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json' -; done
  ```
  Esperado: dos líneas con `Invalid: 0, Errors: 0, Skipped: 0`.

- [ ] **CA-3** — Schedule, concurrencia, imagen y credenciales del CronJob.
  ```bash
  $K build deploy/flux/prod | $Y 'select(.kind == "CronJob" and .metadata.name == "evidence-backup") | .spec.schedule + " " + .spec.concurrencyPolicy + " " + .spec.jobTemplate.spec.template.spec.containers[0].image'
  $K build deploy/flux/prod | $Y 'select(.kind == "CronJob" and .metadata.name == "evidence-backup") | .spec.jobTemplate.spec.template.spec.containers[].env[]? | select(.name | test("PASSWORD|SECRET|ACCESS_KEY")) | select(has("value")) | .name' | wc -l
  $K build deploy/flux/prod | $Y 'select(.kind == "Secret") | .metadata.name' | wc -l
  ```
  Esperado: `0 */6 * * * Forbid amazon/aws-cli:2.18.0`, `0` y `0`.

- [ ] **CA-4** — Lectura de vuelta con la herramienta real: backup cifrado, poda y restore validado, en un MinIO local.
  ```bash
  bash scripts/test/backup-local.sh; echo "rc=$?"; docker ps -a --format '{{.Names}}' | grep -c '^aqs-backup-test'; docker network ls --format '{{.Name}}' | grep -c '^aqs-backup-test'
  ```
  El harness usa el **mismo** digest de MinIO de `deploy/flux/base/minio/statefulset.yaml` (extraído con `yq`, no copiado) y los **mismos** `backup.sh` y `restore.sh` del ConfigMap. Hace lo siguiente:
  1. Siembra 3 objetos en `evidence`.
  2. Crea a mano en el destino un prefijo con fecha de hace 31 días.
  3. Ejecuta `backup.sh`.
  4. Lee con `aws s3api head-object` el `ServerSideEncryption` de un objeto respaldado.
  5. Comprueba que el prefijo viejo desapareció y que el nuevo sigue.
  6. Vacía `evidence` y ejecuta `restore.sh` con el prefijo nuevo.
  7. Compara el checksum de los 3 objetos restaurados con los originales.

  Esperado, en este orden: `backup objetos=3`, `sse=AES256`, `poda viejo=0 nuevo=1`, `restore iguales=3`, luego `rc=0`, `0` y `0`.

- [ ] **CA-5** — Los scripts que ejecuta el CronJob son exactamente los del repo.
  ```bash
  for s in backup.sh restore.sh; do $K build deploy/flux/prod | $Y 'select(.kind == "ConfigMap" and (.metadata.name | test("^evidence-backup"))) | .data["'$s'"]' | sed '$d' | diff - deploy/flux/base/backup/$s >/dev/null && echo "$s IGUAL" || echo "$s DISTINTO"; done
  ```
  Esperado: `backup.sh IGUAL` y `restore.sh IGUAL`.

- [ ] **CA-6** — Las alertas de backup pasan `promtool` y tienen severidad y resumen.
  ```bash
  t=$(mktemp -d); chmod 755 "$t"; $K build deploy/flux/prod | $Y 'select(.kind == "PrometheusRule" and .metadata.name == "aqs-backup-rules") | .spec' > "$t/rules.yaml"
  docker run --rm --security-opt label=disable -v "$t":/r -w /r --entrypoint promtool prom/prometheus:v2.55.1 check rules rules.yaml
  $Y '.groups[].rules[] | select(has("alert")) | .alert' < "$t/rules.yaml" | sort
  $Y '.groups[].rules[] | select(has("alert")) | select((.labels.severity // "") == "" or (.annotations.summary // "") == "") | .alert' < "$t/rules.yaml" | wc -l
  rm -rf "$t"
  ```
  Esperado: `SUCCESS: 2 rules found` (o más), luego `AqsBackupFailed` y `AqsBackupStale`, y `0`.

- [ ] **CA-7** — Los documentos existen, están enlazados y cubren lo que piden los requisitos.
  ```bash
  for f in README runbook-restore gestion-de-cambios respuesta-a-incidentes; do test -f docs/operaciones/$f.md || echo "FALTA $f"; done
  grep -c -E '\]\((runbook-restore|gestion-de-cambios|respuesta-a-incidentes)\.md\)' docs/operaciones/README.md
  grep -c -i -E '^#+ .*(RTO|RPO|validaci|failover|failback)' docs/operaciones/runbook-restore.md
  grep -c -E 'restore\.sh|evidence-backup|AqsBackupFailed|AqsBackupStale' docs/operaciones/runbook-restore.md
  grep -c -i -E '^#+ .*(registro|aprobaci|rollback)' docs/operaciones/gestion-de-cambios.md
  grep -c -i -E '^#+ .*(alerta|runbook|post-mortem|COE|plantilla)' docs/operaciones/respuesta-a-incidentes.md
  ```
  Esperado: ninguna línea `FALTA`, luego `3`, y números ≥ `5`, ≥ `4`, ≥ `3` y ≥ `4`.

- [ ] **CA-8** — Solo se añadió la entrada permitida en `kustomization.yaml`, en su posición.
  ```bash
  git diff --numstat $(git merge-base HEAD origin/main) -- deploy/flux/base/kustomization.yaml; grep -A1 -E '^  - namespaces.yaml$' deploy/flux/base/kustomization.yaml | tail -1
  ```
  Esperado: `1\t0\tdeploy/flux/base/kustomization.yaml` y `  - backup`.

- [ ] **CA-9** — `shellcheck` y árbol limpio.
  ```bash
  docker run --rm --security-opt label=disable -v "$PWD":/mnt koalaman/shellcheck:v0.10.0 deploy/flux/base/backup/backup.sh deploy/flux/base/backup/restore.sh scripts/test/backup-local.sh; echo "rc=$?"
  git status --short | wc -l
  ```
  Esperado: `rc=0` y `0`.

---

## Plan de pruebas

- Rojo inicial: el comando literal de CA-1 sobre la base no lista nada.
- CA-4 es la lectura de vuelta con la herramienta real y prueba los tres comportamientos: cifrado, poda y restore con checksums.
- Pruebas negativas sobre copias temporales:
  - Un `backup.sh` sin `--sse AES256` hace fallar el harness en `sse=`.
  - Un `restore.sh` que no copia nada hace fallar el harness en `restore iguales=`.
- La poda no toca prefijos que no tengan formato de fecha. El harness siembra uno (`manual/`) y comprueba que sigue ahí.

**Rojo primero:** el codificador registra en su bitácora la salida del comando literal de CA-1 antes de crear nada.

---

## Notas

- Valores fijados por la tarea: RPO ≤ 6 h (schedule `0 */6 * * *`), retención de 30 días, prefijo `YYYY-MM-DDTHH` en UTC, y la alerta de obsolescencia a las 8 h. Lo que la tarea no fija (claves del Secret `backup-target`, umbrales de las expresiones, `backoffLimit`, etc.) va en la sección "Decisiones no fijadas por la tarea" de la bitácora.
- Las alertas pueden usar métricas de kube-state-metrics, que instala kube-prometheus-stack en U5-T07: `kube_job_status_failed` y `kube_cronjob_status_last_successful_time`.
- El CronJob habla con MinIO por `https://minio.aqs-system.svc:9000`, con el CA bundle de `minio-tls`, igual que `minio-init`, y lleva `serviceAccountName`.
- `docs/operaciones/` es nuevo; `docs/` ya existe con la documentación de producto.
- La bitácora pega el **comando literal** de cada criterio y su salida. El CA-9 posterior al último commit va en el informe de vuelta, con una nota en la bitácora que lo diga.
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
