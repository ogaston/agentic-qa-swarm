# Runbook de backup y restore de evidencias

Alcance: bucket `evidence` de MinIO (namespace `aqs-system`). Fuera de alcance: `warm-db` (desechable, se resetea y reconstruye) y la base de datos de la plataforma (pendiente de U2/U4, aun no existe en los manifiestos; candidata registrada).

## Como funciona el backup

- CronJob `evidence-backup` (`deploy/flux/base/backup/`), schedule `0 */6 * * *`, `concurrencyPolicy: Forbid`. Los scripts viven en el ConfigMap `evidence-backup-scripts` (namespace `aqs-system`).
- `backup.sh` copia `s3://evidence` a `s3://<bucket>/<YYYY-MM-DDTHH>/` (UTC) con `--sse AES256` y borra los prefijos de fecha con mas de 30 dias. Los prefijos sin formato de fecha (p. ej. `manual/`) no se tocan. Si falla la copia o el listado de la poda, el Job falla.
- El destino se lee del Secret `backup-target` (claves `endpoint`, `bucket`, `access-key-id`, `secret-access-key`), referenciado y no creado en el repo. Debe estar **fuera del cluster** (quien lo provee: candidata C-08). El origen usa los Secrets `minio-root` (`root-user`, `root-password`) y `minio-tls` (`ca.crt`).
- Alertas:
  - `AqsBackupFailed`: hay un Job de `evidence-backup` fallido **posterior al ultimo backup exitoso** (o no hubo nunca un exito). Se apaga sola cuando un backup posterior termina bien, aunque el Job fallido siga en el historial.
  - `AqsBackupStale`: mas de 8 h sin backup exitoso, o sin la metrica. **En una instalacion nueva salta hasta el primer backup exitoso** (hasta 6 h): es esperado.
  - La regla se prueba con `promtool test rules` (`deploy/flux/base/backup/rules_test.yaml`).

## RTO y RPO

### RPO

- RPO: 6 h o menos (un backup cada 6 h; `AqsBackupStale` alerta a las 8 h).

### RTO

- RTO: objetivo de 4 h para restaurar `evidence` desde el ultimo prefijo (valor propuesto, pendiente de confirmar en C-08).

## Procedimiento de restore

1. Elegir el prefijo: ejecutar un `aws s3 ls s3://<bucket>/` con las credenciales de `backup-target` y tomar el mas reciente con formato `YYYY-MM-DDTHH` (por ejemplo `2026-10-01T18`).
2. Asegurar que MinIO y el bucket `evidence` existen: `kubectl -n aqs-system get job minio-init` debe estar `Complete`.
3. Guardar el manifiesto siguiente como `restore-job.yaml` (reutiliza imagen, ConfigMap de scripts, CA de `minio-tls` y Secrets del CronJob), sustituir `PREFIJO_AQUI` y crearlo:

```bash
sed 's/PREFIJO_AQUI/2026-10-01T18/' restore-job.yaml | kubectl -n aqs-system create -f -
kubectl -n aqs-system wait --for=condition=complete --timeout=3600s job -l app.kubernetes.io/name=evidence-restore
kubectl -n aqs-system logs job -l app.kubernetes.io/name=evidence-restore
```

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  generateName: evidence-restore-
  namespace: aqs-system
  labels:
    app.kubernetes.io/name: evidence-restore
