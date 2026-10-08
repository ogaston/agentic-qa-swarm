# Ronda 1 — U2-T07

VEREDICTO: NO-VERDE

Hay dos NARANJA, que corrigen manifiestos pequeños. No hay ROJO. CA-1 a CA-6 y CA-8 pasan en mi ejecución, y el CA-7 está recortado, así que no aplica. El worktree quedó limpio (`git status` = 0 líneas). Mis mutaciones las hice en copias dentro del scratchpad. El diff contra el merge-base toca solo `deploy/flux/base/`, la bitácora y `observability/`, con cero archivos Go.

## Criterios de aceptación, verificados por mí
Usé el build real de kustomize v5.4.3 de `deploy/flux/dev` y `deploy/flux/prod`, leído con `yq` mikefarah 4.44.3.

| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Políticas | `bash scripts/ci/policies.sh \| grep -E '^(OK\|FALLA)'` | pasa: 11 `OK` (kubeconform, conftest-test, conftest-combine y rbac-matrix para dev y prod; conftest-verify; promtool-rules; check-secrets) y 0 `FALLA` |
| 2 | Cuota y LimitRange | `yq ... ResourceQuota ... keys\|sort\|join` | pasa: `count/jobs.batch,limits.cpu,limits.memory,pods,requests.cpu,requests.memory` |
| 2b | Segunda expresión del CA-2 | `yq` con la expresión literal de la tarea | **falla por texto del CA**: imprime `false` por cada uno de los 71 documentos |
| 3 | Tres servicios cableados (dev y prod) | bucle de la tarea | pasa: `/readyz /healthz true true` ×3, `RUN_PHASES=real` sin `RUN_ALLOW_FAKE_PHASES`, `go-reset Recreate 65532`, `go-run-controller Recreate 65532` |
| 4 | HPA y CronJobs | `yq` sobre HPA y CronJob | pasa: `go-run-controller 1 1 go-run-controller`; `housekeeping`, `idle-check` y `rebuild` con `Forbid` |
| 5 | Sin latest y Secrets cifrados | `check-no-latest.sh deploy` y `check-secrets.sh` | pasa: rc=0 y rc=0. Sin argumento el script da rc=1 (exige un directorio) |
| 6 | Alertas | `policies.sh` y `promtool test rules` / `check rules` sobre el `.spec` extraído de `aqs-u2-rules` | pasa: `OK promtool-rules`, `SUCCESS`, `SUCCESS: 3 rules found`, conteo grep = 3+1+1+4 (≥ 3) |
| 7 | Circuito | — | recortado por el orquestador, no aplica |
| 8 | Alcance | `git status --short` y `git diff --name-only` contra el merge-base | pasa: 0 archivos fuera de alcance y 0 Go tocados |

Sobre la variante de yq del codificador para el CA-2: ni la expresión de la tarea ni `(select(...)|.spec.limits[0]) | A and B and C` dan `true` solas. La que funciona lleva paréntesis externos alrededor del `and`: `(select(...)|.spec.limits[0]) | ((A) and (B) and (C))` da 70 `false` y 1 `true`. Es equivalente como comprobación, y la mejor forma para el CA es `... | .spec.limits[0] | [.default.cpu != null, .defaultRequest.memory != null, .max.cpu != null] | join(",")`, que da `true,true,true`. Hay que reconciliar el texto del CA-2, y también el del CA-5, que debe pasar el directorio `deploy`.

## Hallazgos

### F-01 · NARANJA · `warm.yaml` (`ResourceQuota aqs-test-quota`) · `count/jobs.batch: 10` se agota tras pocas corridas
La cuota se dimensionó por paralelismo (4 Jobs simultáneos), pero `count/jobs.batch` cuenta todos los objetos Job, incluidos los terminados. En el código:
- Los Jobs de `rehearsal/job.go` y `runner/job.go` no tienen `ttlSecondsAfterFinished` (grep de `TTLSeconds`: 0 resultados).
- `DeleteJob` solo se llama para Jobs vencidos (`runner/launcher.go:232`), y el ensayo nunca se borra.
- Una corrida con N flujos crea N+1 Jobs. Con más de 9 flujos, o tras 2 o 3 corridas, el namespace rechaza todo Job nuevo por cuota excedida y la corrida falla.

