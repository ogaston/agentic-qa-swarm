# U10-T02 — Inbox fiel al mockup

**Unidad:** U10 — Fidelidad visual del dashboard al mockup
**Historias que implementa:** S5, US-M1 (confirmación humana, sin cambiar comportamiento).
**Depende de:** U10-T01 **fusionada**. **Ola 2.** **Tope: 3 rondas**; en la 3.ª NO-VERDE se escala.

## Alcance
**Dentro:**
> Rehacer `/inbox` con la estructura de la «Vista Inbox» del mockup (líneas ~1122-1230) usando las primitivas de T01.

- `PageHeader` «Inbox de notificaciones» con descripción y franja de metadatos (Entorno, Reset verificado, Familias activas) alimentada **solo con datos reales** (`GET /warm`, notificaciones); si un dato no existe en la API, se omite la celda, no se inventa.
- Subbarra con `az-tablist` (Pendientes (n) / Confirmadas (n) / Rechazadas (n)), con roles `tab`/`tabpanel`, `aria-selected` y navegación con flechas; acciones «Actualizar» (sin «Nueva aplicación»).
- Franja de 4 KPI (`kpi-strip`): pendientes, confirmadas, estado del entorno, y «Auto-ejecución sin humano = 0» solo si se deriva de un dato real; si no, se sustituye por el KPI «Rechazadas».
- Listado en tarjeta (`card`, tabla con insignias de estado, metadatos del artefacto) y diálogo «Confirmar corrida» con `modal-backdrop`/`modal-dialog`, foco atrapado, Esc cierra, foco vuelve al disparador.
**Fuera:** endpoints nuevos; cambiar la regla de confirmación humana; datos inventados.

## Archivos de contexto
`mockups/swarm-mock.html`, `DESIGN.md §3-4`, `web/dashboard/src/pages/Inbox/InboxPage.tsx` y su prueba.

## Criterios de aceptación
- [ ] **CA-1** — Comportamiento intacto: `npm run -s lint && npm run -s typecheck && npx vitest run 2>&1 | grep -c -E 'failed|FAIL'` → `0`; el diff de `*.test.tsx` solo cambia selectores.
- [ ] **CA-2** — Estructura: prueba que renderiza `/inbox` con datos MSW y verifica la presencia (por rol/nombre) de: encabezado, `tablist` con 3 pestañas y contadores, 4 KPI, tabla y botón «Actualizar»; y que no hay un «Nueva aplicación».
- [ ] **CA-3** — Fidelidad: `npm run -s e2e:fidelidad -- inbox` (entregado por U10-T05) pasa; mientras T05 no exista, el revisor compara capturas lado a lado (claro/oscuro, 1280 y 375 px) en la bitácora.
- [ ] **CA-4** — CSP: `grep -c -E 'unsafe-inline|unsafe-eval' dist/index.html` → `0`. Alcance: nada fuera de `web/dashboard/**` y los archivos de la tarea.

_Creado con ❤️ por Luis Felipe Ariza Vesga._
