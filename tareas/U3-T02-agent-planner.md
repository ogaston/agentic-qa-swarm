# U3-T02 — `agent-planner`: superficie → `FlowPlan` determinista, límite duro de tiempo/tokens y superficie como dato

**Unidad:** U3 — Agentes LLM
**Historias que implementa:** US-M3, US-M4 (inferir superficie y generar flujos dentro del workflow; límite duro de costo; prompt-injection)
**Depende de:** U3-T01 fusionada. Corre en paralelo con U3-T03 y U3-T04 (no comparten archivos: T02 no toca `agent_planner/k6/` ni `agent-reporter`). **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano. **Versión mínima a propósito.**

---

## Alcance

**Dentro** (una línea, concreta):

> En `agents/agent-planner/`: dado un `SurfaceArtifact` y un workflow, construir un prompt donde la superficie es **dato delimitado**, llamar al `LLMClient` con tope duro de tiempo y de tokens, validar la salida y devolver un `FlowPlan` válido contra `contracts/plans/flow-plan.schema.json` (o fallar cerrado), expuesto por `POST /v1/plan` en un servidor HTTP mínimo con `Dockerfile`.

Detalle:

- **Función pura `plan(surface, workflow, llm, limits) -> FlowPlan`** (`planner.py`). Pasos: (1) rechazar una superficie inválida contra `surface-artifact.schema.json` o con `endpoints: []` (error `no_surface`: nunca se inventa un plan); (2) `build_prompt` (`prompt.py`); (3) `llm.complete(...)`; (4) `parse_and_validate` (`validate.py`); (5) devolver el plan. Ninguna otra llamada: **los runners de U2 no llaman a ningún LLM**; el plan es la única salida del agente.
- **Superficie como dato (prompt-injection).** `build_prompt` produce dos partes: instrucciones fijas (constantes del código, sin interpolar nada del exterior) y un bloque `<datos-superficie>` con la superficie serializada como **JSON canónico** (`json.dumps(sort_keys=True, ensure_ascii=True)`). Las instrucciones dicen que lo que hay dentro del bloque es dato no confiable. Un `path` que contenga `</datos-superficie>` o texto como «ignora las instrucciones anteriores» queda escapado por el JSON y **no** puede cerrar el bloque ni aparecer fuera de él. El prompt nunca incluye código fuente ni variables de entorno.
- **Validación de la salida (la defensa real, porque el modelo puede ser engañado):** la respuesta debe ser JSON que valide contra `flow-plan.schema.json` **y** cumplir: (a) cada `(method, path)` de cada paso pertenece **exactamente** a `surface.endpoints`; (b) `run_id` del plan == `run_id` de la superficie y `workflow` == el solicitado; (c) `flow_id` único y con la forma `^[a-z0-9]([a-z0-9-]{0,40}[a-z0-9])?$` (U2 lo usa en el nombre de un Job); (d) a lo sumo `PLANNER_MAX_FLOWS` flujos (10 por defecto) y `PLANNER_MAX_STEPS` pasos por flujo (20); (e) sin claves extra. Cualquier violación → `PlanRejected(reason)`; **no** se repara ni se recorta el plan (determinismo y fail-closed).
- **Límite duro de tiempo y tokens.** `Limits{timeout_s=25, max_input_tokens, max_output_tokens}` desde `PLANNER_TIMEOUT_S`, `PLANNER_MAX_INPUT_TOKENS`, `PLANNER_MAX_OUTPUT_TOKENS` (valores por defecto documentados en el README; 25 s es menor que los 30 s del cliente de U2). Antes de llamar: si `ceil(len(prompt)/4) > max_input_tokens` → `BudgetExceeded("input")` **sin llamar al modelo** (superficie combinatoria extrema, fila 3 de la tabla de red-teaming del PRD). Se pasa `max_tokens=max_output_tokens` y `timeout_s` al cliente; si el resultado declara más tokens de los permitidos o el cliente lanza `LLMTimeout` → `BudgetExceeded("output"|"time")`. Sin reintentos. El error es de la fase, no un plan parcial.
- **Servidor** (`server.py`, solo biblioteca estándar: `http.server`, sin framework): `POST /v1/plan` con `{surface, workflow}` → `200 FlowPlan` | `422 {error: "no_surface"|"plan_rejected"|"invalid_request", detail}` | `504 {error: "budget_exceeded"}` | `502 {error: "llm_unavailable"}`; `GET /healthz`, `GET /readyz`. Cuerpo máximo 256 KiB; solo `application/json`; el `detail` nunca contiene contenido de la superficie ni del modelo. Logs JSON con `run_id` y motivo, sin el prompt. Métricas en `GET /metrics` (texto Prometheus escrito a mano): `aqs_planner_requests_total{result}` y `aqs_planner_tokens_total{direction}`.
- **Cableado por entorno.** `LLM_PROVIDER=fake` solo se permite con `PLANNER_ALLOW_FAKE=true` y se rechaza con `PLANNER_ENV=prod`; cualquier otro valor falla al arrancar con «proveedor no implementado» (el adaptador real es U3-T07). Sin esa configuración el proceso no arranca (fail-closed).
- **`Dockerfile`**: imagen `python` con tag fijado (el codificador fija el digest y lo anota), usuario `65532:65532`, sin `latest`, `pip install` desde `requirements.txt` con versiones exactas (solo `jsonschema` y su cadena; sin `pytest`/`hypothesis`), sin copiar tests ni dataset. Con él `list-services.sh` ve el servicio y la CI lo construye y prueba.
- **`README.md`**: API, límites por defecto, el aviso «el agente trata la superficie como dato; la defensa es la validación posterior, no el prompt».

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Generar o publicar scripts k6 (U3-T03), `agent-reporter`, `redactSecrets` (U3-T04), evaluación y métricas de calidad (T05), propiedades Hypothesis (T06; aquí solo pruebas de ejemplo).
- Adaptador de proveedor LLM real, claves, circuito/colas hacia el proveedor (candidatas), cola de peticiones, autenticación del servicio (la NetworkPolicy y la identidad servicio a servicio son C-50/C-51).
- Reparar o «completar» un plan inválido; reintentar al modelo; leer código fuente o fuera de `surface`.
- Cambiar `contracts/**`, `deploy/**`, `policy/**` o workflows. Si el esquema de `FlowPlan` resulta insuficiente (sin cuerpos, parámetros ni encabezados), se reporta: es la candidata C-n sobre ampliar el contrato.

