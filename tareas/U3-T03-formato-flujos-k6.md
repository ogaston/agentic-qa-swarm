# U3-T03 — Formato de flujos inspectable: generación de k6 desde `FlowPlan`, validación y publicación

**Unidad:** U3 — Agentes LLM
**Historias que implementa:** US-M4 (flujos como artefacto estándar ejecutable e inspectable, validado antes de publicar)
**Depende de:** U3-T01 fusionada. Corre en paralelo con U3-T02 y U3-T04: vive en un subpaquete nuevo (`agent_planner/k6/`) y **no** edita `planner.py`, `server.py` ni `pyproject.toml`; el cableado al endpoint es de U3-T07. **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano. **Versión mínima a propósito.**

---

## Alcance

**Dentro** (una línea, concreta):

> Añadir a `agents/agent-planner/src/agent_planner/k6/` un generador **determinista** que convierte cada flujo de un `FlowPlan` en un script k6 estándar (plantilla fija + los pasos como literal JSON), un validador de tres capas (esquema del plan, estructura del script, `k6 inspect` real) y un `publish_flows` que solo escribe en un `FlowStore` lo que pasó las tres.

Detalle:

- **Decisión de formato.** El artefacto inspectable es **k6 (JavaScript)**, uno por flujo: `flows/<run_id>/<flow_id>.k6.js`. El script es una **plantilla fija revisada una vez** más una única constante `const FLOW = <JSON>;` con `{flow_id, name, invariant, steps}` serializada con `json.dumps(sort_keys=True, ensure_ascii=True)`. Nada del plan se interpola como código: no hay inyección de JavaScript por construcción (un `path` con comillas, `</script>` o saltos queda como cadena JSON). El runner de U2 hoy ejecuta los pasos del `FlowPlan` con su motor `HTTPStepsExecutor`; el k6 es la copia **auditable y ejecutable fuera de la plataforma** (US-M4), y ambos nacen del mismo plan.
- **Plantilla** (`template.js`, versionada con `// aqs-k6-template: v1`): `import http from 'k6/http'; import { check } from 'k6';`; `export const options = { vus: 1, iterations: 1, thresholds: { checks: ['rate==1'] } };`; la URL base solo de `__ENV.BASE_URL` (si falta o no es `http(s)`, el script falla con `fail()` antes de la primera petición); un bucle sobre `FLOW.steps` con `http.request(step.method, BASE + step.path)` y `check` de `res.status === step.expect_status` con el nombre `${flow_id}:${i}`. Sin `import` de otros módulos, sin `eval`/`Function`/`open()`/`require`, sin acceso a `__ENV` salvo `BASE_URL`, sin cuerpos, encabezados ni credenciales.
- **Funciones** (`render.py`, `validate.py`, `publish.py`): `render_flow(plan, flow) -> str` (puro); `extract_flow(script) -> dict` (inversa: lee el literal `FLOW`); `validate_script(script, plan, flow)` (capa 2); `inspect_with_k6(path)` (capa 3, ejecuta `k6 inspect` con el binario de `K6_BIN` o por `podman run … docker.io/grafana/k6:0.55.0 inspect`); `publish_flows(plan, store, *, k6=inspect_with_k6) -> list[PublishedFlow]`.
- **Validación en tres capas, todas antes de publicar:** (1) el `FlowPlan` valida contra `contracts/plans/flow-plan.schema.json`; (2) el script contiene exactamente la plantilla `v1` (hash de la plantilla fijado en una constante) y `extract_flow(script) == flow` (round-trip); ninguna cadena prohibida fuera del literal; (3) `k6 inspect` termina con código 0 (parsea y construye el módulo). Si **cualquiera** de los flujos falla, **no se publica ninguno** (todo o nada) y se lanza `FlowValidationError(flow_id, capa)`.
- **`FlowStore`**: puerto (`put(key, bytes)`, `get(key)`), implementado con un directorio local (`DirFlowStore`, escritura atómica `rename`, lectura de vuelta y comparación de hash tras escribir) y un `FakeFlowStore` en memoria con fallos programables. El almacén S3 es de U3-T07. Claves y nombres de archivo derivados solo de `run_id`/`flow_id` ya validados con el patrón de T02 (sin `/`, `..` ni mayúsculas).
- **Determinismo:** dos renderizados del mismo plan son idénticos byte a byte (sin fechas, sin aleatoriedad), y el `README.md` del subpaquete explica en ≤ 30 líneas cómo ejecutar un script a mano: `k6 run -e BASE_URL=http://… flows/<run>/<flow>.k6.js`.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Construir la imagen `RUNNER_IMAGE` con k6 y hacer que U2 ejecute k6 en vez de `HTTPStepsExecutor` (candidata), plugins/extensiones de k6 (`xk6`), `k6 cloud`, métricas de carga, cuerpos/encabezados/autenticación (el `FlowPlan` no los tiene: candidata de contrato).
- Cablear `publish_flows` a `POST /v1/plan` o al almacén S3: U3-T07.
- Modificar `planner.py`, `server.py`, `pyproject.toml`, el `Dockerfile`, `contracts/**` o workflows.
- Instalar k6 en el sistema o en la imagen de producción: es una herramienta de prueba.

