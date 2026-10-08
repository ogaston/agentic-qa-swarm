# Ronda 2 — U3-T05

VEREDICTO: NO-VERDE

Los 6 criterios pasan en ejecución fresca y F-01 y F-02 de la ronda 1 quedan corregidos. Pero al repetir las mutaciones aparecen tres comportamientos que la tarea exige y que ninguna prueba fija. Son cobertura ausente (NARANJA), no código roto: verifiqué a mano que el comportamiento es correcto.

Rama `tarea/U3-T05`, sha `4f0c638a423c05f739e685b2747ffdb1d5886b10`. Worktree limpio después de revisar (`git status --short` da 0).

**Corrección sobre mi ronda 1.** Mis scripts de mutación escribían bytecode, y el `__pycache__` obsoleto pudo hacer que algunas mutaciones salieran «KILLED» sin serlo. Esta ronda los repetí con `PYTHONDONTWRITEBYTECODE=1`, tras verificar que la línea base da 96 passed. Con esa corrección aparecen los supervivientes de F-01 de esta ronda, que en la ronda 1 salieron como muertos. Esto afecta solo a mis mutaciones, no a los criterios de aceptación.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Entorno limpio, pruebas y umbrales | venv nuevo + `pip install -r requirements-dev.txt`, `pytest -q \| tail -n 2`, `python -m agent_eval run --out out` | pasa: `96 passed in 1.79s` (el mínimo era 15); cuatro líneas `OK` y `rc=0` |
| 2 | Informe válido y números | validación jsonschema + `jq` | pasa: `schema ok`; `fake pipeline 12 10 1.0 0.0 1.0 true true true`; `per_artifact` tiene 12 |
| 3 | Las métricas se calculan | `pytest -k 'detects or threshold' -v` | pasa: 22 `PASSED` (18 pruebas distintas, una parametrizada ×6), 0 `SKIPPED`, 0 `FAILED`. Hay dos nuevas: causa raíz errada baja la factualidad, y la factualidad sola hace rc=1 |
| 4 | Bucle de regresión | `add-regression` + `run` sobre copia del dataset | pasa: rc=0, 1 directorio; `run` da rc=1 y `1 12 11 0.2 0.9091`. Original intacto: `git status --short agents/dataset \| wc -l` da 0 |
| 5 | Determinismo y cero red | `cmp out1 out2`; `pytest -k no_network` | pasa: `identico`; 6 `PASSED` |
| 6 | Higiene y alcance | `list-services`, `compileall -W error`, `git status`, diff fuera del filtro | pasa: `0`, `ok`, `0`, `0` |

Corrí los seis completos, no solo los cuatro que re-corrió el codificador. El único cambio de código fuente de la ronda es el bloque de duplicados en `regression.py`; el resto son pruebas y bitácora.

## Verificación de lo corregido
- **F-01 (factualidad).**
  - Mutación «hay algún hallazgo» en lugar de comparar la causa: muerta.
  - Mutación `all_met` sin factualidad: muerta por `test_threshold_factualidad_alone_gates_exit_code`.
  - Mutación factualidad sobre todos los artefactos: muerta.
  - Constante del umbral cambiada a 0.2: muerta.
- **F-02 (regresión en precisión).** Mutación «regresión fuera de `LABELLED_KINDS`»: muerta. Está pinada `labelled == 11` y `precision == 10/11` (mi CA-4 dio 0.9091). Mutación «precisión sobre todos los artefactos, incluido `no-arranca`»: muerta.
- **Prueba vacua.** Reescrita con aciertos mezclados. Barrí las demás aserciones `== 0`, `== []` y `not …` de `tests/`:
  - `precision([bad,bad])`, `precision([right])/([wrong])`: sin vacuidad.
  - `factualidad([gold]) == 0`: es una definición de borde, y otras aserciones del mismo test (`[hit,miss,gold] == 1/2`) matan la mutación «sobre todos».
  - `ruido([])`, `precision([])`, `adherencia([])`: bordes definidos. La mutación «denominador 0 → 1» está muerta.
  - `g.attempts == []` (loopback): tiene control positivo en las otras pruebas del guard.
  - «Sin restos» tras un `add` fallido: la mutación «sin limpieza del tmp» está muerta.
