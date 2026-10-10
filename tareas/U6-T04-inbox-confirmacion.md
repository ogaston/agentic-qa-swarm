# U6-T04 — Inbox de notificaciones y confirmación de corridas

**Unidad:** U6 — Frontend web (S5)
**Historias que implementa:** S5 (dashboard inbox), US-M1 (notificación sin auto-run) y US-M2 (confirm humano antes del deploy).
**Depende de:** U6-T03 **fusionada**. **Ola 3**, en paralelo con U6-T05 (no comparten páginas; si ambas tocan `App.tsx`, solo para registrar su ruta). **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea, concreta):

> Implementar la vista `/inbox`: lista de notificaciones de `GET /api/notifications` con filtro por estado, detalle de cada una y confirmación explícita con `POST /api/notifications/{id}/confirm`, que al tener éxito lleva a `/runs/{run_id}`.

Detalle:

- **Lista**: pestañas `Pendientes` (por defecto), `Confirmadas`, `Rechazadas` que mapean a `?state=`. Cada fila: repo, evento (`commit`/`pull_request`/`tag`), `sha` corto (7), tipo y referencia del artefacto si viene, estado. Estados de vista: cargando, vacío («No hay notificaciones pendientes»), error (`<ApiErrorNotice>` de T03). Botón «Actualizar»; **sin** polling automático.
- **Confirmación**: solo en notificaciones `pending`. Abre un panel (no un `window.confirm`) con el resumen (repo, sha, artefacto), una lista de familias de flujos con casillas —la constante `FLOW_FAMILIES = ['happy-path']` en `src/inbox/flows.ts`, marcada por defecto— y el botón «Confirmar corrida». El botón está deshabilitado sin al menos una familia y mientras la petición está en vuelo (un solo `POST`). Nada se confirma sin esa acción explícita: ni al abrir el panel, ni al cargar la lista.
- **Respuestas**: `201` → navega a `/runs/{run_id}` del `ConfirmationReceipt`; `409` → «Esta notificación ya no está pendiente» y recarga la lista; `404` → «La notificación ya no existe» y recarga; `400`/`413`/`415` → `<ApiErrorNotice>`; `401` lo trata la sesión (T03).
- **Accesibilidad mínima**: la lista es una tabla con cabeceras o una lista con `role`, las pestañas con `role="tablist"`, el panel con foco inicial en su título y `Escape` para cerrarlo.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Rechazar notificaciones (no hay ruta en la API), editar flujos o elegir familias distintas de la constante (S2 está fuera).
- Vista de corrida y de warm (U6-T05).
- Cambios fuera de `web/dashboard/**`.

---

## Archivos de contexto

- `tareas/U6-T03-login-sesion.md` y `bitacoras/U6-T03.md` (sesión, `<ApiErrorNotice>`)
- `contracts/openapi/control-plane.yaml` (`listNotifications`, `confirmRun`, `Notification`, `ConfirmationReceipt`)
- `services/ui-api/README.md` (códigos de respuesta del confirm)
- `mockups/swarm-mock.html` (referencia visual)

---

## Criterios de aceptación

Desde `web/dashboard/`, con `npm ci` hecho.

- [ ] **CA-1** — Lo anterior sigue en verde.
  ```bash
  npm run -s lint && npm run -s typecheck && npm run -s check:api && npx vitest run 2>&1 | grep -c -E 'failed|FAIL'
  ```
  Esperado: `0`.

- [ ] **CA-2** — Lista y filtros.
  ```bash
  npx vitest run src/pages/Inbox -t 'lista' 2>&1 | tail -n 3
  ```
  Esperado: sin `failed` y al menos 5 pruebas (MSW): cada pestaña pide su `?state=` y muestra solo lo devuelto; vacío → mensaje de vacío; `503` → aviso de servicio no disponible; `sha` truncado a 7; «Actualizar» hace exactamente una petición nueva y no hay peticiones en segundo plano (2 s de temporizador falso sin peticiones).

- [ ] **CA-3** — Confirmación explícita.
  ```bash
  npx vitest run src/pages/Inbox -t 'confirma' 2>&1 | tail -n 3
  ```
  Esperado: sin `failed` y al menos 7 pruebas: abrir el panel no envía nada; el cuerpo es `{"flows":["happy-path"]}`; sin familias el botón está deshabilitado; doble clic → un solo `POST`; `201` → URL `/runs/<run_id>`; `409` y `404` → su mensaje y una recarga de la lista; notificaciones `confirmed`/`rejected` no ofrecen confirmar; `Escape` cierra el panel sin enviar.

- [ ] **CA-4** — Nunca se confirma solo.
  ```bash
  npx vitest run src/pages/Inbox -t 'sin auto' 2>&1 | tail -n 3
  ```
  Esperado: sin `failed`; una prueba que carga el inbox con 3 pendientes, recorre las pestañas, abre y cierra un panel, y comprueba que MSW recibió **0** peticiones `POST`.

- [ ] **CA-5** — Alcance.
  ```bash
  cd ../.. && git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(web/dashboard/|bitacoras/U6-T04\.md|revisiones/U6-T04/)' | wc -l
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

- Testing Library + `user-event` + MSW con fixtures construidas con los tipos de `schema.gen.ts` (un fixture que no tipa no compila).
- **Rojo primero:** pegar la salida de `npx vitest run src/pages/Inbox` antes de implementar.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
