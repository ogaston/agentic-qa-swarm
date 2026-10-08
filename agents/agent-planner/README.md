# agent-planner

Convierte un `SurfaceArtifact` y un workflow en un `FlowPlan` valido contra
`contracts/plans/flow-plan.schema.json`, o falla cerrado. Una sola llamada al `LLMClient`, sin reintentos.

> **Aviso: el agente trata la superficie como dato; la defensa es la validacion posterior, no el prompt.**
> El prompt delimita la superficie en `<datos-superficie>` (JSON canonico, con `<` escapado), pero el modelo
> puede ser enganado. Por eso la salida se valida: esquema, cada `(method, path)` debe pertenecer exactamente a la
> superficie, `run_id`/`workflow` coinciden, `flow_id` unico con la forma `^[a-z0-9]([a-z0-9-]{0,40}[a-z0-9])?$`,
> tope de flujos y pasos, sin claves extra. Un plan invalido se rechaza (`PlanRejected`); nunca se repara ni se recorta.

## API

- `POST /v1/plan` con `{"surface": SurfaceArtifact, "workflow": "..."}` (`application/json`, maximo 256 KiB)
  - `200` FlowPlan
  - `422 {error: no_surface | plan_rejected | invalid_request, detail}`
  - `504 {error: budget_exceeded}`; `502 {error: llm_unavailable}`; `413` tamano; `415` tipo
- `GET /healthz`, `GET /readyz`, `GET /metrics` (`aqs_planner_requests_total{result}`, `aqs_planner_tokens_total{direction}`)

`detail` es un codigo fijo; nunca contiene contenido de la superficie ni del modelo. Los logs son JSON con `run_id`
(solo si tiene forma segura) y motivo; nunca el prompt.

## Limites y configuracion (por entorno)

| Variable | Defecto | Efecto |
|---|---|---|
| `PLANNER_TIMEOUT_S` | `25` | tope de tiempo al modelo (menor que los 30 s del cliente de U2) |
| `PLANNER_MAX_INPUT_TOKENS` | `8000` | si `ceil(len(prompt)/4)` lo supera: `BudgetExceeded("input")` sin llamar al modelo |
| `PLANNER_MAX_OUTPUT_TOKENS` | `4000` | se pasa como `max_tokens`; si la salida declara mas: `BudgetExceeded("output")` |
| `PLANNER_MAX_FLOWS` / `PLANNER_MAX_STEPS` | `10` / `20` | topes de forma del plan |
| `PLANNER_PORT` / `PLANNER_HOST` | `8080` / `0.0.0.0` | escucha |
| `LLM_PROVIDER` | (obligatorio) | solo `fake`; otro valor: «proveedor no implementado» (el real es U3-T07) |
| `PLANNER_ALLOW_FAKE` | | debe ser `true` para `fake`; con `PLANNER_ENV=prod` el arranque se rechaza |

Valores no positivos o no numericos impiden el arranque. Un timeout del modelo es `BudgetExceeded("time")` (504).

## Notas

- `src/agent_planner/schemas/` es copia byte a byte de `contracts/plans/` (la imagen se construye solo con este
  directorio); una prueba vigila la deriva.
- Con `LLM_PROVIDER=fake` el `FakeLLM` arranca sin respuestas registradas: todo prompt falla cerrado (502).
- Pruebas: `python -m venv .venv && .venv/bin/pip install -r requirements-dev.txt -e . && .venv/bin/python -m pytest -q`.
