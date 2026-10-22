# Respuesta a la ronda 1 — U10-T01

Bitácora: `bitacoras/U10-T01.md`, sección «Ronda 2». SHA: el del commit de la ronda 2 sobre `f2e4453` (rama `tarea/U10-T01`).

F-01: Corregido en el commit de la ronda 2: `.card` del mockup sin `padding` ni `margin-bottom`. El relleno y margen heredados pasan a la clase puente `.card-vista` en las nueve tarjetas de vistas actuales (LoginPage, InboxPage ×3, RunsPage, Resultados ×3, WarmPage); el aspecto no cambia. `Card pad={false}` renderiza solo `.card` y la regla no tiene relleno. Prueba nueva `src/components/Card.test.tsx` que lee la regla de la hoja. Rojo verificado: con `padding: 16px` en `.card`, 2 pruebas fallan; revertido, 4 pasan.

F-02: Corregido en el commit de la ronda 2: `.btn:disabled` pasa a `opacity: 0.5`, como el mockup. No hay motivo de contraste que lo impida (estado deshabilitado, exento).

F-03: Corregido en el commit de la ronda 2: `SelectorTema` tiene `aria-label` y `title` dinámicos («Cambiar a tema oscuro» / «Cambiar a tema claro»), como el mockup. Prueba nueva `src/session/cabecera.test.tsx` que cubre iniciales (AN, OD), «Rol: user», «Salir», nombre accesible `Principal` y la etiqueta dinámica del tema.

F-04: Fuera de alcance: no se toca. Queda como candidata (vendorizar IBM Plex con `font-src 'self'`, idealmente en T05).

F-05: Fuera de alcance: no se toca. Queda como candidata (paridad bidireccional y `@media` en T05).
