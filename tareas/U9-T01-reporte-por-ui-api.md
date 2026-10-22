# U9-T01 — El reporte post-mortem se lee por `ui-api` (`GET /reports/{run}`)

**Unidad:** U9 — Resultados visibles en el dashboard
**Historias que implementa:** US-M9 (post-mortem legible por quien confirmó la corrida), US-M8.3 (control de propiedad por resource ID). Cierra la parte de lectura de **C-93**.
**Depende de:** U8 cerrada. **Ola 1**, en paralelo con U9-T02 y U9-T05. **Bloquea a U9-T03.** **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea, concreta):

> Implementar `GET /reports/{run}` en `ui-api`: control de propiedad, `report_uri` desde `go-run-controller` y lectura del reporte a través de un `GET` nuevo de `agent-reporter`, con el esquema de la OpenAPI alineado al reporte real.

Detalle:

- **Contrato.** El esquema `Report` de `contracts/openapi/control-plane.yaml` hoy no coincide con el reporte real (`contracts/plans/report.schema.json`: `run_id`, `verdict`, `summary`, `findings[]` con `finding_id`, `root_cause`, `invariant`, `method`, `path`, `evidence_uris`, y `report_uri`). Se alinea la OpenAPI con `report.schema.json` (por `$ref` o copia fiel; documentar cuál) y se regeneran los tipos del dashboard (`npm run gen:api`). Respuestas: `200`, `401`, `403` (corrida de otro usuario), `404` (corrida sin recibo o sin `report_uri` todavía), `502`/`503` (reporter o controlador caídos, con código de error estable).
- **`agent-reporter`: `GET /v1/reports?uri=<s3-uri>`.** Devuelve el JSON guardado con `S3ReportStore.get` tal cual (ya pasó por `redactSecrets` al escribirse). Solo acepta URIs `s3://<bucket configurado>/runs/<run_id>/report-<sha256>.json`; cualquier otra forma → `400 invalid_uri` sin tocar MinIO. Sin LLM. Solo lo puede llamar `ui-api` (NetworkPolicy), y exige el mismo token de servicio que ya presenta el controlador al `POST /v1/report`, si existe; si hoy no hay token, se documenta y se deja como candidata (no se inventa autenticación nueva).
- **`ui-api`.**
  1. Autentica como el resto de rutas.
  2. **Propiedad**: el `run` debe estar en los recibos de confirmación (`GET /confirmations`) del usuario autenticado; si no lo está → `403` (si existe para otro usuario) o `404` (si no existe). Mismo criterio y mismos códigos que el resto de `ui-api`; documentar cuál se aplica.
  3. Pide `GET /runs/{id}` a `go-run-controller` (`RUN_CONTROLLER_URL`, opcional como `WARM_URL`: sin ella la ruta responde `503 reports_unavailable`). Sin `report_uri` → `404 report_not_ready`.
  4. Pide el reporte al reporter (`REPORTER_URL`, mismo patrón), valida que su `run_id` coincide con el de la ruta (si no → `502`) y lo devuelve.
  5. Timeouts y cortacircuitos como los de `/warm` (U2-T7).
- **Despliegue.** Variables nuevas de `ui-api` en `deploy/flux/base/control-plane.yaml` (y `kind` si hace falta); NetworkPolicy de egreso `ui-api → go-run-controller` y `ui-api → agent-reporter`, e ingreso del reporter desde `ui-api` en `deploy/flux/base/security/networkpolicies-system.yaml` (o donde vivan hoy las del reporter en `agents.yaml`). `conftest`/`scripts/ci/policies.sh` siguen en verde.
- **`kind-e2e.sh`**: comprobación nueva `reporte-en-ui-api` tras `reporte-en-minio`: con el token del usuario demo, `GET /reports/<run>` devuelve `200` con el mismo `verdict` y la misma cantidad de hallazgos que el reporte bajado de MinIO; con un token de otro usuario devuelve `403` o `404`.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Cualquier cambio en `web/dashboard/src/**` salvo los tipos generados (`schema.gen.ts`). La vista del reporte es U9-T03.
- URLs prefirmadas de MinIO o descarga de evidencia desde el navegador.
- Marca de falso positivo y `add-regression` (resto de C-93).
- Autenticación de `GET /runs/{id}` en `go-run-controller` (si la falta de auth es un hallazgo, va a `candidatas.md`).

