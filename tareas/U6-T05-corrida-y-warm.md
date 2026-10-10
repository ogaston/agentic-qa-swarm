# U6-T05 — Vista de corrida, mis corridas y estado del entorno warm

**Unidad:** U6 — Frontend web (S5)
**Historias que implementa:** S5 (estado del warm `ready`/`dirty`/`cuarentena`/`idle-escalado`), US-M2/US-M5/US-M6/US-M7.1 (seguimiento del ciclo de la corrida).
**Depende de:** U6-T02 y U6-T03 **fusionadas**. **Ola 3**, en paralelo con U6-T04. **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea, concreta):

> Implementar `/runs/:id` (estado de la corrida con `GET /api/runs/{id}` y sondeo hasta un estado terminal), la lista «Mis corridas» con `GET /api/confirmations`, y `/warm` (estado del warm con `GET /api/warm`).

Detalle:

- **`/runs/:id`**: línea de tiempo con las fases del contrato en orden `confirmed → warm_ready → deploying → inferring → rehearsing → running → resetting → reporting → done`, marcando hechas, actual y pendientes; `failed` se muestra como estado terminal en rojo con la última fase alcanzada si se conoce (si no, solo «Fallida»). Sondeo cada 3 s mientras el estado no sea `done`/`failed`; se detiene en terminal, al salir de la vista y con la pestaña oculta (`visibilitychange`), y se reanuda al volver. Un error de red o `5xx` no detiene la vista: muestra el aviso, reintenta en el siguiente ciclo y, tras 5 fallos seguidos, deja de sondear con un botón «Reintentar». `404` → «Corrida no encontrada» y sin sondeo. Se muestra `trace_id` si viene. El `id` de la ruta se valida (`^run-[0-9a-f]{32}$`); uno inválido no hace petición.
- **Mis corridas** (en `/runs` o como panel de `/inbox`, a elección; documentarlo): los recibos de `GET /api/confirmations?limit=20` con `run_id` (enlace a `/runs/:id`), `notification_id`, `confirmed_at` en hora local y `confirmed_by`. Sin sondeo.
- **`/warm`**: tarjeta con `state` (insignia distinta por cada uno de los 4 estados, con texto, no solo color), `reset_verified` (sí/no, y un aviso explícito si `state=ready` y `reset_verified=false`), `baseline_version` y `warm_id`. Botón «Actualizar» y sondeo cada 10 s con las mismas reglas de pausa y de 5 fallos. `503 warm_unavailable` → «Estado del warm no disponible» (no un error genérico).
- Enlaces en la cabecera: Inbox, Mis corridas, Warm.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Acciones sobre el warm o la corrida (cancelar, reintentar fase, reset, cuarentena): la UI es de solo lectura aquí.
- Reporte post-mortem (`/reports/{run}`, C-93), evidencia o logs de la corrida.
- Streaming (SSE/WebSocket) o cambios en cualquier servicio. Cambios fuera de `web/dashboard/**`.

---

## Archivos de contexto

- `tareas/U6-T02-ui-api-lectura-warm-confirmaciones.md`, `tareas/U6-T03-login-sesion.md` y sus bitácoras
- `contracts/openapi/control-plane.yaml` (`getRun`, `Run`, y las rutas `/warm`, `/confirmations` de T02)
- `services/go-run-controller/README.md` (fases y fail-closed), `contracts/plans/warm-state.schema.json`
- `mockups/swarm-mock.html` (vista dashboard como referencia)

---

## Criterios de aceptación

Desde `web/dashboard/`, con `npm ci` hecho.

- [ ] **CA-1** — Lo anterior sigue en verde.
  ```bash
  npm run -s lint && npm run -s typecheck && npm run -s check:api && npx vitest run 2>&1 | grep -c -E 'failed|FAIL'
  ```
  Esperado: `0`.

- [ ] **CA-2** — Corrida y sondeo.
  ```bash
  npx vitest run src/pages/Run 2>&1 | tail -n 3
  ```
  Esperado: sin `failed` y al menos 8 pruebas (MSW + temporizadores falsos): `running` marca 6 fases hechas y `running` como actual; secuencia `deploying → running → done` con 3 respuestas → exactamente 3 peticiones y ninguna más tras `done` en 10 s; `failed` es terminal y en rojo; pestaña oculta → 0 peticiones en 9 s y se reanuda al volver; desmontar la vista → 0 peticiones después; 5 `503` seguidos → deja de sondear y aparece «Reintentar»; `404` → mensaje y 1 sola petición; `id` inválido → 0 peticiones.

- [ ] **CA-3** — Mis corridas.
  ```bash
  npx vitest run src/pages/Runs 2>&1 | tail -n 3
  ```
  Esperado: sin `failed` y al menos 3 pruebas: pide `?limit=20`; cada `run_id` enlaza a `/runs/<run_id>`; lista vacía → mensaje de vacío.

- [ ] **CA-4** — Estado del warm.
  ```bash
  npx vitest run src/pages/Warm 2>&1 | tail -n 3
  ```
  Esperado: sin `failed` y al menos 6 pruebas: los 4 estados con texto distinto; `ready` + `reset_verified=false` → aviso explícito; `503 warm_unavailable` → «Estado del warm no disponible»; sondeo cada 10 s (3 peticiones en 25 s de reloj falso); pausa con la pestaña oculta; 5 fallos seguidos → «Reintentar».

- [ ] **CA-5** — Alcance.
  ```bash
  cd ../.. && git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(web/dashboard/|bitacoras/U6-T05\.md|revisiones/U6-T05/)' | wc -l
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

- Un hook `usePolling(fn, intervalMs, {isTerminal})` con sus propias pruebas unitarias (pausa por visibilidad, desmontaje, tope de fallos), reutilizado por corrida y warm.
- **Rojo primero:** pegar la salida de `npx vitest run src/pages/Run src/pages/Warm` antes de implementar.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
