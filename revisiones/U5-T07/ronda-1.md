# Ronda 1 — U5-T07

VEREDICTO: NO-VERDE

Hash de la tarea verificado: `a33f37e14ef5028caad8463523ef8c483be9b4ad` (coincide). Worktree `wt-U5-T07` en `fa23748`, limpio antes y después de la revisión (`git status --short | wc -l` = 0). Todas mis copias y renders están en el scratchpad (`.../scratchpad/rev1`), fuera del worktree. No usé kubectl, flux, helm install ni apply. Para contrastar los valores usé `helm template` local (alpine/helm 3.16.2) sobre los tgz reales de los charts, sin clúster.

## Criterios de aceptación, verificados por mí (comandos literales de la tarea, alias `K` e `Y`, CA-4 enmendado y sin `--user root`)
| # | Criterio | Resultado |
|---|---|---|
| 1 | CA-1 objetos de observabilidad en el build | pasa: las 8 líneas esperadas |
| 2 | CA-2 kubeconform estricto dev y prod | pasa: dos líneas `Valid: 34, Invalid: 0, Errors: 0, Skipped: 0` |
| 3 | CA-3 retención | pasa: `90d`, `2160h`, `true` |
| 4 | CA-4 promtool, severidad, summary | pasa: `SUCCESS: 3 rules found`, 3, 0, 3 |
| 5 | CA-5 ServiceMonitor | pasa: `aqs-system go-governance,go-identity,go-intake,go-reset,go-run-controller,go-warm-manager,ui-api http/metrics` |
| 6 | CA-6 versiones y dashboard | pasa: 2, true, 1 |
| 7 | CA-7 doc | pasa: 5 y solo `FIN` |
| 8 | CA-8 kustomization base | pasa: `1\t0\t...` y `  - observability` |
| 9 | CA-9 árbol limpio | pasa: 0 |

Pruebas negativas sobre copias temporales:
- Alerta sin `severity`: el conteo de incompletas da 1.
- PromQL roto (`up{ == 0`): `promtool` falla con `parse error: unexpected "=" in label matching`, rc=1.
- Retención en `15d`: el primer valor pasa a `15d`.

Alcance "Fuera": el diff toca solo `deploy/flux/base/observability/*`, `docs/observability.md`, la bitácora y `deploy/flux/base/kustomization.yaml` (+1 línea, la permitida). No hay Secrets ni credenciales literales. `grafana-admin` se referencia y el Secret no se crea; el dueño del Secret ya está en la candidata C-08.

La evidencia de la bitácora de la ronda 2 coincide con mis salidas. No hay desborde.

Los 9 criterios pasan, pero el Loki del HelmRelease no se puede instalar (F-01) y la retención de Prometheus no persiste (F-02). Por eso el veredicto es NO-VERDE.

## Puntos que me pediste juzgar
1. **Versiones de chart:** correctas. Con el `index.yaml` real descargué las dos entradas:
   - `kube-prometheus-stack` 91.8.2 existe y es la primera del índice.
   - `loki` 7.3.0 existe y es del chart `loki` (appVersion 3.6.12).
   - En grafana, `7.3.0` aparece 2 veces porque también lo tiene el chart `grafana`. La bitácora lo explica y la entrada de `loki` es la correcta.
2. **Values de kube-prometheus-stack:** correctos. `helm template` con esos values sobre el tgz 91.8.2 renderiza 125 objetos sin error y deja esto en el CR `Prometheus`:
   - `retention: "90d"`, `serviceMonitorSelector: {}`, `serviceMonitorNamespaceSelector: {}`, `ruleSelector: {}` y `ruleNamespaceSelector: {}`.
   - Con `*NilUsesHelmValues: false`, el ServiceMonitor y la regla en `aqs-observability` se descubrirán.
   - Grafana usa `grafana-admin` con las claves `admin-user` y `admin-password`, y el sidecar queda con `LABEL=grafana_dashboard`, `LABEL_VALUE=1` y `NAMESPACE=ALL`.
   - **Values de Loki: no se instalan** (F-01). Las claves `loki.limits_config.retention_period`, `loki.compactor.retention_enabled` y `loki.compactor.delete_request_store` sí son las que lee el chart, pero faltan otras obligatorias.
3. **Reglas y alertas:** las expresiones tienen sentido.
   - `increase(aqs_reset_not_verified_total[15m]) > 0` sin `for` es razonable para un KPI "debe ser 0". Ver F-04.
   - Las tres métricas (`up` y las dos `aqs_*`) están en la doc.
   - Los 7 Services de `aqs-system` tienen `app.kubernetes.io/name` en sus labels y el puerto `http`.
   - El formato de log tiene `timestamp`, `request_id`, `trace_id`, `level` y `message`.
4. **Dashboard:** usa solo `up{namespace="aqs-system"}`, `aqs_reset_not_verified_total` y `aqs_warm_quarantined`, todas del contrato. No hay métricas inventadas.
5. **Secrets:** ninguno creado, y no hay credencial literal.

## Hallazgos

