# U5-T07 — Observabilidad base (métricas, logs, alertas, dashboard, retención ≥ 90 días)

**Unidad:** U5 — Plataforma & GitOps
**Historias que implementa:** US-M10
**Depende de:** U5-T04 (namespaces y Services del control plane). Ola 3, en paralelo con U5-T05 y U5-T06.

---

## Alcance

**Dentro** (una línea, concreta):

> Crear `deploy/flux/base/observability/` con su propio `kustomization.yaml`, que contenga:
> - El namespace `aqs-observability`.
> - Los `HelmRepository` `prometheus-community` y `grafana`.
> - Los `HelmRelease` `kube-prometheus-stack` y `loki`, con versión de chart exacta. La retención es de `90d` en Prometheus y de `2160h` en Loki, con el compactor activado.
> - Un `ServiceMonitor` `aqs-control-plane` que seleccione los 7 Services de `aqs-system` por `app.kubernetes.io/name`, en el puerto `http` y la ruta `/metrics`.
> - Un `PrometheusRule` `aqs-rules`, con al menos las alertas `AqsControlPlaneDown`, `AqsResetNotVerified` y `AqsWarmQuarantined`.
> - Un `ConfigMap` `aqs-dashboard` con la etiqueta `grafana_dashboard: "1"` y un dashboard JSON.
>
> Escribir también `docs/observability.md`, con el formato de log estructurado (`timestamp`, `request_id`, `trace_id`, `level`, `message`) y el contrato de métricas `aqs_*` que usan las alertas y el dashboard.
>
> En `deploy/flux/base/kustomization.yaml`, añadir `- observability` justo **después** de `- control-plane.yaml`.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Instrumentar servicios: que emitan métricas o logs es trabajo de U1–U4. Aquí solo se define el contrato.
- Modificar `control-plane.yaml`, `warm.yaml`, `namespaces.yaml` o los overlays. El ServiceMonitor selecciona por las etiquetas que ya existen.
- NetworkPolicy o RBAC (**U5-T06**, candidata C-12), MinIO (**U5-T05**) y backups (**U5-T08**).
- Secrets (por ejemplo la contraseña de Grafana): solo se referencian, no se crean.
- En `deploy/flux/base/kustomization.yaml`, cualquier cambio distinto de añadir `- observability` después de `- control-plane.yaml`.
- Cualquier `kubectl`, `flux`, `helm install` o `apply` contra un clúster.

---

## Archivos de contexto

- `unidades-y-tareas.md`
- `aidlc-docs/inception/application-design/unit-task-plans/U5.md`
- `aidlc-docs/inception/requirements/requirements.md` (KPIs: reset verificado 100%, warm en cuarentena; NF de observabilidad, retención y trazado)
- `aidlc-docs/inception/application-design/services.md` (servicios y eventos `reset.verified`, `warm.quarantined`)
- `deploy/flux/base/control-plane.yaml` (etiquetas y puertos de los Services)
- `tareas/candidatas.md`

---

## Criterios de aceptación

Desde la raíz del worktree. Se usan estos alias:

```bash
K='docker run --rm --security-opt label=disable -v '"$PWD"':/w -w /w registry.k8s.io/kustomize/kustomize:v5.4.3'
Y='docker run --rm -i --security-opt label=disable mikefarah/yq:4.44.3 -N'
```

- [ ] **CA-1** — Los objetos de observabilidad están en el build.
  ```bash
  $K build deploy/flux/prod | $Y 'select(.metadata.namespace == "aqs-observability" or (.kind == "Namespace" and .metadata.name == "aqs-observability")) | .kind + "/" + .metadata.name' | sort
  ```
  Esperado, exactamente:
  ```
  ConfigMap/aqs-dashboard
  HelmRelease/kube-prometheus-stack
  HelmRelease/loki
  HelmRepository/grafana
  HelmRepository/prometheus-community
  Namespace/aqs-observability
  PrometheusRule/aqs-rules
  ServiceMonitor/aqs-control-plane
  ```
  Antes de la tarea: ninguna línea (rojo inicial).

- [ ] **CA-2** — `kubeconform` estricto con los CRDs de Flux y de Prometheus Operator, sin recursos omitidos.
  ```bash
  for e in dev prod; do $K build deploy/flux/$e | docker run --rm -i ghcr.io/yannh/kubeconform:v0.6.7 -strict -summary -schema-location default -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json' -; done
  ```
  Esperado: dos líneas con `Invalid: 0, Errors: 0, Skipped: 0`.

- [ ] **CA-3** — Retención configurada, visible en los manifiestos.
  ```bash
  $K build deploy/flux/prod | $Y 'select(.kind == "HelmRelease" and .metadata.name == "kube-prometheus-stack") | .spec.values.prometheus.prometheusSpec.retention'
  $K build deploy/flux/prod | $Y 'select(.kind == "HelmRelease" and .metadata.name == "loki") | .spec.values.loki.limits_config.retention_period'
  $K build deploy/flux/prod | $Y 'select(.kind == "HelmRelease" and .metadata.name == "loki") | .spec.values.loki.compactor.retention_enabled'
  ```
  Esperado: `90d`, `2160h` y `true`.