Los pods terminados sí quedan fuera de las cuotas de `pods` y de cpu/memoria; el problema es solo el conteo de Jobs. Arreglo simple en manifiesto: subir `count/jobs.batch` a un valor que cubra los Jobs retenidos (por ejemplo 100), con un comentario de una línea que diga que no hay TTL ni limpieza. Añadir `ttlSecondsAfterFinished` en el código de los Jobs es tarea candidata.

Una nota menor sobre el dimensionado: los comentarios usan 500m/512Mi por runner, pero el código real pide 500m/256Mi (runner) y 250m/128Mi (ensayo). Queda sobredimensionado, no roto, y los pods de `warm-app`, `warm-db` y `warm-redis` caben holgados.

### F-02 · NARANJA · `control-plane.yaml` (`go-warm-manager`, `go-run-controller`) · TLS de MinIO sin confianza: todas las operaciones S3 fallarían
- MinIO sirve solo TLS, con una CA propia (`minio-tls`); lo muestran `minio/statefulset.yaml` y `docs/operaciones/secrets.md`.
- `go-warm-manager` usa `WARM_OBJECT_STORE=s3` con `WARM_S3_SECURE` sin definir, que es seguro por defecto (`main.go`, `!= "false"`).
- El controlador usa `EVIDENCE_ENDPOINT=https://minio...` con `runner.NewS3`.
- Ninguno de los dos monta la CA ni define `SSL_CERT_FILE` (grep en el build: ninguna referencia).
- Resultado: `x509: certificate signed by unknown authority` en el primer Put o Get.

El codificador lo anotó como candidata 5, pero es cableado de esta tarea y el arreglo es corto: montar `ca.crt` de `minio-tls` (el Secret existe en `aqs-system`) en ambos Deployments y fijar `SSL_CERT_FILE`.

### F-03 · AMARILLO (no bloquea) · Familia «cableado sin prueba»
Mis mutaciones en copia temporal, corriendo `policies.sh` completo (11 `OK` sin mutar), dieron este resultado:

| Mutación | ¿`policies.sh` la detecta? |
|---|---|
| Secret en claro en `dev/secrets` (el codificador lo probó) | sí, `FALLA check-secrets` |
| `RESET_SERVICE_TOKEN` con `value:` literal en vez de `secretKeyRef` | sí, `FALLA conftest-test` dev y prod |
| Quitar una variable obligatoria (`RESET_REDIS_ADDR`) | no |
| Quitar `ResourceQuota` y `LimitRange` | no |
| `readOnlyRootFilesystem: false` | no |
| `runAsNonRoot: false` | no |
| Nombre de Secret mal escrito (`go-reset-service-tokn`) | no |
| Borrar el PVC `go-reset-data` (referencia colgante) | no |
| Quitar `strategy: Recreate` | no |
| `RUN_PHASES=fake` + `RUN_ALLOW_FAKE_PHASES=true` | no |
| Límite del LimitRange por encima del máximo | no |
| `image: ...:latest` en `base/` | no: `check-no-latest.sh deploy` da rc=1, pero `check-no-latest.sh deploy/flux/prod` (lo que corre `ci.yml:198`) da rc=0 |

Lo único que atrapa las mutaciones es el build manual del CA-3 y las lecturas de referencias. Sí verifiqué que cada `secretKeyRef`, PVC y ConfigMap del build coincide con lo definido: los nombres cuadran y las claves son las que lee el código.

