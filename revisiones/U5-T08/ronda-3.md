# Ronda 3 — U5-T08

VEREDICTO: VERDE

Hash de la tarea: 115cac143b9bbeeed28d020932de8d1de7a36289. Worktree en 0a3062d. `git status --short | wc -l` da 0 antes y después. Contenedores y redes `aqs-backup-test*`: 0 y 0 tras mi corrida del harness.

## Alcance de esta ronda
`git diff 5caf31c..tarea/U5-T08 --stat` da `bitacoras/U5-T08.md | 11 +++++++----`, un único archivo. El código no cambió desde la ronda 2, así que corrí CA-1, CA-5, CA-7, CA-8 y CA-9. Además volví a correr CA-4 (el harness, 20 s) por precaución.

## Criterios de aceptación, verificados por mí (comandos literales, alias K e Y)
| # | Criterio | Resultado |
|---|---|---|
| 1 | CronJob y regla en el build | pasa: `CronJob/evidence-backup` y `PrometheusRule/aqs-backup-rules` |
| 4 | harness con MinIO local (extra) | pasa: `backup objetos=3`, `sse=AES256`, `poda viejo=0 nuevo=1`, `restore iguales=3`, `rc=0`, `0`, `0` |
| 5 | scripts del ConfigMap = repo | pasa: `backup.sh IGUAL` y `restore.sh IGUAL` |
| 7 | documentos | pasa: sin `FALTA`, `3`, `6`, `9`, `3`, `5` |
| 8 | solo `- backup` | pasa: `1 0 deploy/flux/base/kustomization.yaml` y `  - backup` |
| 9 | shellcheck y árbol limpio | pasa: `rc=0` y `0` (posterior al último commit) |

CA-2, CA-3 y CA-6 no los repetí: el código es idéntico al de la ronda 2, donde pasaron.

## F-06 · RESUELTO
- **Bloque inválido delimitado.** La línea 26 es el título "Verde (ronda 1) — INVÁLIDO — salida de otra tarea por colisión en el scratchpad; sustituido por la sección Ronda 2". La línea 27 es `INICIO DEL BLOQUE INVÁLIDO` (se conserva solo como evidencia del incidente F-02/F-06, no es evidencia de U5-T08). La línea 64 es `FIN DEL BLOQUE INVÁLIDO`.
- **Coincidencias dentro del bloque.** `grep -n -E "sa-local|housekeeping|go-reset|320 tests"` da las líneas 32, 33, 46, 50 y 54. Todas caen entre 27 y 64. No hay ninguna fuera.
- **Barrido de "Decisiones no fijadas" contra los archivos reales:**
  - `AqsBackupFailed`: la bitácora lo describe como `kube_job_status_failed > 0`, `kube_job_status_start_time` posterior a `kube_cronjob_status_last_successful_time`, o fallo sin ningún éxito (`unless on(namespace)`), con `for 5m` y severidad `critical`. Coincide con `prometheusrule.yaml:12-30`.
  - `AqsBackupStale`: `time() - ... > 8*3600` o `absent(...)`, `for 10m`. Coincide.
  - ConfigMap: `evidence-backup-scripts` con `namespace: aqs-system`. Coincide con `kustomization.yaml` de `backup/`, y CA-1 confirma el build.
  - Poda: comparación numérica `YYYYMMDDHH` con `tr -d "T-"` y `-lt`, con el listado capturado en `LISTING`. Coincide con `backup.sh`.
  - Resto: `backoffLimit: 2`, `activeDeadlineSeconds: 3000`, historiales 3/3, recursos 100m/128Mi a 500m/512Mi, emptyDir `/staging` de 10Gi, `restartPolicy: Never`, ServiceAccount `aqs-backup` con `automountServiceAccountToken: false`, claves de `backup-target`, `restore.sh` que valida el prefijo y falla si está vacío. Todo coincide con `cronjob.yaml`, `serviceaccount.yaml` y `restore.sh`.
- **Barrido del resto de la bitácora:**
  - El mensaje `la poda toco el prefijo manual/` existe en `backup-local.sh:89`, y el de `no hay prefijo nuevo en el destino` en `:73`, a través de `fail`.
  - "AqsBackupStale salta en instalación nueva" está en `runbook-restore.md:12`.
  - La sección Ronda 2 (líneas 103-198) ya la validé línea a línea en la ronda 2 y el código no cambió.
  - No quedan en docs, scripts ni manifiestos menciones a "léxica", a la expresión simple de `kube_job_status_failed` como regla final, ni a "sin namespace". El único `kube_job_status_failed` fuera de la regla está en `rules_test.yaml`, que es una serie de prueba.
  - Las afirmaciones de la ronda 1 que quedan fuera del bloque inválido (rojo inicial, rojo intermedio del harness, negativas) no contradicen el código final.

## Hallazgos en pie
Ninguno ROJO ni NARANJA.

## AMARILLO (no bloquean; heredados, sin cambio)
- El Job de restore del runbook: `b="$(S s3 cp ... | sha256sum)" || b=ausente` nunca llega a `ausente` por falta de `pipefail`. Un objeto ausente da el sha256 de vacío y sigue marcando `DIFIERE`.
- `kubectl wait --for=condition=complete` espera el timeout completo si el Job falla. Convendría documentar `kubectl logs`.
- El harness corre sin TLS, así que la rama `*_CA_BUNDLE` no se ejercita en las pruebas.
- El pod no declara `ephemeral-storage` ni `securityContext` (ya en C-15).

## Tareas candidatas (fuera de alcance)
- C-31 (`minio-init` sin namespace) lo lleva U5-T11.
- Pasar `promtool test rules` (`rules_test.yaml`) por CI. Hoy queda manual, fuera del kustomization.

VEREDICTO: VERDE
INFORME: revisiones/U5-T08/ronda-3.md
