# U11-T04 — Un `flow_id` que el modelo invente nunca debe romper el runner

**Unidad:** U11 — Demo con repositorio y modelo reales
**Historias que implementa:** US-M4 (flujos inspectables), US-M6 (corrida + evidencia), US-M9.
**Depende de:** U11-T01 fusionada. Independiente de U11-T02/T03 (otros archivos). **Tope: 3 rondas**.

---

## Origen (evidencia real de U11-T02, ronda 1)

`kind-e2e.sh --bug off` contra el repositorio real falló en la fase `runners`: el log de `go-run-controller` dice
`configuración del Job runner inválida: run_id/flow_id no aptos para un nombre de Job` (run `run-20d9efb7096b73d9`).
Causa en el código: `services/go-run-controller/runner/job.go` exige `idRe = ^[a-z0-9]([-a-z0-9]{0,40}[a-z0-9])?$` para el flujo y
`len("runner-"+run+"-"+flow) <= 63`. Con un `run_id` de 20 caracteres (`run-` + 16 hex) quedan **35 como máximo** para el `flow_id`,
pero el contrato (`contracts/plans/flow-plan.schema.json`, `flow_id: {type: string, minLength: 1}`) acepta cualquier cadena
y el planner no la normaliza. Un modelo real (DeepSeek) puede producir ids con mayúsculas, guiones bajos, espacios o más de 35 caracteres:
el ensayo pasa, y el runner revienta después, tras haber gastado el ciclo. Con el modelo falso nunca se vio.

## Alcance

**Dentro** (una línea, concreta):

> `flow_id` pasa a ser una etiqueta DNS de 1 a 35 caracteres (`^[a-z0-9]([-a-z0-9]{0,33}[a-z0-9])?$`) en el contrato y en todos los validadores, el planner normaliza de forma determinista cualquier id que el modelo produzca (sin colisiones dentro del plan), y el controlador rechaza en la admisión del plan —antes del ensayo— un id que no cumpla.

Detalle:

1. **Contrato:** `flow_id` con ese `pattern` en `contracts/plans/flow-plan.schema.json` y en la copia del planner (`agents/agent-planner/src/agent_planner/schemas/`, deben quedar idénticas); ejemplos `valid`/`invalid` nuevos (mayúsculas, `_`, espacio, 36 caracteres, vacío). Si el id aparece también en el esquema de evidencia/reporte, se alinea.
2. **Planner:** función pura `normalize_flow_id` (minúsculas; cualquier carácter fuera de `[a-z0-9]` → `-`; colapsar guiones; recortar guiones de los extremos; si queda vacío → `flujo`; si pasa de 35, cortar a 30 y añadir `-` + 4 hex de un hash estable del id original) y deduplicación dentro del plan (`-2`, `-3`… respetando los 35). Se aplica **antes** de validar el plan. Fixtures del LLM falso y prompt (`prompt.py`) piden ids cortos en minúsculas con guiones; el cambio del prompt obliga a regenerar `deploy/flux/kind/agents/*/fixtures.json` con `scripts/kind/gen_agent_fixtures.py` (C-122): se hace aquí.
3. **Controlador:** `services/go-run-controller/plan/validate.go` rechaza un `flow_id` fuera del patrón con un error que nombra el id y el motivo; `JobName`/`BuildJob` mantienen su comprobación. Prueba: un plan con `Oversell_Probe` o con 36+ caracteres se rechaza al admitirlo, **no** llega a crear un Job.
4. **Paridad:** el patrón vive en un solo valor por módulo con una prueba de paridad contra el del contrato (como `ValidateSurface`).

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Cambiar `run_id` (formato `run-` + 16 hex, U8-T08a) o el límite de 63 del nombre del Job.
- Prompt-engineering más allá de la regla de ids; precisión del plan; `invariant_probe`; el reporter.
- `scripts/kind/` (salvo regenerar fixtures con el script existente), manifiestos, `go-warm-manager`, `go-intake`, `ui-api`.
- Etiquetas de Kubernetes del Job más allá de las ya existentes.

---

## Archivos de contexto

- `services/go-run-controller/runner/job.go` (`idRe`, `JobName`, `BuildJob`, `maxNameLength`), `plan/validate.go`, `adapters/flowsource_u3.go`
- `contracts/plans/flow-plan.schema.json`, `contracts/plans/examples/`, `agents/agent-planner/src/agent_planner/` (`planner.py`, `validate.py`, `prompt.py`, `schemas/`, `fakes/`, `tests/`)
- `scripts/kind/gen_agent_fixtures.py`, `deploy/flux/kind/agents/planner/`, `deploy/flux/kind/agents/reporter/fixtures.json`
- `revisiones/` y `bitacoras/U11-T02.md` (sección CA-6, run `run-20d9efb7096b73d9`)

---

## Criterios de aceptación

