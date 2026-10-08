# Ronda 3 — U3-T05

VEREDICTO: VERDE

Corrí los seis criterios de aceptación completos y pasan. Los 4 mutantes de la ronda 2 y la nueva prueba de la línea NOTA (C-85) mueren con bytecode limpio. No queda ningún ROJO ni NARANJA. El humano fusiona.

Rama `tarea/U3-T05`, sha `0be6937c8656e4cfa764240d9a850c19c963d554`. Entre la ronda 2 y esta solo cambiaron pruebas, bitácora y el informe previo: `test_regression.py`, `test_run_detects.py`, `bitacoras/U3-T05.md` y `revisiones/U3-T05/ronda-2.md`. No hay cambios de código fuente. El worktree quedó limpio (`git status --short` da 0).

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Entorno limpio, pruebas y umbrales | `__pycache__` borrado, venv nuevo, `pip install -r requirements-dev.txt`, `pytest -q \| tail -n 2`, `python -m agent_eval run --out out` | pasa: `102 passed in 1.78s` (el mínimo era 15); cuatro líneas `OK` y `rc=0` |
| 2 | Informe válido y números | validación jsonschema + `jq` | pasa: `schema ok`; `fake pipeline 12 10 1.0 0.0 1.0 true true true`; `per_artifact` tiene 12 |
| 3 | Las métricas se calculan | `pytest -k 'detects or threshold' -v` | pasa: 26 `PASSED`, 0 `SKIPPED`, 0 `FAILED` |
| 4 | Bucle de regresión | `add-regression` + `run` sobre copia del dataset | pasa: rc=0, 1 directorio con los 6 elementos exigidos; `run` da rc=1 con `1 12 0.2` y `FALLA ruido 0.2000`; `git status --short agents/dataset \| wc -l` da 0 |
| 5 | Determinismo y cero red | `cmp out1 out2`; `pytest -k no_network` | pasa: `identico`; 6 `PASSED` |
| 6 | Higiene y alcance | `list-services`, `compileall -W error`, `git status`, diff fuera del filtro | pasa: `0`, `ok`, `0`, `0` |

## Mutaciones
Copia en el scratchpad, `PYTHONDONTWRITEBYTECODE=1`, `__pycache__` borrado, pytest completo (102 passed en la línea base de la copia) y fuente de la copia idéntica a la del worktree. Hice unas 90 mutaciones en cuatro lotes.

**Los cuatro de la ronda 2 y la NOTA, todos muertos:**
- `--label inconcluso` forzado a `sin-hallazgos`: muerto por `test_regression_label_inconcluso_is_written_to_expected`.
- `tag = "OK"` fijo: muerto por `test_threshold_failing_metric_prints_falla_not_ok` y `test_threshold_noise_at_limit_prints_falla_ruido`. El tag invertido también muere.
- Duplicados que comparan solo `source_finding`: muerto por `test_regression_same_finding_in_different_artifacts_coexist`.
- Duplicados solo contra `-fp-1`: muerto por `test_regression_duplicate_is_checked_against_every_existing_one`. Duplicados desactivados, ignorando el finding y con OR: muertos.
- Línea NOTA eliminada: muerta por `test_threshold_run_output_states_fake_llm_scope` y por `test_threshold_lines_one_per_metric`.

**Barrido final por comportamientos de alcance y CA sin aserción.** Ahora todos muertos:
- Informe no escrito y `--out` no creado.
- `llm` y `valid_for` del informe cambiados.
- Respuesta canned registrada bajo un hash errado.
- `expected` vacío en el registro por artefacto.
- Aviso de finding desconocido eliminado o desactivado.
- rc=2 para dataset inválido.
- Regresión sobre un artefacto sin evidencia.
- `kind` y `expected.json` sin validar, también el del artefacto de origen.
- Falso positivo por causa distinta.
- A2 sin comprobar ruta absoluta, `..`, `//` ni host.
- Los de la ronda 1 y 2: umbrales, A1 a A7, ruido por artefacto, precisión sin causa, ids con `match`, `regressions/` ignorado, regresión fuera de `LABELLED_KINDS`, factualidad sin comparar causa y fuera de `all_met`.

**Supervivientes, ninguno es hallazgo:**
- `ID_RE.match` sobre `artifact_id` en `regression.py`. Equivalente: `arts.get()` lo rechaza después.
- A5 sin escanear hojas o sin el `json.dumps`. Redundantes entre sí.
- `d.resolve().parent` en `dataset._check_dir`. Defensa en profundidad, detrás del chequeo de symlink y de `fullmatch`.
- Quitar `sort_keys`. El orden de claves ya es determinista en la corrida; la prueba byte a byte pasa.
- No vaciar `self._saved` en la guarda de red. Equivalente: la prueba comprueba que `socket.socket.connect is orig` tras salir.

## Hallazgos
Ninguno bloqueante.

AMARILLO: ninguna prueba comprueba que `load_dataset` salte directorios `.tmp-*` de un `add-regression` en curso. El caso solo aparece con `add-regression` y `run` en paralelo, y el riesgo es bajo.

## Puntos para el humano
Ninguno cambia el veredicto.
- Umbrales propuestos que el PRD no fija: factualidad `> 0.80` y adherencia `== 1.0` (C-89). Hay que confirmarlos.
- El informe y la salida declaran `llm: fake` y `valid_for: pipeline`. Con respuestas canned, el informe no demuestra que la precisión del producto supere 80 %. Eso exige T07 con un modelo real.
- `--finding` es solo metadato: la regresión etiqueta el artefacto completo y el id se avisa por stderr si no figura en la respuesta canned (CA-4 exige `f1` y rc=0).

## Tareas candidatas (fuera de alcance)
- Ampliar `agents/dataset/schema/meta.schema.json` para admitir `regresion-fp`, `source_artifact` y `source_finding`, y validar `regressions/*` en los tests del dataset. Hoy el `meta.json` de una regresión no valida contra ese esquema, y nadie lo comprueba.
- Patrones de secreto independientes del redactor del reporter para A5 (hoy la regla es circular).
- Que `--finding` identifique el hallazgo concreto y se valide contra los `flow_id` o `finding_id` reales.
- Workflow de CI para `agents/eval`.
- Medición con modelo real (T07).

VEREDICTO: VERDE
