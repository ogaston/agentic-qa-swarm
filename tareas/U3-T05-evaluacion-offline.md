# U3-T05 — Evaluación offline contra el dataset: factualidad, adherencia, ruido y bucle de regresión por falso positivo

**Unidad:** U3 — Agentes LLM
**Historias que implementa:** US-M9 (post-mortem evaluado: precisión > 80 %, ruido < 20 % del PRD §10), con la planificación de US-M3/US-M4 cubierta por las reglas de adherencia
**Depende de:** U3-T02 y U3-T04 **fusionadas** (la evaluación ejecuta ambos agentes; no necesita T03). **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano. **Versión mínima a propósito.**

---

## Alcance

**Dentro** (una línea, concreta):

> Crear el proyecto `agents/eval/` con un comando `python -m agent_eval run` que ejecuta `agent-planner` y `agent-reporter` sobre los 12 artefactos del dataset con el `FakeLLM`, calcula factualidad, precisión, adherencia y ruido, escribe un informe JSON, **falla (código ≠ 0) si no se cumplen los umbrales del §10**, y permite convertir un falso positivo marcado por un usuario en una entrada nueva del dataset (`add-regression`).

Detalle:

- **Proyecto** `agents/eval/` (layout `src/agent_eval/`, `pyproject.toml`, `requirements-dev.txt` con las mismas versiones exactas de T01 más `-e ../agent-planner -e ../agent-reporter`, `.gitignore` con `.venv/`, `out/`). **Sin `Dockerfile`**: no es un servicio; no entra en `list-services.sh`. Importa los paquetes de los agentes como biblioteca (no por HTTP) y reutiliza su `FakeLLM`.
- **Ejecución por artefacto.** Planner: `plan(surface, …)` con la respuesta canned registrada bajo el hash del prompt real; reporter: `correlate_postmortem(…)` con `DirEvidenceReader` sobre `evidence/`. Cada artefacto produce un registro `{id, kind, planner: {outcome, flows, error}, reporter: {verdict, findings, error}, checks: [...]}`. Los `no-arranca` deben terminar en error tipado (`no_surface`/`NoEvidence`) en ambos agentes.
- **Métricas** (definiciones fijas, documentadas en `README.md`):
  - **factualidad** = hallazgos con `(invariant, method, path)` igual a la causa raíz esperada ÷ artefactos `bug-sembrado` (4);
  - **precisión del post-mortem** = artefactos etiquetados (`golden`, `bug-sembrado`, `trampa-esquema`: 10) con veredicto igual al esperado y, si es `bug`, causa raíz igual ÷ 10;
  - **ruido** = hallazgos emitidos que son falsos positivos (cualquier hallazgo en un artefacto cuyo veredicto esperado no es `bug`, o con causa raíz distinta de la esperada) ÷ hallazgos emitidos (0 hallazgos → ruido `0.0`, definido, no `NaN`);
  - **adherencia** = comprobaciones binarias superadas ÷ comprobaciones totales, con estas reglas por artefacto (las que dependen del LLM y de los agentes de U3; reset verificado, aislamiento del namespace y tope de intentos las imponen U2/U4 y **no** se evalúan aquí): A1 todo paso del plan cita un endpoint observado; A2 el plan respeta `PLANNER_MAX_FLOWS`/`PLANNER_MAX_STEPS` y no hay rutas absolutas, `..` ni host en `path`; A3 `no-arranca` falla cerrado en ambos agentes; A4 todo hallazgo cita solo URIs recibidas; A5 ningún prompt ni reporte contiene un patrón de secreto (se siembra uno en una copia del dataset); A6 cero conexiones no locales durante la corrida (contador de la guarda sin red); A7 el prompt del planner no incluye nada ajeno a la superficie canónica (comparación con la plantilla).
