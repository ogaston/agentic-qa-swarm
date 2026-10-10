# U3-T07 — Integración con U2 real: planificar sobre superficie real de dev, reportar la corrida y medir el costo por corrida

**Unidad:** U3 — Agentes LLM
**Historias que implementa:** US-M4, US-M9 (camino feliz del journey 7.1 con los agentes reales; costo de inferencia y de cómputo medido por corrida)
**Depende de:** U3-T01 a U3-T06 **fusionadas**; **U2-T08** (adaptador `FlowSource` hacia U3 y workflow de contrato) y **U2-T05b** fusionadas; la **aprobación humana del entorno dev** (cada paso que toca el clúster la requiere); y la implementación previa del transporte NATS JetStream (C-45) y del despliegue/egress de LiteLLM y los agentes (C-83/C-51). El humano decidió LiteLLM compatible con OpenAI hacia `deepseek/deepseek-chat`; los agentes no reciben la clave de DeepSeek. Cierra U3. **No es despachable** mientras falte cualquiera. **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea, concreta):

> Sustituir los fakes de borde de los dos agentes por adaptadores reales (proveedor LLM por HTTP con circuito, lector/almacén S3 de MinIO), publicar los k6 desde `POST /v1/plan`, registrar el costo de inferencia y de cómputo por corrida, y dejar escrito y probado el **procedimiento de demostración en dev** que ejecuta el humano.

Detalle:

