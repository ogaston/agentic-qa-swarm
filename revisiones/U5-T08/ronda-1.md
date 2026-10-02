# Ronda 1 — U5-T08

VEREDICTO: NO-VERDE

Hash de la tarea verificado: 115cac143b9bbeeed28d020932de8d1de7a36289. Worktree en 91de149 y limpio: `git status --short | wc -l` da 0 antes y después. Los contenedores y redes `aqs-backup-test` quedan en 0 tras todas mis corridas. Las copias temporales las hice en el scratchpad.

## Criterios de aceptación, verificados por mí (comandos literales, alias K e Y; CA-6 con chmod 755)
| # | Criterio | Resultado |
|---|---|---|
| 1 | CronJob y regla en el build | pasa: `CronJob/evidence-backup`, `PrometheusRule/aqs-backup-rules` |
| 2 | kubeconform estricto | pasa: dev y prod `Valid: 45, Invalid: 0, Errors: 0, Skipped: 0` |
| 3 | schedule, imagen y credenciales | pasa: `0 */6 * * * Forbid amazon/aws-cli:2.18.0`, `0`, `0` |
| 4 | harness con MinIO local | pasa: `backup objetos=3`, `sse=AES256`, `poda viejo=0 nuevo=1`, `restore iguales=3`, `rc=0`, `0`, `0`. Tardó 19.9 s (docker real, no es una suite saltada). |
| 5 | scripts del ConfigMap = repo | pasa: `backup.sh IGUAL`, `restore.sh IGUAL` |
| 6 | promtool y alertas | pasa: `SUCCESS: 2 rules found`, `AqsBackupFailed`, `AqsBackupStale`, `0` |
| 7 | documentos | pasa: sin `FALTA`, `3`, `6`, `6`, `3`, `5` |
| 8 | solo `- backup` | pasa: `1 0 deploy/flux/base/kustomization.yaml` y `  - backup` |
| 9 | shellcheck y árbol limpio | pasa: `rc=0` y `0` (posterior al último commit; va aquí porque la bitácora lo manda al informe) |

Pruebas negativas sobre copias temporales, todas fallan donde deben:
- Sin `--sse AES256`: `sse=None` y `FALLA`.
- `restore.sh` que no copia: `restore iguales=0` y `FALLA`.
- Sin `dst s3 rm`: `poda viejo=1 nuevo=1` y `FALLA: poda inesperada`.
- Poda agresiva que borra todo: rc=1 (ver A-02).

Los tres comportamientos del harness son reales. Usa el digest extraído con `yq` del StatefulSet, `head-object`, y checksums tras vaciar `evidence`. `manual/` se siembra y se comprueba (`N_MAN`). Los scripts del harness son los del repo, y CA-5 prueba que son idénticos a los del ConfigMap.

## Punto 1 (prioritario): el namespace del ConfigMap

Medido en `$K build deploy/flux/prod | $Y '.kind+"/"+.metadata.name+" ns="+(.metadata.namespace // "NONE")'`:
- `ConfigMap/evidence-backup ns=NONE`
- `ConfigMap/minio-init ns=NONE`
- `CronJob/evidence-backup ns=aqs-system`

No hay `targetNamespace` ni `namespace:` en ningún `kustomization.yaml` ni en `deploy/flux/*/flux-kustomization.yaml` (`grep -rn "targetNamespace\|^namespace:" deploy/` sin resultados). Con `aqs-prod` aplicado por Flux sin `targetNamespace`, el ConfigMap de `evidence-backup` queda en `default`. No pude ejecutarlo porque no hay clúster; es el comportamiento conocido de Flux/SSA para objetos namespaced sin namespace. El `warm-policy` de `aqs-test` sí lo fija, así que el patrón correcto ya existe en el repo.

### F-01 · ROJO · `deploy/flux/base/backup/kustomization.yaml:6-11` · El ConfigMap de scripts queda sin namespace y el CronJob no puede montarlo
El pod de `aqs-system` referencia `configMap: evidence-backup`, que está en `default`. Resultado: `FailedMount` y el backup nunca corre. Es la funcionalidad central de la tarea, aunque CA-1..CA-9 pasen. Solo `AqsBackupStale` lo detectaría, por la cláusula `absent`, y no `AqsBackupFailed`.

El argumento de la bitácora ("fijarlo añadiría una línea a CA-1") es cierto solo para el nombre `evidence-backup`. Fue una elección del codificador, no una restricción de la tarea. CA-1 selecciona por `.metadata.name == "evidence-backup"` y no filtra por `kind`. CA-5 usa `test("^evidence-backup")`, que admite un sufijo.

