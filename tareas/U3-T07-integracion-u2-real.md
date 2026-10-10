# U3-T07 — Integración local U2↔U3: adaptador LLM compatible con OpenAI y recorrido completo en loopback

**Unidad:** U3 — Agentes LLM
**Historias que implementa:** US-M4, US-M9 (camino feliz del journey 7.1 con los agentes reales, verificado en local)
**Depende de:** U3-T01 a U3-T06 y U2-T08 **fusionadas** (todas lo están). **Despachable ya**: no requiere clúster, proveedor LLM real, NATS ni aprobación del entorno dev. Cierra U3 y el MVP. **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

> **Simplificada (2026-10-10, decisión del humano).** La versión anterior exigía la infraestructura de C-45 (NATS JetStream), C-83/C-51 (LiteLLM y Deployments con egress), adaptadores S3 de MinIO, medición de costo por corrida y un guion de demo en dev. Todo eso pasa al backlog post-MVP de `unidades-y-tareas.md`. Las decisiones C-45, C-82 y C-83 siguen vigentes para cuando se reactive; esta tarea no las contradice: el adaptador habla el protocolo compatible con OpenAI, que es el que expone LiteLLM.

---

## Alcance

**Dentro** (una línea, concreta):

> Añadir a los dos agentes un adaptador LLM HTTP compatible con OpenAI (probado contra un servidor simulado en loopback), unos scripts `u3-up.sh`/`u3-down.sh` que levantan planner y reporter en loopback con el `FakeLLM`, y un recorrido local que pasa el contrato de U2 contra el planner real y produce un reporte a partir de evidencia en disco.

Detalle:

- **Adaptador LLM** (`llm_http.py` en `agent_planner` y en `agent_reporter`, duplicado con una nota; solo `urllib.request` de la biblioteca estándar). Implementa `LLMClient.complete(prompt, *, max_tokens, timeout_s)` con `POST {LLM_BASE_URL}/chat/completions` (`model`, `messages=[{role: user, content: prompt}]`, `max_tokens`, `temperature: 0`). Devuelve `LLMResult` con `choices[0].message.content` y los tokens de `usage`. Configuración: `LLM_PROVIDER=http`, `LLM_BASE_URL` (solo `https`; `http` únicamente hacia loopback), `LLM_MODEL`, `LLM_API_KEY_FILE` (la clave se lee de archivo; nunca aparece en logs, `/metrics` ni en el `detail` de un error). Timeout → `LLMTimeout`; error HTTP, respuesta no JSON o sin campos → `LLMUnavailable`. **Sin reintentos y sin circuito** (el controlador de U2 ya tiene circuito hacia U3).
- **Cableado**: `config.py` del planner y `build_service` del reporter aceptan `LLM_PROVIDER=http` además de `fake`; cualquier otro valor sigue impidiendo el arranque.
- **Arranque local** `scripts/test/u3-up.sh` y `scripts/test/u3-down.sh` (nuevos): planner en `127.0.0.1:18400` y reporter en `127.0.0.1:18401`, ambos con `LLM_PROVIDER=fake` (+ `PLANNER_ALLOW_FAKE=true` / `REPORTER_ALLOW_FAKE=1`). El `FakeLLM` del planner responde también a las superficies de `U3_FAKE_SURFACES_DIR` si se define (para la superficie del contrato de U2). El reporter lee evidencia de `REPORTER_EVIDENCE_DIR` (el `DirEvidenceReader` existente).
- **Recorrido local** `scripts/test/u3-demo-local.sh` (nuevo): `u3-up` → contrato de U2 contra el planner real (`U3_URL=http://127.0.0.1:18400 go test -tags contract -run 'Contract(U3)'`) → `POST /v1/report` con evidencia sembrada de un artefacto del dataset → reporte válido contra `report.schema.json` con enlaces a evidencia → `u3-down`. Imprime `OK|FALLA <comprobación>` por cada paso.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Clúster, nube, proveedor LLM real, LiteLLM, NATS, MinIO/S3, manifiestos, Secrets, imágenes.
- Medición de costo por corrida, circuito en los agentes, cola, caché o múltiples proveedores.
- Guion de demo en dev (`docs/demo-dev-u3.md`).
- Cambiar el dataset o los umbrales de T05, las propiedades de T06, `contracts/**`, los servicios de U1/U2/U4/U5 o los workflows.

---

