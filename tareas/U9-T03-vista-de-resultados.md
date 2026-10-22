# U9-T03 — Vista de resultados en el dashboard (reporte, plan y ruta raíz)

**Unidad:** U9 — Resultados visibles en el dashboard
**Historias que implementa:** US-M9 (post-mortem visible), US-M4 (flujos inspectables), S5 (dashboard).
**Depende de:** U9-T01 y U9-T02 **fusionadas**. **Ola 2**. **Bloquea a U9-T04.** **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea, concreta):

> En `/runs/:id`, mostrar el veredicto y el post-mortem (`GET /api/reports/{run}`) y el plan de flujos (`GET /api/runs/{id}/plan`) cuando la corrida termina, y redirigir `/` a `/inbox`.

Detalle:

- **Ruta raíz.** `/` redirige a `/inbox` (que ya manda a `/login?next=%2Finbox` sin sesión). El resto de rutas desconocidas sigue en «No encontrado».
- **Veredicto.** Cuando `GET /api/runs/{id}` trae `verdict`, se muestra con texto e insignia distintos para `bug`, `sin-hallazgos` e `inconcluso` (texto, no solo color).
- **Post-mortem.** Al llegar a `done`, la vista pide `GET /api/reports/{run}` **una vez** (no entra en el sondeo). Muestra `summary` y, por cada hallazgo: `method` + `path`, `invariant`, `root_cause` y la lista de `evidence_uris` como texto copiable (no son enlaces navegables: son `s3://`). Sin hallazgos → mensaje explícito. Errores: `404 report_not_ready` → «El informe aún no está disponible» con botón «Reintentar»; `403`/`404` → «Sin acceso al informe»; `5xx` → aviso y «Reintentar». Todo el texto del reporte se renderiza como texto (sin `dangerouslySetInnerHTML`); la CSP estricta del build sigue igual.
- **Plan.** Sección plegable «Flujos ejecutados» con `GET /api/runs/{id}/plan` (una vez, al llegar a un estado desde `rehearsing` en adelante): por flujo, su nombre/familia y sus pasos (`method`, `path`, y el `body` si lo hay, en `<pre>` con JSON formateado). `404 plan_not_ready` → «Plan aún no disponible».
- **Corrida `failed`.** Si hay plan, se muestra; el post-mortem solo si hay `report_uri`.
- **`dashboard-smoke.sh`**: el paso hoy `PENDIENTE` de confirmar y seguir la corrida pasa a comprobarse de verdad si la plataforma lo permite: confirmar una notificación de prueba, seguir la corrida hasta `done` y comprobar `OK reporte-visible` (el `GET /reports` por el origen del dashboard devuelve `200` con `verdict`). Si depende del LLM real y no está configurado, sigue `PENDIENTE` con su motivo; documentarlo.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Estilos y maquetación (U9-T04): aquí basta HTML semántico con las clases que ya existan.
- Marca de falso positivo, descarga de evidencia, URLs prefirmadas.
- Cambios fuera de `web/dashboard/**` y `scripts/kind/dashboard-smoke.sh`.

---

## Archivos de contexto

- `web/dashboard/src/App.tsx`, `src/pages/Run/RunPage.tsx` y sus pruebas, `src/hooks/` (`usePolling`), `src/api/`
- `tareas/U9-T01-reporte-por-ui-api.md`, `tareas/U9-T02-plan-de-flujos-inspectable.md` y sus bitácoras (códigos de error exactos)
- `contracts/plans/report.schema.json`, `contracts/plans/flow-plan.schema.json`
- `scripts/kind/dashboard-smoke.sh`, `tareas/U7-T05-dashboard-contra-kind.md`

---

## Criterios de aceptación

Desde `web/dashboard/`, con `npm ci` hecho.

- [ ] **CA-1** — Lo anterior sigue en verde.
  ```bash
  npm run -s lint && npm run -s typecheck && npm run -s check:api && npx vitest run 2>&1 | grep -c -E 'failed|FAIL'
  ```
  Esperado: `0`.

- [ ] **CA-2** — Ruta raíz.
  ```bash
  npx vitest run src/App 2>&1 | tail -n 3
  ```
  Esperado: sin `failed`; `/` termina en `/inbox` con sesión y en `/login?next=%2Finbox` sin ella; `/xyz` sigue en «No encontrado».

- [ ] **CA-3** — Reporte y plan.
  ```bash
  npx vitest run src/pages/Run 2>&1 | tail -n 3
  ```
  Esperado: sin `failed` y al menos 12 pruebas nuevas (MSW + temporizadores falsos): los 3 veredictos con texto distinto; `done` → exactamente 1 `GET /reports` y ninguno más en 30 s; antes de `done` → 0 `GET /reports`; hallazgo con `method`, `path`, `invariant`, `root_cause` y sus `evidence_uris`; `findings: []` → mensaje sin hallazgos; `404 report_not_ready` → mensaje y «Reintentar» (que hace 1 petición más); `403` → «Sin acceso al informe»; `502` → aviso; un `summary` con `<img src=x onerror=...>` aparece como texto y no crea ningún `img`; plan con `body` en `<pre>`; `404 plan_not_ready`; corrida `failed` sin `report_uri` → 0 `GET /reports`.

- [ ] **CA-4** — CSP intacta.
  ```bash
  npm run -s build >/dev/null && grep -c -E 'unsafe-inline|unsafe-eval' dist/index.html
  ```
  Esperado: `0`.

- [ ] **CA-5** — Contra kind (dentro de `flock ${XDG_RUNTIME_DIR:-/tmp}/aqs-kind.lock`).
  ```bash
  cd ../.. && bash scripts/kind/dashboard-smoke.sh 2>&1 | grep -E '^(OK|FALLA|PENDIENTE) '; echo "rc=${PIPESTATUS[0]}"
  ```
  Esperado: ningún `FALLA`, `rc=0`, y `OK reporte-visible` o `PENDIENTE reporte-visible (<motivo documentado>)`.

- [ ] **CA-6** — Alcance.
  ```bash
  cd ../.. && git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(web/dashboard/|scripts/kind/dashboard-smoke\.sh|tareas/U9-T03-vista-de-resultados\.md|bitacoras/U9-T03\.md|revisiones/U9-T03/)' | wc -l
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

- Fixtures de reporte generadas desde `contracts/plans/examples/` (o válidas contra `report.schema.json`), no inventadas a mano.
- **Rojo primero:** pegar la salida de `npx vitest run src/App src/pages/Run` antes de implementar.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
