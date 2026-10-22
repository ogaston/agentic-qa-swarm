# U9-T02 — El plan de flujos de una corrida es inspectable (`GET /runs/{id}/plan`)

**Unidad:** U9 — Resultados visibles en el dashboard
**Historias que implementa:** US-M4 (flujos inspectables), US-M5 (lo que se ensayó es lo que se ejecutó).
**Depende de:** U8 cerrada. **Ola 1**, en paralelo con U9-T01 y U9-T05. **Bloquea a U9-T03.** **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea, concreta):

> Que `go-run-controller` guarde el `FlowPlan` que obtuvo del planner para una corrida y lo exponga en `GET /runs/{id}/plan`, sin volver a llamar al planner ni al LLM.

Detalle:

- Hoy `rehearsal/launcher.go` y `runner/launcher.go` llaman a `Plans.Flows(run.ID)` y el plan no queda guardado en ningún sitio del controlador. **Persistencia:** la primera vez que el controlador obtiene un plan válido para una corrida lo guarda junto al estado de la corrida (mismo store, misma atomicidad; documentar el formato). Ensayo y runners leen el plan **guardado** si existe, de modo que lo ensayado y lo ejecutado son el mismo plan byte a byte. Si el plan guardado no valida contra `contracts/plans/flow-plan.schema.json` al leerse, falla cerrado como hoy.
- **`GET /runs/{id}/plan`**: `200` con el `FlowPlan` guardado; `404 run_not_found`; `404 plan_not_ready` si la corrida aún no tiene plan. Nunca llama a `Plans.Flows`. Se añade a la OpenAPI (`getRunPlan`, esquema `FlowPlan` por `$ref` a `flow-plan.schema.json` o copia fiel) y se regeneran los tipos del dashboard (`npm run gen:api`).
- Mismo nivel de autenticación que `GET /runs/{id}` hoy (sin cambios). El proxy del dashboard ya envía `/api/runs/**` al controlador; no hace falta tocar `web/dashboard/src/proxy`.
- **`kind-e2e.sh`**: comprobación nueva `plan-inspectable`: tras `run-terminado`, `GET /runs/<run>/plan` devuelve `200`, valida contra `flow-plan.schema.json` y la cantidad de flujos coincide con la cantidad de Jobs `runner-<run>-*` creados.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Mostrar el plan en el dashboard (U9-T03), salvo los tipos generados.
- Exportar el plan como archivo k6 descargable.
- Autenticación del controlador; cambios en `agent-planner`.

---

## Archivos de contexto

- `services/go-run-controller/adapters/flowsource_u3.go`, `rehearsal/launcher.go`, `runner/launcher.go`, `plan/plan.go`, `plan/validate.go`
- `services/go-run-controller/internal/httpapi/api.go` (`GET /runs/{id}`), `runctl/ports.go` (`Run`, store)
- `contracts/plans/flow-plan.schema.json`, `contracts/openapi/control-plane.yaml`
- `scripts/kind/kind-e2e.sh` (`run-terminado`, `runners`)

---

## Criterios de aceptación

- [ ] **CA-1** — Contratos y tipos.
  ```bash
  bash contracts/validate.sh >/dev/null 2>&1; echo "rc=$?"; (cd web/dashboard && npm run -s check:api >/dev/null 2>&1; echo "rc=$?")
  ```
  Esperado: `rc=0` y `rc=0`.

- [ ] **CA-2** — Controlador.
  ```bash
  (cd services/go-run-controller && go vet ./... && go test ./... 2>&1 | grep -c FAIL; go test -run 'Plan(Stored|Endpoint)' -v ./... 2>&1 | grep -c '^--- PASS')
  ```
  Esperado: `0` fallos y al menos 6 pruebas: el plan se guarda en la primera obtención; ensayo y runners de la misma corrida llaman a `Plans.Flows` **una sola vez** en total (doble que cuenta); tras reiniciar el controlador (store reabierto) el plan sigue y no se vuelve a pedir; `GET /runs/{id}/plan` → `200` con el mismo JSON; corrida inexistente → `404 run_not_found`; sin plan aún → `404 plan_not_ready`; plan guardado corrupto → falla cerrado.

- [ ] **CA-3** — En kind (dentro de `flock ${XDG_RUNTIME_DIR:-/tmp}/aqs-kind.lock`, con las imágenes de HEAD reconstruidas y cargadas).
  ```bash
  bash scripts/kind/kind-e2e.sh --bug on 2>&1 | grep -E '^(OK|FALLA) plan-inspectable'; bash scripts/kind/kind-e2e.sh --bug on 2>&1 | grep -c '^FALLA'
  ```
  Esperado: `OK plan-inspectable` y `0`.

- [ ] **CA-4** — Alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(contracts/|services/go-run-controller/|scripts/kind/kind-e2e\.sh|web/dashboard/src/api/schema\.gen\.ts|tareas/U9-T02-plan-de-flujos-inspectable\.md|bitacoras/U9-T02\.md|revisiones/U9-T02/)' | wc -l
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

- Doble de `FlowSource` que cuenta llamadas; store real en un `t.TempDir()` para la prueba de reinicio.
- **Rojo primero:** pegar la salida de `go test -run 'Plan(Stored|Endpoint)' ./...` antes de implementar.
- U9-T01 también toca `contracts/openapi/control-plane.yaml`, `schema.gen.ts` y `kind-e2e.sh`: la que se fusione segunda rebasa y regenera `schema.gen.ts`.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
