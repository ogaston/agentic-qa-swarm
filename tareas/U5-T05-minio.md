# U5-T05 — MinIO in-cluster (bucket `evidence`, cifrado y TLS)

**Unidad:** U5 — Plataforma & GitOps
**Historias que implementa:** US-M10 (habilita US-M6: evidencia de las corridas)
**Depende de:** U5-T04 (namespaces y estructura `deploy/flux/base`). Ola 3, en paralelo con U5-T06 y U5-T07.

---

## Alcance

**Dentro** (una línea, concreta):

> Crear `deploy/flux/base/minio/` con su propio `kustomization.yaml`. Debe contener:
> - Un StatefulSet `minio` en `aqs-system` con la imagen `cgr.dev/chainguard/minio@sha256:9dcc028b309030afa86fc1fc8d93907ae373ea3fb75277cca3fc77e4645932b7`, un PVC por `volumeClaimTemplates`, TLS desde el Secret `minio-tls` montado como `--certs-dir`, y las credenciales y la clave KMS desde Secrets referenciados (`minio-root`, `minio-kms`) y no creados.
> - Un Service `minio`.
> - Un Job `minio-init` (imagen `amazon/aws-cli:2.18.0`) que ejecuta `init-bucket.sh`, montado desde un `configMapGenerator`. El script crea el bucket `evidence` y le aplica cifrado por defecto SSE (`AES256`), y es idempotente.
>
> Además, `scripts/test/minio-local.sh` corre ese mismo script contra un MinIO local en docker y lee de vuelta el resultado.
>
> En `deploy/flux/base/kustomization.yaml` se añade `- minio` como **primera** entrada de `resources`.

Decisión del humano: las imágenes oficiales `minio/minio` y `minio/mc` ya no se pueden descargar (`denied`). Se usa MinIO de Chainguard **fijado por digest** y, como cliente, `aws-cli`.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Crear los Secrets `minio-root`, `minio-kms` o `minio-tls`, o cualquier credencial real. Solo se referencian por nombre.
- NetworkPolicy y RBAC: es **U5-T06**. Observabilidad o ServiceMonitor de MinIO: es **U5-T07**. Backups del bucket: es **U5-T08**.
- Modificar `control-plane.yaml`, `warm.yaml`, `namespaces.yaml` o los overlays `dev` y `prod`.
- En `deploy/flux/base/kustomization.yaml`, cualquier cambio distinto de añadir `- minio` como primera entrada. U5-T06 y U5-T07 añaden sus entradas en otras posiciones para que las tres ramas se fusionen sin conflicto.
- Usar `minio/minio`, `minio/mc` o cualquier imagen con tag `latest`.
- Cualquier `kubectl`, `flux` o `apply` contra un clúster.

---

## Archivos de contexto

- `unidades-y-tareas.md`
- `aidlc-docs/inception/application-design/unit-task-plans/U5.md`
- `aidlc-docs/inception/application-design/components.md` (C4 y C6: puerto `Evidence`)
- `aidlc-docs/inception/requirements/requirements.md` (MinIO in-cluster, NF-SEG-01 cifrado en reposo y en tránsito)
- `deploy/flux/base/` (lo que dejó U5-T04) y `bitacoras/U5-T04.md` (decisiones de namespaces)
- `tareas/candidatas.md`

---

## Criterios de aceptación

Desde la raíz del worktree. Se usan estos alias:

```bash
K='docker run --rm --security-opt label=disable -v '"$PWD"':/w -w /w registry.k8s.io/kustomize/kustomize:v5.4.3'
Y='docker run --rm -i --security-opt label=disable mikefarah/yq:4.44.3 -N'
```

- [ ] **CA-1** — Los overlays construyen e incluyen MinIO.
  ```bash
  for e in dev prod; do $K build deploy/flux/$e > /dev/null || { echo "FALLA $e"; exit 1; }; done; $K build deploy/flux/prod | $Y 'select(.metadata.namespace == "aqs-system" and (.metadata.name == "minio" or .metadata.name == "minio-init")) | .kind + "/" + .metadata.name' | sort
  ```
  Esperado: `Job/minio-init`, `Service/minio` y `StatefulSet/minio`, en ese orden. Antes de la tarea: ninguna línea (rojo inicial).

- [ ] **CA-2** — `kubeconform` estricto, sin recursos omitidos.
  ```bash
  for e in dev prod; do $K build deploy/flux/$e | docker run --rm -i ghcr.io/yannh/kubeconform:v0.6.7 -strict -summary -schema-location default -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json' -; done
  ```
  Esperado: dos líneas con `Invalid: 0, Errors: 0, Skipped: 0`.

