# Ronda 1 — U3-T03

VEREDICTO: NO-VERDE

Revisé el sha 2e97210a0e4fbfe0a2afe74c4b1cce1fc57f8ae0 (rama tarea/U3-T03). Los seis criterios pasan en mi ejecución. Quedan dos hallazgos NARANJA que no rompen ningún criterio: una validación de ids que deja pasar un salto de línea, y un reintento fallido que borra lo ya publicado. El worktree quedó limpio (`git status --short | wc -l` = 0). Las mutaciones las hice en una copia en el scratchpad.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Pruebas en verde, capa 3 con k6 real, sin SKIP | `pytest -q -m 'not pbt_demo' tests/k6 -rs` | pasa: `45 passed in 3.58s`, sin skipped. Podman y la imagen 0.55.0 están presentes, y los 3.6 s son coherentes con 6 pruebas de contenedor |
| 2 | `k6 inspect` real acepta los generados y rechaza el roto | `render-examples`, bucle `podman … inspect`, `sed` roto | pasa: 24 scripts, 24 con `rc=0`, `roto rc=107` |
| 3 | `k6 run` real: pasa si coincide, falla si no | `-k k6_run` | pasa: 3 `PASSED`, 0 `SKIPPED`. Comprobé a mano contra un servidor loopback: estado distinto da `rc=99` (umbral `checks`) y sin `BASE_URL` da `rc=99` |
| 4 | Sin inyección de código | `-k 'k6 and (injection or forbidden)'` y `grep -c` | pasa: 24 + 4 `PASSED`, `grep` imprime `2`. Un script real con `"`, U+2028, `*/`, `</script>`, salto de línea y `'); fail('x` pasa `k6 inspect` con `rc=0` |
| 5 | Todo o nada, validación antes de publicar, lectura de vuelta | `-k publish` | pasa: 8 `PASSED`, 0 `SKIPPED` |
| 6 | Higiene y alcance | `compileall -W error`, suite completa, `git status`, `git diff --name-only $(merge-base)` | pasa: `62 passed`, `0` y `0` |

Alcance: el diff son 13 archivos, todos en `agents/agent-planner/{src/agent_planner/k6,tests/k6}` más la bitácora. No toca `pyproject.toml`, `planner.py`, `server.py`, el `Dockerfile`, `contracts/**` ni los workflows (`git diff 2548161 HEAD -- pyproject.toml` está vacío). No hay desborde.

Mutaciones (copia, `timeout 60`, cada una restaurada después):
| Mutación | Resultado |
|---|---|
| interpolar sin JSON | 3 pruebas rojas |
| omitir capa 3 | rojo (`test_publish_second_flow_rejected_by_k6_publishes_nothing`) |
| publicar antes de validar todos | 3 rojas |
| no borrar parciales | 2 rojas |
| invertir el orden de los pasos | 24 rojas |
| quitar el hash de la plantilla | 1 roja |
| quitar la comprobación de lectura de vuelta | 1 roja |
| quitar la comparación del esqueleto | 1 roja |
| quitar las cadenas prohibidas (`FORBIDDEN`) | **0 rojas** (ver F-03) |

Sobre lo que pidió juzgar el orquestador:
- **Planes derivados del dataset en `examples.py`** (`expect_status` 201 para POST, 200 para el resto): lo acepto. Solo alimenta `render-examples` y los ejemplos del round-trip, y `planner.response.json` no trae `FlowPlan`. Cada plan derivado valida contra el esquema.
- **Defecto de `BASE_URL` ausente**: corregido con un check fallido antes de `fail()`, y verificado con `rc=99` real.
- **`--user 0:0`**: lo acepto. Es root dentro de un contenedor rootless (el uid del host), y la capa 3 corre con `--network none`.
- **`FlowStore` con `delete`**: lo acepto, el cleanup de parciales lo necesita. Ver F-02.
- **`ID_RE` replicado de T02**: la expresión es idéntica a la de T02. El defecto está en cómo se aplica (F-01).

## Hallazgos
### F-01 · NARANJA · `store.py:8,19`, `validate.py:29,33` · `ID_RE` acepta un salto de línea final y se salta la capa 1
El patrón termina en `$` y se usa con `.match`. En Python `$` coincide antes de un `\n` final, así que `"f1\n"` y `"run-001\n"` pasan la capa 1 y `flow_key`. Lo comprobé con un script (`probe.py` en el scratchpad):
- `flow_id="f1\n"` → `FakeFlowStore` **acepta** y publica la clave `flows/run-001/f1\n.k6.js`.
- `run_id="run-001\n"` → `FakeFlowStore` acepta y publica `flows/run-001\n/f1.k6.js`.
- Con `DirFlowStore`, el mismo plan lanza un `ValueError` crudo desde `put`, no `FlowValidationError(flow_id, capa)`. El rechazo llega tarde, en la fase de escritura, y no por la validación previa que exige la tarea.

