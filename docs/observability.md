# Observabilidad de la plataforma (U5-T07)

Contrato entre la plataforma y los servicios de U1-U4. Aqui solo se define; instrumentar es trabajo de cada unidad.

## Log estructurado

Una linea JSON por evento, con estos campos:

- `timestamp`: RFC 3339 en UTC.
- `request_id`: identificador de la peticion o del mensaje de entrada.
- `trace_id`: identificador de traza distribuida, propagado entre servicios.
- `level`: `debug`, `info`, `warn` o `error`.
- `message`: texto legible del evento.

## Metricas

Cada servicio de `aqs-system` expone `/metrics` en el puerto `http`.

| Metrica | Tipo | Significado |
|---|---|---|
| `aqs_reset_not_verified_total` | counter | Resets del warm que terminaron sin verificacion (KPI: debe ser 0). |
| `aqs_warm_quarantined` | gauge | 1 si el warm esta en cuarentena (evento `warm.quarantined`), 0 si no. |

Requisito: `aqs_reset_not_verified_total` se debe publicar en 0 al arrancar el servicio (U2). Sin la serie inicializada, `increase(...)` no detecta el primer incremento si la serie aparece ya en 1.

La alerta `AqsControlPlaneDown` usa la metrica estandar `up`.

## Retencion

Prometheus 90d; Loki 2160h (90 dias) con compactor activo.

## Almacenamiento persistente (sin `storageClassName`, pendiente C-17)

- Prometheus: PVC de 50Gi para 90d de retencion (dimensionado a ojo; revisar con la carga real).
- Loki: PVC de 20Gi (`singleBinary.persistence.size`).
- Grafana: PVC de 5Gi.
