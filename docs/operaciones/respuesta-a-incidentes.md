# Respuesta a incidentes

## Alerta

Las alertas de Prometheus llegan con `severity` y `summary`. Las de backup: `AqsBackupFailed` y `AqsBackupStale`. Quien recibe la alerta la reconoce y abre un registro de incidente con hora de inicio y severidad.

## Runbook

1. Contener: evitar mas dano; si el backup corrompe datos, suspender el CronJob: `kubectl -n aqs-system patch cronjob evidence-backup -p '{"spec":{"suspend":true}}'` (reanudar con `false`).
2. Diagnosticar: `kubectl -n aqs-system get jobs -l app.kubernetes.io/name=evidence-backup`, luego `kubectl -n aqs-system logs job/<nombre-del-job>`; revisar que existe el Secret `backup-target` (`kubectl -n aqs-system get secret backup-target`) y la conectividad al destino y a MinIO.
3. Recuperar: reintentar el Job o ejecutar el [runbook de restore](runbook-restore.md).
4. Verificar: `AqsBackupFailed` se apaga cuando un backup posterior termina bien (aunque el Job fallido siga en el historial); `AqsBackupStale` se apaga con el primer backup exitoso. En una instalacion nueva `AqsBackupStale` salta hasta ese primer backup.
5. Cerrar y abrir el post-mortem.

## Post-mortem y COE

Todo incidente de severidad critical lleva un post-mortem sin culpas (COE, correction of errors) en los 5 dias habiles siguientes, con acciones correctivas con responsable y fecha.

## Plantilla

```markdown
# COE: <titulo>
- Fecha y duracion:
- Impacto:
- Linea de tiempo (UTC):
- Causa raiz (5 porques):
- Que funciono / que no:
- Acciones correctivas (responsable, fecha):
```