---

## Archivos de contexto

- `contracts/openapi/control-plane.yaml` (`getReport`, `Report`, `Run`), `contracts/plans/report.schema.json`
- `services/ui-api/internal/httpapi/warm.go` y `warm_test.go` (patrón de llamada a otro servicio con token, timeout y 503), `services/ui-api/cmd/ui-api/main.go` (variables)
- `agents/agent-reporter/src/agent_reporter/server.py`, `s3.py` (`S3ReportStore.get`)
- `services/go-run-controller/runctl/ports.go` (`Run.ReportURI`, `Run.Verdict`)
- `deploy/flux/base/control-plane.yaml`, `deploy/flux/base/agents.yaml`, `deploy/flux/base/security/networkpolicies-system.yaml`
- `scripts/kind/kind-e2e.sh` (`reporte-en-minio`), `tareas/U6-T02-ui-api-lectura-warm-confirmaciones.md`

---

## Criterios de aceptación

- [ ] **CA-1** — Contratos y tipos.
  ```bash
  bash contracts/validate.sh >/dev/null 2>&1; echo "rc=$?"; (cd web/dashboard && npm run -s check:api >/dev/null 2>&1; echo "rc=$?")
  ```
  Esperado: `rc=0` y `rc=0`.

- [ ] **CA-2** — `ui-api`.
  ```bash
  (cd services/ui-api && go vet ./... && go test ./... 2>&1 | grep -c FAIL; go test -run Report -v ./... 2>&1 | grep -c '^--- PASS')
  ```
  Esperado: `0` fallos y al menos 9 pruebas `Report` que pasan: `200` con el reporte; sin token `401`; corrida de otro usuario `403`/`404` sin llamar al controlador ni al reporter; sin `report_uri` `404 report_not_ready`; sin `RUN_CONTROLLER_URL` o sin `REPORTER_URL` `503`; controlador caído `503`; reporter caído `502`; `run_id` del reporte distinto al de la ruta `502`; `run` mal formado `400` sin peticiones salientes.

- [ ] **CA-3** — `agent-reporter`.
  ```bash
  (cd agents/agent-reporter && python -m pytest -q 2>&1 | tail -n 1; python -m pytest -q -k get_report 2>&1 | tail -n 1)
  ```
  Esperado: sin `failed`; al menos 4 pruebas `get_report`: URI válida devuelve el JSON guardado; bucket distinto, clave fuera de `runs/<run>/report-<sha>.json` o esquema `http(s)://` → `400 invalid_uri` sin llamar al store; store caído → `502 store_unavailable`.

- [ ] **CA-4** — Políticas.
  ```bash
  bash scripts/ci/policies.sh 2>&1 | tail -n 3; echo "rc=${PIPESTATUS[0]}"
  ```
  Esperado: `rc=0`.

- [ ] **CA-5** — En kind (dentro de `flock ${XDG_RUNTIME_DIR:-/tmp}/aqs-kind.lock`, con las imágenes de HEAD reconstruidas y cargadas).
  ```bash
  bash scripts/kind/kind-e2e.sh --bug on 2>&1 | grep -E '^(OK|FALLA) reporte-en-ui-api'; bash scripts/kind/kind-e2e.sh --bug on 2>&1 | grep -c '^FALLA'
  ```
  Esperado: `OK reporte-en-ui-api` y `0`.

- [ ] **CA-6** — Alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(contracts/|services/ui-api/|agents/agent-reporter/|deploy/flux/(base|kind)/|scripts/kind/|web/dashboard/src/api/schema\.gen\.ts|tareas/U9-T01-reporte-por-ui-api\.md|bitacoras/U9-T01\.md|revisiones/U9-T01/)' | wc -l
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

- `ui-api`: dobles `httptest` del controlador y del reporter que cuentan peticiones (la prueba de propiedad exige 0 peticiones salientes).
- Reporter: `S3ReportStore` falso; la validación de la URI se prueba sin cliente S3.
- **Rojo primero:** pegar la salida de `go test -run Report ./...` y `pytest -k get_report` antes de implementar.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
