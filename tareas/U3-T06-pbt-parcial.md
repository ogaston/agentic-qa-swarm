# U3-T06 — PBT parcial: round-trip de planes y reportes, invariantes, generadores y seed (Hypothesis)

**Unidad:** U3 — Agentes LLM
**Historias que implementa:** US-M3, US-M9 (calidad de la capa de agentes; extensión Property-Based Testing parcial: PBT-02, PBT-03, PBT-07, PBT-08, PBT-09)
**Depende de:** U3-T02, U3-T03 y U3-T04 **fusionadas** (hay código real que probar). Va después de U3-T05 por orden del plan, sin dependencia de archivos con ella. **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano. **Versión mínima a propósito.**

---

## Alcance

**Dentro** (una línea, concreta):

> Añadir pruebas basadas en propiedades con Hypothesis a `agents/agent-planner` y `agents/agent-reporter`: generadores de dominio reutilizables (PBT-07), propiedades de **round-trip** (PBT-02) y de **invariante** (PBT-03), seed registrado siempre y reproducible por seed con shrinking activo (PBT-08), y un `PBT.md` por servicio con el framework fijado (PBT-09).

Propiedades mínimas (funciones de prueba con prefijo `test_pbt_`, para seleccionarlas con `-k pbt`):

**agent-planner**

1. *(PBT-02)* Para todo `FlowPlan` generado válido: `parse(serialize(plan)) == plan`, y el JSON serializado valida siempre contra `flow-plan.schema.json`.
2. *(PBT-02)* Para todo flujo generado: `extract_flow(render_flow(plan, flow)) == flow` (el k6 conserva los pasos, su orden y los textos con cualquier Unicode, comillas, barras y saltos de línea) y `render_flow` es idéntico en dos llamadas.
3. *(PBT-03)* Para toda `(superficie, respuesta del modelo)` generadas: si `parse_and_validate` **acepta**, entonces todo `(method, path)` de todo paso ∈ `surface.endpoints`, `len(flows) <= PLANNER_MAX_FLOWS`, los `flow_id` son únicos y con la forma permitida, y `run_id`/`workflow` coinciden; y si algún paso cita un endpoint no observado, **rechaza siempre**.
4. *(PBT-03)* Para toda superficie generada (incluidos `path` con `</datos-superficie>`, comillas y Unicode): el prefijo de instrucciones del prompt es idéntico para todas, el bloque de datos aparece una sola vez y el prompt es función pura (misma entrada → mismo `prompt_hash`).
5. *(PBT-03)* Para todo `Limits` y prompt generados: si `ceil(len(prompt)/4) > max_input_tokens` el `FakeLLM` **no** recibe ninguna llamada.

**agent-reporter**

6. *(PBT-02)* Para todo `Report` generado válido: `parse(serialize(report)) == report` y valida contra `report.schema.json`.
7. *(PBT-03)* **`redact_secrets`:** para todo texto generado que contenga secretos sembrados de cada tipo en posiciones y contextos arbitrarios (JSON, cabeceras, URL, bloques PEM): ningún secreto sembrado sobrevive; `redact(redact(x)) == redact(x)`; un texto sin ningún patrón no cambia; y `len(redact(x))` no supera la entrada más un margen acotado por el número de reemplazos.
8. *(PBT-03)* Para todo conjunto generado de `result.json` y respuesta del modelo: si el validador **acepta**, entonces `bug ⇒ ∃ flujo fallido`, `sin-hallazgos ⇒ ∀ flujos pasados`, y cada URI citada ∈ las URIs recibidas.
9. *(PBT-03)* Para toda evidencia generada (con tamaños que cruzan los topes): el prompt nunca supera `REPORTER_MAX_TOTAL_BYTES` de evidencia, el recorte es determinista y conserva cabeza y cola.

**Generadores (PBT-07)**, en `tests/gen.py` de cada servicio (duplicados con una nota, sin librería compartida): `run_id`, `workflow`, `flow_id` (válido y con variantes inválidas controladas), `method`, `path` (con Unicode, vacíos, `..`, delimitadores del prompt, longitud límite), `surface` (con y sin endpoints, endpoints duplicados), `step`, `flow`, `flow_plan` (1 a `MAX_FLOWS`), `model_response` (válida, con endpoint no observado, JSON roto, claves extra), `evidence_uris`, `result_json`, `logs_text` (con y sin instrucciones), `secret(tipo)` (**construido por concatenación en tiempo de ejecución**; ninguna cadena con forma de secreto vive en el repo), `report`. Una prueba `test_pbt_generator_coverage` demuestra que, en 500 sorteos, aparece al menos una vez cada clase relevante (superficie vacía, plan con `MAX_FLOWS` flujos, respuesta con endpoint no observado, cada tipo de secreto, cada veredicto).