La barrera `latest` de CI es vacía para `base/`, porque `deploy/flux/prod` solo contiene `kustomization.yaml` y no hay `image:` ahí. Eso ya existía y está fuera de alcance, pero la afirmación «el CI corre check-no-latest aparte» no protege `base/`.

Juicio sobre la brecha de fakes: es aceptable para fusionar en desarrollo. La barrera real está en el código (`KUBERNETES_SERVICE_HOST` los rechaza dentro del clúster) y arreglarla exige Rego nueva, que está fuera de alcance. Va como candidata, junto con ampliar `policies.sh` para probar `aqs-u2-rules`.

### F-04 · AMARILLO · Variables de entorno (punto 1)
Contrasté las variables de los tres servicios con `loadConfig`, `config.Load` y `run`. Sin variables obligatorias que falten ni sobrantes inventadas:
- **`go-run-controller`:** cubre todas las obligatorias de `RUN_PHASES=real` (`WARM_URL`/`TOKEN`, `RESET_URL`/`TOKEN`, `RUN_ARTIFACT_REF`, `REHEARSAL_IMAGE`/`TARGET_URL`, `RUNNER_IMAGE`, `EVIDENCE_*`).
- **`go-warm-manager`:** cubre `WARM_NAMESPACE`, `WARM_SERVICE_TOKEN`, `WARM_OUTBOX_FILE`, `WARM_OBJECT_STORE=s3`, `WARM_S3_*` y `WARM_APP_URL`.
- **`go-reset`:** `RESET_NAMESPACE`, `RESET_OUTBOX_FILE`, `RESET_DATA_DIR`, `RESET_BASELINE_SCRIPT`, `RESET_REDIS_ADDR` y `RESET_WARM_ID`.
- **Ningún overlay lleva fakes.** El grep del build de dev y de prod solo encuentra `WARM_KUBE=incluster`, y los tokens van todos por `secretKeyRef`.
- **CronJobs de `go-reset`:** usan `config.Load(false)` y no necesitan `RESET_SERVICE_TOKEN`.
- **Imágenes:** los `Dockerfile` tienen `USER 65532:65532` numérico, así que `runAsNonRoot` funciona.

Dos matices menores:
- La bitácora dice que la imagen de `go-reset` es distroless. Es falso: el `Dockerfile` usa `FROM alpine:3.24` y trae `/bin/sh`. Un `baseline.sh` con `#!/bin/sh` sí corre; lo que falta sería el cliente de base de datos (`psql`). Hay que corregir ese «límite real» para U2-T08.
- Las imágenes `target-app`, `rehearsal` y `runner` `:0.0.0` son marcadores y los Secrets/ConfigMap los crea un humano. Las dos cosas son aceptables como candidatas, no bloquean.

### F-05 · AMARILLO · RBAC (punto 5)
Verificado en el código: no se usa el subrecurso `scale` (grep de `GetScale`/`UpdateScale`: 0 resultados). `go-reset` hace Patch de `Deployments` y `go-warm-manager` hace Get+Update y Patch de Deployments y StatefulSets. El Role `aqs-test-operator` ya cubre:
- `pods` (get/list/delete) y `pods/log`;
- `configmaps`, incluido `warm-state` y `warm-policy`;
- `deployments` y `statefulsets` con get/update/patch;
- `jobs` completo para el controlador.

No hace falta `deployments/status`. La matriz RBAC sigue en `OK`. Sin hallazgo.

### F-06 · AMARILLO · CronJobs y horarios (punto 3)
- **`podAffinity` a `go-reset`:** funciona con `Recreate` y 1 réplica. Es frágil: si no hay un pod `go-reset` (por ejemplo, el Deployment no arranca por el ConfigMap/Secret ausente), el Job queda `Pending` para siempre y `Forbid` bloquea los siguientes. Falta `activeDeadlineSeconds` o `startingDeadlineSeconds`. Es deuda razonable.
- **Horarios:** `housekeeping` corre cada hora en `:00` (`0 * * * *`), no el domingo a las 03:00, así que el comentario del `rebuild` está inexacto. `rebuild` a las `30 3 * * 0` más `idle-check` en `:35` pueden solaparse sin Lease; ya está registrado como candidata.
- **Estilo:** `warm.yaml` tiene un `---` doble (documento vacío, inocuo para kubeconform).

