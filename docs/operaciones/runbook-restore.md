# Runbook de backup y restore de evidencias

Alcance: bucket `evidence` de MinIO (namespace `aqs-system`). Fuera de alcance: `warm-db` (desechable, se resetea y reconstruye) y la base de datos de la plataforma (pendiente de U2/U4, aun no existe en los manifiestos; candidata registrada).

## Como funciona el backup

- CronJob `evidence-backup` (`deploy/flux/base/backup/`), schedule `0 */6 * * *`, `concurrencyPolicy: Forbid`.
- `backup.sh` copia `s3://evidence` a `s3://<bucket>/<YYYY-MM-DDTHH>/` (UTC) con `--sse AES256` y borra los prefijos de fecha con mas de 30 dias. Los prefijos sin formato de fecha (p. ej. `manual/`) no se tocan.
- El destino se lee del Secret `backup-target` (claves `endpoint`, `bucket`, `access-key-id`, `secret-access-key`), referenciado y no creado en el repo. Debe estar **fuera del cluster** (quien lo provee: candidata C-08).
- Alertas: `AqsBackupFailed` (un Job del CronJob fallo) y `AqsBackupStale` (mas de 8 h sin backup exitoso).

## RTO y RPO

### RPO

- RPO: 6 h o menos (un backup cada 6 h; `AqsBackupStale` alerta a las 8 h).
### RTO

- RTO: objetivo de 4 h para restaurar `evidence` desde el ultimo prefijo (valor propuesto, pendiente de confirmar en C-08).

## Procedimiento de restore

1. Elegir el prefijo: listar el destino (`aws s3 ls s3://<bucket>/`) y tomar el mas reciente valido.
2. Asegurar que MinIO y el bucket `evidence` existen (Job `minio-init`).
3. Ejecutar `restore.sh <YYYY-MM-DDTHH>` con las mismas variables `SRC_*` y `DST_*` del CronJob `evidence-backup`; por ejemplo, un Job de un solo uso con la misma imagen y el ConfigMap `evidence-backup`. El script no borra objetos existentes.

## Validacion

1. Comparar el numero de objetos: `aws s3 ls s3://<bucket>/<prefijo>/ --recursive | wc -l` contra `aws s3 ls s3://evidence --recursive | wc -l`.
2. Comparar checksums de una muestra de objetos entre destino y `evidence`.
3. Leer `ServerSideEncryption` con `aws s3api head-object`: debe ser `AES256`.
4. Confirmar que `AqsBackupStale` y `AqsBackupFailed` no estan activas tras el siguiente backup.

La prueba local equivalente es `bash scripts/test/backup-local.sh`.

## Failover

Si el cluster primario no esta disponible: levantar el cluster alterno con el mismo GitOps, crear los Secrets (`minio-*`, `backup-target`), esperar `minio-init` y ejecutar el procedimiento de restore con el ultimo prefijo. Registrar el cambio segun [gestion de cambios](gestion-de-cambios.md) y abrir un incidente segun [respuesta a incidentes](respuesta-a-incidentes.md).

## Failback

Con el cluster primario recuperado: congelar escrituras en el alterno, hacer un backup final (`evidence-backup`), restaurar ese prefijo en el primario, validar como arriba, conmutar el trafico y desmantelar el alterno.

## Pendientes

- Backup de la base de datos de la plataforma: pendiente de U2/U4.
- Destino fuera del cluster y gestion de credenciales: C-08.