---

## Archivos de contexto

- `unidades-y-tareas.md` (U3-T3), `unit-task-plans/U3.md`, `aidlc-docs/inception/requirements/requirements.md` (M4 y NF de inspectabilidad)
- `contracts/plans/flow-plan.schema.json` y ejemplos
- `agents/agent-planner/` (U3-T01) y, si ya está fusionado, el patrón de `flow_id` de `validate.py` de U3-T02 (si no, se replica la expresión de la tarea T02)
- `tareas/U2-T05-runners-evidencia.md` (`HTTPStepsExecutor`, motor enchufable)

---

## Criterios de aceptación

Desde la raíz del worktree; `.venv` de T01. Alias para la herramienta real (k6 **no** está instalado; se usa la imagen fijada, `:z` por podman):

```bash
K6=(podman run --rm -v "$PWD:/w:z" -w /w docker.io/grafana/k6:0.55.0)   # digest verificado: sha256:f0573f397c9de5ce0635bec9ece3e3f1387b410b54dbe72d9b3f2a57ce8f2490
```

- [ ] **CA-1** — Pruebas en verde (las de capa 3 usan el k6 real, sin `SKIP`).
  ```bash
  cd agents/agent-planner && .venv/bin/python -m pytest -q -m 'not pbt_demo' tests/k6 -rs 2>&1 | tail -n 3
  ```
  Esperado: `N passed` con N ≥ `15`, sin `failed` y **sin `skipped`** (si falta `podman` y `K6_BIN` la prueba de la capa 3 **falla**, no se omite). Antes de la tarea: `no tests ran`/directorio inexistente (rojo inicial).

- [ ] **CA-2** — `k6 inspect` real acepta cada flujo generado de los ejemplos y rechaza uno corrupto.
  ```bash
  cd agents/agent-planner && rm -rf /tmp/aqs-k6 && .venv/bin/python -m agent_planner.k6 render-examples --out /tmp/aqs-k6
  cd /tmp/aqs-k6 && ls *.k6.js | wc -l
  for f in /tmp/aqs-k6/*.k6.js; do podman run --rm -v /tmp/aqs-k6:/w:z docker.io/grafana/k6:0.55.0 inspect /w/$(basename $f) >/dev/null 2>&1; echo "$(basename $f) rc=$?"; done | grep -c 'rc=0'
  sed 's/export default function/export default function(/' /tmp/aqs-k6/$(ls /tmp/aqs-k6 | head -n1) > /tmp/aqs-k6/roto.js; podman run --rm -v /tmp/aqs-k6:/w:z docker.io/grafana/k6:0.55.0 inspect /w/roto.js >/dev/null 2>&1; echo "roto rc=$?"
  ```
  Esperado: `render-examples` genera un script por cada flujo de los planes de ejemplo de `contracts/plans/examples/valid/flow-plan.*.json` y de los planes del dataset (≥ `12`); el conteo de `rc=0` es igual al número de scripts; y `roto rc=` distinto de `0` (en k6 0.55.0 sale `107`).

- [ ] **CA-3** — El script es **ejecutable** de verdad: `k6 run` contra un servidor local pasa los checks cuando el estado coincide y falla cuando no.
  ```bash
  cd agents/agent-planner && .venv/bin/python -m pytest -q -m 'not pbt_demo' -k 'k6_run' -v 2>&1 | grep -E 'PASSED|FAILED|SKIPPED' | sed -E 's/.*::(test_[a-z_0-9]+).*(PASSED|FAILED|SKIPPED).*/\2 \1/'
  ```
  Esperado: `PASSED` en dos pruebas que levantan un `http.server` en loopback y ejecutan `podman run --network host … k6 run -e BASE_URL=http://127.0.0.1:<puerto>`: código 0 cuando todos los `expect_status` coinciden y código distinto de 0 (umbral `checks` incumplido) cuando uno no; ningún `SKIPPED`.

