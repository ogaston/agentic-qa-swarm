# Ronda 1 — U11-T04 (informe del revisor, persistido por el orquestador; resumen fiel del hand-back)

VEREDICTO: NO-VERDE (0 ROJO, 2 NARANJA, 4 AMARILLO)

HEAD d806cfc, hash de la tarea 5a0b833…, worktree limpio. Scratchpad único. CA-1..CA-5 pasan con ejecución propia (CA-1: 5 valid OK, 6 invalid OK(rechaza) por `/flows/0/flow_id`; CA-2: 30 passed; CA-3: sin FAIL; CA-4: FIXTURES_AL_DIA y regenerado idéntico; CA-5: 0/passed(237)/0/0). agents/eval 102 passed; agent-reporter 325 passed.
El fallo `test_uri_parity_with_jsonschema_format_checker` que reporta el codificador NO se reproduce: con un venv desde `requirements-dev.txt` pasa en `main` y en la rama (es de su entorno).

## Adversarial (lo que resistió)
- Fuzz diferencial de 1411 ids (+1500 aleatorios con semilla) contra cinco decisores (plan.ValidateFlowPlan, runner.BuildJob con `run-20d9efb7096b73d9`, flowIDRe del runner, ajv 2020 sobre el esquema, validate_plan_dict del planner): aceptan exactamente los mismos 203 ids; máx len(JobName) aceptado = 63; BuildJob acepta 35 y rechaza 36.
- `normalize_flow_id` (Hypothesis 20000+5000 ejemplos): siempre en patrón y ≤35, idempotente, determinista, único dentro del plan; la deduplicación no puede sacar el id del patrón. Se normaliza antes de validar en `planner.plan()`.
- Usos de flow_id: planner normaliza y valida; el controlador valida en cuatro puntos de admisión (planstore.go:43 y :73, adapters/flowsource_u3.go:148, rehearsal/launcher.go:100, runner/launcher.go:165); BuildJob usa flowIDRe; claves de evidencia y reporter reciben el id ya normalizado.
- Job de ensayo (`rehearsal/job.go`): el nombre es `rehearsal-<run>-<n>` y no lleva el flow_id; `rehearsal.Launch` valida antes de tocar Kubernetes (probado: `Oversell_Probe` y 36 caracteres → error con el id y 0 Jobs). NO puede reventar como el runner: solo candidata. openapi (control-plane.yaml:392) y rehearsal.schema.json sin patrón: describen salidas, solo candidatas.
- Mutaciones: sin normalización → falla CA-2 y la cadena; sin dedupe → 3 pruebas; sin validación del controlador → 2; patrón cambiado en una sola copia (plan, runner, esquema) → falla la paridad Go; en la constante Python NO hay prueba de paridad (F-02).
- Fixtures/prompt: el prompt pide ids en minúsculas/dígitos/guiones, ≤35, único; fixtures y scripts de kind (`flow-1`, `f-happy`, `pedido-stock`…) cumplen el patrón.

## Hallazgos
### F-01 · NARANJA · agents/agent-planner/src/agent_planner/flow_id.py:35 (normalize_flow_id)
`hashlib.sha256(raw.encode("utf-8"))` lanza UnicodeEncodeError con un sustituto solitario (`json.loads` los produce con un escape `\ud800`; el modelo o una inyección desde la superficie puede emitirlo) cuando el id supera 35 caracteres. `st.text()` de Hypothesis excluye esa categoría, así que la propiedad de CA-2 no lo ve.
Reproducción: `normalize_flow_id("a"*40+"\ud800")` → UnicodeEncodeError; `plan(...)` igual. En la base 07c8bd8 el mismo plan da `PlanRejected flow_id_invalido:0` (422). `server.py` solo captura InvalidRequest, NoSurface, PlanRejected, BudgetExceeded y LLMError: la excepción se escapa, no hay 422 y el controlador ve «U3 /v1/plan inalcanzable». Es una regresión del cierre en fallo.
Pedido: hashear con `errors="surrogatepass"`/`"replace"`/`"backslashreplace"` (o rechazar con PlanRejected) y una prueba con alfabeto que incluya sustitutos o un caso explícito.

### F-02 · NARANJA · alcance punto 4 (Paridad) · agents/agent-planner/src/agent_planner/flow_id.py:13
Falta la prueba de paridad en Python: `FLOW_ID_PATTERN` no se compara con el `pattern` del esquema en ninguna prueba (Go sí la tiene). Con la constante en `{0,34}` o `{0,35}` solo avisa `test_pbt_generator_coverage` de rebote.
Pedido: leer `properties.flows.items.properties.flow_id.pattern` del esquema empaquetado y exigir igualdad con `FLOW_ID_PATTERN`.

### F-03 · AMARILLO · services/go-run-controller/runner/job.go:38
Segundo literal del patrón en el módulo cuando `plan.FlowIDPattern` ya es importable (runner ya importa plan): `regexp.MustCompile(plan.FlowIDPattern)`.

### F-04 · AMARILLO · contracts/plans/examples/invalid/flow-plan.flow-id-*.json
Los 6 ejemplos inválidos solo los ejercita el comando manual de CA-1; ninguna prueba de Python ni de Go los recorre por glob; ninguno pasa por ValidateFlowPlan de Go.

### F-05 · AMARILLO · agents/agent-planner/tests/test_flow_id.py
`test_cadena_flow_id_malo_llega_normalizado` cubre un solo id (`Oversell_Probe`); no pasa por `plan()` un id de más de 35 caracteres ni dos que colisionan, ni hay comprobación cruzada con el ValidateFlowPlan de Go. El «rojo primero» del plan de pruebas no se cumple literalmente (el planner antiguo ya rechazaba `Oversell_Probe`); el codificador lo anotó y es correcto.

### F-06 · AMARILLO · detalles
Los planes ya guardados en `r.Plan` con ids de más de 35 caracteres fallarán en `DecodeStoredPlan` («plan guardado inválido»); coherente con el orden de despliegue pero conviene que figure en las notas. El `pattern` de jsonschema en Python (`re.search`) acepta `'a\n'`; solo el `fullmatch` y la normalización lo tapan.

## Candidatas
- Alinear `flow_id` en contracts/openapi/control-plane.yaml:392 y contracts/events/rehearsal.schema.json con el patrón.
- Unificar el `ID_RE` de 42 caracteres de agent_planner/k6/store.py con FLOW_ID_PATTERN.
- Que el servidor del planner cierre en fallo (500/422) ante excepciones inesperadas de `plan()`.
- Registrar estas candidatas en `tareas/candidatas.md` (el codificador solo las dejó en la bitácora).

VEREDICTO: NO-VERDE