### F-07 · AMARILLO · Prod hereda `replicas: 2` sobre estado con PVC RWO
El patch de prod fija `replicas: 2` a todos los Deployments `control-plane`, incluidos `go-reset`, `go-warm-manager` y `go-run-controller`, que ahora tienen un PVC RWO de un solo escritor. El HPA recorta solo al controlador. El patrón ya existía con `go-governance`. Está fuera de alcance («prod sin parches nuevos»), así que va como candidata, pero debe resolverse antes de usar prod.

### F-08 · AMARILLO · Alertas (punto 7)
- Mis 4 mutaciones sobre las reglas (umbral, `for`, namespace, `== 0`) hacen fallar `promtool test`. El caso de no disparo existe para `AqsHandoffRising` y `AqsUpstreamCircuitOpen`. Faltan casos de no disparo para `AqsTestJobFailed` y `AqsWarmQuarantined`.
- `AqsUpstreamCircuitOpen` es una alerta muerta: no existen métricas `aqs_upstream_*` porque el circuito se recortó. Déjala marcada como está. El comentario del YAML basta, pero conviene añadir también una anotación visible.
- La prueba de `aqs-u2-rules` no corre en CI (`policies.sh` solo prueba las reglas de backup), y cuesta ejecutarla porque exige extraer a mano los `.spec`. Se pierde esa cobertura; va como candidata.
- `kube_job_status_failed > 0` es ruidoso porque cuenta fallos de reintentos.

### F-09 · AMARILLO · Secrets documentados (punto 6)
Los Secrets y el ConfigMap quedan descritos solo en un comentario de cabecera de `control-plane.yaml`, con nombres y claves, sin cómo generarlos. No se actualizó `docs/operaciones/secrets.md`; está fuera del alcance del CA-8. Las claves coinciden con lo que el código lee, y un humano que lea el comentario puede crearlos con cierto esfuerzo.

## Tareas candidatas (defectos reales fuera de alcance)
- Añadir `ttlSecondsAfterFinished` o limpieza de Jobs terminados en `rehearsal/job.go` y `runner/job.go`.
- `check-no-latest.sh` en `ci.yml` debe correr sobre `deploy/` y no solo sobre `deploy/flux/prod`.
- Regla Rego o script de cableado que cubra: fakes prohibidos; referencias a Secret/PVC/ConfigMap definidas; presencia de `ResourceQuota` y `LimitRange` en `aqs-test`; `strategy: Recreate` y `runAsNonRoot` en los Deployments con PVC.
- Probar `aqs-u2-rules` desde `policies.sh`.
- El patch de prod debe poner `replicas: 1` en los servicios con PVC.
- Transporte de eventos hacia `RUN_EVENTS_FILE`: el archivo vive en el PVC del controlador y nadie lo alimenta (C-45/C-49).
- Documentar los Secrets nuevos en `docs/operaciones/secrets.md`.
- Corregir en la bitácora la afirmación de que la imagen de `go-reset` es distroless.

VEREDICTO: NO-VERDE
NARANJA|deploy/flux/base/warm.yaml (ResourceQuota count/jobs.batch)|Cuota de Jobs=10 se agota: los Jobs terminados no se borran ni tienen TTL
NARANJA|deploy/flux/base/control-plane.yaml (go-warm-manager y go-run-controller hacia MinIO TLS)|Sin CA de minio-tls ni SSL_CERT_FILE: todas las operaciones S3 fallarían
INFORME: revisiones/U2-T07/ronda-1.md
