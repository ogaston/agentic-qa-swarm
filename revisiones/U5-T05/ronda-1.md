# Ronda 1 — U5-T05

VEREDICTO: VERDE

Worktree `/home/omarjayg/Javeriana/topicos-especiales/wt-U5-T05`, HEAD 795608c. El hash de la tarea coincide (26918637...). No ejecuté ningún kubectl, flux ni apply. El worktree estaba limpio antes de empezar y sigue limpio después de todas mis ejecuciones (`git status --short | wc -l` = 0). Mi prueba negativa y mi variante del harness usaron copias en el scratchpad.

## Criterios de aceptación, verificados por mí
Corrí los comandos literales de la tarea con los alias `K` e `Y`.

| # | Criterio | Resultado |
|---|---|---|
| 1 | Los overlays construyen e incluyen MinIO | pasa: `Job/minio-init`, `Service/minio`, `StatefulSet/minio`, en ese orden |
| 2 | `kubeconform` estricto | pasa: dos líneas `Valid: 30, Invalid: 0, Errors: 0, Skipped: 0` |
| 3 | Digest, `--certs-dir` y Secrets | pasa: digest `9dcc028b...`, `1` y `0` |
| 4 | Lectura de vuelta, idempotencia, sin residuos | pasa: `evidence AES256` dos veces, `rc=0`, `0` contenedores, `0` redes. Tardó 7 s, con imágenes en caché; el tiempo es plausible y no es un salto de la suite. |
| 5 | El Job ejecuta ese script, sin credencial literal | pasa: `IGUAL` y `0` |
| 6 | `- minio` primero, sin otros cambios | pasa: `minio` y `1\t0\tdeploy/flux/base/kustomization.yaml`. El `merge-base` con `origin/main` es eebfdfc, igual a la base del diff. |
| 7 | `shellcheck` | pasa: `rc=0` |
| 8 | Árbol limpio | pasa: `0` |

Prueba negativa de CA-4, sobre una copia temporal de `init-bucket.sh` sin `put-bucket-encryption`:
- El harness falla con `ServerSideEncryptionConfigurationNotFoundError` y `rc=254`.
- Después quedan 0 contenedores y 0 redes `aqs-minio-test`.
- El fallo sale del propio `get-bucket-encryption` y no de la comparación con `AES256`. Detecta el cifrado ausente igualmente.

## Los cuatro puntos que pediste
1. **CA-4 y el harness.**
   - El harness monta el mismo `deploy/flux/base/minio/init-bucket.sh` del manifiesto (`-v "$SCRIPT":/scripts/init-bucket.sh`), sin copia.
   - La lectura de vuelta usa `aws s3api head-bucket` y `get-bucket-encryption` con la herramienta real, y compara con `AES256`.
   - La idempotencia es real. Corrí una variante en el scratchpad que no silencia la salida del script y se ven las dos ramas. La primera ejecución crea el bucket (`"Location": "/evidence"`). La segunda imprime `bucket evidence ya existe` y reaplica el cifrado.
   - El digest del harness es el mismo que el del manifiesto (`grep -c` da 1 en cada archivo). Es una copia literal, sin vínculo mecánico (ver F-01).
2. **TLS.** El Job usa `AQS_S3_ENDPOINT=https://minio.aqs-system.svc:9000` y `AWS_CA_BUNDLE=/etc/minio-ca/ca.crt`, tomado de `minio-tls` (clave `ca.crt`). No hay `--no-verify-ssl`, ni `http://`, ni `latest` en los manifiestos (`grep` sin coincidencias). El harness usa `http://` en local, como permite la nota de la tarea, así que la ruta TLS no se ejercita en CA-4.
3. **Decisiones no fijadas.** Ninguna es un riesgo bloqueante; los puntos menores están en F-02 a F-05.
4. **Secrets, credenciales y `base/kustomization.yaml`.**
   - No se crea ningún Secret (CA-3 da 0).
   - Las credenciales llegan solo por `secretKeyRef`. No hay literales de credencial en los manifiestos; los únicos literales son `testaccesskey` y `testsecretkey123` del harness, que son de prueba.
   - El cambio en `base/kustomization.yaml` es exactamente `+  - minio` en primera posición.

## Alcance «Fuera»
- El diff toca 8 archivos, todos dentro del alcance: la bitácora, `base/kustomization.yaml`, 5 archivos en `base/minio/` y `scripts/test/minio-local.sh`.
- No se tocan `control-plane.yaml`, `warm.yaml`, `namespaces.yaml`, `dev` ni `prod`.
- No hay NetworkPolicy, RBAC, ServiceMonitor ni backups. Las imágenes son solo Chainguard por digest y `amazon/aws-cli:2.18.0`.

