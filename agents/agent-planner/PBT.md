# PBT en agent-planner (PBT-09)

Extension Property-Based Testing en modo parcial (PBT-02, 03, 07, 08, 09). U3-T06.

## Framework

**Hypothesis 6.122.3** (fijado en `requirements-dev.txt`, junto a `pytest==8.3.4`). Se eligio porque el
servicio es Python, porque reduce (shrink) cada contraejemplo a un caso minimo y porque guarda una base
de ejemplos fallidos (`.hypothesis/`, ignorada por git) que se reejecutan primero.

## Que se prueba

- `tests/test_pbt_planner.py`: round-trip del FlowPlan y del script k6 (PBT-02); invariantes del
  validador, del prompt y del tope de entrada (PBT-03); `test_pbt_generator_coverage`.
- `tests/gen.py`: generadores de dominio (run_id, flow_id con variantes invalidas, path hostil,
  superficie, plan, respuesta del modelo). Hay una copia hermana en `agent-reporter/tests/gen.py`.
- `tests/test_pbt_demo.py`: propiedad falsa a proposito (`-m pbt_demo`, fuera del pytest normal).

## Como correr

    cd agents/agent-planner
    .venv/bin/python -m pytest -k pbt -m 'not pbt_demo' -v     # propiedades (sin -q se ve el seed)
    .venv/bin/python -m pytest -m pbt_demo                      # demo de contraejemplo reducido

Cada propiedad corre 200 ejemplos (perfil `aqs` en `tests/conftest.py`, `deadline=None`, `print_blob=True`).
El shrinking esta activo; no se restringen `phases`.

## Como reproducir un fallo

La cabecera de la sesion imprime `hypothesis seed: <n>`. Con ese numero:

    .venv/bin/python -m pytest -k pbt --hypothesis-seed=<n>     # tambien vale PBT_SEED=<n>

Prioridad del seed: `--hypothesis-seed`, luego `PBT_SEED`, y si no uno aleatorio. Tambien se puede pegar
el `@reproduce_failure(...)` que Hypothesis imprime con el contraejemplo.

## Secretos

Ningun generador contiene cadenas con forma de secreto; si hicieran falta, se arman por concatenacion.
