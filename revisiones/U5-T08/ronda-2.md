# Ronda 2 — U5-T08

VEREDICTO: NO-VERDE (un solo hallazgo en pie, NARANJA, solo de la bitácora; el código, las pruebas y los documentos están correctos)

Hash de la tarea: 115cac143b9bbeeed28d020932de8d1de7a36289. Worktree en 5caf31c. `git status --short | wc -l` da 0 antes y después. Contenedores y redes `aqs-backup-test*`: 0 y 0 tras todas mis corridas, incluidas las negativas y mi propio MinIO para F-05. Mis temporales estaban en un `mktemp -d` y ya los borré.

## Criterios de aceptación, verificados por mí (comandos literales, alias K e Y, CA-6 con chmod 755)
| # | Criterio | Resultado |
|---|---|---|
| 1 | CronJob y regla en el build | pasa: `CronJob/evidence-backup`, `PrometheusRule/aqs-backup-rules` (exactamente 2 líneas) |
| 2 | kubeconform estricto | pasa: dev y prod `Valid: 53, Invalid: 0, Errors: 0, Skipped: 0` |
| 3 | schedule, concurrencia, imagen y credenciales | pasa: `0 */6 * * * Forbid amazon/aws-cli:2.18.0`, `0`, `0` |
| 4 | harness con MinIO local | pasa: `backup objetos=3`, `sse=AES256`, `poda viejo=0 nuevo=1`, `restore iguales=3`, `rc=0`, `0`, `0` (20.3 s, docker real) |
| 5 | scripts del ConfigMap = repo | pasa: `backup.sh IGUAL`, `restore.sh IGUAL` |
| 6 | promtool y alertas | pasa: `SUCCESS: 2 rules found`, `AqsBackupFailed`, `AqsBackupStale`, `0` |
| 7 | documentos | pasa: sin `FALTA`, `3`, `6`, `9`, `3`, `5` |
| 8 | solo `- backup` | pasa: `1 0 deploy/flux/base/kustomization.yaml` y `  - backup` |
| 9 | shellcheck y árbol limpio | pasa: `rc=0` y `0` (posterior al último commit) |

Pruebas negativas sobre copias temporales, todas fallan donde deben:
- Sin `--sse AES256`: `sse=None` y `FALLA`.
- `restore.sh` que no copia: `restore iguales=0` y `FALLA`.
- Sin `dst s3 rm`: `poda viejo=1 nuevo=1` y `FALLA: poda inesperada`.
- Sin el `sync` al destino: `FALLA: no hay prefijo nuevo en el destino`.

## Hallazgos de la ronda 1, por identificador

### F-01 · RESUELTO
- Build de prod: `ConfigMap evidence-backup-scripts aqs-system`. El volumen `scripts` del CronJob referencia `evidence-backup-scripts`. El runbook lo cita con su namespace.
- CA-1 da exactamente sus 2 líneas, y CA-5 pasa por el `test("^evidence-backup")`.
- Barrido de objetos sin `metadata.namespace` en el build de prod: solo los 3 `Namespace` (cluster-scoped, correcto) y `ConfigMap minio-init`, que es C-31 y lo corrige U5-T11. Ningún otro.

### F-02 · RESUELTO en la sección "Ronda 2", pero queda un residuo (ver F-06)
La sección "Ronda 2" imprime `pwd` y toda su salida coincide con mi ejecución. Lo comparé línea a línea:
- CA-1, CA-2 (53 recursos), CA-3, CA-4, CA-5, CA-6 (incluido `promtool test rules` con `SUCCESS`), CA-7 (`3 6 9 3 5`) y CA-8.
- La salida de F-04 y de las negativas (`sse=None`, `restore iguales=0`, `no hay prefijo nuevo`).
Ninguna línea de esa sección es de U5-T09.

### F-03 · RESUELTO
- `promtool test rules rules_test.yaml` sobre el build real de prod: `SUCCESS`, rc=0.
- Escribí mi propio caso con etiquetas realistas de kube-state-metrics (`reason`, `job`, `instance`):
  - fallo A: alerta encendida a 10 min;
  - éxito a las 2 h: apagada a 130 min;
  - fallo B: encendida a 250 min;
  - éxito: apagada a 340 min;
  - fallo C: encendida a 500 min, y A y B siguen apagados;
  - un Job con un pod fallido que terminó bien (start 18000, éxito 18100): no alerta.
  Todo pasa con `SUCCESS`.
