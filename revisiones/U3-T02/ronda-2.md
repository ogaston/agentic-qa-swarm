# Ronda 2 — U3-T02

VEREDICTO: VERDE

Revisé el sha bf1eb1109e941189ec22458a2734a65b4edd7514. Corrí yo mismo los 6 criterios de aceptación y ambos hallazgos de la ronda 1 están corregidos. No queda ningún ROJO ni NARANJA.

## Criterios de aceptación, verificados por mí
| # | Resultado |
|---|---|
| 1 | `pytest -q`: `88 passed in 5.16s` (≥35). `-k dataset`: 17 PASSED (≥12). |
| 2 | injection: 4 pruebas distintas, 6 casos PASSED, ningún SKIPPED. |
| 3 | budget/timeout: 9 PASSED. El grep de variables del README devuelve 3. |
| 4 | server: 8 PASSED. Arranque con fake en prod → `rc=2`. `LLM_PROVIDER=otro` → `rc=2` con «proveedor no implementado». |
| 5 | no_leak: PASSED. |
| 6 | Imagen `65532:65532`. `import pytest` → `ModuleNotFoundError`. `list-services` → 1. `:latest` o FROM sin pin → 0. `compileall -W error` → `ok`. `git status` → 0. Archivos fuera de alcance → 0. |

`addopts` sigue intacto en `pyproject.toml:25`. Entre la ronda 1 y esta, el diff de `pyproject.toml` y `Dockerfile` es 0.

## Hallazgos de la ronda anterior
### F-01 (flow_id con `\n` final): corregido
- `validate.py:10,40`: `FLOW_ID_RE` quedó sin `^`/`$` y se usa con `fullmatch`.
- `server.py:17,47`: `_SAFE_ID` quedó igual, con `fullmatch`.
- Comprobación directa:
  - `FLOW_ID_RE.fullmatch` rechaza `"f1\n"`, `"\nf1"`, `"a\n"` y `"f1\r"`.
  - Acepta `"f1"` y `a`×42, y rechaza `a`×43.
  - `_safe_run_id` devuelve `desconocido` para `"run-1\n"` y para `"a\r\nb"`, y `run-1` para `"run-1"`.
- Pruebas nuevas:
  - `"f1\n"`, `"\nf1"` y `"a\n"` en `test_validate_flow_id_invalid`.
  - `test_server_safe_run_id_rejects_trailing_newline`.
- Mutaciones mías, cada una sobre una copia en el scratchpad:
  - `FLOW_ID_RE` con `^…$` y `.match`: 2 rojos (`[f1\n]`, `[a\n]`).
  - `_SAFE_ID` con `^…$` y `.match`: 1 rojo (`test_server_safe_run_id_rejects_trailing_newline`).

### F-02 (literales incrustados): corregido
- Constantes con nombre: `MAX_WORKFLOW_LEN = 128` (`planner.py:13`), `HANDLER_TIMEOUT_S = 10` (`server.py:15`), y `MAX_BODY` ya estaba definida.
- La tabla del README ahora documenta las tres.
- Prueba nueva: `test_validate_workflow_too_long_is_invalid_request`.
- Mutación mía, quitar el tope de `workflow`: 2 rojos.

### F-03: aceptado sin cambio
La adaptación `to_flow_plan` del dataset sigue como candidata. Está bien documentada en la bitácora.

## Barrido de clase
Revisé todo `src/` en busca de regex y literales numéricos.
- No queda ninguna regex con `^…$` fuera de los esquemas JSON, y esos usan `re.search` con `^/`, que no tiene el problema.
- Los literales restantes son:
  - los defectos de `Limits` y `config.py`, ya documentados como variables en el README;
  - los códigos HTTP;
  - el `{1,64}` de `_SAFE_ID`, que solo filtra qué `run_id` se escribe en el log y no se devuelve al usuario.
- Un punto menor que no bloquea (AMARILLO): los defectos numéricos están duplicados en `Limits` y `config.py`. Conviene unificarlos más adelante.
- El worktree quedó limpio tras mis ejecuciones.

## Tareas candidatas
- Alinear `agents/dataset/**/planner.response.json` al esquema `FlowPlan`.
- Ampliar el contrato `FlowPlan`/`SurfaceArtifact` (cuerpos, parámetros).
- Revisar en U2 el mismo patrón `^…$` con `.match` en el consumidor de `flow_id`.
- Que la CI ejecute la prueba de igualdad de esquemas copiados.

VEREDICTO: VERDE