---

## Archivos de contexto

- `unidades-y-tareas.md` (U3, riesgos y red-teaming), `unit-task-plans/U3.md`, `component-methods.md` (`recommendFlows`)
- `specs/prd.md` §11 (inyección de prompt, agotamiento de costo)
- `contracts/plans/flow-plan.schema.json`, `surface-artifact.schema.json` y sus ejemplos
- `agents/agent-planner/` y `agents/dataset/` (U3-T01: `FakeLLM`, guarda sin red, dataset)
- `tareas/U2-T08-integracion-u3-dev.md` (cliente HTTP de U2: timeout 30 s, valida la respuesta, espera `FlowPlan` puro)

---

## Criterios de aceptación

Desde la raíz del worktree; `cd agents/agent-planner` con el `.venv` de T01.

- [ ] **CA-1** — Pruebas en verde y el dataset produce los planes esperados.
  ```bash
  cd agents/agent-planner && .venv/bin/python -m pytest -q 2>&1 | tail -n 2; .venv/bin/python -m pytest -q -k 'dataset' -v 2>&1 | grep -c PASSED
  ```
  Esperado: `N passed` con N ≥ `35` y sin `failed`; el segundo número ≥ `12` (los 12 artefactos: 10 con plan válido contra el esquema y cobertura de `must_cover_invariants`, 2 `no-arranca` → `no_surface`). Antes de la tarea: no existe `planner.py` (rojo inicial).

- [ ] **CA-2** — **Prompt-injection:** la superficie es dato y una salida secuestrada se rechaza.
  ```bash
  cd agents/agent-planner && .venv/bin/python -m pytest -q -k 'injection' -v 2>&1 | grep -E 'PASSED|FAILED|SKIPPED' | sed -E 's/.*::(test_[a-z_0-9]+).*(PASSED|FAILED|SKIPPED).*/\2 \1/'
  ```
  Esperado: `PASSED` en al menos cuatro pruebas: (1) las instrucciones del prompt son idénticas con superficie benigna y maliciosa (mismo prefijo byte a byte); (2) un `path` con `</datos-superficie>` no produce un segundo cierre del bloque; (3) un `FakeLLM` que «obedece» y devuelve un plan con un endpoint **no observado** → `PlanRejected`; (4) igual con un método fuera del enum o un `workflow` distinto. Ningún `SKIPPED`.

- [ ] **CA-3** — **Límite duro de tiempo y tokens, documentado y probado.**
  ```bash
  cd agents/agent-planner && .venv/bin/python -m pytest -q -k 'budget or timeout' -v 2>&1 | grep -E 'PASSED|FAILED|SKIPPED' | sed -E 's/.*::(test_[a-z_0-9]+).*(PASSED|FAILED|SKIPPED).*/\2 \1/'; grep -c -E 'PLANNER_TIMEOUT_S|PLANNER_MAX_INPUT_TOKENS|PLANNER_MAX_OUTPUT_TOKENS' README.md
  ```
  Esperado: `PASSED` en pruebas de: superficie enorme → `BudgetExceeded("input")` con `FakeLLM.calls == 0`; salida que declara más tokens que el tope; latencia simulada > `timeout_s`; `max_tokens` y `timeout_s` llegan al cliente tal cual; sin reintento tras el fallo; y el `grep` devuelve ≥ `3`.

