# U8-T08b — Cuerpo opcional en los pasos del plan y ensayo que no bloquea por el flujo de sondeo

**Unidad:** U8 — Producto demostrable (ciclo completo en kind)
**Historias que implementa:** US-M5 (ensayo bloqueante), US-M6 (runners con evidencia), US-M9 (hallazgos de lógica de negocio).
**Depende de:** U8-T08a fusionada (lo está). **Bloquea a U8-T08** (CA-2, ronda 3). **Tope: 3 rondas**.

---

## Origen (evidencia de U8-T08, ronda 2)

Con la plataforma real en kind, `--bug oversell` no puede producir un hallazgo, por dos huecos del contrato (detalle y salidas en `bitacoras/U8-T08.md`, sección «Ronda 2», rama `tarea/U8-T08`):

1. **El plan no puede enviar un cuerpo JSON.** `contracts/plans/flow-plan.schema.json` limita cada paso a `method`, `path`, `expect_status` (`additionalProperties: false`), y `services/aqs-runner` envía siempre `bytes.NewReader(nil)`. El defecto sembrado de `demo/target-app` (oversell) solo se activa en `POST /orders` con `qty` en el cuerpo (`demo/target-app/main.go`, handler `POST /orders`), así que ningún flujo puede provocarlo.
2. **El ensayo bloquea por el flujo que debe fallar.** El ensayo ejecuta el plan contra el warm y exige que cada paso cumpla su `expect_status`. Un flujo que sondea una invariante (cuyo fallo es el hallazgo) mata el ensayo: el controlador pasa a `resetting → failed` y nunca llega a los runners.

---

## Decisión de diseño (tomada al redactar; el humano puede revertirla)

- Paso: campo opcional **`body`** (objeto JSON). El runner lo envía con `Content-Type: application/json`. Sin `body`, el comportamiento actual (sin cuerpo) no cambia.
- Flujo: campo opcional booleano **`invariant_probe`** (por defecto `false`). Lo marca el planner en los flujos que **sondean** una invariante (casos límite/negativos cuyo fallo sería un hallazgo).
- **Ensayo:** para un flujo con `invariant_probe: true` el ensayo exige que sea **ejecutable** (cada paso recibe respuesta HTTP, sin error de red ni timeout, y estado `< 500`), no que cumpla `expect_status`. Para los demás flujos el ensayo sigue **estricto** (cada paso cumple `expect_status`). Esto no relaja el gate para los flujos normales: un plan mal formado o con rutas inventadas sigue sin pasar el ensayo.
- **Runners:** siempre estrictos (`expect_status`): un paso que no lo cumple es evidencia de un hallazgo, con o sin `invariant_probe`.

---

## Alcance

**Dentro** (una línea, concreta):

> Añadir `body` (paso) e `invariant_probe` (flujo) como campos opcionales de `flow-plan` en el esquema de `contracts/plans/`, en su copia de `agents/agent-planner/`, en `plan.go`/`validate.go` del controlador y en `aqs-runner` (envía el cuerpo; el modo `rehearse` aplica la semántica de ejecutable a los flujos de sondeo), con el planner capaz de emitirlos y con ejemplos y pruebas que cubran ambos caminos.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- `scripts/kind/` y todo lo de U8-T08 (rama `tarea/U8-T08`): el e2e se ajusta en la ronda 3 de U8-T08, no aquí.
- Cambiar la semántica de `expect_status` en los runners, el gate duro `ensayo_passed` del controlador, o `go-governance`/`go-warm-manager`/`go-reset`.
- Cuerpos que no sean un objeto JSON (arrays, texto, multipart), cabeceras personalizadas, autenticación de pasos.
- Que el planner LLM razone sobre el defecto: solo debe poder **emitir** los campos nuevos (esquema y prompt); la fixture determinista de kind se ajusta en U8-T08.
- Bootstrap de kind, fixture del planner atada al `run_id`, logs del controlador (candidatas de U8-T08).

---

## Archivos de contexto

- `bitacoras/U8-T08.md` (rama `tarea/U8-T08`, worktree `../agentic-qa-swarm-wt-U8-T08`; sección «Ronda 2»)
- `contracts/plans/flow-plan.schema.json`, `contracts/plans/examples/` (valid e invalid)
- `agents/agent-planner/src/agent_planner/schemas/flow-plan.schema.json` (copia idéntica hoy), `prompt.py`, `validate.py`, `k6/render.py`, `k6/validate.py`, `agents/agent-planner/tests/`
- `services/go-run-controller/plan/plan.go`, `plan/validate.go`, `rehearsal/job.go`, `runner/executor.go`, `adapters/flowsource_u3*.go`
- `services/aqs-runner/cmd/aqs-runner/main.go` y `main_test.go`
- `demo/target-app/main.go` (solo lectura: el handler `POST /orders` y el defecto)

---

## Criterios de aceptación

