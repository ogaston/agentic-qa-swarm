# U3-T01 — Stubs: LLM fake determinista por prompt-hash, dataset sintético y esqueleto de `agent-planner` y `agent-reporter`

**Unidad:** U3 — Agentes LLM
**Historias que implementa:** US-M3, US-M9 (base de pruebas offline para la inferencia de flujos y el post-mortem)
**Depende de:** U2-T01 fusionada (los esquemas `contracts/plans/*` ya existen) y U5 completa (CI). Es la primera tarea de U3; **bloquea** U3-T02, T03 y T04. **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano. **Versión mínima a propósito.**

---

## Alcance

**Dentro** (una línea, concreta):

> Crear los proyectos Python vacíos `agents/agent-planner/` y `agents/agent-reporter/` (solo `pyproject.toml`, `requirements-dev.txt`, paquete con `__init__.py` y el LLM fake, **sin `Dockerfile` ni servidor**), el dataset sintético `agents/dataset/` (12 artefactos con superficie, evidencia, respuestas canned del LLM y etiqueta esperada) y una guarda que haga fallar cualquier prueba que intente abrir un socket no local.

Detalle:

- **Lenguaje y herramientas (decisión del humano).** Python con Hypothesis. `requires-python = ">=3.12"` (el entorno local tiene 3.14.7). Dependencias fijadas **exactamente** en `requirements-dev.txt` de cada servicio: `pytest==8.3.4`, `hypothesis==6.122.3`, `jsonschema==4.23.0`, `rfc3339-validator==0.1.4` (verificadas instalables con 3.14.7). Sin SDK de ningún proveedor LLM ni cliente HTTP de terceros (`anthropic`, `openai`, `requests`, `httpx`): el adaptador real es de U3-T07. Cada proyecto lleva su propio `.gitignore` con `.venv/`, `__pycache__/`, `.pytest_cache/`, `.hypothesis/` (no se toca el `.gitignore` de la raíz).
- **Paquetes.** `agents/agent-planner/src/agent_planner/` y `agents/agent-reporter/src/agent_reporter/` (layout `src`, instalables con `pip install -e .`). `pyproject.toml` declara los marcadores `pbt_demo`, `k6` y `minio` y `addopts = "-m 'not pbt_demo and not k6 and not minio'"` (los usan T03 y T06; el criterio de las tareas que necesitan k6 es `-m 'not pbt_demo'`; **T02–T06 no editan `pyproject.toml`**: si necesitan algo, lo reportan, así corren en paralelo sin conflicto).
- **Puerto `LLMClient`** (en cada paquete, `llm.py`): `complete(prompt: str, *, max_tokens: int, timeout_s: float) -> LLMResult` con `LLMResult{text, input_tokens, output_tokens}`; errores tipados `LLMTimeout`, `LLMUnavailable`, `UnscriptedPrompt`.
- **`FakeLLM`** (`fakes/fake_llm.py`, duplicado en ambos paquetes; no hay librería compartida, igual que en Go): respuestas registradas por `prompt_hash = sha256(prompt codificado en UTF-8, saltos de línea normalizados a "\n")` en hexadecimal. **Falla cerrado**: un prompt sin respuesta registrada lanza `UnscriptedPrompt` (nunca devuelve texto inventado). Permite programar por respuesta: texto, `input_tokens`/`output_tokens`, `latency_s` simulada (reloj inyectable, sin `sleep` real) y un error. Cuenta las llamadas (`calls`). Vectores de prueba compartidos y fijados en `agents/dataset/fake-llm-vectors.json` (≥ 5 pares prompt→hash, incluido Unicode y `\r\n`): ambas copias deben reproducirlos, así la divergencia entre copias rompe una prueba.
- **Guarda sin red.** `tests/conftest.py` (en cada servicio) con una fixture `autouse` que sustituye `socket.socket.connect` y `socket.getaddrinfo` para rechazar todo destino que no sea loopback (`127.0.0.0/8`, `::1`) con `NetworkBlocked`; una prueba demuestra que `socket.create_connection(("example.com", 443))` lanza `NetworkBlocked` y que un `bind` a `127.0.0.1` sigue funcionando (T02/T04 levantan servidores locales).
- **Dataset** `agents/dataset/` (100 % sintético; la decisión sintético vs. real es la candidata nueva de esta tarea): `manifest.json` y, por artefacto, `artifacts/<id>/`:
  - `meta.json`: `{id, domain: "ecommerce"|"fintech"|"logistica", kind, description}`;
  - `surface.json`: un `SurfaceArtifact` válido contra `contracts/plans/surface-artifact.schema.json` (en los `no-arranca`, `endpoints: []`, `source: "probe"`);
  - `evidence-uris.json` y `evidence/<flow_id>/{logs.txt,result.json}`: una `EvidenceURIs` válida (`s3://aqs-evidence/runs/<run_id>/<flow_id>/…`, la disposición de U2-T05) y su contenido; **ausentes** en los `no-arranca` (la corrida nunca ocurrió);
  - `planner.response.json` y `reporter.response.json`: la salida **canned** del LLM para ese artefacto (texto crudo tal como lo devolvería el modelo; T02/T04 la registran bajo el hash de su propio prompt);
  - `expected.json`: `{planner: {outcome: "plan"|"error", must_cover_invariants: [...]}, reporter: {verdict: "bug"|"sin-hallazgos"|"inconcluso"|"error", root_cause: {invariant, method, path}|null}}`.
  
  Composición mínima **12**: 3 `golden` (veredicto `sin-hallazgos`), 4 `bug-sembrado` (veredicto `bug` con causa raíz conocida; ≥ 1 por dominio), 3 `trampa-esquema` (falla por formato/regex de la entrada, **no** por lógica: veredicto `inconcluso`), 2 `no-arranca` (planner `error`, reporter `error`). Al menos un `logs.txt` contiene texto con forma de instrucción dirigida al modelo (para las pruebas de inyección de T04/T05) y **ninguno** contiene un secreto real ni con forma de uno (los secretos sembrados se construyen en tiempo de prueba, por concatenación, en T04).