- **Umbrales** (de `specs/prd.md` §10, constantes en `thresholds.py` y citadas en el informe): precisión `> 0.80`, ruido `< 0.20`, adherencia `== 1.0` (cumplimiento binario de reglas duras: umbral propuesto aquí, no escrito en el PRD → candidata de confirmación). `run` sale con `0` si los tres se cumplen y con `1` si no, e imprime una línea por métrica con `OK|FALLA`.
- **Informe** `out/report.json` (validado en la prueba contra `agents/eval/report.schema.json`, nuevo): `{dataset: {artifacts, labelled, regressions}, llm: "fake", valid_for: "pipeline", metrics: {factualidad, precision, ruido, adherencia}, thresholds, thresholds_met, per_artifact: [...]}`. Los campos `llm: "fake"` y `valid_for: "pipeline"` son **obligatorios y literales**: con respuestas canned el informe prueba que el pipeline, las validaciones y la redacción funcionan y que una regresión se detecta; **no** mide la calidad de un modelo real (eso es de U3-T07 y de una candidata nueva). Determinista: dos ejecuciones producen el mismo archivo byte a byte (sin fechas).
- **Bucle de regresión.** `python -m agent_eval add-regression --artifact <id> --finding <finding_id> --label sin-hallazgos|inconcluso [--dataset agents/dataset]` copia la superficie y la evidencia del artefacto a `<dataset>/regressions/<id>-fp-<n>/` (mismo diseño que un artefacto, `meta.kind = "regresion-fp"`), fija `expected.json` con la etiqueta dada y **conserva** la respuesta canned del reporter (la que produjo el falso positivo), de modo que la regresión falla hasta que el agente o su dato se corrijan. `run` incluye `agents/dataset/regressions/*` en el cómputo (`dataset.regressions`). Una prueba crea una regresión en una copia temporal, comprueba que `run` pasa a evaluar 13 (`dataset.artifacts = 12`, `dataset.regressions = 1`) y que el hallazgo falso hace subir el **ruido** a `0.2` (1 falso de 5 hallazgos; el umbral es estricto `< 0.20`) y `run` sale con `1`.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Llamar a un LLM real o medir calidad de un modelo (T07); muestreo continuo en producción (§11 punto 2), recolección de falsos positivos desde la UI (U1) y su transporte.
- Cambiar los agentes para que las métricas salgan bien: si una métrica falla, se arregla el dato canned o se reporta el defecto del agente; no se afloja un umbral ni una regla. Editar código de T02/T04 solo con un defecto demostrado por una prueba, anotado en la bitácora.
- Ampliar el dataset más allá de los 12 (el mínimo del §11 es 10–15); abrir 100 % sintético vs. real (decisión abierta).
- Workflow de CI para `agents/eval` (candidata: hoy `ci.yml` no lo ve), `Dockerfile`, `contracts/**`, `deploy/**`, `policy/**`.

---

## Archivos de contexto

- `unidades-y-tareas.md` (U3-T5), `unit-task-plans/U3.md`
- `specs/prd.md` §10 (umbrales de precisión y ruido) y §11 (dataset, criterios de calidad, cómo se revisan los outputs, red-teaming)
- `agents/dataset/` (U3-T01), `agents/agent-planner/`, `agents/agent-reporter/` (T02, T04)
- `tareas/U3-T01-stubs.md`, `U3-T02-agent-planner.md`, `U3-T04-agent-reporter.md`

---

## Criterios de aceptación

Desde la raíz del worktree.

- [ ] **CA-1** — Entorno limpio, pruebas en verde y la evaluación completa pasa los umbrales.
  ```bash
  cd agents/eval && rm -rf .venv out && python3 -m venv .venv && .venv/bin/pip install -q -r requirements-dev.txt 2>&1 | grep -v -i notice; .venv/bin/python -m pytest -q 2>&1 | tail -n 2
  .venv/bin/python -m agent_eval run --out out; echo "rc=$?"
  ```
  Esperado: `N passed` con N ≥ `15` y sin `failed`; cuatro líneas `OK <métrica>` (factualidad, precisión, ruido, adherencia) y `rc=0`. Antes de la tarea: `cd: agents/eval: No such file or directory` (rojo inicial).

- [ ] **CA-2** — El informe es válido, declara su alcance y los números cuadran.
  ```bash
  cd agents/eval && .venv/bin/python -c "import json,jsonschema;r=json.load(open('out/report.json'));jsonschema.Draft202012Validator(json.load(open('report.schema.json'))).validate(r);print('schema ok')"
  jq -r '[.llm,.valid_for,.dataset.artifacts,.dataset.labelled,.metrics.precision,.metrics.ruido,.metrics.adherencia,.thresholds_met.precision,.thresholds_met.ruido,.thresholds_met.adherencia]|map(tostring)|join(" ")' out/report.json
  jq '.per_artifact|length' out/report.json
  ```
  Esperado: `schema ok`; `fake pipeline 12 10 …` con precisión `> 0.8`, ruido `< 0.2`, adherencia `1` y los tres `true`; y `12`.

