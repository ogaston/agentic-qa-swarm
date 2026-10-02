# Respuesta a incidentes

## Alerta

Las alertas de Prometheus llegan con `severity` y `summary`. Las de backup: `AqsBackupFailed` y `AqsBackupStale`. Quien recibe la alerta la reconoce y abre un registro de incidente con hora de inicio y severidad.

## Runbook

1. Contener: evitar mas dano (suspender el CronJob si corrompe datos).
2. Diagnosticar: `kubectl logs` del Job fallido, estado del Secret `backup-target`, conectividad al destino y a MinIO.
3. Recuperar: reintentar el Job o ejecutar el [runbook de restore](runbook-restore.md).
4. Verificar: la alerta se resuelve tras el siguiente backup exitoso.
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