- **Adaptador LLM** (`llm_http.py` en cada servicio, solo `urllib.request` de la biblioteca estándar): implementa `LLMClient` contra el endpoint compatible con OpenAI de LiteLLM con `LLM_MODEL=deepseek/deepseek-chat`. Configuración por entorno: `LLM_PROVIDER=http`, `LLM_BASE_URL` (solo `https`; `http` únicamente a loopback en pruebas), `LLM_MODEL`, `LLM_API_KEY_FILE` (la clave se lee de un archivo montado desde un Secret; **nunca** por variable con valor, ni en logs, ni en `/metrics`, ni en el `detail`), `temperature=0`. LiteLLM conserva la clave de DeepSeek; los agentes usan únicamente su propia credencial de servicio si el gateway la exige. Respeta `timeout_s` y `max_tokens` del llamador. **Circuito**: tras `LLM_BREAKER_FAILURES` (5) fallos consecutivos queda abierto `LLM_BREAKER_OPEN_S` (30 s) y responde `LLMUnavailable` sin llamar; una llamada de prueba lo cierra. Sin reintentos automáticos.
- **MinIO** (dependencia `minio==7.2.15` en `requirements.txt` de runtime, comprobada descargable): `S3EvidenceReader` (lee `s3://<bucket>/runs/<run_id>/<flow_id>/…`), `S3ReportStore` (`runs/<run_id>/report/report.json`) y `S3FlowStore` (`runs/<run_id>/flows/<flow_id>.k6.js`), todos con lectura de vuelta y comparación de hash tras escribir. Configuración `EVIDENCE_ENDPOINT`, `EVIDENCE_BUCKET` y credenciales por archivo (`EVIDENCE_ACCESS_KEY_FILE`, `EVIDENCE_SECRET_KEY_FILE`), como U2-T05. El agente solo lee `runs/*/*/{logs.txt,result.json}` y solo escribe bajo `report/`, `flows/` y `_cost/`.
- **Cableado del planner.** `POST /v1/plan` sigue devolviendo **solo** el `FlowPlan` (U2-T08 lo valida contra su esquema), pero antes de responder llama a `publish_flows` (T03): si la publicación falla (validación de k6 incluida), la respuesta es `502 {error: "flows_not_publishable"}` y **no** se devuelve plan. `k6 inspect` real no está en la imagen de producción: en el servicio se usa la validación de capas 1–2 y la capa 3 queda en las pruebas (decisión documentada en el README).
- **Cableado del reporter.** Un consumidor de `run.done` (puerto `EventSubscriber` + el fake de T04; el transporte real depende de C-45) llama a `correlate_postmortem`, guarda el reporte y publica `report.ready`. El reporter tolera `run.done` duplicado (at-least-once): el segundo evento no publica un segundo `report.ready` (idempotencia por `run_id`, comprobada por lectura del almacén).
- **Costo por corrida.** Por cada `run_id`, cada agente escribe `runs/<run_id>/_cost/<agente>.json` con `{run_id, agent, llm: {calls, input_tokens, output_tokens, latency_s, cost_usd}, compute: {cpu_seconds, wall_seconds}}`. `cost_usd = input_tokens/1e6*LLM_PRICE_IN_PER_MTOK + output_tokens/1e6*LLM_PRICE_OUT_PER_MTOK` con precios por entorno (**sin valores por defecto**: si faltan, el servicio no arranca en modo `http`). Métricas `aqs_u3_llm_cost_usd_total{agent}`, `aqs_u3_llm_tokens_total{agent,direction}`, `aqs_u3_cpu_seconds_total{agent}`. El **costo del cómputo de los runners** lo da U2; la suma por corrida se hace en el guion de la demo, no aquí.
- **Servicios de arranque local** `scripts/test/u3-up.sh` y `scripts/test/u3-down.sh` (nuevos): levantan el planner en `127.0.0.1:18400` y el reporter en `127.0.0.1:18401` con `LLM_PROVIDER=fake` (+ `PLANNER_ALLOW_FAKE`/`REPORTER_ALLOW_FAKE`) cuyas respuestas salen del dataset (y de `U3_FAKE_SURFACES_DIR` si se define, para superficies de la prueba de contrato de U2), y MinIO local; es lo que U2-T08 usa como `up`/`down` en su criterio de contrato.
- **Guion de la demostración en dev** `docs/demo-dev-u3.md` (nuevo): comandos exactos, en orden, con la salida esperada, **sin credenciales** (Secrets por nombre) y el marcador `APROBACIÓN HUMANA REQUERIDA` antes de cada comando que cambia algo en el clúster. Cubre: `run.confirmed` real → superficie → plan → k6 publicados → ensayo/ejecución (U2) → `run.done` → reporte con enlaces a evidencia → `report.ready`; **costo por corrida** (suma de `_cost/*.json` y del cómputo de runners); **inyección** (una app de prueba cuya respuesta contiene una instrucción: el plan y el reporte no cambian de comportamiento); **proveedor caído** (circuito abierto → la fase falla cerrada y la corrida llega a `resetting`).
- **Script de verificación local** `scripts/test/u3-demo-local.sh` (nuevo): ejecuta la parte sin clúster (MinIO local + proveedor LLM simulado en loopback + ambos servicios reales) e imprime `OK|FALLA <comprobación>`; es lo que el codificador y el revisor corren en el loop.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Ejecutar nada contra un clúster, la nube o un proveedor LLM real en el loop. **El humano ejecuta `docs/demo-dev-u3.md` en dev**, con su aprobación, y anota los resultados (incluida la métrica de costo) en la bitácora.
- Manifiestos de despliegue de los agentes, NetworkPolicy de egress hacia el proveedor, Secrets y SOPS (C-51, C-53, C-74: decisión y tarea aparte); construir y publicar imágenes.
- Cambiar los umbrales o el dataset (T05), el contrato `contracts/**`, los servicios de U1/U2/U4/U5, `ci.yml` u otros workflows.
- Cola de peticiones, autoescalado, caché de respuestas del LLM, múltiples proveedores.
- Medir calidad con el modelo real sobre el dataset (candidata): la demo mide costo y comportamiento, no los umbrales del §10.

---

## Archivos de contexto

