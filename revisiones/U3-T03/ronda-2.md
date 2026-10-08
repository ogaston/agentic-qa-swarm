# Ronda 2 — U3-T03

VEREDICTO: NO-VERDE

Revisé el sha 876e26e5079295cd7eb684ded677603dd3a69f79. F-01, F-02, F-03 y F-04 quedan corregidos en el código y verificados por mí. El barrido de la clase destapó un hueco de cobertura en `DirFlowStore` (F-05, NARANJA), y eso mantiene el veredicto NO-VERDE. El worktree quedó limpio (`git status --short | wc -l` = 0), y las mutaciones las hice en una copia en el scratchpad.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Pruebas en verde, capa 3 con k6 real, sin SKIP | `pytest -q -m 'not pbt_demo' tests/k6 -rs` | pasa: `53 passed in 3.58s`, sin skipped |
| 2 | `k6 inspect` real acepta los generados y rechaza el roto | `render-examples`, bucle `podman … inspect`, `sed` roto | pasa: 24 scripts, 24 con `rc=0`, `roto rc=107` |
| 3 | `k6 run` real | `-k k6_run` | pasa: 3 `PASSED`, 0 `SKIPPED`. Los casos de fallo ahora afirman `== 99` |
| 4 | Sin inyección de código | `-k 'k6 and (injection or forbidden)'` y `grep -c` | pasa: 24 + 5 `PASSED` (con `test_k6_forbidden_filter_isolated`), `grep` imprime `2` |
| 5 | Todo o nada, validación antes de publicar, lectura de vuelta | `-k publish` | pasa: 15 `PASSED`, 0 `SKIPPED` |
| 6 | Higiene y alcance | `compileall -W error`, suite completa, `git status`, `git diff --name-only $(merge-base)` | pasa: `70 passed`, `0` y `0`. `pyproject.toml` no aparece en el diff |

## Verificación de los hallazgos de la ronda 1
| Hallazgo | Estado | Evidencia |
|---|---|---|
| F-01 (`$` acepta `\n`) | corregido | `ID_RE` y `KEY_RE` sin anclas y con `fullmatch` en `store.py` y `validate.py`. Con `"f1\n"`, `"f1\r\n"` y `run_id "run\n"`, tanto `FakeFlowStore` como `DirFlowStore` lanzan `FlowValidationError` con capa `plan`, y el almacén queda vacío. Mutación `fullmatch`→`match` en `validate.py`: 6 pruebas rojas. Barrido de `$`/`.match(` en `k6/*.py`: no queda ninguno |
| F-02 (reintento destruye lo publicado) | corregido | Se guarda `prev = store.get(key)` y se restaura con `put(prev)`, o se borra si la clave era nueva. Sonda propia con `DirFlowStore` (fallo en el 3.er `put`): quedan los bytes anteriores intactos. Mutaciones "restaurar→borrar" y "sin restauración": `test_publish_republish_failure_keeps_previous_version` roja en ambas |
| F-03 (`FORBIDDEN` sin prueba) | corregido | `test_k6_forbidden_filter_isolated` parchea el esqueleto y el hash para llegar al filtro. Con `FORBIDDEN` apagado, o quitando solo el patrón `eval`, la prueba se pone roja |
| F-04 (`tempfile` sin usar, `!= 0`) | corregido | El import se eliminó, y los dos casos de `k6 run` ahora afirman `== 99` |

## Hallazgos nuevos (barrido de la clase «almacén y anclado de regex»)
### F-05 · NARANJA · `tests/k6/test_k6_flows.py:173-177`, `store.py:19,33` · La defensa de `DirFlowStore` contra claves hostiles no tiene cobertura de las anclas
Mutación: cambiar `fullmatch` por `match` en `store.py` (afecta a `flow_key` y a `DirFlowStore._path`) deja `53 passed`. La prueba `test_publish_dir_store_rejects_traversal` solo usa `"flows/../x/a.k6.js"` y `"flows/Run/a.k6.js"`, que se rechazan aunque el patrón no esté anclado al final. Con el mutante, `DirFlowStore.put` **acepta** `flows/a/b.k6.js/../../../escape.txt` y `flows/a/b.k6.js\n`. El código real rechaza ambas con `ValueError`, que también rechaza `flows/a\n/b.k6.js`. Es exactamente la clase de F-01 (ancla y salto de línea), pero a nivel de puerto, donde `DirFlowStore.put` es API pública y la tarea exige explícitamente claves sin `/`, `..` ni mayúsculas.

Arreglo esperado: añadir al test de traversal directo del store los casos `"flows/a/b.k6.js/../../x"`, `"flows/a/b.k6.js\n"` y `"flows/a\n/b.k6.js"`, y un caso `flow_key("run\n", "f1")` que lance `ValueError`.

## Hallazgos AMARILLO (no bloquean)
- Si el almacén sigue fallando durante el rollback, `publish.py` traga la excepción (`except Exception: pass`). Sonda: con un almacén "muerto" tras el 2.º `put`, queda `f1` nuevo con `f2` y `f3` antiguos y solo se propaga el error original, sin indicar que el rollback también falló. Conviene anotar el fallo del rollback, por ejemplo con `add_note` o encadenándolo, para que T07 no lo pierda.
- La restauración depende de que `get` de una clave ausente lance `KeyError` o `FileNotFoundError`. Un almacén que use otra excepción (sonda con `LookupError`) hace fallar la publicación inicial. El `Protocol` `FlowStore` no documenta ese contrato. Anotarlo en el docstring del puerto, o dejarlo para T07 (S3).

## Tareas candidatas (heredadas de la ronda 1, siguen vigentes)
- `jsonschema` se importa en el código de producción, pero solo figura en `requirements-dev.txt` y `pyproject.toml` tiene `dependencies = []` (no se puede declarar aquí).
- `SCHEMA_PATH` y `examples.ROOT` usan `parents[5]`, que asume la disposición del monorepo.
- Que U2 ejecute k6 en lugar de `HTTPStepsExecutor`, y que la CI instale k6 con versión y digest fijados.

VEREDICTO: NO-VERDE
NARANJA|agents/agent-planner/tests/k6/test_k6_flows.py:173-177; store.py:19,33|DirFlowStore: la mutación fullmatch->match sobrevive (53 passed) y acepta `flows/a/b.k6.js/../../../x` y claves con `\n`; falta cobertura de anclas a nivel de puerto
