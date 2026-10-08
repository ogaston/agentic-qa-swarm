# Ronda 1 — U3-T02

VEREDICTO: NO-VERDE

Los 6 criterios de aceptación pasan con mis propias ejecuciones. El motivo del NO-VERDE es un hallazgo NARANJA de la validación de `flow_id` y otro NARANJA por un literal incrustado. No encontré ningún ROJO.

## Criterios de aceptación, verificados por mí
Los corrí desde el worktree (sha 1868a027c0b6d1bcd02dd436ffafd75fc06fe787), con el `.venv` de T01 y fresco.

| # | Criterio | Resultado |
|---|---|---|
| 1 | `pytest -q` y `-k dataset` | `83 passed in 5.11s` (≥35), dataset PASSED = 17 (≥12). Los 12 artefactos están parametrizados: 10 con plan y 2 `noarranca` → `NoSurface` con `calls == 0`. |
| 2 | `-k injection` | 4 pruebas distintas, 6 casos, todos PASSED, ningún SKIPPED. Cubren prefijo de instrucciones idéntico byte a byte, un solo cierre de bloque, endpoint no observado y método/workflow/run_id secuestrados. |
| 3 | `-k 'budget or timeout'` + grep | 9 PASSED, grep = 3. Cubren entrada enorme con `calls == 0` y salida declarada por encima del tope. Cubren latencia mayor que `timeout_s`, `max_tokens`/`timeout_s` entregados al cliente y ausencia de reintento. |
| 4 | `-k server` + arranques | 7 PASSED. `PLANNER_ENV=prod` + fake → `rc=2` con mensaje en stderr. `LLM_PROVIDER=otro` → `rc=2` y «proveedor no implementado». |
| 5 | `-k no_leak` | PASSED. |
| 6 | Imagen e higiene | Resultados abajo. |

Resultados del criterio 6:
- Usuario de la imagen: `65532:65532`.
- `import pytest` dentro de la imagen: `ModuleNotFoundError`.
- `list-services.sh` → 1.
- `:latest` o FROM sin pin → 0.
- `compileall -W error` → `ok`.
- `git status --short | wc -l` → 0.
- Archivos fuera de alcance contra el merge-base `2548161` → 0.

Comprobaciones adicionales:
- En la imagen, el servidor arranca con `fake` sin ser root. Los esquemas empaquetados se cargan, `/v1/plan` responde `502 llm_unavailable` (fake sin respuestas registradas) y `/metrics` cuenta ese resultado.
- La prueba de igualdad byte a byte de `src/agent_planner/schemas/` con `contracts/plans/` pasa. Con `cmp` también son idénticos.

## Mutaciones (copia en el scratchpad, `timeout 60`)
Con una excepción, cada mutante pone al menos una prueba en rojo.
- Las 5 de la tarea: sin chequeo (a) de endpoints → rojo; sin tope de entrada → 4 rojos; sin `max_tokens` → 1 rojo; fake en prod → 2 rojos; recorte de flujos → 1 rojo.
- Otras 12 mías, todas en rojo:
  - quitar el escape de `<`, el regex de `flow_id`, la unicidad de `flow_id`, `max_steps`, el chequeo de `run_id`, el de `workflow`, el rechazo de superficie vacía, la validación de la superficie, el mapeo `LLMTimeout`→`time`, el chequeo de salida declarada y la validación del esquema `FlowPlan`;
  - chequear solo la ruta sin el método.
- También en rojo: no pasar `timeout_s`, quitar `PLANNER_ALLOW_FAKE` y aceptar topes no positivos.
- La mutación del `detail` del servidor no la cuento: mi sed no introducía una fuga real, así que no demuestra nada.

## Hallazgos

### F-01 · NARANJA · `src/agent_planner/validate.py:10,40` · `FLOW_ID_RE` con `^...$` y `.match` acepta un salto de línea final
Es el patrón que el coordinador pidió comprobar.

- **Demostración:**
  - Con `flow_id = "f1\n"` mediante `plan(SURFACE, "wf-1", llm_for(...), Limits())` obtuve `'f1\n' ACEPTADO 'f1\n'`.
  - `$` coincide antes de un `\n` final y `.match` no ancla el final.
  - La tarea exige (c) la forma `^[a-z0-9]([a-z0-9-]{0,40}[a-z0-9])?$` porque U2 usa `flow_id` en el nombre de un Job. Un `\n` en ese nombre es el valor que se quería impedir.