- [ ] **CA-3** — **Los números se calculan, no se declaran:** con una respuesta mala, las métricas se mueven y `run` falla.
  ```bash
  cd agents/eval && .venv/bin/python -m pytest -q -k 'detects or threshold' -v 2>&1 | grep -E 'PASSED|FAILED|SKIPPED' | sed -E 's/.*::(test_[a-z_0-9]+).*(PASSED|FAILED|SKIPPED).*/\2 \1/'
  ```
  Esperado: `PASSED` en pruebas (con copias temporales del dataset) que: voltean el veredicto de 3 de los 10 etiquetados → precisión `0.7` y `run` sale con `1`; añaden un hallazgo en un `golden` → ruido `> 0` y el hallazgo cuenta como falso positivo; hacen que un plan cite un endpoint no observado (sorteando el validador con un doble) → falla A1 y la adherencia baja de `1`; un `no-arranca` que produce un plan → falla A3; un secreto sembrado en un reporte → falla A5; y ruido con 0 hallazgos es `0.0` (no `NaN`). Ningún `SKIPPED`.

- [ ] **CA-4** — Bucle de regresión por falso positivo: de extremo a extremo con un comando real.
  ```bash
  cd agents/eval && rm -rf /tmp/aqs-ds && cp -r ../dataset /tmp/aqs-ds && id=$(jq -r '[.artifacts[]|select(.kind=="bug-sembrado")][0].id' ../dataset/manifest.json)
  .venv/bin/python -m agent_eval add-regression --dataset /tmp/aqs-ds --artifact "$id" --finding f1 --label sin-hallazgos; echo "rc=$?"; ls /tmp/aqs-ds/regressions | wc -l
  .venv/bin/python -m agent_eval run --dataset /tmp/aqs-ds --out /tmp/aqs-ds-out; echo "rc=$?"; jq '.dataset.regressions, .dataset.artifacts, .metrics.ruido' /tmp/aqs-ds-out/report.json
  ```
  Esperado: `add-regression` con `rc=0` y `1` directorio creado (con `meta.json`, `surface.json`, `evidence-uris.json`, `evidence/`, `expected.json` y `reporter.response.json`); la evaluación imprime `rc=1` y, con `jq`, `1`, `12` y `0.2`. El dataset original queda **sin cambios**: `git status --short agents/dataset | wc -l` → `0`.

- [ ] **CA-5** — Determinismo y cero red.
  ```bash
  cd agents/eval && .venv/bin/python -m agent_eval run --out out1 >/dev/null; .venv/bin/python -m agent_eval run --out out2 >/dev/null; cmp out1/report.json out2/report.json && echo identico; rm -rf out1 out2
  .venv/bin/python -m pytest -q -k 'no_network' -v 2>&1 | grep -E 'PASSED|FAILED|SKIPPED'
  ```
  Esperado: `identico` y `PASSED` (la guarda de red de T01 está activa durante la corrida completa y registra 0 intentos no locales).

- [ ] **CA-6** — Higiene y alcance.
  ```bash
  bash scripts/ci/list-services.sh | grep -c 'agents/eval'
  (cd agents/eval && .venv/bin/python -W error -m compileall -q src tests && echo ok)
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(agents/eval/|bitacoras/U3-T05.md|revisiones/U3-T05/)' | wc -l
  ```
  Esperado: `0`, `ok`, `0` y `0` (el dataset y los agentes no se modifican; si hizo falta arreglar un defecto de T02/T04, la tarea lo declara en la bitácora y el revisor amplía el filtro de este criterio a ese archivo con su prueba).

---

## Plan de pruebas

- Unitarias de cada métrica con entradas mínimas hechas a mano (casos de borde: 0 hallazgos, todos acertados, todos errados), de cada regla A1–A7 con su caso rojo, y de `add-regression` sobre un dataset temporal.
- **Mutaciones** (copia, `timeout 60`): calcular el ruido sobre artefactos en vez de hallazgos, contar el acierto de `bug` sin comparar la causa, invertir `<` por `<=` en un umbral, saltar la regla A5, ignorar `regressions/`. Cada una debe poner una prueba en rojo; pegar el rojo.
- Negativa: subir la precisión mínima a `1.01` en una copia debe hacer que `run` falle.

**Rojo primero:** pegar la salida del primer comando de CA-1 antes de implementar.

---

## Notas

- **Qué demuestra y qué no.** Con `FakeLLM` el informe es una prueba del pipeline y de su sensibilidad a regresiones. Afirmar «precisión > 80 %» del producto exige medir con un modelo real sobre el dataset (candidata ligada a la decisión del proveedor y del costo).
- `eval/run` del plan de tareas se materializa como `python -m agent_eval run`.
- Python y herramientas como en U3-T01. Ningún comando contra un clúster, la nube ni un proveedor LLM.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
