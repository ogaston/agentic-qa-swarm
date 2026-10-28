# Verificación independiente de la corrida sana

Comprobado el 2026-10-28T15:10:48Z mediante consultas de lectura a las APIs locales y Kubernetes. No se lanzó ni confirmó una corrida ni se ejecutó un reset.

## Identidad

- Corrida: `run-d3fdad3c9118b08a`.
- Confirmación más reciente del usuario: `2026-10-28T15:07:07Z`.
- Notificación: `n-49ac8baa-8885-4d6d-a70a-d77a1bbab148`.
- Repositorio: `ogaston/demo-target-app-qa-swarm`.
- SHA: `460b66b36e0db2ad12e7a7437fbfa43039541692`.
- Trace: `0f9977d38f312f6716dd2b512c9e1fbc`.

## Resultado

`GET /runs/run-d3fdad3c9118b08a` devuelve `state=done`, `verdict=sin-hallazgos` y referencia al reporte:

`s3://evidence/runs/run-d3fdad3c9118b08a/report-d64e279670f8170c31637e0643fedc72de2fe8b57f8e760658a2f03216640e5e.json`

`GET /reports/run-d3fdad3c9118b08a` devuelve el mismo veredicto y `findings=[]`. Su resumen indica que todas las pruebas reportan `ok:true` con los estados esperados y no muestran violaciones de invariantes en los datos proporcionados.

Los logs de los siete runners corroboran individualmente:

| Flujo | HTTP observado | ok |
|---|---:|---|
| health-check | 200 | true |
| list-products | 200 | true |
| list-orders | 200 | true |
| create-product | 201 | true |
| create-order | 201 | true |
| order-nonexistent-product | 404 | true |
| order-qty-exceeds-stock | 409 | true |

Los HTTP 404 y 409 son los resultados esperados de los dos casos negativos; no son fallos de la corrida. Ocho Jobs de esta corrida completados: un ensayo y siete runners, todos `succeeded=1`, sin intentos fallidos registrados en sus estados.

## Imagen y limpieza

- Imagen actual de `deployment/warm-app`: `ghcr.io/ogaston/demo-target-app-qa-swarm:460b66b36e0db2ad12e7a7437fbfa43039541692`, coincide con el SHA confirmado.
- Deployment warm: una réplica actualizada, disponible y lista.
- Variable del contenedor consultada de forma acotada: `TARGET_APP_BUG=off` (valor por defecto si no está definida).
- `GET /warm`: `state=ready`, `reset_verified=true`, `baseline_version=v1`.
- Pods de plataforma sin fases problemáticas ni contenedores en espera al consultar.

## Conclusión y límites

La corrida sana revisada completó el ciclo y su resultado concuerda con los siete resultados observados. El warm quedó listo con reset verificado. Esta revisión no ejecuta ni demuestra el caso con el defecto `oversell`; tampoco es una auditoría de toda la aplicación ni una verificación nueva de todos los objetos almacenados en MinIO. El reporte se leyó por ui-api y los resultados se contrastaron con los logs de Kubernetes, sin imprimir credenciales.