- **Prueba que falta:**
  - `test_validate_flow_id_invalid` (`tests/test_validate.py:64`) cubre `Flow`, `-a`, `a-`, `a_b`, `a*43`, vacío y `a b`, pero no `"f1\n"`.
  - Por eso ningún mutante lo detecta.
- **Corrección esperada:**
  - Usar `re.fullmatch` con `\Z` o sin `$`.
  - Añadir a la parametrización `"f1\n"`, `"\nf1"` y `"a\n"`.
- **Barrido de clase:**
  - `server.py:16,46`: `_SAFE_ID = ^[A-Za-z0-9._-]{1,64}$` con `.match` deja pasar `"run\n"` como `run_id` seguro. Es inocuo (el log usa `json.dumps`, que escapa el `\n`), pero conviene corregirlo igual (AMARILLO).
  - No hay más `re.compile` ni `.match` fuera de los esquemas.
  - Los `pattern` de los esquemas JSON (`^/`) no tienen el problema, porque `jsonschema` usa `re.search` y `^/` no depende del final.

### F-02 · NARANJA · `src/agent_planner/planner.py:20` · Literal 128 no documentado en un punto de decisión
- `len(workflow) > 128` → `InvalidRequest("workflow_invalido")` → `422` visible para el usuario.
- La tarea no fija ese tope ni el README lo menciona; está incrustado en el código.
- Corrección esperada: constante con nombre o `PLANNER_MAX_WORKFLOW_LEN`, y documentarla en la tabla del README.
- Menor, mismo tipo (AMARILLO): `timeout = 10` del Handler y `MAX_BODY` (este sí viene de la tarea) tampoco aparecen en el README.

### F-03 · AMARILLO · `tests/helpers.py:to_flow_plan` · Adaptación del dataset
- `to_flow_plan` fija `expect_status=200` y `name=id`, y une las `invariants` con `"; "`. El plan «esperado» sale de la misma conversión que se pasa por el `FakeLLM`.
- Aun así, el planner valida y devuelve ese plan, el esquema y la cobertura de `must_cover_invariants` se comprueban, y la segunda corrida es idéntica byte a byte. La tarea prohíbe tocar `agents/dataset`, porque CA-6 limita el diff a `agents/agent-planner/`. Mi juicio: es aceptable y está bien documentada en la bitácora.
- Candidata: alinear `planner.response.json` al esquema `FlowPlan`.

## Juicio sobre los puntos que pidió evaluar el orquestador
- **Esquemas copiados con prueba byte a byte:** correcto. El contexto de build es solo `agents/agent-planner`, el `contracts/**` no se toca y la deriva queda vigilada. Mejora posible (AMARILLO): que la CI también ejecute esa prueba.
- **Escape manual de `<`:** correcto y verificado. `json.dumps(...).replace("<", "\\u003c")` conserva el JSON válido, incluso con una barra invertida antes de `<`. Quitar el escape pone la prueba en rojo.
- **`pyproject.toml`:** la edición era necesaria y está dentro de `agents/agent-planner/`.
  - Añade la dependencia `jsonschema==4.23.0` (alineada con `requirements.txt`) y `package-data` para los `.json`.
  - `addopts` está intacto (el diff no lo toca).
- **Imagen:** `python:3.12-slim@sha256:2b4f…4ffe`, usuario 65532, `pip` con versiones exactas, `.dockerignore` excluye `tests` y `.venv`. Sin pytest en la imagen y arranca funcional.
- **Alcance:** sin desborde, sin cambios en `contracts`, `deploy`, `policy` ni workflows.
- **Worktree:** limpio tras mis ejecuciones.

## Tareas candidatas
- Alinear `agents/dataset/**/planner.response.json` al esquema `FlowPlan` (`flow_id`, `name`, `invariant`, `expect_status`).
- Ampliar el contrato `FlowPlan`/`SurfaceArtifact` (cuerpos, parámetros). La tarea ya lo anota.
- Revisar el mismo patrón `^...$` + `.match` en U2 (el consumidor del `flow_id`), si usa un regex equivalente.

VEREDICTO: NO-VERDE
NARANJA|src/agent_planner/validate.py:10,40|FLOW_ID_RE con .match acepta "f1\n" (sin prueba que lo cubra; barrido: server.py _SAFE_ID)
NARANJA|src/agent_planner/planner.py:20|Literal 128 para la longitud de workflow, no documentado