- [ ] **CA-4** — Sin inyección de código: ninguna entrada del plan escapa del literal JSON.
  ```bash
  cd agents/agent-planner && .venv/bin/python -m pytest -q -m 'not pbt_demo' -k 'k6 and (injection or forbidden)' -v 2>&1 | grep -E 'PASSED|FAILED|SKIPPED' | sed -E 's/.*::(test_[a-z_0-9]+).*(PASSED|FAILED|SKIPPED).*/\2 \1/'
  grep -c -E '\b(eval|Function|require|open)\(|import .* from' agents/agent-planner/src/agent_planner/k6/template.js
  ```
  Esperado: `PASSED` en pruebas con `path`/`name`/`invariant` que contienen `"`, `\`, `*/`, `</script>`, saltos de línea, U+2028 y `'); fail('x`: `extract_flow(render_flow(...)) == flow` y `validate_script` acepta; un script al que se le inyecta código fuera del literal es rechazado por la capa 2; y el `grep` imprime `2` (solo los dos `import` de `k6/http` y `k6`; el codificador lo explica en la bitácora si cambia el número, pero nunca incluye `eval`/`Function`/`require`/`open`).

- [ ] **CA-5** — Todo o nada, validación antes de publicar y lectura de vuelta.
  ```bash
  cd agents/agent-planner && .venv/bin/python -m pytest -q -m 'not pbt_demo' -k 'publish' -v 2>&1 | grep -E 'PASSED|FAILED|SKIPPED' | sed -E 's/.*::(test_[a-z_0-9]+).*(PASSED|FAILED|SKIPPED).*/\2 \1/'
  ```
  Esperado: `PASSED` en pruebas de: plan inválido → `FlowValidationError` y el almacén queda **vacío**; con 3 flujos y el 2.º rechazado por `k6 inspect` (inspector falso que falla en ese) → 0 objetos publicados; fallo de escritura del almacén a mitad → no queda ningún objeto parcial (se borran los ya escritos) y el error se propaga; la lectura de vuelta compara hash; el directorio resultante contiene exactamente `<flow_id>.k6.js` por flujo; publicar dos veces el mismo plan es idempotente (bytes idénticos).

- [ ] **CA-6** — Higiene y alcance.
  ```bash
  (cd agents/agent-planner && .venv/bin/python -W error -m compileall -q src tests && .venv/bin/python -m pytest -q -m 'not pbt_demo' 2>&1 | tail -n 1)
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(agents/agent-planner/(src/agent_planner/k6/|tests/k6/)|bitacoras/U3-T03.md|revisiones/U3-T03/)' | wc -l
  ```
  Esperado: `N passed` sin `failed` (toda la suite, incluida la de T01/T02 si ya está fusionada), `0` y `0`.

---

## Plan de pruebas

- Unitarias: render (golden file `tests/k6/golden/*.k6.js` fijado), `extract_flow`, cada prohibición de la capa 2, publicación y fallos del almacén.
- Integración con la herramienta real: `k6 inspect` y `k6 run` (CA-2 y CA-3).
- **Mutaciones** (copia, `timeout 60`): interpolar `step.path` sin JSON, omitir la capa 3, publicar antes de validar todos, no borrar parciales, cambiar el orden de los pasos. Cada una debe poner una prueba en rojo; pegar el rojo.
- Negativa: modificar un byte de la plantilla debe hacer fallar la comprobación de hash de la capa 2.

**Rojo primero:** pegar en la bitácora la salida del primer comando de CA-1 antes de implementar y la de un `k6 inspect` sobre un script roto (rc 107).

---

## Notas

- **k6 no está instalado en el entorno local ni en la CI de GitHub.** Versión pineada `grafana/k6:0.55.0` (comprobado: `inspect` sale con `0` en un script válido y `107` en uno con error de sintaxis; no existe `k6 lint` en esa versión, por eso el criterio del plan «`k6 lint`» se cumple con `k6 inspect`). Se obtiene con `podman pull docker.io/grafana/k6:0.55.0`; el montaje lleva `:z`. Las pruebas de capa 3 llaman a `K6_BIN` si está definido y, si no, a `podman`. **Consecuencia:** el job `test` de `ci.yml` no garantiza k6 ni podman. Por eso T01 fija `addopts = "-m 'not pbt_demo and not k6'"`: las pruebas marcadas `@pytest.mark.k6` (capa 3 y `k6 run`) **no corren en la CI de GitHub** y se verifican en el loop con `-m 'not pbt_demo'` (que las incluye, como hacen los criterios de arriba). Que la CI las ejecute (instalar k6 con versión y digest fijados) es una candidata. Marcar como `k6` solo lo que de verdad llama a k6; el resto de la suite de esta tarea sí corre en la CI.
- Si el registro `docker.io` no es alcanzable desde el entorno del loop (ver C-58), se reporta como bloqueo, no se sustituye por una comprobación casera.
- Python y herramientas como en U3-T01. Ningún comando contra un clúster, la nube ni un proveedor LLM.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
