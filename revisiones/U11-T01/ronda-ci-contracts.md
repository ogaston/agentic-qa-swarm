# Arreglo de CI de contracts/validate.sh (commit c1062c6, PR #134) — informe del revisor, persistido por el orquestador (resumen fiel)

VEREDICTO: VERDE

Ejecución propia en un scratchpad único; worktree limpio; sin procesos en segundo plano.
1a. `bash contracts/validate.sh` en HEAD: rc=0, 50 `ok`, 0 FALLA/SIN ESQUEMA. 1b. En 07c8bd8 (git archive): rc=1 con exactamente `FALLA type 'run.confirmed' no coincide con el nombre del archivo: examples/valid/run.confirmed.with-artifact.json` (49 ok): el rojo es real.
2. Lint OpenAPI (`@redocly/cli@1.25.0`): valid, rc=0. 4. Diff de la salida completa antes/después: la única diferencia es la línea de run.confirmed.with-artifact.json (FALLA → ok); los 49 ok previos idénticos; `schema_for` fuera del diff. 6. Nada más depende del nombre de los ejemplos válidos (solo notify.created.json y flow-plan.tres-flujos.json, sin cambios).

## Adversarial (24 casos válidos adversariales en una copia reducida)
`deploy.done.x.json` con type `deploy.failed` → FALLA (el nombre no empieza por el type); type vacío / ausente / numérico / array / `*` → FALLA; `run.confirmed.foo.json` con type `run.done` → FALLA no consistente; `foo.json` → SIN ESQUEMA; mayúsculas → SIN ESQUEMA/FALLA; `run.confirmed.a.b.c.json` → ok; type solo prefijo del esquema (`deploy`) → lo rechaza ajv por el enum; type con `\n` final → pasa la regla del script y lo rechaza ajv; JSON roto → FALLA. Pares `deploy.done/failed`, `rehearsal.passed/failed`, `run.done/confirmed` con type cruzado → FALLA; los archivos reales pasan.
Mutaciones: regla original → rechaza `run.confirmed.a.b.c.json` (detecta el rojo); sin la regla de nombre → `deploy.failed.json` copiado como `deploy.done.x.json` pasa (la prueba lo detecta); sin la regla de type o con ambas débiles → ajv sigue rechazando un type incoherente (con otro mensaje).

## Hallazgos (no bloquean)
- F-01 · AMARILLO · validate.sh:38: la regla de type es en gran parte redundante con ajv (solo mejora el mensaje); la regla de nombre (línea 39) sí aporta sensibilidad.
- F-02 · AMARILLO · validate.sh:39: los casos de sensibilidad solo viven en la bitácora (no hay test automatizado).

## Bitácora y commit
La bitácora dice la verdad en los números reproducibles (49 ok → 50 ok, rc=1 → rc=0, mensajes, lint). El commit solo toca validate.sh y la bitácora; trailer `Co-Authored-By: Claude Haiku 5.5`.

## Candidata
- Mini-conjunto de ejemplos adversariales y verificación en CI para que la sensibilidad de la regla no dependa de la bitácora.

VEREDICTO: VERDE
