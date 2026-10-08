# Ronda 1 — U3-T05

VEREDICTO: NO-VERDE

Los 6 criterios de aceptación pasan en mi ejecución fresca. Los dos NARANJA son cobertura ausente: en mis mutaciones sobrevivieron 3 que ninguna prueba mata. No hay ROJO.

Rama `tarea/U3-T05`, sha `0dc8b1e0547eaa15419b60f919a7f20e9f9ce02f`. El worktree quedó limpio (`git status --short` da 0 líneas).

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Entorno limpio, pruebas y umbrales | venv nuevo + `pip install -r requirements-dev.txt`, `pytest -q \| tail -n 2`, `python -m agent_eval run --out out` | pasa: `92 passed in 2.04s` (el mínimo era 15). Cuatro líneas `OK` (factualidad, precisión, ruido, adherencia) y `rc=0`. |
| 2 | Informe válido y números | validación jsonschema + `jq` | pasa: `schema ok`; `fake pipeline 12 10 1.0 0.0 1.0 true true true`; `per_artifact` tiene 12 |
| 3 | Las métricas se calculan | `pytest -k 'detects or threshold'` | pasa: 21 `PASSED`, 0 `SKIPPED`, 0 `FAILED`. Cubren precisión 0.7 con rc 1, falso positivo en golden, A1 por doble, A3 en plan y en reporte, A5, ruido con 0 hallazgos y precisión mínima 1.01 |
| 4 | Bucle de regresión | `add-regression` + `run` sobre una copia del dataset | pasa: rc=0, 1 directorio con los 6 elementos exigidos. Luego `run` da rc=1 y `1 12 0.2`. `git status --short agents/dataset \| wc -l` da 0 |
| 5 | Determinismo y cero red | `cmp out1 out2`; `pytest -k no_network` | pasa: `identico`; 6 `PASSED` |
| 6 | Higiene y alcance | `list-services`, `compileall -W error`, `git status`, diff fuera del filtro | pasa: `0`, `ok`, `0`, `0`. No toca dataset, planner ni reporter |

La salida de CA-1 a CA-6 coincide con la que declara la bitácora, así que la evidencia del codificador corresponde al diff actual. El tiempo de 0.2 s en `run` es plausible: son 12 artefactos con FakeLLM, y `per_artifact` muestra planner y reporter ejecutados de verdad en cada uno.

## Mis mutaciones
Hice 46 mutaciones en una copia del scratchpad, con `timeout 60` cada una y el pytest completo. Murieron 41. Dos no aplicaron por un error de mi guion, y la mutación `A2 sin ..` la repetí después y murió. De las supervivientes, una es equivalente y tres son hallazgos (F-01 y F-02).

- Cálculo de métricas: ruido por artefacto en vez de por hallazgo; `bug` sin comparar la causa; falso positivo que ignora el veredicto no-bug; denominador 0 que devuelve 1.
- Umbrales: `<=` en ruido, `>=` en precisión, adherencia laxa, constantes de 0.7, 0.25 y 0.2. Todas murieron.
- Reglas A1 a A7: A1 solo por path, A2 sin `..`, sin `//` o sin host o control, A2 con límites de flujos y pasos elevados, A3 sin tipo de error, A4 desactivada, A5 sin escanear prompts, reportes u hojas, A6 forzada a 0, A7 sin comparar datos. Todas murieron.
- Regresión y dataset: `match` en vez de `fullmatch` en el id del dataset y en el id del finding, symlinks en evidencia y en directorios, directorio fuera del dataset, ignorar `regressions/`, no conservar `reporter.response.json`, etiqueta ignorada, `labelled` como literal, rc siempre 0, línea siempre `OK`. Todas murieron.
- Sobrevivieron: `all_met` sin factualidad, factualidad que solo mira si hay algún hallazgo, y regresión fuera de `LABELLED_KINDS` (F-01 y F-02).
- Sobrevivió una equivalente: `ID_RE.match` sobre `artifact_id` en `regression.py`. No es hallazgo, porque el `arts.get(artifact_id)` posterior lo rechaza.

Ids hostiles, probados a mano en el manifest: `../x`, `a\n`, `BUG`, `a--b`, `a b`, vacío, `.hidden` y `x/../y` se rechazan. En `add-regression`, `artifact` con `\n`, `finding` con `\n` o `../../x` y `--label bug` se rechazan. Un directorio `regressions/ev il` da `error: id invalido en regressions` y rc=2. No encontré forma de escapar del dataset.

## Hallazgos
### F-01 · NARANJA · `tests/test_metrics.py:68-91` y `thresholds.py` · Factualidad: la comparación de causa y su papel en el código de salida no tienen prueba
Dos mutaciones sobrevivieron con `92 passed`:
- Reemplazar `any(_triple(f) == _rc(...))` por «el artefacto tiene algún hallazgo» en `metrics.factualidad`.
- Quitar `factualidad` de `report.all_met`.