- Mutación: sustituyendo la expresión por el `kube_job_status_failed > 0` anterior, mi caso da `FAILED`. La prueba discrimina.
- Matching de etiquetas: `and on(namespace, job_name)` y `> on(namespace) group_left()` contra la serie única de `cronjob="evidence-backup"` no deja la expresión vacía con las etiquetas reales. Se enciende en los casos con y sin éxito previo (la rama `unless on(namespace)` cubre el "nunca hubo éxito"). Sin falso negativo.

### F-04 · RESUELTO
Con un `aws` simulado cuyo `s3 ls` devuelve 1: `backup 2026-10-02T02 listo` y `rc=1`, sin `poda completada`. Con un `ls` vacío y exitoso: `poda completada`, `rc=0`.

### F-05 · RESUELTO
- Extraje el manifiesto del runbook (78 líneas) y lo pasé por `kubeconform -strict`: `Valid: 1`. También con `name:` en lugar de `generateName`.
- Usa el ConfigMap `evidence-backup-scripts`, los Secrets `minio-root` y `backup-target`, el CA bundle de `minio-tls` (`ca.crt`), el ServiceAccount `aqs-backup`, el comando `restore.sh` y el prefijo por `RESTORE_PREFIX`.
- Ejecuté el script embebido contra un MinIO local, con la salida de `backup.sh` real:
  - Caso feliz tras vaciar `evidence`: `restore ... listo`, `validados=2 diferentes=0`, rc=0.
  - Un objeto alterado en `evidence`: `DIFIERE dos.txt`, `validados=2 diferentes=1`, rc=1.
  - Prefijo inexistente: rc=1.
  - Prefijo inválido: rc=2.
- Los `kubectl` del runbook están bien formados, pero no los ejecuté (no hay clúster).

## Hallazgo nuevo

### F-06 · NARANJA · `bitacoras/U5-T08.md:28-60` y `:81-91` · La evidencia falsa de U5-T09 sigue en la bitácora y las "Decisiones" están obsoletas
- La sección "Ronda 1 / Verde" conserva intacta la salida de otra tarea. Los números de línea y los valores son:
  - `aqs-system/housekeeping=go-reset` y `aqs-system/rebuild=go-reset` (líneas 31-32);
  - `40 resources`;
  - `320 tests, 320 passed`;
  - `sa-local rc=1` (línea 53).
- `grep -n -E "sa-local|housekeeping|go-reset|320 tests" bitacoras/U5-T08.md` da 5 coincidencias, todas en ese bloque.
- La ronda 2 corrige el origen de la contaminación y dice "ver abajo", pero no marca ese bloque como inválido ni lo elimina. Esa bitácora se fusiona tal cual, con "comandos exactamente los de la tarea" encima de salida que no pudo salir de este diff.
- "Decisiones no fijadas" contradice el código actual en dos líneas:
  - "El ConfigMap generado no fija namespace, igual que minio-init". Es justo lo que F-01 corrigió.
  - `AqsBackupFailed: kube_job_status_failed > 0 for 5m`. La expresión real es otra.
- Arreglo mínimo: borrar o marcar como "INVÁLIDO, sustituido por la Ronda 2" el bloque Verde de la ronda 1, y corregir esas dos líneas de "Decisiones" (y la mención de comparación léxica de la poda, que ahora es numérica). Cuando se haga, la tarea queda VERDE: todo lo demás está en pie y pasa.

## AMARILLO (no bloquean)
- En el Job de restore del runbook, `b="$(S s3 cp ... | sha256sum)" || b=ausente` nunca llega a `ausente`: el pipeline no usa `pipefail`. Un objeto ausente en `evidence` da el sha256 de vacío y sigue marcando `DIFIERE`. Solo habría un falso OK si ambos lados fallaran a la vez.
- `kubectl wait --for=condition=complete` del runbook espera hasta el timeout de 3600 s si el Job de restore falla. Convendría documentar `kubectl logs` para ese caso.
- Heredados de la ronda 1 y sin cambio: el harness corre sin TLS, así que la rama `*_CA_BUNDLE` no se ejercita en las pruebas (sí en mi ejecución del Job de restore, con un bundle ficticio). El pod no declara `ephemeral-storage` ni `securityContext` (ya en C-15).

## Tareas candidatas (fuera de alcance)
- C-31 (`minio-init` sin namespace en main): sigue vigente, lo lleva U5-T11.
- Probar con `promtool test rules` en CI las reglas de backup. Hoy `rules_test.yaml` queda manual, fuera del kustomization.

VEREDICTO: NO-VERDE
NARANJA|bitacoras/U5-T08.md:28-60 y 81-91|F-06 La salida de U5-T09 sigue en la bitácora (ronda 1) y las "Decisiones" contradicen el código final (namespace del ConfigMap, expresión de AqsBackupFailed, poda)
INFORME: revisiones/U5-T08/ronda-2.md