- **Duplicados en `add-regression`.** Funciona a mano:
  - `(bug-ecom-01, f1)` repetido da `error: ya existe la regresion bug-ecom-01-fp-1 para bug-ecom-01 / f1` y rc=2, sin dejar restos.
  - Mismo artefacto con `flow-2` crea `-fp-2`.
  - Otro artefacto (`bug-fintech-01`) con el mismo `flow-1` crea su regresión.
  - Repetir `(bug-ecom-01, flow-2)` se rechaza.
  - Mutaciones muertas: duplicados desactivados, ignorar el finding (rechazaría legítimas), duplicado con OR.
  - Mutación que ignora el artefacto: sobrevive (F-01 de esta ronda).

## Hallazgos
### F-01 · NARANJA · `tests/test_regression.py`, `tests/test_run_detects.py:148-152` · Comportamientos exigidos por la tarea sin prueba (4 mutantes sobreviven con `96 passed`)
Mismo tipo que F-01 y F-02 de la ronda 1, barrido de la clase en esta ronda.

1. **`--label inconcluso`.** Mutación: forzar `verdict: "sin-hallazgos"` en `expected.json` sin mirar la etiqueta. Sobrevive. Las pruebas que usan `inconcluso` no leen `expected.json`; la única aserción sobre el contenido (`exp["reporter"] == {"verdict": "sin-hallazgos", …}`) usa la otra etiqueta. La tarea exige «fija `expected.json` con la etiqueta dada». A mano: `expected.json` queda `{"root_cause":null,"verdict":"inconcluso"}`, pero ninguna prueba lo ve.
2. **Línea `FALLA`.** Mutación: `tag = "OK"` fijo en `cli._run`. Sobrevive. `test_threshold_lines_one_per_metric` solo comprueba la línea base con todo en `OK`. La tarea pide «una línea por métrica con `OK|FALLA`», y ninguna prueba ve `FALLA` en la salida. En CA-4 sí sale `FALLA ruido 0.2000`.
3. **Duplicado con el mismo finding en otro artefacto.** Mutación: comparar solo `source_finding`. Sobrevive. Es el caso realista, porque los ids `flow-1` existen en todos los artefactos, y una regresión legítima de `bug-fintech-01 / flow-1` quedaría bloqueada. Mutación «duplicado solo contra `-fp-1`»: sobrevive también, porque la prueba solo repite el primero.

Arreglo propuesto, tres aserciones:
- Que `add-regression --label inconcluso` deje `expected.json` con `inconcluso`.
- Que, con ruido ≥ 0.2, la salida de `run` contenga `FALLA ruido` y no `OK ruido`.
- Que `(A,f)` y `(B,f)` convivan, y que `(A,f1),(A,f2),(A,f2)` rechace el tercero.

## Supervivientes que no son hallazgos
- Equivalente: `ID_RE.match` sobre `artifact_id` en `regression.py`. El `arts.get(artifact_id)` posterior lo rechaza.
- Redundantes entre sí: A5 sin escanear las hojas y A5 sin el `json.dumps` del reporte. Cada camino cubre lo del otro.
- Defensa en profundidad: la comprobación de `d.resolve().parent` en `dataset._check_dir`, detrás del chequeo de symlink y de `fullmatch`.
- AMARILLO: no hay prueba de que `load_dataset` salte directorios `.tmp-*` de un `add-regression` en curso.

## Alcance y puntos de la ronda 1
Sin cambios: el diff no toca dataset, planner ni reporter, y no hay Dockerfile. Los puntos a juzgar y las tareas candidatas de mi informe de ronda 1 siguen vigentes: confirmación humana de factualidad > 0.80 y adherencia == 1.0; ampliar `meta.schema.json` para `regresion-fp`; patrones de secreto independientes para A5; que `--finding` identifique el hallazgo concreto; CI para `agents/eval`; medición con modelo real (T07).

VEREDICTO: NO-VERDE
NARANJA|tests/test_regression.py + tests/test_run_detects.py:148-152|Sin prueba: `--label inconcluso` en expected.json, línea `FALLA` en la salida y duplicados con el mismo finding en otro artefacto (4 mutantes sobreviven)