`test_factualidad_counts_seeded_bugs_only` solo distingue «hallazgo / sin hallazgo». Ninguna prueba pone un artefacto `bug-sembrado` con veredicto `bug` y una causa raíz errada, que es el caso que hace útil a la métrica. Tampoco hay prueba de que factualidad ≤ 0.80 haga que `run` salga con 1. El umbral lo propuso el codificador y gatea el código de salida, pero ningún test fija ese comportamiento.
Arreglo: un test unitario con un hallazgo de otro `(invariant, method, path)` que dé factualidad 0, y una prueba de `run` con copia temporal en la que baje la factualidad sin bajar la precisión.

### F-02 · NARANJA · `tests/test_metrics.py:57-60` · Prueba vacua de que la regresión cuenta en precisión y `labelled`
`test_precision_ignores_unlabelled_kind_and_counts_regression` afirma `M.precision([reg]) == 0`. Con la regresión fuera de `LABELLED_KINDS` el denominador es 0 y la métrica también vale 0, así que la aserción pasa en ambos casos. La mutación sobrevivió. La decisión «regresión en precision y labelled» (11 etiquetados, precisión 10/11 en CA-4) está documentada, pero ninguna prueba la fija. Mi `run` de CA-4 dio precisión 0.9091.
Arreglo: en `test_regression.py`, con una regresión sobre la copia, afirmar `dataset.labelled == 11` y `metrics.precision == 10/11`. En el test unitario, mezclar la regresión con un acierto.

## Puntos a juzgar
- **Umbral de factualidad > 0.80 como gate.** Aceptable como propuesta. La tarea exige cuatro líneas `OK|FALLA`, y eso implica un umbral para cada métrica, aunque la prosa diga «los tres». Está declarado en `thresholds.py`, README y bitácora como «propuesto». Es una candidata de confirmación del humano, junto con adherencia `== 1.0` (decisión C-89, que la tarea ya marca). F-01 pide que el comportamiento quede fijado por una prueba.
- **Regresiones dentro de precision y labelled.** Razonable: sin eso una regresión nunca bajaría la precisión. Se aparta de la definición literal de «10 etiquetados» solo cuando existen regresiones, y está en el README. Falta la prueba (F-02).
- **`--finding f1` frente a `flow-1`: acepto el aviso.** La tarea fija `f1` en CA-4 y exige rc=0, así que no se puede exigir existencia. El aviso sale por stderr y el id queda en `meta.source_finding`. El límite es que `--finding` es solo metadato: la regresión etiqueta el artefacto entero, no identifica un hallazgo. No oculta un fallo, pero conviene que el humano lo sepa (candidata).
- **`-e .` en `requirements-dev.txt`: aceptable y necesario.** Sin él `python -m agent_eval` no resuelve desde `agents/eval`, por el layout `src/`. Las versiones coinciden con T01: el único diff contra `agent-reporter/requirements-dev.txt` son las tres líneas `-e`.
- **`meta.kind = regresion-fp`.** El `report.schema.json` propio sí admite `regresion-fp`: mi informe de CA-4 validó. Lo que no valida es el `meta.json` de la regresión contra `agents/dataset/schema/meta.schema.json` (T01): fallan el enum de `kind` y `additionalProperties` por `source_artifact` y `source_finding`. Hoy nadie lo valida: los tests de T02 y T04 recorren solo el manifest. La tarea manda `kind = "regresion-fp"` y prohíbe tocar el dataset, así que no es defecto de esta ronda. Es tarea candidata.
- **Circularidad de A5.** Se confirma: el detector es el redactor del reporter. Un tipo de secreto que el redactor no conoce no se detecta. Hay pruebas de que el detector funciona (secreto sembrado, redacción rota). Está declarado en el README y en la bitácora. Tarea candidata.
- **Aviso C-85** (`llm: fake`, `valid_for: pipeline`): presente en el README, en el informe (como literales del esquema más `note`) y en la salida de `run`.
- **Alcance:** diff limpio, sin cambios en `agents/dataset`, `agent-planner` ni `agent-reporter`. No hay Dockerfile y `list-services` no lista `agents/eval`.

## Hallazgos menores (AMARILLO, no bloquean)
- La bitácora enumera las mutaciones que mueren con el nombre de la prueba, pero no pega la salida roja que pide el plan de pruebas.
- `add-regression` permite regresionar un artefacto sin hallazgos (por ejemplo un golden) con un `--finding` inexistente, y solo avisa. Tampoco rechaza duplicados del mismo artefacto y hallazgo: crea `-fp-2`.

## Tareas candidatas (fuera de alcance)
- Ampliar `agents/dataset/schema/meta.schema.json` para admitir `regresion-fp` y los campos `source_*`, y validar `regressions/*` en los tests del dataset.
- Patrones de secreto independientes del redactor del reporter para A5 (hoy es circular).
- Que `--finding` identifique el hallazgo concreto y se valide contra los `flow_id` o `finding_id` reales.
- CI para `agents/eval` (ya anotada en la tarea).
- Medición con modelo real (T07).
- Confirmación humana de los umbrales propuestos: factualidad `> 0.80` y adherencia `== 1.0`.

VEREDICTO: NO-VERDE
NARANJA|tests/test_metrics.py:68-91 + report.py all_met|Factualidad: comparación de causa raíz y su efecto en el código de salida sin prueba (2 mutantes sobreviven)
NARANJA|tests/test_metrics.py:57-60|Prueba vacua (denominador 0) de que la regresión cuenta en precision/labelled; mutante sobrevive