## Archivos de contexto

- `unidades-y-tareas.md` (U3), `tareas/U2-T08-integracion-u3-dev.md` y `bitacoras/U2-T08.md` (adaptador `FlowSourceU3` y contrato; `TestContractU3ValidFlowPlan` ya usa `U3_URL` si está definida)
- `agents/agent-planner/src/agent_planner/{llm,config,server}.py`, `agents/agent-reporter/src/agent_reporter/{llm,server}.py` y sus `fakes/`
- `agents/dataset/` (superficies y evidencias), `contracts/plans/report.schema.json`
- `scripts/test/u2-demo-local.sh` (formato `OK|FALLA`)

---

## Criterios de aceptación

Desde la raíz del worktree; `.venv` de cada servicio con `pip install -r requirements-dev.txt -e .`.

- [ ] **CA-1** — Lo anterior sigue en verde.
  ```bash
  for s in agent-planner agent-reporter eval; do (cd agents/$s && .venv/bin/python -m pytest -p no:cacheprovider -m 'not pbt_demo' 2>&1 | tail -n 1); done
  (cd agents/eval && .venv/bin/python -m agent_eval run --out out >/dev/null; echo "eval rc=$?")
  ```
  Esperado: `N passed` sin `failed` en los tres y `eval rc=0`.

- [ ] **CA-2** — El adaptador LLM cumple su contrato (servidor simulado en loopback).
  ```bash
  for s in agent-planner agent-reporter; do (cd agents/$s && .venv/bin/python -m pytest -p no:cacheprovider -k 'llm_http' -v 2>&1 | grep -c PASSED); done
  ```
  Esperado: al menos `6` por servicio, que cubren: éxito con texto y tokens de la respuesta; `max_tokens` y `temperature: 0` llegan al servidor; timeout → `LLMTimeout`; 5xx, no JSON o sin `choices` → `LLMUnavailable`; `http://` no loopback rechazado al configurar; la clave leída de archivo va en `Authorization` y no aparece en logs ni en el mensaje del error (marcador único); exactamente 1 petición por llamada.

- [ ] **CA-3** — Arranque y parada locales.
  ```bash
  bash scripts/test/u3-up.sh && curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:18400/healthz http://127.0.0.1:18401/healthz
  bash scripts/test/u3-down.sh; curl -s -o /dev/null -w '%{http_code}\n' --max-time 2 http://127.0.0.1:18400/healthz
  ```
  Esperado: `200`, `200` y, tras parar, `000`.

- [ ] **CA-4** — Recorrido local completo.
  ```bash
  bash scripts/test/u3-demo-local.sh 2>&1 | grep -E '^(OK|FALLA)'; echo "rc=${PIPESTATUS[0]}"
  ```
  Esperado: solo líneas `OK`, al menos `contrato-u2-plan-valido`, `contrato-u2-plan-invalido`, `contrato-u2-u3-detenido`, `reporte-valido`, `reporte-con-evidencia`; ninguna `FALLA`; `rc=0`; tras terminar, ningún proceso escuchando en 18400/18401.

- [ ] **CA-5** — Higiene y alcance.
  ```bash
  grep -r -l -i -E 'anthropic|openai|httpx|requests' agents/agent-planner/requirements*.txt agents/agent-reporter/requirements*.txt 2>/dev/null | wc -l
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(agents/(agent-planner|agent-reporter)/|scripts/test/u3-(up|down|demo-local)\.sh|bitacoras/U3-T07\.md|revisiones/U3-T07/)' | wc -l
  ```
  Esperado: `0` (ningún SDK ni cliente HTTP de terceros), `0` y `0`.

---

## Plan de pruebas

- Unitarias del adaptador con un `http.server` en loopback que cuenta peticiones y registra cuerpo y cabeceras. La guarda sin red de T01 sigue activa: solo loopback.
- Recorrido local con los servicios reales y el `FakeLLM` (CA-4).

**Rojo primero:** pegar la salida de `pytest -k llm_http` (no existe) y de `bash scripts/test/u3-demo-local.sh` (no existe) antes de implementar.

---

## Notas

- Ninguna ronda del loop llama a un proveedor real. Probar contra DeepSeek/LiteLLM es un paso manual del humano, fuera de esta tarea.
- Python y herramientas como en U3-T01. Go como en U2-T08 (`GOTOOLCHAIN=auto` si hace falta).

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