Lo demostré en una copia temporal:
- Generador `name: evidence-backup-scripts` con `namespace: aqs-system`, y el volumen del CronJob apuntando a ese nombre.
- Build resultante: `evidence-backup-scripts ns=aqs-system`.
- CA-1 da exactamente `CronJob/evidence-backup` y `PrometheusRule/aqs-backup-rules`.
- CA-5 da `backup.sh IGUAL` y `restore.sh IGUAL`.
- kustomize actualiza la referencia del volumen; el CronJob sigue llamándose `evidence-backup`.

Se sacrificó un comportamiento correcto por una línea de salida de un criterio, y había una alternativa que cumplía ambos. El runbook también cita el "ConfigMap `evidence-backup`" en `aqs-system`, que no existiría.

### M-01 · Hallazgo sobre main (NO es defecto de esta tarea) · `deploy/flux/base/minio/kustomization.yaml:7-10`
El ConfigMap `minio-init` de U5-T05 (ya fusionado) tiene el mismo defecto. El build de prod lo da con `ns=NONE`, y el Job `minio-init` de `aqs-system` lo monta. Bajo Flux sin `targetNamespace`, el Job se queda en `FailedMount` y el bucket `evidence` y su cifrado por defecto no se crean. Esto bloquea también el backup, porque `evidence` no existiría.

Es la misma causa raíz y la misma solución: `namespace: aqs-system` en el generador, y un nombre que no choque con los filtros por nombre de los CA de esa tarea. `minio-init` no entra en el filtro de CA-1 de T08, pero sí en el de U5-T05, que usa `test("^minio-init")` para `kind == "ConfigMap"` y no filtra por namespace; revisar ahí. Lo lleva el orquestador al humano como tarea nueva sobre main. No exijo arreglarlo aquí; `minio/` está fuera de alcance.

El barrido de clase sobre el build de prod encuentra solo esos dos objetos namespaced sin namespace. El resto (excepto Namespaces y `Kustomization/aqs-prod` en `flux-system`) lo trae fijado.

## Hallazgos

### F-02 · ROJO · `bitacoras/U5-T08.md` (sección "Verde") · Evidencia del commit final es de otra tarea
En 91de149 el bloque "Verde" fue sustituido por salidas que no pueden venir de este diff:
- CA-1 muestra `aqs-system/housekeeping=go-reset` y `aqs-system/rebuild=go-reset`.
- CA-2 muestra `40 resources` (el build real da 45).
- CA-4 muestra `22 tests, 22 passed`, `320 tests` y `combine rc=0`.
- CA-5 muestra `sa-local rc=1` y `sa-control-plane rc=0`.
- Faltan CA-7, CA-8 y CA-9.

Coincide con el bloque de `wt-U5-T09/bitacoras/U5-T09.md` (`grep -l "sa-local rc"` lista ambos). La versión de 5551d9e tenía la salida correcta. El commit final la pisó con la de U5-T09 y afirma "los comandos son exactamente los de la tarea". Por la invariante 3, evidencia que no pudo salir del diff actual es ROJO por sí sola. Además, tras el cambio de `backup.sh` en 91de149 (SC3012, comparación numérica de la poda) no queda ninguna salida de CA-4 propia en la bitácora. La que sí es mía está arriba.

### F-03 · NARANJA · `deploy/flux/base/backup/prometheusrule.yaml:14` y `cronjob.yaml` (`failedJobsHistoryLimit: 3`) · `AqsBackupFailed` no se apaga tras recuperarse
`kube_job_status_failed > 0` persiste mientras exista el Job fallido, que el historial conserva. Lo demostré con `promtool test rules` en el scratchpad:
- Job fallido constante y un backup exitoso a las 7 h.
- A las 20 h la alerta sigue en `got:[{alertname="AqsBackupFailed", job_name="evidence-backup-100", ...}]`, y esperaba `[]`.

Contradice dos afirmaciones de los documentos:
- `respuesta-a-incidentes.md`: "la alerta se resuelve tras el siguiente backup exitoso".
- `runbook-restore.md`, Validación 4: "Confirmar que AqsBackupFailed no está activa tras el siguiente backup".

Resultado: una alerta crítica pegada, fatiga de alertas, y operadores que no distinguen incidente vivo de histórico. Hay que acotar la expresión a fallos posteriores al último éxito (por ejemplo con `kube_job_status_start_time` frente a `kube_cronjob_status_last_successful_time`) o corregir ambos documentos. Una prueba `promtool test rules` en el repo la cubriría.

