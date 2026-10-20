# U8-T09a — La superficie describe el cuerpo de los POST y el planner lo usa

**Unidad:** U8 — Producto demostrable (ciclo completo en kind)
**Historias que implementa:** US-M4 (superficie externa), US-M5 (plan ensayable), US-M9 (veredicto sin falsos positivos).
**Depende de:** U8-T08b fusionada (lo está: el plan ya admite `body`). **Bloquea a U8-T09** (CA-2, ronda 2). **Tope: 3 rondas**.

---

## Origen (evidencia de U8-T09, ronda 1)

Con LiteLLM + DeepSeek reales, `kind-e2e.sh --bug off` falla con `veredicto-sin-hallazgos`: el plan del LLM envía `POST /orders` sin cuerpo, la app sana responde `400` y el reporter lo declara defecto (detalle en `bitacoras/U8-T09.md`, rama `tarea/U8-T09`, sección CA-2).

La causa es de contrato, no solo de prompt: `SurfaceArtifact` (`contracts/plans/surface-artifact.schema.json`, `additionalProperties: false`) lleva solo `method` y `path` por endpoint. `parseOpenAPI` en `services/go-warm-manager/service.go` descarta el `requestBody` del OpenAPI de la app (`demo/target-app/openapi.json` sí lo declara para `POST /orders`: `product_id` y `qty` enteros ≥ 1). El LLM no puede inventar un cuerpo que nunca vio.

---

## Alcance

**Dentro** (una línea, concreta):

> Añadir `request_body` (esquema JSON del cuerpo `application/json`, opcional) a cada endpoint de la superficie, extraerlo del OpenAPI en `go-warm-manager`, y hacer que el planner lo reciba en el prompt y emita `body` válido en los pasos de POST/PUT/PATCH, con una validación determinista que rechace un plan cuyo `body` incumpla ese esquema.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- `scripts/kind/`, `deploy/flux/`, LiteLLM, secretos y todo lo de U8-T09 (rama `tarea/U8-T09`): el e2e real se repite en la ronda 2 de U8-T09, no aquí.
- Cambiar `expect_status`, `invariant_probe`, el gate de ensayo o los runners (U8-T08b ya los fijó).
- Cabeceras, autenticación, cuerpos que no sean objeto JSON, `$ref` externos del OpenAPI, parámetros de ruta/consulta.
- Generar datos «inteligentes» (valores que provoquen el defecto): el planner solo debe producir cuerpos **válidos** según el esquema; sondear invariantes sigue siendo del flujo con `invariant_probe`.
- Persistir uso de tokens (P4), TLS de base/dev/prod, `scripts/test/u8-agents-kind.sh`.

---

## Archivos de contexto

- `bitacoras/U8-T09.md` (rama `tarea/U8-T09`, worktree `../agentic-qa-swarm-wt-U8-T09`; CA-2)
- `contracts/plans/surface-artifact.schema.json`, `contracts/plans/examples/`, y su copia en `agents/agent-planner/src/agent_planner/schemas/`
- `services/go-warm-manager/service.go` (`Endpoint`, `parseOpenAPI`), `surface.go` (`ValidateSurface` y su prueba de paridad con el esquema)
- `agents/agent-planner/src/agent_planner/prompt.py`, `planner.py`, `validate.py`, `fakes/surfaces.py`, `tests/`
- `services/go-run-controller/plan/validate.go`, `adapters/flowsource_u3.go` (reenvía la superficie al planner)
- `demo/target-app/openapi.json` (solo lectura)

---

## Criterios de aceptación

- [ ] **CA-1** — El contrato acepta `request_body` opcional, rechaza lo malformado y las copias del esquema no divergen.
  ```bash
  diff contracts/plans/surface-artifact.schema.json agents/agent-planner/src/agent_planner/schemas/surface-artifact.schema.json && echo IGUALES
  cd services/go-warm-manager && go test ./... -count=1 2>&1 | tail -4
  ```
  Esperado: `IGUALES` y `go test` en verde, con ejemplos `valid` (endpoint con `request_body`, endpoint sin él) e `invalid` (`request_body` que no es objeto, campo extra) y la prueba de paridad de `ValidateSurface` cubriéndolos.