**Seed y reproducción (PBT-08).** `tests/conftest.py` fija el seed al inicio: toma `--hypothesis-seed` si se pasa, luego la variable `PBT_SEED`, y si no, uno aleatorio; lo asigna a `config.option.hypothesis_seed` y lo imprime en la cabecera de la sesión como `hypothesis seed: <n>` (visible sin `-q`). `settings.register_profile("aqs", max_examples=200, deadline=None, print_blob=True)` y perfil cargado por defecto; el **shrinking queda activo** (prohibido `phases` sin `shrink`). Una prueba marcada `@pytest.mark.pbt_demo` (excluida del `pytest` normal por el `addopts` de T01) afirma una propiedad deliberadamente falsa (p. ej. «todo `flow_id` generado tiene longitud < 5») para demostrar contraejemplo reducido y reproducción.

**Framework (PBT-09).** `hypothesis==6.122.3` ya fijado en `requirements-dev.txt` por T01 (esta tarea no lo cambia). `agents/agent-planner/PBT.md` y `agents/agent-reporter/PBT.md`, de 10 a 40 líneas cada uno: por qué Hypothesis (Python, shrinking, base de ejemplos), cómo correr (`pytest -k pbt`), cómo reproducir (`--hypothesis-seed=<n>`), cuántos ejemplos y dónde están los generadores.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Otras reglas PBT (PBT-01, 04, 05, 06, 10: la extensión está en modo parcial) y cualquier otra librería de propiedades (`pytest-quickcheck`, `fuzzing` con `atheris`).
- Cambiar código de producción para que una propiedad pase sin dejar rastro: **si una propiedad encuentra un defecto real, se registra en la bitácora con el ejemplo reducido, se arregla en la ronda y el arreglo lleva su prueba de ejemplo fija** (regresión).
- Pruebas de integración, de rendimiento, de red o con k6 real (las de T03 ya existen; las propiedades de k6 aquí son sobre el literal, sin ejecutar k6).
- Editar `pyproject.toml` o `requirements-dev.txt`; modificar `ci.yml`. Las pruebas PBT corren dentro del `pytest` que la CI ya ejecuta; si el tiempo de la suite pasa de 60 s, se baja `max_examples` y se anota.

---

## Archivos de contexto

- `unidades-y-tareas.md` (U3-T6), `unit-task-plans/U3.md`
- `aidlc-docs/inception/requirements/requirements.md` (NF-PBT-01 a NF-PBT-05) y `.aidlc-rule-details/extensions/testing/property-based/property-based-testing.md` (reglas PBT-02/03/07/08/09 en modo parcial)
- `agents/agent-planner/`, `agents/agent-reporter/` (T01–T04), `contracts/plans/*.schema.json`
- `tareas/U1-T05-pbt-parcial.md` (patrón: propiedades mínimas, generadores, seed, demo con etiqueta)

---

## Criterios de aceptación

Desde la raíz del worktree; `.venv` de T01 en cada servicio.

- [ ] **CA-1** — Las propiedades corren y pasan en ambos servicios, con seed visible.
  ```bash
  for s in agent-planner agent-reporter; do (cd agents/$s && l=$(mktemp) && .venv/bin/python -m pytest -k pbt -m 'not pbt_demo' -v -p no:cacheprovider > "$l" 2>&1; echo "$s passed=$(grep -c -E 'test_pbt_.* PASSED' "$l") seeds=$(grep -c -E '^hypothesis seed: [0-9]+' "$l") failed=$(grep -c -E 'FAILED|ERROR' "$l")"); done
  ```
  Esperado: `agent-planner passed=…` con ≥ `6` (5 propiedades más `generator_coverage`), `agent-reporter` con ≥ `5` (4 propiedades más `generator_coverage`), `seeds=1` y `failed=0` en cada uno. Antes de la tarea: `passed=0` (rojo inicial).

- [ ] **CA-2** — Un fallo muestra contraejemplo reducido y seed, y se reproduce con ese seed.
  ```bash
  cd agents/agent-planner && l=$(mktemp) && .venv/bin/python -m pytest -m pbt_demo -p no:cacheprovider > "$l" 2>&1; grep -E '^hypothesis seed:|Falsifying example|flow_id=|failed' "$l" | head -n 5
  s=$(grep -o -E '^hypothesis seed: [0-9]+' "$l" | grep -o '[0-9]*$'); .venv/bin/python -m pytest -m pbt_demo -p no:cacheprovider --hypothesis-seed=$s 2>&1 | grep -E 'flow_id=' | head -n 1; grep -E 'flow_id=' "$l" | head -n 1
  ```
  Esperado: líneas con `hypothesis seed:`, `Falsifying example` y un `flow_id=` reducido (cadena mínima); y las dos últimas líneas son **idénticas** (mismo contraejemplo con el mismo seed). El `pytest` normal no ejecuta la demo.

