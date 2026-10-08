# Ronda 3 — U3-T03

VEREDICTO: VERDE

Revisé el sha aef381c6354951b2dc43c3ccb7f8e1225dece855 (`git rev-parse HEAD`). Corrí CA-1..CA-6 yo mismo y todos pasan. No queda ningún ROJO ni NARANJA en pie. El worktree quedó limpio (`git status --short | wc -l` = 0). Las mutaciones las hice en una copia en el scratchpad.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Pruebas en verde, capa 3 con k6 real, sin SKIP | `pytest -q -m 'not pbt_demo' tests/k6 -rs` | pasa: `68 passed in 3.78s`, sin skipped |
| 2 | `k6 inspect` real acepta los generados y rechaza el roto | `render-examples`, bucle `podman … inspect`, `sed` roto | pasa: 24 scripts, 24 con `rc=0`, `roto rc=107` |
| 3 | `k6 run` real | `-k k6_run` | pasa: 3 `PASSED`, 0 `SKIPPED` (los casos de fallo afirman `== 99`) |
| 4 | Sin inyección de código | `-k 'k6 and (injection or forbidden)'` y `grep -c` | pasa: 24 + 5 `PASSED`, `grep` imprime `2` |
| 5 | Todo o nada, validación antes de publicar, lectura de vuelta | `-k publish` | pasa: 24 `PASSED`, 0 `FAILED`, 0 `SKIPPED` |
| 6 | Higiene y alcance | `compileall -W error`, suite completa, `git status`, `git diff --name-only $(merge-base)` | pasa: `85 passed`, `0` y `0`. `pyproject.toml` no aparece en el diff |

## Verificación de lo pendiente de la ronda 2
**F-05 (anclas de regex sobre `DirFlowStore` y `flow_key`): corregido.** Cambié cada uso de `fullmatch` por `match`, uno por uno:
| Mutación | Resultado |
|---|---|
| `flow_key`, `run_id` | roja: `test_flow_key_rejects_hostile_ids[run\n-f1]` |
| `flow_key`, `flow_id` | 2 rojas: `[run-f1\n]` y `[run-a/b]` |
| `DirFlowStore._path` (cubre `put`/`get`/`delete`) | 3 rojas: `…/../../x`, `…\n` y `…js/` |
| `validate_plan`, `run_id` | 2 rojas |
| `validate_plan`, `flow_id` | 3 rojas |

`test_publish_dir_store_hostile_keys_all_ops` ejercita `put`, `get` y `delete` con 8 claves hostiles (`..`, mayúsculas, `/../..` al final, `\n` al final y dentro de un segmento, `/` final, `/` inicial y prefijo extra), exige `ValueError` y verifica que no se crea ningún archivo. En el código real hay exactamente 4 usos de `fullmatch` y ninguno de `.match(` ni de `$` en `k6/*.py`.

**AMARILLO 1 (nota del rollback): corregido.** `publish.py` anota con `err.add_note("rollback fallo para …")` en lugar de tragar la excepción. `requires-python >=3.12` cubre `add_note`. Quitar `add_note` pone en rojo `test_publish_rollback_failure_is_noted`.

**AMARILLO 2 (contrato de `get` para clave ausente): corregido.** El docstring de `FlowStore` fija que `get` de una clave ausente debe lanzar `KeyError` o `FileNotFoundError`.

**Barrido de la clase (almacén, anclas, idempotencia).** Sin hallazgos nuevos. Siguen verdes y con su mutación roja:
- la restauración de bytes previos (`test_publish_republish_failure_keeps_previous_version`),
- la lectura de vuelta,
- la limpieza de parciales,
- la idempotencia en `DirFlowStore`.

## Tareas candidatas (fuera de alcance, heredadas)
- `jsonschema` se importa en el código de producción, pero solo figura en `requirements-dev.txt` y `pyproject.toml` tiene `dependencies = []`. Declararlo en T07.
- `SCHEMA_PATH` y `examples.ROOT` usan `parents[5]`, que asume la disposición del monorepo. Resolverlo al cablear en T07.
- Que U2 ejecute k6 en lugar de `HTTPStepsExecutor`, y que la CI instale k6 con versión y digest fijados.
- Con `S3`, confirmar en T07 que `get` de una clave ausente cumple el contrato `KeyError`/`FileNotFoundError` del puerto.

VEREDICTO: VERDE