### F-04 · NARANJA · `deploy/flux/base/backup/backup.sh:50` · La poda puede fallar en silencio
`dst s3 ls ... | while read ...` en `sh` sin `pipefail`: si el `ls` falla (permisos de listado a nivel de bucket, red), el pipeline sale con 0 y el script imprime `poda completada` y termina bien. Lo comprobé con un `dst` simulado que devuelve 1: `poda completada rc=0`, `rc=0`. La retención de 30 días (requisito de la tarea) dejaría de aplicarse sin que salte `AqsBackupFailed`. Debe capturar el listado en una variable (como ya hace `restore.sh`) o comprobar su estado, y que el Job falle.

### F-05 · NARANJA · `docs/operaciones/*.md` · Pasos que no son ejecutables (barrido de la clase)
Los documentos pasan CA-7 por sus encabezados, pero estos pasos no tienen comando o recurso concreto:
- `runbook-restore.md`, Procedimiento paso 3: "un Job de un solo uso con la misma imagen y el ConfigMap `evidence-backup`". No hay manifiesto ni `kubectl create job`, ni cómo pasar el prefijo, ni el CA bundle de `minio-tls`. Es el paso central del restore. Además cita un ConfigMap que, tal como está, no existe en `aqs-system` (F-01).
- Validación 2: "comparar checksums de una muestra", sin comando.
- Validación 1: compara el número de objetos de `evidence` con el del prefijo, pero `restore.sh` no borra, así que `evidence` puede tener más objetos y el criterio es ambiguo.
- `respuesta-a-incidentes.md`: "suspender el CronJob" y "`kubectl logs` del Job" sin comando.
- Failback: "congelar escrituras en el alterno" sin mecanismo.
- Failover: "Secrets (`minio-*`)" en lugar de los nombres reales (`minio-root`, `minio-kms`, `minio-tls`).

Lo que sí usa nombres reales: `restore.sh`, `evidence-backup`, `AqsBackupFailed`, `AqsBackupStale`, `backup-target` y sus claves.

## Puntos 3 y 5, sin hallazgo
- `emptyDir` con `sizeLimit: 10Gi`, `backoffLimit: 2` y `serviceAccountName: aqs-backup` están presentes. El SA lleva `automountServiceAccountToken: false`. Los valores están en "Decisiones no fijadas" de la bitácora.
- Las alertas usan `for: 5m` y `for: 10m`, `severity: critical`, resumen y `absent()`.
- RTO de 4 h: marcado como propuesto y pendiente de confirmar.
- Alcance: el diff solo toca `deploy/flux/base/backup/`, `scripts/test/backup-local.sh`, `docs/operaciones/`, la bitácora y `- backup` tras `- namespaces.yaml`. Sin desborde. `Secret` creados: 0.

## AMARILLO (no bloquean)
- `AqsBackupStale` se dispara en una instalación nueva hasta el primer backup exitoso (hasta 6 h), por la cláusula `absent`. No está documentado.
- El harness corre sin TLS, así que la rama `*_CA_BUNDLE` de `src()` y `dst()` (la que usa producción) nunca se ejecuta en las pruebas.
- El harness sale con rc=1 y sin mensaje `FALLA` si no hay prefijo nuevo (`NEW=$(... | grep ...)` bajo `pipefail`).
- El pod no declara `ephemeral-storage` ni `securityContext` (ya está en C-15 para minio-init).

## Tareas candidatas (fuera de alcance)
- Sobre main (M-01): fijar `namespace: aqs-system` en el ConfigMap `minio-init`. El orquestador debe llevarlo al humano.
- Candidata de política: denegar objetos namespaced sin `metadata.namespace` en el build de prod. Ya existe C-26 para NetworkPolicy; ampliarla a ConfigMap.
- Prueba `promtool test rules` en CI para las alertas de backup.

## Rutas
- Pruebas de `promtool` y copia del experimento del ConfigMap: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/4ed52fd5-a142-46f0-ac9b-995972cfe97d/scratchpad/{pt,copy}`

VEREDICTO: NO-VERDE
ROJO|deploy/flux/base/backup/kustomization.yaml:6-11 (CA-1 y CA-5 no lo exigen)|F-01 ConfigMap evidence-backup sin namespace: el CronJob no puede montarlo bajo Flux; el argumento de CA-1 es falso con otro nombre
ROJO|bitacoras/U5-T08.md (Verde)|F-02 Evidencia del commit final pertenece a U5-T09; falta CA-7/8/9 y CA-4 propio
NARANJA|prometheusrule.yaml:14 / docs|F-03 AqsBackupFailed no se apaga tras un backup exitoso; docs afirman lo contrario
NARANJA|backup.sh:50|F-04 Fallo del listado en la poda es silencioso (sin pipefail)
NARANJA|docs/operaciones/*.md|F-05 Pasos de restore, validación, contención y failback sin comando ni manifiesto
INFORME: revisiones/U5-T08/ronda-1.md
