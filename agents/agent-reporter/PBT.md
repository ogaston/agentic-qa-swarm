# PBT en agent-reporter (PBT-09)

Extension Property-Based Testing en modo parcial (PBT-02, 03, 07, 08, 09). U3-T06.

## Framework

**Hypothesis 6.122.3** (fijado en `requirements-dev.txt`, junto a `pytest==8.3.4`). Se eligio porque el
servicio es Python, porque reduce (shrink) cada contraejemplo a un caso minimo y porque guarda una base
de ejemplos fallidos (`.hypothesis/`, ignorada por git) que se reejecutan primero.

## Que se prueba

- `tests/test_pbt_reporter.py`: round-trip del Report (PBT-02); `redact_secrets` (ningun secreto
  sembrado sobrevive, idempotencia, texto limpio sin cambios, crecimiento acotado), validador del
  reporter y recorte de evidencia (PBT-03); `test_pbt_generator_coverage`.
- `tests/gen.py`: generadores de dominio (evidencia, result.json, logs con y sin instrucciones,
  respuestas del modelo, `secret(tipo)`, report). Copia hermana en `agent-planner/tests/gen.py`.
- `tests/test_pbt_demo.py`: propiedad falsa a proposito (`-m pbt_demo`, fuera del pytest normal).
- `tests/test_redact_regression.py`, `tests/test_truncate_regression.py`: ejemplos fijos de los
  defectos que las propiedades hallaron.

## Como correr

    cd agents/agent-reporter
    .venv/bin/python -m pytest -k pbt -m 'not pbt_demo' -v     # propiedades (sin -q se ve el seed)
    .venv/bin/python -m pytest -m pbt_demo                      # demo de contraejemplo reducido

Cada propiedad corre 200 ejemplos (perfil `aqs` en `tests/conftest.py`, `deadline=None`, `print_blob=True`).
El shrinking esta activo; no se restringen `phases`.

## Como reproducir un fallo

La cabecera de la sesion imprime `hypothesis seed: <n>`. Con ese numero:

    .venv/bin/python -m pytest -k pbt --hypothesis-seed=<n>     # tambien vale PBT_SEED=<n>

## Secretos

Los secretos se arman en tiempo de ejecucion por concatenacion de fragmentos; ninguna cadena con forma
de secreto vive en el repositorio.