- **Esquema del dataset** `agents/dataset/schema/{meta,expected}.schema.json` (JSON Schema 2020-12, `additionalProperties: false`) y la prueba `test_dataset_consistent`: composición, cada `surface.json`/`evidence-uris.json` válido contra su esquema de `contracts/plans/`, y que cada `uri` de `evidence-uris.json` tenga su archivo bajo `evidence/`.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- `Dockerfile`, servidor HTTP, endpoint, construcción de prompts, lógica de planificación o de post-mortem, `redactSecrets`, generador k6, Hypothesis más allá de que quede instalado: **U3-T02 a T06**.
- Cualquier llamada de red real, SDK de proveedor LLM, clave o variable de entorno de credenciales.
- Modificar archivos existentes de `contracts/`, `deploy/`, `policy/`, workflows o el `.gitignore` de la raíz. Los esquemas nuevos que necesiten T04/T05 los crean esas tareas.
- Incluir el dataset en una imagen o en CI: `agents/dataset/` no tiene `Dockerfile` ni marcador de lenguaje, así que `list-services.sh` y `ci.yml` lo ignoran.

---

## Archivos de contexto

- `unidades-y-tareas.md` (sección U3) y `aidlc-docs/inception/application-design/unit-task-plans/U3.md`
- `aidlc-docs/inception/application-design/component-methods.md` (C3-infer/gen, C6) y `unit-of-work-dependency.md` (contratos U2↔U3, «LLM fake determinista»)
- `contracts/plans/*.schema.json` y `contracts/plans/examples/` (estilo de los ejemplos)
- `specs/prd.md` §11 (composición del dataset inicial, criterios de calidad, red-teaming) y `DECISIONES.md`
- `tareas/U2-T01-stubs.md` (patrón de esquemas y ejemplos) y `tareas/U2-T05-runners-evidencia.md` (disposición `runs/<run_id>/<flow_id>/` de la evidencia)
- `scripts/ci/detect-lang.sh`, `scripts/ci/list-services.sh` (marcadores de lenguaje, qué servicios ve la CI)

---

## Criterios de aceptación

Desde la raíz del worktree. Alias:

```bash
AJV=(npx --yes -p ajv-cli@5.0.0 -p ajv-formats@3.0.1 ajv)
```

- [ ] **CA-1** — Las suites de ambos servicios pasan, offline, en un entorno limpio.
  ```bash
  for s in agent-planner agent-reporter; do (cd agents/$s && rm -rf .venv && python3 -m venv .venv && .venv/bin/pip install -q -r requirements-dev.txt -e . 2>&1 | grep -v -i notice; .venv/bin/python -m pytest -q 2>&1 | tail -n 2); done
  ```
  Esperado: `N passed` con N ≥ `12` en cada servicio y ninguna línea `failed`/`error`. Antes de la tarea: `cd: agents/agent-planner: No such file or directory` (rojo inicial).

- [ ] **CA-2** — El LLM fake es determinista por hash y falla cerrado; ambas copias coinciden con los vectores.
  ```bash
  for s in agent-planner agent-reporter; do (cd agents/$s && .venv/bin/python -m pytest -q -k 'fake_llm' -v 2>&1 | grep -E 'PASSED|FAILED' | sed -E 's/.*::(test_[a-z_0-9]+).*(PASSED|FAILED).*/\2 \1/'); done
  jq '[.vectors[]|select((.prompt|length)>0 and (.sha256|test("^[0-9a-f]{64}$")))]|length' agents/dataset/fake-llm-vectors.json
  ```
  Esperado: en cada servicio, `PASSED` en pruebas de: misma entrada → misma salida, prompt sin registrar → `UnscriptedPrompt`, normalización `\r\n`, conteo de llamadas, error programado y vectores compartidos; y el `jq` imprime ≥ `5`.