- [ ] **CA-2** — La inferencia conserva el cuerpo del OpenAPI de la app objetivo.
  ```bash
  cd services/go-warm-manager && go test ./... -count=1 -run 'OpenAPI.*(Body|Cuerpo)' -v 2>&1 | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS` en pruebas que alimentan `demo/target-app/openapi.json` y comprueban que `POST /orders` sale con `request_body` que exige `product_id` y `qty`, y que `GET /orders` sale sin él; un OpenAPI sin `requestBody` o con `$ref` externo no rompe la inferencia (el endpoint sale sin cuerpo).

- [ ] **CA-3** — El planner usa el esquema: el prompt lo incluye y un plan con cuerpo inválido no pasa.
  ```bash
  cd agents/agent-planner && python -m pytest -q -k 'prompt or body or validate or plan' 2>&1 | tail -3
  ```
  Esperado: verde, con (a) prueba de que el prompt documenta que los pasos de POST/PUT/PATCH con `request_body` en la superficie deben llevar `body` conforme; (b) prueba de que `validate.py` rechaza un plan cuyo paso `POST /orders` no trae `body` o trae `qty` como texto, y acepta `{"product_id":1,"qty":1}`; (c) los planes y superficies existentes sin `request_body` siguen validando igual.

- [ ] **CA-4** — La lectura de vuelta contra la app real: un plan con el cuerpo generado deja pasar un flujo sano.
  ```bash
  cd services/aqs-runner && go test ./... -count=1 -run 'Orders.*(Cuerpo|Body)' -v 2>&1 | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS` en una prueba que toma la superficie inferida del OpenAPI de `demo/target-app`, el plan determinista del planner con `LLM_PROVIDER` falso para esa superficie, y ejecuta el flujo `POST /orders` contra el handler real de `demo/target-app` (sin defecto): el paso recibe `201` y el runner sale `0`. Sin `body` el mismo paso recibe `400` (par falla-pasa).

- [ ] **CA-5** — Alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(contracts/plans/|agents/agent-planner/|services/go-warm-manager/|services/go-run-controller/(plan|adapters)/|services/aqs-runner/|demo/target-app/|bitacoras/U8-T09a\.md|revisiones/U8-T09a/)' | wc -l; git diff --name-only $b -- scripts/kind deploy services/go-governance services/go-reset | wc -l
  ```
  Esperado: `0`, `0` y `0` (más el archivo de esta tarea si el worktree parte de una rama de docs; el codificador lo aclara en la bitácora).

---

## Plan de pruebas

- **Rojo primero:** antes de tocar el contrato, validar una superficie con `request_body` contra el esquema actual y pegar el rechazo por `additionalProperties`; y correr la prueba de CA-2 contra `parseOpenAPI` actual: debe fallar porque descarta el cuerpo.
- Negativas del esquema y de `ValidateSurface` (que replica el esquema): `request_body` no objeto, campo extra en el endpoint.
- Validación del cuerpo del plan contra `request_body`: tipos, `required`, `minimum`; un esquema que el validador no soporta degrada a «no comprobar» y se documenta, nunca a error de ejecución.
- Sensibilidad de CA-4: quitando `body` del paso, la prueba debe fallar con `400`.

---

## Notas

- La superficie viaja por MinIO (`surface_uri`) y por `POST {U3_URL}/v1/plan`; el campo nuevo es opcional, así que superficies viejas siguen válidas.
- El prompt del reporter y las fixtures del LLM falso dependen del texto del prompt del planner (candidata C-122): si cambia la plantilla, regenerar con `scripts/kind/gen_agent_fixtures.py` queda para U8-T09 (ronda 2), no aquí; el codificador lo anota en su bitácora.
- Archivos que se **modifican en su sitio**; nada de duplicados con sufijo. Rama `tarea/U8-T09a`, worktree `../agentic-qa-swarm-wt-U8-T09a`, desde `main`.
- Tras fusionarla, el orquestador rebasa `tarea/U8-T09` y despacha su ronda 2.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
