# U8-T06 — Fase `report` real y fuente de flujos U3 en el controlador

**Unidad:** U8 — Producto demostrable (ciclo completo en kind)
**Historias que implementa:** US-M9 (post-mortem al final de cada corrida), US-M4 (plan desde el planner real).
**Depende de:** U7 cerrada. **Ola 2** (no depende de T05: se prueba con stubs HTTP). **Tope: 3 rondas**.

---

## Alcance

**Dentro** (una línea, concreta):

> En `go-run-controller`, implementar la fase `report` en `adapters.RealPhases` con un cliente HTTP del reporter (`REPORTER_URL`): `POST {REPORTER_URL}/v1/report {run_id, evidence_uris}` con los URIs de evidencia que la fase `run` ya guardó, validar la respuesta contra `contracts/plans/report.schema.json`, guardar en la corrida el veredicto y el URI del reporte y exponerlos en `GET /runs/{id}`; y cablear en `deploy/flux/base/control-plane.yaml` `RUN_FLOW_SOURCE=u3`, `U3_URL=http://agent-planner.aqs-system.svc:8080`, `U3_DEFAULT_WORKFLOW=happy-path` y `REPORTER_URL=http://agent-reporter.aqs-system.svc:8080`.

Detalle:

- Mismo patrón que `adapters.FlowSourceU3`: solo `http(s)`, sin redirecciones, plazo (60 s), circuito, y todo error, `4xx/5xx`, cuerpo inválido o reporte de otra corrida es **fallo de fase** (fail-closed; nunca se marca la corrida como reportada sin reporte válido).
- La fase `report` va **después** de `reset` (como hoy en la máquina de estados); si la máquina actual lo ordena distinto, se respeta el orden existente y se documenta.
- `GET /runs/{id}` añade `verdict` y `report_uri` (cambio compatible del contrato OpenAPI de la corrida; si el esquema es estricto, se amplía `contracts/openapi/control-plane.yaml` y se regeneran los tipos del dashboard con el comando existente).
- Prueba de contrato `-tags contract -run ContractReporter` contra un stub HTTP local y, con `REPORTER_URL`, contra el reporter real (como `ContractU3`).

**Fuera**:

- Cambios en los agentes (U8-T05) o en NATS (U8-T02).
- Publicar `report.ready` por NATS (candidata).
- Vista del reporte en el dashboard (U8-T08 la comprueba por API).

---

## Archivos de contexto

- `services/go-run-controller/adapters/phases.go`, `adapters/` (cliente U3), `runctl/` (máquina de estados, `PhaseReport`), `cmd/go-run-controller/{main,real}.go`, `README.md`
- `agents/agent-reporter/src/agent_reporter/server.py` (`POST /v1/report`), `contracts/plans/report.schema.json`
- `contracts/openapi/control-plane.yaml`, `web/dashboard/` (generación de tipos, solo si cambia el contrato)
- `scripts/test/u3-demo-local.sh` (arranque del reporter real en loopback)

---

## Criterios de aceptación

- [ ] **CA-1** — Pruebas unitarias y fail-closed.
  ```bash
  (cd services/go-run-controller && go test ./... ; echo "rc=$?")
  ```
  Esperado: `rc=0`, con pruebas de: reporte válido → corrida con `verdict`; reporter caído, `500`, reporte inválido o de otra corrida → fallo de fase sin `verdict`.

- [ ] **CA-2** — Contrato contra el reporter real en loopback.
  ```bash
  bash scripts/test/u8-report-contract.sh; echo "rc=$?"
  ```
  Esperado: arranca el reporter real (fake LLM con fixture, evidencia en disco) como `u3-demo-local.sh`, corre `go test -tags contract -run ContractReporter ./adapters` con `REPORTER_URL`: `OK contrato-reporter-valido`, `OK contrato-reporter-caido`, `rc=0`, y deja los puertos libres.

- [ ] **CA-3** — Manifiestos cableados y contrato sin deriva.
  ```bash
  kubectl kustomize deploy/flux/kind | grep -A1 -E 'name: (RUN_FLOW_SOURCE|U3_URL|REPORTER_URL)$' | grep -c -E 'value: (u3|http://agent-(planner|reporter)\.aqs-system\.svc:8080)$'
  bash scripts/ci/policies.sh 2>&1 | grep -c '^FALLA'; bash contracts/validate.sh >/dev/null 2>&1; echo "contracts rc=$?"
  ```
  Esperado: `3`, `0` y `contracts rc=0`.

- [ ] **CA-4** — Alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/go-run-controller/|deploy/flux/(base|kind)/|contracts/openapi/control-plane\.yaml|web/dashboard/src/api/|scripts/test/u8-report-contract\.sh|bitacoras/U8-T06\.md|revisiones/U8-T06/)' | wc -l
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

- **Rojo primero:** hoy `RealPhases.Launch(report)` devuelve `ErrNotImplemented`; pegar la prueba que lo muestra.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