spec:
  backoffLimit: 0
  template:
    metadata:
      labels:
        app.kubernetes.io/name: evidence-restore
    spec:
      serviceAccountName: aqs-backup
      restartPolicy: Never
      containers:
        - name: restore
          image: amazon/aws-cli:2.18.0
          command:
            - /bin/sh
            - -c
            - |
              set -eu
              /bin/sh /scripts/restore.sh "$RESTORE_PREFIX"
              # Validacion: cada objeto del prefijo debe existir en evidence con el mismo sha256.
              S() { AWS_ACCESS_KEY_ID="$SRC_ACCESS_KEY_ID" AWS_SECRET_ACCESS_KEY="$SRC_SECRET_ACCESS_KEY" AWS_CA_BUNDLE="$SRC_CA_BUNDLE" aws --endpoint-url "$SRC_ENDPOINT" "$@"; }
              D() { AWS_ACCESS_KEY_ID="$DST_ACCESS_KEY_ID" AWS_SECRET_ACCESS_KEY="$DST_SECRET_ACCESS_KEY" aws --endpoint-url "$DST_ENDPOINT" "$@"; }
              D s3 ls "s3://$DST_BUCKET/$RESTORE_PREFIX/" --recursive > /tmp/lista
              bad=0; n=0
              while read -r _ _ _ key; do
                k="${key#"$RESTORE_PREFIX"/}"
                a="$(D s3 cp "s3://$DST_BUCKET/$key" - | sha256sum)"
                b="$(S s3 cp "s3://evidence/$k" - | sha256sum)" || b=ausente
                n=$((n + 1))
                if [ "$a" != "$b" ]; then echo "DIFIERE $k"; bad=$((bad + 1)); fi
              done < /tmp/lista
              echo "validados=$n diferentes=$bad"
              [ "$bad" = 0 ] && [ "$n" -gt 0 ]
          env:
            - name: RESTORE_PREFIX
              value: PREFIJO_AQUI
            - name: AWS_DEFAULT_REGION
              value: us-east-1
            - name: STAGING_DIR
              value: /staging
            - name: SRC_ENDPOINT
              value: https://minio.aqs-system.svc:9000
            - name: SRC_CA_BUNDLE
              value: /etc/minio-ca/ca.crt
            - name: SRC_ACCESS_KEY_ID
              valueFrom: {secretKeyRef: {name: minio-root, key: root-user}}
            - name: SRC_SECRET_ACCESS_KEY
              valueFrom: {secretKeyRef: {name: minio-root, key: root-password}}
            - name: DST_ENDPOINT
              valueFrom: {secretKeyRef: {name: backup-target, key: endpoint}}
            - name: DST_BUCKET
              valueFrom: {secretKeyRef: {name: backup-target, key: bucket}}
            - name: DST_ACCESS_KEY_ID
              valueFrom: {secretKeyRef: {name: backup-target, key: access-key-id}}
            - name: DST_SECRET_ACCESS_KEY
              valueFrom: {secretKeyRef: {name: backup-target, key: secret-access-key}}
          volumeMounts:
            - {name: scripts, mountPath: /scripts, readOnly: true}
            - {name: ca, mountPath: /etc/minio-ca, readOnly: true}
            - {name: staging, mountPath: /staging}
      volumes:
        - name: scripts
          configMap:
            name: evidence-backup-scripts
        - name: ca
          secret:
            secretName: minio-tls
            items:
              - {key: ca.crt, path: ca.crt}
        - name: staging
          emptyDir:
            sizeLimit: 10Gi
```

`restore.sh` no borra objetos existentes en `evidence`: los objetos que ya estaban y no pertenecen al prefijo permanecen.

## Validacion

Criterio unico y no ambiguo: el Job de restore termina `Complete` (exit 0) y su ultima linea de log es `validados=N diferentes=0` con `N > 0`. Eso significa que **cada objeto del prefijo restaurado existe en `evidence` con el mismo sha256** (calculado por el propio Job con `aws s3 cp ... - | sha256sum`). El numero de objetos de `evidence` puede ser mayor que el del prefijo; no es un fallo.

Comprobaciones adicionales:

1. Cifrado del backup: `aws s3api head-object --bucket <bucket> --key <prefijo>/<objeto>` debe mostrar `"ServerSideEncryption": "AES256"`.
2. Tras el siguiente backup, `AqsBackupStale` y `AqsBackupFailed` no estan activas.

La prueba local equivalente es `bash scripts/test/backup-local.sh`.

## Failover

Si el cluster primario no esta disponible: levantar el cluster alterno con el mismo GitOps, crear los Secrets `minio-root`, `minio-kms`, `minio-tls` y `backup-target`, esperar `kubectl -n aqs-system wait --for=condition=complete job/minio-init` y ejecutar el procedimiento de restore con el ultimo prefijo. Registrar el cambio segun [gestion de cambios](gestion-de-cambios.md) y abrir un incidente segun [respuesta a incidentes](respuesta-a-incidentes.md).

## Failback

Con el cluster primario recuperado:

1. Congelar escrituras en el alterno: `kubectl -n aqs-system scale deployment -l aqs.io/tier=control-plane --replicas=0`.
2. Backup final: `kubectl -n aqs-system create job --from=cronjob/evidence-backup backup-failback` y esperar `kubectl -n aqs-system wait --for=condition=complete job/backup-failback`.
3. En el primario, restaurar ese ultimo prefijo con el procedimiento de arriba y validar con el mismo criterio.
4. Conmutar el trafico al primario y desmantelar el alterno.

## Pendientes

- Backup de la base de datos de la plataforma: pendiente de U2/U4.
- Destino fuera del cluster y gestion de credenciales: C-08.