- [ ] **CA-1** — El contrato acepta lo nuevo, rechaza lo malformado y las dos copias del esquema no divergen.
  ```bash
  diff contracts/plans/flow-plan.schema.json agents/agent-planner/src/agent_planner/schemas/flow-plan.schema.json && echo IGUALES
  cd agents/agent-planner && python -m pytest -q 2>&1 | tail -3
  cd ../../services/go-run-controller && go test ./plan/... ./adapters/... -count=1 2>&1 | tail -4
  ```
  Esperado: `IGUALES`; pytest y go test en verde, con ejemplos `valid` nuevos (paso con `body`, flujo con `invariant_probe`) y `invalid` nuevos (`body` que no es objeto, `invariant_probe` no booleano, campo extra en el paso).

- [ ] **CA-2** — El runner envía el cuerpo y no cambia el comportamiento sin él.
  ```bash
  cd services/aqs-runner && go test ./... -count=1 -v 2>&1 | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS` en pruebas que, contra un servidor de prueba, comprueban (a) con `body`, el servidor recibe el JSON y `Content-Type: application/json`; (b) sin `body`, el cuerpo llega vacío y sin esa cabecera.

- [ ] **CA-3** — Semántica del ensayo: estricto por defecto, «ejecutable» solo para flujos de sondeo.
  ```bash
  cd services/aqs-runner && go test ./... -count=1 -run 'Rehearse.*(Sondeo|Estricto)' -v 2>&1 | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS` en cuatro casos: flujo normal con estado distinto al esperado → sale `1`; flujo de sondeo con estado distinto pero `< 500` → sale `0`; flujo de sondeo con `5xx` → sale `1`; flujo de sondeo con error de red → sale `1`. En modo `http-steps` (runner) el estado distinto sigue saliendo `1`, con y sin `invariant_probe`.

- [ ] **CA-4** — La lectura de vuelta contra el sistema real: el defecto sembrado es alcanzable y detectable con el contrato nuevo, sin clúster.
  ```bash
  cd demo/target-app && go test ./... -count=1 2>&1 | tail -3
  cd ../../services/aqs-runner && go test ./... -count=1 -run 'Oversell' -v 2>&1 | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: una prueba de `aqs-runner` levanta el handler real de `demo/target-app` (o una réplica literal citada con archivo:línea si el paquete no es importable) con y sin `TARGET_APP_BUG=oversell`, y ejecuta el flujo de sondeo `POST /orders {"product_id":1,"qty":<mayor que el stock>}` con `expect_status: 409`: sin el defecto el runner sale `0`, con el defecto (responde `201`) sale `1` y el resultado cita la invariante. Es el par falla-pasa que hace posible el CA-2 de U8-T08.

- [ ] **CA-5** — El planner puede emitir los campos nuevos y los planes viejos siguen siendo válidos.
  ```bash
  cd agents/agent-planner && python -m pytest -q -k 'plan or schema or prompt or k6' 2>&1 | tail -3
  ```
  Esperado: verde, incluida una prueba de que un plan sin los campos nuevos (los ejemplos `valid` existentes) sigue validando y renderizando en k6 sin cambio, y una de que el prompt documenta `body` e `invariant_probe`.

- [ ] **CA-6** — Alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(contracts/plans/|agents/agent-planner/|services/go-run-controller/(plan|rehearsal|runner|adapters)/|services/aqs-runner/|demo/target-app/|bitacoras/U8-T08b\.md|revisiones/U8-T08b/)' | wc -l; git diff --name-only $b -- scripts/kind services/go-governance services/go-warm-manager services/go-reset | wc -l
  ```
  Esperado: `0`, `0` y `0` (más los dos archivos de docs de la tarea si el worktree parte de una rama de docs; el codificador lo aclara en la bitácora).

---

## Plan de pruebas

- **Rojo primero:** antes de tocar el contrato, el codificador (a) valida un plan con `body` contra el esquema actual y pega el rechazo por `additionalProperties`, y (b) escribe la prueba de CA-4 y la corre contra el runner actual: debe fallar porque no puede enviar `qty`. Luego implementa y repite.
- Pruebas negativas del esquema y de `validate.go` del controlador (que replica el esquema): `body` no objeto, `invariant_probe` no booleano, ambos en su sitio equivocado.
- La prueba existente que contrasta `validate.go` con el esquema JSON sigue en verde con los casos nuevos.
- Sensibilidad de CA-3: quitando la rama de sondeo del `rehearse`, el caso «sondeo con estado distinto» debe fallar.

---

## Notas

- Archivos que se **modifican en su sitio**; nada de duplicados con sufijo. Rama `tarea/U8-T08b`, worktree `../agentic-qa-swarm-wt-U8-T08b`, desde `main`.
- Los Jobs reciben los pasos por `REHEARSAL_STEPS`/`RUNNER_STEPS` (JSON). Cómo llega `invariant_probe` al proceso de ensayo (campo dentro del mismo JSON, o variable nueva) lo decide el codificador y lo documenta; no puede abrir egreso ni montar secretos.
- Esta tarea consume la capacidad de U8-T08: tras fusionarla, el orquestador rebasa `tarea/U8-T08` y despacha su ronda 3 (la última) con las candidatas de la ronda 2 ya registradas en `tareas/candidatas.md`.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