El script temporal también se escribe con `f"{fid}.k6.js"` usando ese id. La tarea pide claves derivadas solo de ids "ya validados con el patrón de T02 (sin `/`, `..` ni mayúsculas)", y esa defensa no se cumple para el carácter `\n`. T02 usa la misma expresión en U2 para nombrar un Job, así que el defecto se propagaría.

Arreglo esperado: usar `fullmatch` o `\Z` en `ID_RE` y `KEY_RE`, y añadir una prueba con `"f1\n"` y `"run\n"` (hoy `test_publish_bad_flow_id_rejected` solo prueba `"../x"`).

### F-02 · NARANJA · `publish.py:34-48` · Un reintento de publicación fallido destruye la versión ya publicada
El cleanup borra todas las claves que intentó escribir, también las que ya existían de una publicación anterior. Lo reproduje con `FakeFlowStore`: publiqué `tres-flujos` (f1, f2, f3), hice fallar el 2.º `put` del reintento, y el almacén quedó solo con `flows/run-001/f3.k6.js`. Pasó de 3 objetos a 1 parcial, que es lo que "todo o nada" debe evitar. La tarea dice literalmente "se borran los ya escritos", así que el codificador siguió el texto. Aun así, el almacén queda en un estado parcial peor que antes del intento.

Arreglo esperado, cualquiera de los dos:
- restaurar los bytes previos (leer el valor antiguo antes de sobrescribir y reponerlo al fallar), o
- escribir solo las claves nuevas y documentar la semántica.

Añadir una prueba de "republicar con fallo conserva la versión anterior". Si el orquestador considera que el texto de la tarea lo cubre, puede degradarse a AMARILLO.

### F-03 · AMARILLO · `validate.py:17-20,54-57` · `FORBIDDEN` es código sin prueba que lo ejerza
Con `if False:` en lugar de `pat.search(outside)`, la suite sigue en `45 passed`. La comparación exacta del esqueleto (que tiene su propia mutación roja) ya rechaza cualquier inyección. `test_k6_forbidden_patterns_rejected` pasa por esa vía y no demuestra que el filtro por cadenas funcione. O se elimina `FORBIDDEN`, o se prueba aislado, parcheando el esqueleto para que coincida y comprobando que el patrón rechaza.

### F-04 · AMARILLO · varios
- `validate.py:5` importa `tempfile` sin usarlo.
- `test_k6_run_fails_when_status_differs` y `test_k6_run_fails_without_base_url` solo afirman `!= 0`. Un fallo de podman también daría verde. La prueba de la rama que pasa mitiga el riesgo, pero conviene afirmar `== 99`.

## Tareas candidatas (defectos reales fuera de alcance)
- `jsonschema` se importa en `agent_planner.k6.validate` (código de producción), pero solo figura en `requirements-dev.txt`, y `pyproject.toml` tiene `dependencies = []`. No se puede declarar aquí porque `pyproject.toml` está fuera de alcance. Declararlo cuando se cablee en U3-T07.
- `SCHEMA_PATH` y `examples.ROOT` usan `parents[5]`, que asume la disposición del monorepo. En un paquete instalado o en el contenedor, `contracts/` no estará. Resolver al cablear en T07 (por ejemplo, copiar el esquema o pasar la ruta por configuración).
- Heredado de la tarea: instalar k6 con versión y digest fijados en CI, y que U2 ejecute k6 en lugar de `HTTPStepsExecutor`.

VEREDICTO: NO-VERDE
NARANJA|agents/agent-planner/src/agent_planner/k6/store.py:8,19; validate.py:29,33|ID_RE acepta salto de línea final (flow_id/run_id "x\n" pasan la capa 1; FakeFlowStore publica la clave; DirFlowStore lanza ValueError tardío)
NARANJA|agents/agent-planner/src/agent_planner/k6/publish.py:34-48|Republicar con fallo borra la versión ya publicada y deja el almacén parcial (3 objetos pasan a 1)
AMARILLO|agents/agent-planner/src/agent_planner/k6/validate.py:17-20|FORBIDDEN sin prueba que lo ejerza (la mutación sobrevive), y `tempfile` sin usar