- `unidades-y-tareas.md` (U3-T7; dimensionado especial: costo/latencia, timeouts, circuit breaker), `unit-task-plans/U3.md`, `unit-of-work-dependency.md`
- `tareas/U2-T08-integracion-u3-dev.md` (cliente de U2, workflow de contrato, guion `docs/demo-dev-u2.md`, `scripts/test/u2-demo-local.sh`) y `U2-T05-runners-evidencia.md` / `U2-T05b-…` (disposición de la evidencia y `result.json`)
- `agents/agent-planner/`, `agents/agent-reporter/`, `agents/eval/` (T01–T06)
- `scripts/test/minio-local.sh` (MinIO local con imagen fijada por digest) y `.github/workflows/integration-u1-u4.yml` (patrón de integración)
- `docs/` (formato de documentación existente), `tareas/candidatas.md` (C-45, C-51, C-53 y las nuevas de U3)

---

## Criterios de aceptación

Desde la raíz del worktree; `.venv` de cada servicio reinstalado con `pip install -r requirements-dev.txt -e .` (los `requirements-dev.txt` incluyen `-r requirements.txt`).

- [ ] **CA-1** — Pruebas en verde de los dos servicios y de la evaluación, con lo anterior intacto.
  ```bash
  for s in agent-planner agent-reporter eval; do (cd agents/$s && .venv/bin/pip install -q -r requirements-dev.txt -e . 2>&1 | grep -v -i notice; l=$(mktemp); .venv/bin/python -m pytest -p no:cacheprovider -m 'not pbt_demo' > "$l" 2>&1; tail -n 1 "$l"; grep -c -E 'FAILED|ERROR' "$l"); done
  (cd agents/eval && .venv/bin/python -m agent_eval run --out out >/dev/null; echo "eval rc=$?")
  ```
  Esperado: por servicio, `N passed` (sin `failed`/`skipped` en las pruebas que usan k6, podman o MinIO), `0`, y `eval rc=0` (los umbrales de T05 siguen cumpliéndose).

- [ ] **CA-2** — El adaptador LLM respeta límites, circuito y secreto (contra un servidor simulado en loopback).
  ```bash
  for s in agent-planner agent-reporter; do (cd agents/$s && .venv/bin/python -m pytest -p no:cacheprovider -k 'llm_http' -v 2>&1 | grep -E 'PASSED|FAILED|SKIPPED' | sed -E 's/.*::(test_[a-z_0-9]+).*(PASSED|FAILED|SKIPPED).*/\2 \1/'); done
  ```
  Esperado: `PASSED` en al menos ocho pruebas por servicio: éxito con tokens reales de la respuesta; `timeout_s` y `max_tokens` llegan al proveedor; 5 fallos seguidos abren el circuito y la 6.ª llamada **no** llega al servidor (contador del servidor simulado); tras `LLM_BREAKER_OPEN_S` una llamada de prueba lo cierra; `http://` no loopback rechazado al arrancar; clave leída de archivo y ausente de logs, `/metrics` y `detail` (marcador único); respuesta no JSON o sin campos → `LLMUnavailable`; sin reintentos (1 petición por llamada).

- [ ] **CA-3** — Evidencia, reporte y k6 viajan por **MinIO real** y se leen de vuelta.
  ```bash
  cd agents/agent-reporter && .venv/bin/python -m pytest -p no:cacheprovider -m minio -k 's3' -v 2>&1 | grep -E 'PASSED|FAILED|SKIPPED' | sed -E 's/.*::(test_[a-z_0-9]+).*(PASSED|FAILED|SKIPPED).*/\2 \1/'
  cd ../agent-planner && .venv/bin/python -m pytest -p no:cacheprovider -m minio -k 's3' -v 2>&1 | grep -E 'PASSED|FAILED|SKIPPED' | sed -E 's/.*::(test_[a-z_0-9]+).*(PASSED|FAILED|SKIPPED).*/\2 \1/'
  ```
  Esperado: `PASSED` en pruebas que levantan MinIO con la imagen fijada por digest de `scripts/test/minio-local.sh` y: leen la evidencia sembrada de un artefacto del dataset con la disposición `runs/<run_id>/<flow_id>/`; guardan y releen `report.json` con el mismo hash; publican y releen los `*.k6.js`; un bucket inexistente o una escritura rechazada → error tipado y **nada** declarado como publicado; el agente **no** escribe fuera de `report/`, `flows/` y `_cost/` (intento → error); ningún `SKIPPED`.