- [ ] **CA-4** — Las reglas pasan `promtool` y cada alerta tiene severidad y resumen.
  ```bash
  t=$(mktemp -d); $K build deploy/flux/prod | $Y 'select(.kind == "PrometheusRule" and .metadata.name == "aqs-rules") | .spec' > "$t/rules.yaml"
  docker run --rm --security-opt label=disable -v "$t":/r -w /r --entrypoint promtool prom/prometheus:v2.55.1 check rules rules.yaml
  $Y '[.groups[].rules[] | select(has("alert"))] | length' < "$t/rules.yaml"
  $Y '.groups[].rules[] | select(has("alert")) | select((.labels.severity // "") == "" or (.annotations.summary // "") == "") | .alert' < "$t/rules.yaml" | wc -l
  $Y '.groups[].rules[] | select(has("alert")) | .alert' < "$t/rules.yaml" | grep -c -E '^(AqsControlPlaneDown|AqsResetNotVerified|AqsWarmQuarantined)$'
  rm -rf "$t"
  ```
  Esperado: `SUCCESS: N rules found` con N ≥ 3, luego un número ≥ 3, `0` y `3`.

- [ ] **CA-5** — El ServiceMonitor apunta a los 7 servicios de `aqs-system`.
  ```bash
  $K build deploy/flux/prod | $Y 'select(.kind == "ServiceMonitor" and .metadata.name == "aqs-control-plane") | .spec.namespaceSelector.matchNames[0] + " " + (.spec.selector.matchExpressions[0].values | sort | join(",")) + " " + .spec.endpoints[0].port + .spec.endpoints[0].path'
  ```
  Esperado: `aqs-system go-governance,go-identity,go-intake,go-reset,go-run-controller,go-warm-manager,ui-api http/metrics`.

- [ ] **CA-6** — Versiones de chart exactas y dashboard válido.
  ```bash
  $K build deploy/flux/prod | $Y 'select(.kind == "HelmRelease") | .spec.chart.spec.version' | grep -c -E '^[0-9]+\.[0-9]+\.[0-9]+$'
  $K build deploy/flux/prod | $Y 'select(.kind == "ConfigMap" and .metadata.name == "aqs-dashboard") | .data | to_entries | .[0].value' | jq -e '(.panels | length) >= 3' 
  $K build deploy/flux/prod | $Y 'select(.kind == "ConfigMap" and .metadata.name == "aqs-dashboard") | .metadata.labels.grafana_dashboard'
  ```
  Esperado: `2`, `true` y `1`.

- [ ] **CA-7** — El contrato de logs y métricas está documentado, y cada métrica `aqs_*` de las alertas aparece en la doc.
  ```bash
  grep -c -E '`(timestamp|request_id|trace_id|level|message)`' docs/observability.md
  for m in $($K build deploy/flux/prod | $Y 'select(.kind == "PrometheusRule") | .spec' | grep -o -E 'aqs_[a-z0-9_]+' | sort -u); do grep -q -- "$m" docs/observability.md || echo "FALTA $m"; done; echo FIN
  ```
  Esperado: un número ≥ 5 y solo `FIN`.

- [ ] **CA-8** — Solo se añadió la entrada permitida en `kustomization.yaml`, en su posición.
  ```bash
  git diff --numstat $(git merge-base HEAD origin/main) -- deploy/flux/base/kustomization.yaml; grep -A1 -E '^  - control-plane.yaml$' deploy/flux/base/kustomization.yaml | tail -1
  ```
  Esperado: `1\t0\tdeploy/flux/base/kustomization.yaml` y `  - observability`.

- [ ] **CA-9** — Árbol limpio tras el commit.
  ```bash
  git status --short | wc -l
  ```
  Esperado: `0`.

---

## Plan de pruebas

- Rojo inicial: el comando literal de CA-1 sobre la base no lista nada.
- Prueba negativa de CA-4 sobre una copia temporal: una alerta sin `labels.severity` hace que el conteo de alertas incompletas dé `1`, y una expresión PromQL rota hace fallar a `promtool`.
- Prueba negativa de CA-3: con la retención en `15d`, el primer valor deja de ser `90d`.

**Rojo primero:** el codificador registra en su bitácora la salida del comando literal de CA-1 antes de crear nada.

---

## Notas

- Las versiones de chart se toman del índice real del repositorio de Helm. Se registra en la bitácora el comando con que se comprobó que existen (por ejemplo, `curl -s <repo>/index.yaml | grep -c 'version: <v>'`). No se inventan versiones.
- El ServiceMonitor necesita que kube-prometheus-stack lo descubra en otros namespaces: se configura en los `values`, sin tocar los Services.
- La bitácora pega el **comando literal** de cada criterio y su salida. El CA-9 posterior al último commit va en el informe de vuelta, con una nota en la bitácora que lo diga.
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