### F-01 · NARANJA · `deploy/flux/base/observability/helmreleases.yaml:46-57` · El HelmRelease `loki` no renderiza: Loki no se instalaría y la retención de 2160h nunca entraría en vigor
Los values nuevos son los de la tarea, pero no bastan para el chart 7.3.0. Lo comprobé sobre el tgz real con los values literales del HelmRelease:
```
helm template loki ./loki -f vals.yaml
Error: execution error at (loki/templates/single-binary/statefulset.yaml:44:28): Please define loki.storage.bucketNames.chunks
```
Fui añadiendo claves hasta que renderizó. Cada error es una clave que falta en el HelmRelease:
- Con `loki.storage.type: filesystem` aparece `validate.yaml:19: Cannot run scalable targets (backend, read, write) ... without an object storage backend`.
  - Hace falta `backend.replicas: 0`, `read.replicas: 0` y `write.replicas: 0`. Es lo que hace `single-binary-values.yaml` del propio chart.
  - Con `deploymentMode: SingleBinary` el default de `singleBinary.replicas` es 0 en este chart, así que hace falta `singleBinary.replicas: 1`.
- Sin `loki.schemaConfig` falla `validate.yaml:40` ("You must provide a schema_config for Loki").
- `loki.commonConfig.replication_factor` por defecto es 3, y con un solo binario debe ser 1.

Con estas claves añadidas (`loki.storage.type: filesystem`, `loki.schemaConfig` tsdb/v13/filesystem, `replication_factor: 1`, `singleBinary.replicas: 1` y `backend`, `read` y `write` en 0) el chart renderiza 20 objetos. `retention_period: 2160h`, `retention_enabled: true` y `delete_request_store: filesystem` aparecen en el config y el StatefulSet trae `volumeClaimTemplates`. Mis values de prueba están en `.../scratchpad/rev1/vals3.yaml`.

Los CA se cumplen a nivel YAML, pero eso no prueba que el release funcione, y la bitácora no registra ningún render del chart. Lo dejo en NARANJA y no en ROJO porque ningún CA literal falla. Aun así bloquea: el objetivo de la tarea (Loki con retención 90d) no se cumple. Falta además un criterio que lo detecte, y eso es una brecha de la especificación.

### F-02 · NARANJA · `deploy/flux/base/observability/helmreleases.yaml:15-17` · `retention: 90d` en Prometheus sin almacenamiento persistente
El render del chart deja el CR `Prometheus` sin `storage` (0 coincidencias de "storage" en el spec). Prometheus usa un `emptyDir`, y Grafana también (`emptyDir: {}` en el render). Los datos se pierden al reiniciar el pod, así que la retención de 90d es solo nominal y no cumple "retención ≥ 90 días" (US-M10, NF-SEG-14).
Falta `prometheus.prometheusSpec.storageSpec.volumeClaimTemplate`. Loki sí tendrá PVC una vez corregido F-01.

### F-03 · AMARILLO · Cobertura: ningún criterio ejecuta un render de los charts
CA-2 valida con kubeconform solo los CRDs de Flux y de Prometheus Operator, no los `values`. Por eso F-01 pasó los 9 criterios. Se recomienda una prueba con `helm template` sobre el tgz fijado, o una nota explícita en la tarea de que los `values` quedan sin verificar.

### F-04 · AMARILLO · `docs/observability.md` · Falta el requisito de inicializar el counter
`increase(aqs_reset_not_verified_total[15m]) > 0` no detecta el primer incremento si la serie aparece ya en 1. La doc, que es el contrato para U2, debería exigir que el counter se publique en 0 al arrancar. Además `AqsControlPlaneDown` (`up == 0`) no cubre un servicio que desaparezca por completo (sin series, solo con `absent()`).

### F-05 · AMARILLO · Orden de CRDs en Flux (riesgo no verificado)
El `ServiceMonitor` y el `PrometheusRule` están en la misma Kustomization que el `HelmRelease` que instala sus CRDs. Con kustomize-controller esto suele fallar en el primer dry-run ("no matches for kind") hasta que existan los CRDs. No lo pude comprobar sin clúster. Anótalo como tarea candidata: separar en dos Kustomizations con `dependsOn`.

## Tareas candidatas
- Dos Kustomizations de Flux con `dependsOn` (charts, luego CRs) para observabilidad (F-05).
- Convención de `storageClass` y tamaños de PVC para Prometheus, Loki y Grafana, si el humano prefiere no resolver F-02 aquí.
- Criterio de verificación con `helm template` del chart fijado para toda tarea con `HelmRelease` (F-03).
- C-08 ya cubre el dueño del Secret `grafana-admin` y la confirmación de nombres `aqs_*` y umbrales.

## Rutas
- Charts descargados y renders: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/4ed52fd5-a142-46f0-ac9b-995972cfe97d/scratchpad/rev1/` (`vals.yaml`, `vals3.yaml`, `kr.yaml`, `r3.yaml`).
- Worktree revisado, intacto: `/home/omarjayg/Javeriana/topicos-especiales/wt-U5-T07`.

VEREDICTO: NO-VERDE
NARANJA|deploy/flux/base/observability/helmreleases.yaml:46-57|El HelmRelease loki no renderiza (faltan storage, schemaConfig, replicas de backend/read/write y replication_factor)
NARANJA|deploy/flux/base/observability/helmreleases.yaml:15-17|retention 90d en Prometheus sin storageSpec persistente (emptyDir)
INFORME: revisiones/U5-T07/ronda-1.md