- [ ] **CA-4** — Cableado del planner: publica antes de responder y falla cerrado.
  ```bash
  cd agents/agent-planner && .venv/bin/python -m pytest -p no:cacheprovider -k 'server and publish' -v 2>&1 | grep -E 'PASSED|FAILED|SKIPPED' | sed -E 's/.*::(test_[a-z_0-9]+).*(PASSED|FAILED|SKIPPED).*/\2 \1/'
  ```
  Esperado: `PASSED` en pruebas de: `200` devuelve exactamente un `FlowPlan` válido (sin claves extra) y el almacén contiene un k6 por flujo; fallo de publicación → `502 flows_not_publishable` y el almacén queda vacío; la respuesta es idéntica con y sin almacén disponible cuando la publicación se omite por configuración explícita (`PLANNER_PUBLISH=false`, solo para pruebas, rechazado con `PLANNER_ENV=prod`).

- [ ] **CA-5** — Reporter: evento idempotente y orden guardar → publicar.
  ```bash
  cd agents/agent-reporter && .venv/bin/python -m pytest -p no:cacheprovider -k 'consumer or idempot' -v 2>&1 | grep -E 'PASSED|FAILED|SKIPPED' | sed -E 's/.*::(test_[a-z_0-9]+).*(PASSED|FAILED|SKIPPED).*/\2 \1/'
  ```
  Esperado: `PASSED` en pruebas de: `run.done` → reporte guardado → un `report.ready` válido contra su esquema; el mismo `run.done` dos veces → un solo `report.ready`; `run.done` inválido contra `run.done.schema.json` → se descarta con métrica y sin reporte; fallo del almacén → sin evento; caída del LLM → sin evento y error visible en métrica (la corrida de U2 no se bloquea).

- [ ] **CA-6** — **Costo por corrida registrado** y con lectura de vuelta.
  ```bash
  cd agents/agent-planner && .venv/bin/python -m pytest -p no:cacheprovider -k 'cost' -v 2>&1 | grep -E 'PASSED|FAILED|SKIPPED' | sed -E 's/.*::(test_[a-z_0-9]+).*(PASSED|FAILED|SKIPPED).*/\2 \1/'; cd ../agent-reporter && .venv/bin/python -m pytest -p no:cacheprovider -k 'cost' -v 2>&1 | grep -E 'PASSED|FAILED|SKIPPED' | sed -E 's/.*::(test_[a-z_0-9]+).*(PASSED|FAILED|SKIPPED).*/\2 \1/'
  ```
  Esperado: `PASSED` en pruebas de: `_cost/<agente>.json` con tokens exactos de las respuestas simuladas y `cost_usd` igual a la fórmula con precios dados (cálculo a mano en la prueba); sin precios en modo `http` el servicio no arranca (`rc` distinto de 0); los contadores `aqs_u3_*` coinciden con el archivo; un fallo de escritura del costo **no** tumba la corrida (se cuenta y se registra).

- [ ] **CA-7** — Recorrido local completo (sin clúster) con los servicios reales.
  ```bash
  bash scripts/test/u3-demo-local.sh
  ```
  Esperado: líneas `OK <comprobación>` y ninguna `FALLA`, que cubren: superficie del dataset → plan válido → k6 en MinIO → evidencia sembrada → reporte con enlaces a evidencia → `report.ready`; costo por corrida presente; inyección en superficie y en logs sin cambio de comportamiento; proveedor simulado caído → ambas fases fallan cerrado; MinIO caído → idem. Termina con `rc=0` y deja el árbol limpio.