- [ ] **CA-3** — La suite normal sigue en verde y no ejecuta la demo.
  ```bash
  for s in agent-planner agent-reporter; do (cd agents/$s && l=$(mktemp) && .venv/bin/python -m pytest -p no:cacheprovider > "$l" 2>&1; tail -n 1 "$l"; grep -c -E 'pbt_demo|Falsifying' "$l"); done
  ```
  Esperado: por servicio, `N passed` (sin `failed`) y `0`.

- [ ] **CA-4** — Los generadores cubren las clases de dominio (no de oídas).
  ```bash
  for s in agent-planner agent-reporter; do (cd agents/$s && .venv/bin/python -m pytest -k 'pbt_generator_coverage' -v -p no:cacheprovider 2>&1 | grep -E 'PASSED|FAILED'); done
  ```
  Esperado: `PASSED` en ambos.

- [ ] **CA-5** — **Las propiedades matan mutaciones reales** (se aplican en una copia, `timeout 60` por mutante, y el rojo se pega en la bitácora).
  Mutaciones mínimas: (a) en el validador del planner, aceptar un endpoint con otro método; (b) `render_flow` sin escapar `ensure_ascii` (Unicode); (c) `redact_secrets` sin el tipo JWT; (d) `redact_secrets` no idempotente (reemplazo que contiene su propio patrón); (e) el validador del reporter acepta `bug` sin flujo fallido; (f) el recorte de evidencia descarta la cola.
  ```bash
  cd agents/agent-planner && .venv/bin/python -m pytest -k pbt -m 'not pbt_demo' -p no:cacheprovider 2>&1 | tail -n 1; cd ../agent-reporter && .venv/bin/python -m pytest -k pbt -m 'not pbt_demo' -p no:cacheprovider 2>&1 | tail -n 1
  ```
  Esperado en el código sin mutar: `N passed`. La bitácora lista cada mutación con la propiedad que la mata (`FAILED test_pbt_…`) y, para cada superviviente, una justificación de equivalencia o una propiedad nueva. Al menos 5 de las 6 deben morir.

- [ ] **CA-6** — PBT-09: framework fijado y documentado.
  ```bash
  grep -c '^hypothesis==6.122.3$' agents/agent-planner/requirements-dev.txt agents/agent-reporter/requirements-dev.txt; wc -l < agents/agent-planner/PBT.md; wc -l < agents/agent-reporter/PBT.md
  grep -c -E 'hypothesis-seed|pytest -k pbt' agents/agent-planner/PBT.md agents/agent-reporter/PBT.md
  ```
  Esperado: `…:1` dos veces; dos números entre `10` y `40`; al menos `1` coincidencia en cada `PBT.md`.

- [ ] **CA-7** — Sin secretos con forma real, higiene y alcance.
  ```bash
  grep -r -n -E 'AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9]{36}|eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.' agents --exclude-dir=.venv --exclude-dir=__pycache__ --exclude-dir=.hypothesis | wc -l
  for s in agent-planner agent-reporter; do (cd agents/$s && .venv/bin/python -W error -m compileall -q src tests && echo "$s ok"); done
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(agents/(agent-planner|agent-reporter)/(tests/|PBT\.md)|bitacoras/U3-T06.md|revisiones/U3-T06/)' | wc -l
  ```
  Esperado: `0`, dos `ok`, `0` y `0`. (Si una propiedad halla un defecto real, el filtro se amplía en la bitácora al archivo de producción corregido, con su prueba de regresión.)

---

## Plan de pruebas

- Las propiedades de arriba, más `test_pbt_generator_coverage` y la demo etiquetada. Las pruebas de ejemplo de T02–T04 se conservan.
- Mutaciones de CA-5. Negativa: con `PBT_SEED=1` dos ejecuciones imprimen el mismo seed y recorren los mismos ejemplos; sin la variable, el seed cambia entre ejecuciones.

**Rojo primero:** pegar en la bitácora la salida del primer comando de CA-1 antes de implementar (`passed=0`).

---

## Notas

- **Hypothesis 6.122.3** instalado y comprobado con Python 3.14.7 y `pytest==8.3.4`. La cabecera `hypothesis seed:` solo aparece sin `-q`; los criterios no lo usan.
- Los generadores de secretos deben producir cadenas que **no** disparen escáneres de secretos en el repositorio: se arman en tiempo de ejecución.
- Python y herramientas como en U3-T01. Ningún comando contra un clúster, la nube ni un proveedor LLM.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