- [ ] **CA-4** — El servidor cumple el contrato HTTP y falla cerrado (servidor real en loopback).
  ```bash
  cd agents/agent-planner && .venv/bin/python -m pytest -q -k 'server' -v 2>&1 | grep -E 'PASSED|FAILED|SKIPPED' | sed -E 's/.*::(test_[a-z_0-9]+).*(PASSED|FAILED|SKIPPED).*/\2 \1/'
  PLANNER_ENV=prod LLM_PROVIDER=fake PLANNER_ALLOW_FAKE=true .venv/bin/python -m agent_planner; echo "rc=$?"; LLM_PROVIDER=otro .venv/bin/python -m agent_planner; echo "rc=$?"
  ```
  Esperado: `PASSED` en pruebas de: `200` con un plan que **valida contra el esquema** y no trae claves extra; `422` por superficie inválida, vacía y plan rechazado; `504` por presupuesto; `502` por proveedor caído; `413`/`415` por tamaño y tipo; `/healthz`, `/readyz`, `/metrics` con los contadores. Los dos arranques terminan con `rc=` distinto de `0` y un mensaje en stderr (fake en prod; proveedor no implementado).

- [ ] **CA-5** — El `detail` y los logs no filtran la superficie ni el prompt.
  ```bash
  cd agents/agent-planner && .venv/bin/python -m pytest -q -k 'no_leak' -v 2>&1 | grep -E 'PASSED|FAILED|SKIPPED' | sed -E 's/.*::(test_[a-z_0-9]+).*(PASSED|FAILED|SKIPPED).*/\2 \1/'
  ```
  Esperado: `PASSED`: con una superficie que contiene un marcador único, ni la respuesta de error, ni los logs capturados, ni `/metrics` lo contienen.

- [ ] **CA-6** — Imagen, higiene y alcance.
  ```bash
  docker build -q -t aqs-agent-planner:ci agents/agent-planner >/dev/null && docker inspect aqs-agent-planner:ci --format '{{.Config.User}}'
  docker run --rm aqs-agent-planner:ci python -c "import pytest" 2>&1 | tail -n 1
  bash scripts/ci/list-services.sh | grep -c 'agents/agent-planner'; grep -c -E ':latest|^FROM [^@:]+$' agents/agent-planner/Dockerfile
  (cd agents/agent-planner && .venv/bin/python -W error -m compileall -q src tests && echo ok)
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(agents/agent-planner/|bitacoras/U3-T02.md|revisiones/U3-T02/)' | wc -l
  ```
  Esperado: `65532:65532`; `ModuleNotFoundError` (no hay herramientas de prueba en la imagen); `1`; `0`; `ok`; `0` y `0`.

---

## Plan de pruebas

- Ejemplo: un caso por artefacto del dataset (T01), cada rama de `parse_and_validate` (JSON roto, esquema, endpoint no observado, `run_id`/`workflow` distintos, `flow_id` duplicado/ inválido, demasiados flujos o pasos), presupuestos y servidor.
- **Mutaciones** con una copia del código (`timeout 60` por mutante): quitar el chequeo (a) de endpoints observados, quitar el tope de entrada, no pasar `max_tokens` al cliente, permitir `fake` en prod, aceptar recorte de flujos. Cada una debe poner al menos una prueba en rojo; pegar el rojo en la bitácora.
- Negativa: poner `PLANNER_TIMEOUT_S=0` o un tope de tokens negativo debe impedir el arranque.

**Rojo primero:** pegar en la bitácora la salida de `pytest -k injection` antes de implementar (no existen las pruebas / `ModuleNotFoundError: agent_planner.planner`).

---

## Notas

- **El LLM puede equivocarse o ser engañado; el sistema no depende de que no lo sea.** Por eso la validación posterior y el aislamiento de U2 (sin egress, sin credenciales en los runners) son la defensa; el delimitado del prompt es defensa adicional.
- **Determinismo.** Con el `FakeLLM` el mismo `SurfaceArtifact` produce el mismo plan byte a byte; con un modelo real el determinismo lo da la validación y la temperatura 0 que fije T07 (aquí no hay modelo real).
- El `SurfaceArtifact` actual solo trae método y ruta: el planner no puede razonar sobre cuerpos ni formatos. Es una limitación del contrato (candidata), no un defecto de esta tarea.
- Python y herramientas como en U3-T01. Podman con `:z`. Ningún comando contra un clúster, la nube ni un proveedor LLM.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