- [ ] **CA-3** — Imagen fijada por digest, TLS y secretos solo referenciados.
  ```bash
  $K build deploy/flux/prod | $Y 'select(.kind == "StatefulSet" and .metadata.name == "minio") | .spec.template.spec.containers[0].image'
  $K build deploy/flux/prod | $Y 'select(.kind == "StatefulSet" and .metadata.name == "minio") | .spec.template.spec.containers[0].args | join(" ")' | grep -c -- '--certs-dir'
  $K build deploy/flux/prod | $Y 'select(.kind == "Secret") | .metadata.name' | wc -l
  ```
  Esperado: `cgr.dev/chainguard/minio@sha256:9dcc028b309030afa86fc1fc8d93907ae373ea3fb75277cca3fc77e4645932b7`, `1` y `0`.

- [ ] **CA-4** — Lectura de vuelta con la herramienta real: el script del Job crea `evidence` con cifrado `AES256` en un MinIO local y es idempotente.
  ```bash
  bash scripts/test/minio-local.sh; echo "rc=$?"; docker ps -a --format '{{.Names}}' | grep -c '^aqs-minio-test' ; docker network ls --format '{{.Name}}' | grep -c '^aqs-minio-test'
  ```
  Esperado: el script imprime una línea `evidence AES256` tras la primera ejecución de `init-bucket.sh` y otra igual tras la segunda (idempotencia), luego `rc=0`, `0` y `0` (no deja contenedores ni redes). El harness usa la **misma imagen por digest** del manifiesto, con `MINIO_KMS_SECRET_KEY` de prueba, y el **mismo** `init-bucket.sh` que monta el Job. La lectura de vuelta es `aws s3api get-bucket-encryption` y `aws s3api head-bucket`.

- [ ] **CA-5** — El Job ejecuta exactamente ese script y no tiene la credencial incrustada.
  ```bash
  $K build deploy/flux/prod | $Y 'select(.kind == "ConfigMap" and (.metadata.name | test("^minio-init"))) | .data["init-bucket.sh"]' | sed '$d' | diff - deploy/flux/base/minio/init-bucket.sh && echo IGUAL
  $K build deploy/flux/prod | $Y 'select(.kind == "StatefulSet" or .kind == "Job") | .spec.template.spec.containers[].env[]? | select(.name | test("PASSWORD|SECRET|KMS|ACCESS_KEY")) | select(has("value")) | .name' | wc -l
  ```
  Esperado: `IGUAL` y `0` (ninguna variable de credencial con `value:` literal; las credenciales llegan por `secretKeyRef`).

- [ ] **CA-6** — La entrada `- minio` es la primera de `resources` y no hay otros cambios en ese archivo.
  ```bash
  $Y '.resources[0]' < deploy/flux/base/kustomization.yaml; git diff --numstat $(git merge-base HEAD origin/main) -- deploy/flux/base/kustomization.yaml
  ```
  Esperado: `minio` y `1\t0\tdeploy/flux/base/kustomization.yaml` (una línea añadida, ninguna borrada).

- [ ] **CA-7** — `shellcheck` sobre los scripts nuevos.
  ```bash
  docker run --rm --security-opt label=disable -v "$PWD":/mnt koalaman/shellcheck:v0.10.0 deploy/flux/base/minio/init-bucket.sh scripts/test/minio-local.sh; echo "rc=$?"
  ```
  Esperado: solo `rc=0`.

- [ ] **CA-8** — Árbol limpio tras el commit.
  ```bash
  git status --short | wc -l
  ```
  Esperado: `0`.

---

## Plan de pruebas

- Rojo inicial: el comando literal de CA-1 sobre la base no lista ningún recurso de MinIO.
- CA-4 es la lectura de vuelta con la herramienta real, y además prueba la idempotencia.
- Prueba negativa de CA-4, sobre una copia temporal: con un `init-bucket.sh` que no aplica el cifrado, el harness debe fallar (`rc` distinto de 0) porque la lectura de vuelta no da `AES256`.

**Rojo primero:** el codificador registra en su bitácora la salida del comando literal de CA-1 antes de crear nada.

---

## Notas

- `init-bucket.sh` lee el endpoint, las credenciales y el CA bundle de variables de entorno (`AQS_S3_ENDPOINT`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_CA_BUNDLE`). En el clúster el endpoint es `https://minio.aqs-system.svc:9000`; en el harness local es `http://`.
- Usar `docker` (no `podman`) en el harness, con nombres de contenedor y red con prefijo `aqs-minio-test` y limpieza con `trap`.
- La bitácora pega el **comando literal** de cada criterio y su salida. El CA-8 posterior al último commit va en el informe de vuelta, con una nota en la bitácora que lo diga.
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
