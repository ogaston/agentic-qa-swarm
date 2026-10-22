# U10-T01 — Cáscara y componentes base fieles al mockup

**Unidad:** U10 — Fidelidad visual del dashboard al mockup
**Historias que implementa:** S5 (dashboard presentable).
**Depende de:** U9 cerrada. **Ola 1.** **Bloquea a U10-T02, T03, T04.** **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

## Por qué existe
U9-T04 portó los tokens pero no la estructura: el dashboard usa un vocabulario de clases propio (`app-*`, `page-*`, `notice`, `data-table`) y carece de las piezas del mockup (`az-header`, `az-page-header` con franja de metadatos, `badge`, `card`, `kpi-*`, `segmented`, `modal-*`, `grid-main-side`). El humano compara ambos y «se ven muy distintos».

## Alcance
**Dentro:**
> Dejar en `web/dashboard` la cáscara y las primitivas visuales del mockup, con las mismas medidas y nombres de clase que `mockups/swarm-mock.html`, para que T02–T04 solo las usen.

- Portar de `mockups/swarm-mock.html` (bloque `<style>`) a `src/styles.css`/`src/styles/` las reglas de: `.az-header*`, `.az-brand*`, `.az-logo`, `.az-nav*`, `.az-session-chip`, `.az-avatar`, `.az-principal`, `.az-container`, `.az-page-header`, `.az-page-title`, `.az-header-meta`, `.az-meta-*`, `.az-subbar`, `.az-tablist`, `.az-tab`, `.az-action-link`, `.card`, `.card-pad`, `.card-head`, `.card-title`, `.card-sub`, `.badge*`, `.btn*` (incl. `btn-icon`, `btn-sm`, `btn-primary`), `.alert-box*`, `.segmented`, `.kpi-*`, `.kv-list`, `.stack`, `.grid-main-side`, `.form-field`, `.modal-*`, `.login-wrap`. Sin copiar el JS del mockup.
- `ProtectedLayout`: cabecera idéntica al mockup (60 px, marca `aqs` + «Agentic QA Swarm», navegación con iconos SVG en línea, chip de sesión con avatar de iniciales + `principal_id` + «Rol: …», botón «Salir»).
- **Selector de tema** (`btn-icon`): alterna `data-theme` claro/oscuro, persiste en `localStorage` (con `try/catch`), respeta `prefers-color-scheme` si no hay valor guardado.
- Componentes React reutilizables mínimos: `PageHeader({titulo, descripcion, meta[]})`, `Badge({tono, punto})`, `Card`, `Kpi`. Las vistas actuales siguen funcionando (migran en T02–T04).
- La etiqueta de navegación «Warm» pasa a «Entorno» como en el mockup; se adaptan los selectores de las pruebas (por rol/texto), no sus expectativas de comportamiento.

**Fuera:** reescribir el contenido de las vistas (T02–T04); el interruptor «Previsualizar estado (Demo)», el enlace «Nueva aplicación» y el banner «Iniciar ejecución» del mockup (no existen en el MVP: ver `DESIGN.md §1.3`); librerías de componentes/CSS-in-JS; `style-src 'unsafe-inline'`; cambios fuera de `web/dashboard/**`.

## Archivos de contexto
`mockups/swarm-mock.html`, `DESIGN.md`, `web/dashboard/src/styles.css`, `src/styles/tokens.css`, `src/session/ProtectedLayout.tsx`, `src/session/nav.test.tsx`.

## Criterios de aceptación (desde `web/dashboard/`, tras `npm ci`)
- [ ] **CA-1** — `npm run -s lint && npm run -s typecheck && npm run -s check:api && npx vitest run 2>&1 | grep -c -E 'failed|FAIL'` → `0`.
- [ ] **CA-2** — Paridad de reglas. Una prueba (`src/styles/paridad.test.ts`) extrae del mockup los selectores de clase del inventario de arriba y exige que cada uno exista en el CSS del dashboard con las mismas propiedades `height`, `padding`, `border`, `border-radius`, `font-size`, `font-weight` y `gap`. `npx vitest run src/styles/paridad 2>&1 | grep -c failed` → `0`.
- [ ] **CA-3** — Tema: prueba de componente que conmuta el tema, comprueba `document.documentElement.dataset.theme`, la persistencia y que sin `localStorage` no lanza. Contraste AA de `tokens.test.ts` sigue pasando.
- [ ] **CA-4** — CSP y recursos externos intactos: `npm run -s build >/dev/null && grep -c -E 'unsafe-inline|unsafe-eval' dist/index.html` → `0`.
- [ ] **CA-5** — Alcance: `git diff --name-only $(git merge-base HEAD origin/main) | grep -v -E '^(web/dashboard/|tareas/U10-T01|bitacoras/U10-T01|revisiones/U10-T01/)' | wc -l` → `0`.

**Rojo primero:** pegar la salida de `paridad` fallando antes de implementar.

_Creado con ❤️ por Luis Felipe Ariza Vesga._
