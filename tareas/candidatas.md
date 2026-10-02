# Tareas candidatas

Trabajo que apareció durante las rondas y que **nadie pidió todavía**. No forma parte de ninguna tarea en curso. El humano decide cuáles se convierten en tarea y cuándo.

| # | Origen | Candidata | Notas |
|---|---|---|---|
| C-01 | U5-T01 ronda 1 | Acotar por rutas cualquier criterio que cuente archivos de todo el repo | Ya aplicado en las tareas de las olas 2 y 3 |
| C-02 | U5-T03 (tarea) | Firmar imágenes con `cosign` en `publish` | |
| C-03 | U5-T03 ronda 1, F-04 | `no-latest --strict`: fallar si `deploy/flux/prod` no existe o no tiene `*.yaml` | `deploy/flux/prod` ya tiene manifiestos desde el PR #2 |
| C-04 | U5-T03 ronda 1 | Que el guard `no-latest` detecte `images: - newTag: latest` de Kustomize | |
| C-05 | U5-T03 ronda 2 | Que `detect-lang.sh` soporte servicios Go bajo `go.work` sin `go.mod` propio | |
| C-06 | U5-T03 ronda 2, F-07 | `npm test --if-present` pasa en verde si un `package.json` no tiene script `test`; usar `npm test` a secas | Anotada por decisión del humano |
| C-07 | U5-T04 ronda 1, F-02 | Definir el `GitRepository` `agentic-qa-swarm` y el namespace `flux-system` del bootstrap de Flux | Sin ellos, los `Kustomization` de Flux no sincronizan |
| C-08 | U5-T04 ronda 1, F-02; U5-T05; U5-T07 | Dueño de los Secrets referenciados y no creados (`warm-db-credentials`, `minio-root`, `minio-kms`, `minio-tls`, `grafana-admin`) y confirmación de los valores elegidos por los codificadores: `idleScaleDownAfter`, `minReplicasIdle`, schedules de los CronJobs, nombres de métricas `aqs_*` y umbrales de alertas | |
| C-09 | U5-T04, enmienda F-03 | Quitar los `env: TZ=UTC` que solo existían para el grep de CA-7 | El grep ya está enmendado en main |
| C-10 | U5-T02 ronda 1, F-03 | Acelerar `contracts/validate.sh` con una sola invocación de ajv (hoy tarda alrededor de 1 minuto) | |
| C-11 | U5-T02 ronda 1, F-04 | Alinear el contrato REST con el de eventos cuando U1 implemente el control plane | `Notification.artifact` ya está alineado |
| C-12 | Ola 3, redacción de U5-T06 | NetworkPolicy deny-by-default para `aqs-system` y `aqs-observability` | U5-T06 cubre solo `aqs-test` (test-ns-only, US-M8) |
| C-13 | U5-T06 ronda 1, bloqueo | **Aprobada por el humano (opción A):** mover los CronJobs `housekeeping` y `rebuild` de `aqs-test` a `aqs-system` (SA `go-reset`, que actúa sobre `aqs-test` vía el Role `aqs-test-operator`), para que `aqs-test` siga sin egress | Se redacta como tarea en la ola 4 |
| C-14 | U5-T05 ronda 1, F-01 | Derivar del manifiesto (con `yq`) el digest de MinIO y la imagen de `aws-cli` que usa `scripts/test/minio-local.sh`, para que CA-4 pruebe siempre la imagen real | |
| C-15 | U5-T05 ronda 1, F-04 | `securityContext` endurecido para MinIO y el Job `minio-init` (`allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`, `readOnlyRootFilesystem`) y etiquetas de Pod Security en los namespaces | |
| C-16 | U5-T05 ronda 1, F-03 | Estrategia para recrear `minio-init` cuando cambie el script o el spec (Job con hash en el nombre, `ttlSecondsAfterFinished`, o `force`/`wait` en la Kustomization de Flux) | |
| C-17 | U5-T05 ronda 1, F-02 y F-05 | Service headless para MinIO si escala a más de una réplica; decidir `storageClassName` y tamaño del PVC (hoy 10Gi) | |
| C-18 | U5-T05 ronda 1 | Documentar cómo se crean los Secrets de MinIO: SAN del certificado `minio-tls` (`minio.aqs-system.svc`) y formato de `MINIO_KMS_SECRET_KEY` (`nombre:base64` de 32 bytes) | Relacionada con C-08 |
| C-19 | U5-T06 ronda 1 | **Seguridad:** regla Rego que deniegue egress abierto sin `ipBlock` en `aqs-test` (`egress: [{}]` o `to: [{namespaceSelector: {}}]` hoy pasan las políticas) | Prioritaria: es el guardia del "sin egress a LLM" |
| C-20 | U5-T06 ronda 1, F-02 | Correr `conftest verify` y `conftest test --combine` (regla de conjunto `default-deny`) en CI | |
| C-21 | U5-T06 ronda 1, F-01 | Que la regla de `serviceAccountName` rechace también la cadena vacía | |
| C-22 | U5-T06 ronda 1 | Regla que impida que un Pod de `aqs-test` reactive `automountServiceAccountToken: true` a nivel de pod | Capa de políticas de U4 |
| C-23 | U5-T07 ronda 1, F-05 | Separar la observabilidad en dos Kustomizations de Flux con `dependsOn` (primero los charts, que instalan los CRDs; luego `ServiceMonitor` y `PrometheusRule`) | Riesgo no verificado sin clúster |
| C-24 | U5-T07 ronda 1, F-03 | Estándar para toda tarea con `HelmRelease`: un criterio que renderice el chart fijado con `helm template` y los `values` del manifiesto | Aplicar al redactar las próximas tareas |
| C-25 | U5-T07 ronda 2, F-06 | Desactivar o dimensionar los componentes por defecto de Loki 7.3.0 (`chunks-cache`, `results-cache`, `gateway`, `canary`), que pueden quedar `Pending` en un clúster pequeño | |
| C-26 | U5-T10 ronda 1, F-02 | Denegar las NetworkPolicy (y demás objetos) sin `metadata.namespace`, por si se introduce un transformador de namespace de kustomize; hoy `security.rego` y `egress.rego` no las evalúan | Sin hueco real hoy |
| C-27 | U5-T10 ronda 1, F-01 y F-03 | Pruebas unitarias en `egress_test.rego` para `endPort`, `SCTP`, puerto nombrado, peers mezclados, etiqueta extra y protocolo ausente; usar `not "ipBlock" in object.keys(peer)` | Hoy solo las cubre la matriz del revisor |
| C-28 | U5-T09 ronda 1, F-01 | Regla de aislamiento global: denegar un ServiceAccount de `aqs-test` como sujeto de un RoleBinding en **cualquier** namespace (`aqs-system`, `default`…), no solo en `aqs-test` | U5-T09 cubre los bindings en `aqs-test` |
| C-29 | U5-T09 ronda 1 | Revisar el Role `aqs-test-operator` cuando se implemente `go-reset`: subrecursos `deployments/scale` y `statefulsets/scale`, y `persistentvolumeclaims` si `rebuild` hace teardown completo | Imagen aún placeholder |
| C-30 | U5-T09 ronda 1, F-02 | Limpiar el resto de `aqs-reset` en `policy/security_test.rego:63` (`test_cronjob_with_sa_allowed`) | Solo dato de prueba |