## Hallazgos
Ninguno ROJO ni NARANJA.

### F-01 · AMARILLO · `scripts/test/minio-local.sh:11-12` · Digest e imagen del cliente duplicados sin vínculo mecánico
El digest de MinIO y `amazon/aws-cli:2.18.0` están copiados en el harness y en `statefulset.yaml` y `job-init.yaml`. Hoy coinciden, pero si alguien actualiza el manifiesto y no el harness, CA-4 dejaría de probar la imagen real sin que nada falle. Conviene derivarlos con `yq` del manifiesto, o añadir una comprobación de igualdad.

### F-02 · AMARILLO · `deploy/flux/base/minio/service.yaml` · Service no headless con `serviceName: minio`
El StatefulSet referencia como `serviceName` un Service ClusterIP normal. Con 1 réplica funciona, pero no hay DNS estable por pod. Si se escala, hay que añadir un Service headless.

### F-03 · AMARILLO · `deploy/flux/base/minio/kustomization.yaml:9-10` y `job-init.yaml` · `disableNameSuffixHash` y Job inmutable
- Es una decisión aceptable, pero tiene dos consecuencias.
- Un cambio en `init-bucket.sh` actualiza el ConfigMap pero no relanza el Job, porque el pod template no cambia.
- Un cambio en el `spec` del Job hará fallar el apply de Flux por campo inmutable. `deploy/flux/prod/flux-kustomization.yaml` no tiene `force` ni `wait`.
- `backoffLimit: 6` agota unos 10 minutos de reintentos. Si los Secrets o el MinIO tardan más en existir, el Job queda `Failed` y nada lo recrea.

### F-04 · AMARILLO · `statefulset.yaml` y `job-init.yaml` · Endurecimiento de seguridad de los contenedores
- Falta `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]` y `readOnlyRootFilesystem`. El Job no tiene `securityContext`.
- El Namespace no tiene etiquetas de Pod Security, así que no bloquea hoy.
- Es una mejora razonable para NF-SEG.

### F-05 · AMARILLO · `statefulset.yaml:80-83` · PVC de 10Gi sin `storageClassName`
Depende de que el clúster tenga una StorageClass por defecto; si no, el PVC queda en `Pending`. 10Gi puede ser corto para evidencia. Queda para decidirlo en una tarea posterior.

### F-06 · AMARILLO · `deploy/flux/base/minio/init-bucket.sh:11` · `head-bucket ... 2>/dev/null`
El `head-bucket` silencia el error. Si falla por TLS o por 403, el script cae a `create-bucket`. El error igualmente se ve y el Job falla, por lo que no es un defecto funcional, pero el diagnóstico es menos claro.

## Tareas candidatas (defectos reales fuera de alcance)
- Un vínculo mecánico entre las imágenes del harness y los manifiestos (F-01).
- Un `securityContext` endurecido para MinIO y el Job (F-04).
- Una estrategia para recrear el Job de init cuando cambie el script: nombre con hash, `ttlSecondsAfterFinished`, o `force` y `wait` en la Kustomization de Flux (F-03).
- Un Service headless si MinIO escala a más de una réplica (F-02).
- Documentar los SAN que debe tener el certificado de `minio-tls` (`minio.aqs-system.svc`) y el formato de `MINIO_KMS_SECRET_KEY` (`nombre:base64` de 32 bytes), porque los Secrets se crean fuera de esta tarea.

## Rutas de transcripciones largas
Ninguna: las salidas son cortas y están citadas arriba.

VEREDICTO: VERDE
AMARILLO|scripts/test/minio-local.sh:11-12|Digest e imagen del cliente duplicados sin vínculo mecánico
AMARILLO|deploy/flux/base/minio/service.yaml|Service no headless con serviceName minio
AMARILLO|deploy/flux/base/minio/kustomization.yaml:9-10|disableNameSuffixHash y Job inmutable
AMARILLO|statefulset.yaml, job-init.yaml|Endurecimiento de securityContext ausente
AMARILLO|statefulset.yaml:80-83|PVC de 10Gi sin storageClassName
AMARILLO|deploy/flux/base/minio/init-bucket.sh:11|head-bucket silencia el error
INFORME: revisiones/U5-T05/ronda-1.md
