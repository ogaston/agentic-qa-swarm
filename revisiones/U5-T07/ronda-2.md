# Ronda 2 — U5-T07

VEREDICTO: VERDE

Hash de la tarea verificado: `a33f37e14ef5028caad8463523ef8c483be9b4ad` (coincide). Worktree `wt-U5-T07` en `0f55dc2`, limpio antes y después de la revisión (`git status --short | wc -l` = 0). Todas mis copias y renders están en el scratchpad (`.../scratchpad/rev2`), fuera del worktree. No usé kubectl, flux, helm install ni apply. Solo usé `helm template` local (alpine/helm 3.16.2).

## Criterios de aceptación, verificados por mí (comandos literales, alias `K` e `Y`, CA-4 con `chmod 755`)
| # | Criterio | Resultado |
|---|---|---|
| 1 | CA-1 | pasa: las 8 líneas esperadas |
| 2 | CA-2 kubeconform dev y prod | pasa: dos líneas `Valid: 34, Invalid: 0, Errors: 0, Skipped: 0` |
| 3 | CA-3 | pasa: `90d`, `2160h`, `true` |
| 4 | CA-4 | pasa: `SUCCESS: 3 rules found`, 3, 0, 3 |
| 5 | CA-5 | pasa: `aqs-system go-governance,go-identity,go-intake,go-reset,go-run-controller,go-warm-manager,ui-api http/metrics` |
| 6 | CA-6 | pasa: 2, true, 1 |
| 7 | CA-7 | pasa: 5 y solo `FIN` |
| 8 | CA-8 | pasa: `1\t0\tdeploy/flux/base/kustomization.yaml` y `  - observability` |
| 9 | CA-9 | pasa: 0 |

Pruebas negativas sobre copias temporales, sin dejar nada en el worktree:
- Retención en `15d`: el primer valor pasa a `15d`.
- Alerta sin `labels.severity`: el conteo de alertas incompletas da 1.
- PromQL roto (`up{ == 0`): `promtool` falla con `parse error: unexpected "=" in label matching`, rc=1.

## Verificación por hallazgo de la ronda 1
**Render de los charts.** Extraje los `values` de los HelmRelease actuales con `yq` y comprobé que son semánticamente idénticos a los del `kustomize build prod`. Solo cambia el orden de claves. Rendericé sobre los tgz de las versiones fijadas, que son los mismos de la ronda 1 y del codificador (sha256 idénticos). Renders en `rev2/kps-render.yaml` y `rev2/loki-render.yaml`.

- **F-01 · RESUELTO.** `helm template` de Loki 7.3.0 termina con rc=0 y 20 objetos. En el render están:
  - `replication_factor: 1`, `retention_period: 2160h`, `retention_enabled: true` y `delete_request_store: filesystem`.
  - `StatefulSet/loki` con `replicas: 1`, `volumeClaimTemplates` `storage` de 20Gi y el volumen montado en `/var/loki`.
  - No hay StatefulSet ni Deployment de backend, read o write.
  - Ya no aparece ninguno de los tres errores de la ronda 1.
- **F-02 · RESUELTO.** `helm template` de kube-prometheus-stack 91.8.2 termina con rc=0 y 126 objetos.
  - El CR `Prometheus` tiene `retention: "90d"` y `storage.volumeClaimTemplate` con `ReadWriteOnce` y 50Gi. En la ronda 1 no tenía `storage`, así que Prometheus ya no depende de `emptyDir` para los datos.
  - Los `serviceMonitorNamespaceSelector` y `ruleNamespaceSelector` siguen en `{}`.
  - Grafana trae `PersistentVolumeClaim/kps-grafana` de 5Gi `ReadWriteOnce`, y el volumen `storage` es un `persistentVolumeClaim`, ya no `emptyDir`.
  - No hay `storageClassName`: queda en la candidata C-17, como arbitró el orquestador.
- **F-03 · RESUELTO.** La bitácora (sección "Ronda 3") registra los comandos de descarga, extracción y `helm template`, y los conteos de objetos (126 y 20). Coincide con mi render. No se cambiaron los CA.
- **F-04 · RESUELTO.** `docs/observability.md` exige publicar `aqs_reset_not_verified_total` en 0 al arrancar y explica por qué. La alerta `absent()` era opcional y no se añadió.
- **F-05 · No es defecto.** Queda como candidata C-23, sin cambios en el diff.

## Alcance y desborde
`git diff fa23748..tarea/U5-T07` toca solo `helmreleases.yaml`, `docs/observability.md` y la bitácora. El diff completo contra la base sigue acotado a `deploy/flux/base/observability/*`, `docs/observability.md`, la bitácora y +1 línea en `deploy/flux/base/kustomization.yaml`. No hay Secrets nuevos ni desborde.

## Hallazgos nuevos
- F-06 · AMARILLO · `deploy/flux/base/observability/helmreleases.yaml` (Loki) · No bloquea. El chart de Loki 7.3.0 renderiza por defecto dos StatefulSet de memcached (`loki-chunks-cache` y `loki-results-cache`), `loki-gateway` y `loki-canary`, con sus requests de recursos por defecto. En un clúster pequeño pueden quedar `Pending`. Es una decisión de dimensionamiento fuera del alcance de la tarea.

## Tareas candidatas
- Desactivar o dimensionar chunks-cache, results-cache, gateway y canary de Loki para el entorno warm pequeño (F-06).
- Se mantienen C-17 (`storageClassName` y tamaños de PVC, que son «a ojo») y C-23 (dos Kustomizations con `dependsOn`).

## Rutas
- Renders y values: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/4ed52fd5-a142-46f0-ac9b-995972cfe97d/scratchpad/rev2/`
- Worktree revisado, intacto: `/home/omarjayg/Javeriana/topicos-especiales/wt-U5-T07`

VEREDICTO: VERDE
AMARILLO|deploy/flux/base/observability/helmreleases.yaml (Loki)|Memcached de caché, gateway y canary de Loki con recursos por defecto (candidata)
INFORME: revisiones/U5-T07/ronda-2.md