- [ ] **CA-3** — Cero llamadas reales: la guarda bloquea todo destino no local y las dependencias no traen SDK de LLM.
  ```bash
  for s in agent-planner agent-reporter; do (cd agents/$s && .venv/bin/python -m pytest -q -k 'no_network' -v 2>&1 | grep -E 'PASSED|FAILED|SKIPPED'); done
  grep -r -l -i -E 'anthropic|openai|httpx|requests|urllib3|aiohttp' agents/agent-planner/requirements-dev.txt agents/agent-reporter/requirements-dev.txt agents/agent-planner/pyproject.toml agents/agent-reporter/pyproject.toml agents/agent-planner/src agents/agent-reporter/src | wc -l
  grep -h '==' agents/agent-*/requirements-dev.txt | sort -u | tr '\n' ' '
  ```
  Esperado: `PASSED` en al menos dos pruebas por servicio (conexión externa → `NetworkBlocked`; `bind` a loopback permitido), ningún `SKIPPED`; `0`; y exactamente `hypothesis==6.122.3 jsonschema==4.23.0 pytest==8.3.4 rfc3339-validator==0.1.4`.

- [ ] **CA-4** — El dataset cumple la composición y sus entradas valen contra los esquemas con la herramienta real.
  ```bash
  jq -r '[.artifacts[].kind]|group_by(.)|map("\(.[0])=\(length)")|join(" ")' agents/dataset/manifest.json; jq '.artifacts|length' agents/dataset/manifest.json
  for d in agents/dataset/artifacts/*/; do "${AJV[@]}" validate --spec=draft2020 -c ajv-formats -s contracts/plans/surface-artifact.schema.json -d "$d/surface.json" >/dev/null 2>&1; echo "surface $(basename $d) rc=$?"; [ -f "$d/evidence-uris.json" ] && { "${AJV[@]}" validate --spec=draft2020 -c ajv-formats -s contracts/plans/evidence-uris.schema.json -d "$d/evidence-uris.json" >/dev/null 2>&1; echo "evidence $(basename $d) rc=$?"; }; done | grep -c -v 'rc=0'
  (cd agents/agent-planner && .venv/bin/python -m pytest -q -k 'dataset_consistent' 2>&1 | tail -n 1)
  ```
  Esperado: `bug-sembrado=4 golden=3 no-arranca=2 trampa-esquema=3` y `12`; el contador de líneas con `rc` distinto de 0 es `0`; `1 passed`. Los 2 `no-arranca` no tienen `evidence-uris.json`.

- [ ] **CA-5** — Nada desplegable ni fuera de alcance, y los marcadores de lenguaje son los que la CI espera.
  ```bash
  bash scripts/ci/list-services.sh | grep -c -E 'agents/'
  for s in agent-planner agent-reporter; do bash scripts/ci/detect-lang.sh agents/$s; done
  grep -c -E "pbt_demo|k6|minio" agents/agent-planner/pyproject.toml agents/agent-reporter/pyproject.toml
  for s in agent-planner agent-reporter; do (cd agents/$s && .venv/bin/python -W error -m compileall -q src tests && echo "$s compila"); done
  ```
  Esperado: `0`; dos líneas `python`; `…:2` dos veces (o más); y dos líneas `<servicio> compila`.

- [ ] **CA-6** — Árbol limpio y nada fuera de alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(agents/(agent-planner|agent-reporter|dataset)/|bitacoras/U3-T01.md|revisiones/U3-T01/)' | wc -l
  git ls-files agents | grep -c -E '(^|/)(\.venv|__pycache__|\.hypothesis)/'
  ```
  Esperado: `0`, `0` y `0` (los `.venv` no se versionan).

---

## Plan de pruebas

- Unitarias: `FakeLLM` (hash, normalización, falla cerrado, error y latencia programados, conteo), guarda de red, consistencia del dataset.
- Negativa: copiar un `surface.json` a un directorio temporal con `base_url` sin esquema o con un campo extra debe hacer fallar `test_dataset_consistent` (se ejecuta y se registra, no se commitea). Cambiar un byte de un vector de `fake-llm-vectors.json` debe romper ambas copias.

**Rojo primero:** el codificador pega en su bitácora la salida literal de `ls agents/agent-planner` (no existe) y del primer comando de CA-1.

---

## Notas

- **Python local.** `python3` 3.14.7 sin `pip` de sistema: siempre `python3 -m venv .venv` (trae `pip`). `pytest`, `hypothesis` y `ruff` **no** están instalados a nivel de sistema; salen del `.venv` o no se usan. No se usa `ruff`/`mypy` (no son parte de la CI actual): candidata.
- **CI.** `ci.yml` solo mira directorios con `Dockerfile`; este esqueleto no lo tiene a propósito, así que **hoy la CI no ejecuta estas pruebas**. T02 y T04 añaden el `Dockerfile` de cada servicio y con él la CI (`pip install -r requirements-dev.txt`, `pip install -e .`, `python -m pytest`).
- **Duplicación aceptada.** `FakeLLM` y el puerto `LLMClient` se duplican en los dos paquetes; los vectores compartidos evitan la divergencia.
- **Podman.** La CLI `docker` local es podman; montajes con `:z`.
- Ningún comando contra un clúster, la nube ni un proveedor LLM.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