- [ ] **CA-8** — Los scripts de arranque sirven al contrato de U2.
  ```bash
  bash scripts/test/u3-up.sh && curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:18400/healthz http://127.0.0.1:18401/healthz
  cd services/go-run-controller && U3_URL=http://127.0.0.1:18400 go test -tags contract -run 'Contract(U3)' -v ./... | grep -E '^\s*--- (PASS|FAIL|SKIP)|^(ok|FAIL)'; cd ../.. && bash scripts/test/u3-down.sh; curl -s -o /dev/null -w '%{http_code}\n' --max-time 2 http://127.0.0.1:18400/healthz
  ```
  Esperado: `200` y `200`; al menos 3 `--- PASS` y ningún `FAIL`/`SKIP` (los de U2-T08); tras `u3-down.sh` el último `curl` imprime `000`.

- [ ] **CA-9** — Imágenes, higiene y alcance.
  ```bash
  for s in agent-planner agent-reporter; do docker build -q -t aqs-$s:ci agents/$s >/dev/null && docker inspect aqs-$s:ci --format '{{.Config.User}}'; docker run --rm aqs-$s:ci python -c "import pytest" 2>&1 | tail -n 1; done
  grep -r -l -i -E 'anthropic|openai|httpx|requests' agents/agent-planner/requirements.txt agents/agent-reporter/requirements.txt agents/agent-planner/src agents/agent-reporter/src | wc -l
  grep -r -n -E 'AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9]{36}|sk-[A-Za-z0-9]{20,}' agents scripts docs --exclude-dir=.venv --exclude-dir=__pycache__ --exclude-dir=.hypothesis | wc -l
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(agents/(agent-planner|agent-reporter)/|scripts/test/u3-(up|down|demo-local)\.sh|docs/demo-dev-u3\.md|bitacoras/U3-T07\.md|revisiones/U3-T07/)' | wc -l
  ```
  Esperado: por imagen `65532:65532` y `ModuleNotFoundError`; `0` (ningún SDK de proveedor ni cliente HTTP de terceros declarado; `urllib3` llega solo transitivo vía `minio`); `0`; `0` y `0`. El filtro no incluye `agents/eval`, `contracts/`, `deploy/`, `policy/` ni workflows.

---

## Plan de pruebas

- Unitarias y de contrato con servidor simulado en loopback (proveedor LLM), MinIO real (imagen por digest), fakes de eventos, y los scripts de integración local (CA-7, CA-8). La guarda sin red de T01 sigue activa: solo se permite loopback.
- **Barrido de clase** (qué puede colgar o filtrar): tabla camino→prueba→mutación en la bitácora para circuito, tiempo y tokens, clave del proveedor, escritura/lectura S3, orden guardar → publicar, idempotencia de `run.done`, costo.
- **Mutaciones** (copia, `timeout 60`): sin circuito, `max_tokens` no propagado, clave en un log, publicar el evento antes de guardar, ignorar el duplicado, calcular el costo con solo la entrada, aceptar `http://` público. Cada una debe poner una prueba en rojo; pegar el rojo.

**Rojo primero:** pegar la salida de `pytest -k llm_http` (no existe `llm_http`) y de `bash scripts/test/u3-demo-local.sh` (script inexistente) antes de implementar.

---

## Notas

- **Esta tarea no se despacha** hasta que estén implementados el transporte NATS JetStream (C-45) y el despliegue/egress de LiteLLM y los agentes (C-83/C-51), y el humano apruebe el entorno dev. Las rondas del loop nunca llaman al proveedor real.
- **Costo.** La métrica es de medición, no de presupuesto: el tope de gasto por corrida es una decisión abierta (candidata); el tope duro de tokens y tiempo ya existe desde T02/T04.
- `minio==7.2.15` se instala con `pip` en el `.venv`; MinIO local usa la imagen fijada de `scripts/test/minio-local.sh` (registro `cgr.dev`; si no es alcanzable desde el entorno del loop, es un bloqueo a reportar, ver C-58). `curl` se usa solo contra loopback.
- Python y herramientas como en U3-T01. Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
