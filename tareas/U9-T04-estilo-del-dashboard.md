# U9-T04 — Estilo del dashboard según `mockups/swarm-mock.html`

**Unidad:** U9 — Resultados visibles en el dashboard
**Historias que implementa:** S5 (dashboard presentable).
**Depende de:** U9-T03 **fusionada** (toca las mismas vistas). **Ola 3**. **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea, concreta):

> Llevar el aspecto de login, inbox, corrida (con reporte y plan), «Mis corridas» y warm al lenguaje visual de `mockups/swarm-mock.html`, sin cambiar comportamiento.

Detalle:

- **Tokens.** Colores, tipografía, espaciado y radios del mockup como variables CSS en un único archivo (`src/styles.css` o `src/styles/tokens.css`), con tema claro y oscuro (`prefers-color-scheme`). Fuentes servidas desde el propio build (sin CDN: la CSP es `default-src 'self'`).
- **Estructura.** Cabecera con navegación (Inbox, Mis corridas, Warm, cerrar sesión), tarjetas, insignias de estado (fases de la corrida, veredicto, estados del warm) y la línea de tiempo de fases con el estilo del mockup. Formularios con etiquetas visibles y foco visible.
- **Accesibilidad.** Contraste AA en ambos temas para texto e insignias; el estado nunca se comunica solo con color (ya es requisito de U6-T05 y U9-T03: no se rompe). Diseño usable a 375 px de ancho sin scroll horizontal.
- **Sin cambios de comportamiento:** las pruebas existentes pasan sin tocar sus aserciones de comportamiento; si alguna depende de la estructura del DOM, se adapta el selector (por rol o texto), no la expectativa.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Librerías de componentes o CSS-in-JS nuevas (React + CSS plano, como hoy). Una dependencia nueva exige justificarla en la bitácora y que el revisor la acepte.
- Estilos en línea que obliguen a relajar la CSP (`style-src 'unsafe-inline'`).
- Animaciones más allá de transiciones cortas de estado (respetando `prefers-reduced-motion`).
- Cambios fuera de `web/dashboard/**`.

---

## Archivos de contexto

- `mockups/swarm-mock.html` (referencia visual; no se copia su JS)
- `DESIGN.md` (sistema de diseño derivado del mockup: tokens, pares de contraste, componentes por vista; si choca con el alcance o los criterios de esta tarea, manda la tarea)
- `web/dashboard/src/styles.css`, `src/pages/**`, `src/session/ProtectedLayout.tsx`
- `tareas/U6-T06-recorrido-local-csp.md` (CSP del build)

---

## Criterios de aceptación

Desde `web/dashboard/`, con `npm ci` hecho.

- [ ] **CA-1** — Comportamiento intacto.
  ```bash
  npm run -s lint && npm run -s typecheck && npm run -s check:api && npx vitest run 2>&1 | grep -c -E 'failed|FAIL'
  ```
  Esperado: `0`. `git diff --stat origin/main -- 'src/**/*.test.tsx'` solo muestra cambios de selectores (el revisor lo comprueba leyendo el diff).

- [ ] **CA-2** — CSP intacta y sin recursos externos.
  ```bash
  npm run -s build >/dev/null && grep -c -E 'unsafe-inline|unsafe-eval' dist/index.html; grep -rhoE 'https?://[^"'"'"' )]+' dist/ | grep -v -E '^https?://(www\.)?w3\.org' | sort -u | wc -l
  ```
  Esperado: `0` y `0`.

- [ ] **CA-3** — Contraste.
  ```bash
  npx vitest run src/styles 2>&1 | tail -n 3
  ```
  Esperado: sin `failed`; una prueba lee los tokens de color y comprueba contraste ≥ 4.5:1 para cada par texto/fondo e insignia/fondo declarado, en ambos temas.

- [ ] **CA-4** — Contra kind, con capturas (dentro de `flock ${XDG_RUNTIME_DIR:-/tmp}/aqs-kind.lock`).
  ```bash
  cd ../.. && bash scripts/kind/dashboard-smoke.sh 2>&1 | grep -c '^FALLA'
  ```
  Esperado: `0`. La bitácora adjunta capturas de login, inbox, corrida con reporte y warm, en claro y oscuro, y una a 375 px.

- [ ] **CA-5** — Alcance.
  ```bash
  cd ../.. && git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(web/dashboard/|tareas/U9-T04-estilo-del-dashboard\.md|bitacoras/U9-T04(\.md|/)|revisiones/U9-T04/)' | wc -l
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

- La prueba de contraste parsea el archivo de tokens (sin navegador).
- **Rojo primero:** pegar la salida de `npx vitest run src/styles` antes de implementar.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