- [ ] **CA-1** — El contrato exige el patrón, las copias no divergen y los ejemplos lo prueban.
  ```bash
  diff contracts/plans/flow-plan.schema.json agents/agent-planner/src/agent_planner/schemas/flow-plan.schema.json && echo IGUALES
  V="npx --yes -p ajv-cli@5.0.0 -p ajv-formats@3.0.1 ajv validate --spec=draft2020 -c ajv-formats -s contracts/plans/flow-plan.schema.json"
  for f in contracts/plans/examples/valid/flow-plan*.json; do $V -d "$f" >/dev/null 2>&1 && echo "OK $f" || echo "FALLA $f"; done
  for f in contracts/plans/examples/invalid/flow-plan*flow-id*.json; do $V -d "$f" >/dev/null 2>&1 && echo "FALLA(acepta) $f" || echo "OK(rechaza) $f"; done
  ```
  Esperado: `IGUALES`, todo `valid` con `OK`, y al menos 5 `invalid` de `flow_id` (mayúsculas, `_`, espacio, 36 caracteres, vacío) con `OK(rechaza)`; ninguna línea `FALLA`.

- [ ] **CA-2** — El planner normaliza de forma determinista y sin colisiones.
  ```bash
  cd agents/agent-planner && python -m pytest -q -k 'flow_id or normalize' 2>&1 | tail -4
  ```
  Esperado: verde, con pruebas de: `Oversell_Invariant Probe!` → `oversell-invariant-probe`; id de 60 caracteres → 35 como máximo, estable entre ejecuciones y distinto de otro id largo con el mismo comienzo; vacío o solo símbolos → `flujo`; dos flujos que normalizan igual → `flujo`, `flujo-2`; todo id normalizado cumple el patrón; y una propiedad (Hypothesis, semilla registrada) «para cualquier cadena, `normalize_flow_id` cumple el patrón y es idempotente».

- [ ] **CA-3** — El controlador rechaza el id inválido al admitir el plan, antes del ensayo, y el nombre del Job cabe con un `run_id` real.
  ```bash
  cd services/go-run-controller && GOTOOLCHAIN=auto go test ./... -count=1 -run 'FlowID|JobName|Plan' -v 2>&1 | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `ok`, con pruebas de: plan con `Oversell_Probe` y con 36 caracteres → error nombrando el id y ningún Job creado (cliente de Kubernetes falso sin acciones `create`); `JobName("run-20d9efb7096b73d9", <id de 35>)` ≤ 63 y `BuildJob` lo acepta; el mismo `run_id` con 36 caracteres lo rechaza; paridad del patrón con el del contrato.

- [ ] **CA-4** — Cadena sin clúster: lo que el modelo falso produzca con ids malos llega al runner ya normalizado.
  ```bash
  cd agents/agent-planner && python -m pytest -q -k 'cadena or chain or fixtures' 2>&1 | tail -3; cd ../../ && PYTHONPATH=agents/agent-reporter/src python3 scripts/kind/gen_agent_fixtures.py >/dev/null 2>&1; git diff --exit-code -- deploy/flux/kind/agents >/dev/null && echo FIXTURES_AL_DIA
  ```
  Esperado: verde y `FIXTURES_AL_DIA` (regenerar con el script existente no cambia nada respecto a lo comprometido; el script no tiene modo `--check` y necesita `PYTHONPATH=agents/agent-reporter/src`; solo regenera `reporter/fixtures.json`: la fixture del planner no lleva prompt con hash y no se regenera). Las fixtures se comprometen ya regeneradas.

- [ ] **CA-5** — Módulos y alcance.
  ```bash
  (cd services/go-run-controller && GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -count=1 2>&1 | grep -v 'no test files' | grep -c '^FAIL'); (cd agents/agent-planner && python -m pytest -q 2>&1 | tail -1); git status --short | wc -l; b=$(git merge-base HEAD origin/tarea/U11-T01); git diff --name-only $b | grep -v -E '^(contracts/plans/|agents/agent-planner/|services/go-run-controller/(runner|plan|adapters|runctl|rehearsal)/|services/go-run-controller/README\.md|deploy/flux/kind/agents/|tareas/U11-T0[2-5]-|unidades-y-tareas\.md|bitacoras/U11-T04\.md|revisiones/U11-T04/)' | wc -l
  ```
  Esperado: `0` (sin FAIL), `passed`, `0` y `0`.

---

## Plan de pruebas

- **Rojo primero:** una prueba que alimente el planner falso con un `flow_id` `Oversell_Probe` de 40 caracteres y muestre que hoy el plan valida y que `BuildJob("run-20d9efb7096b73d9", id)` devuelve `ErrConfig` («no aptos para un nombre de Job»). Pegar la salida.
- Negativas: vacío, solo símbolos, unicode, 36/200 caracteres, guiones en los extremos, colisiones.
- Par falla-pasa: quitar la normalización y comprobar que CA-2 falla; quitar la validación del controlador y comprobar que CA-3 falla.
- PBT: normalización siempre en patrón e idempotente (PBT-03), con semilla registrada.

---

## Notas

- Archivos que se **modifican en su sitio**; nada de duplicados con sufijo. Rama `tarea/U11-T04`, worktree `../agentic-qa-swarm-wt-U11-T04`, desde `tarea/U11-T01`.
- Orden de despliegue: controlador (rechaza ids malos) y planner (normaliza) deben salir juntos; un planner viejo con un controlador nuevo falla al admitir el plan, con un mensaje claro.
- Candidatas que surjan van a `tareas/candidatas.md`.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
