# Ronda 1 — U9-T04

VEREDICTO: NO-VERDE

Worktree `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U9-T04`, sha `17770ad`. Tras mis corridas el worktree quedó limpio (`git status --short` vacío). Dejé `dist/` generado, que está ignorado por git. Mis scripts temporales están en `/tmp/r1v/`.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| CA-1 | Comportamiento intacto | `npm run -s lint && typecheck && check:api && npx vitest run \| grep -c -E 'failed\|FAIL'` | pasa: `0`, 14 archivos y 164 tests pasan. El único archivo de test tocado es `src/styles/tokens.test.ts` (nuevo). No se modificó ningún `*.test.tsx`: `git diff --stat origin/main -- 'src/**/*.test.tsx'` sale vacío. |
| CA-2 | CSP intacta, sin externos | build + los dos greps de la tarea | `unsafe-inline` → `0`. URLs → **3** (`http://localhost/`, `https://destino.invalid`, `https://reactjs.org/docs/error-decoder.html?invariant=`). Acepto la decisión 1 del orquestador, porque comprobé que la rama no las agrega: el diff no toca `src/api/client.ts` ni `src/session/next.ts`. `src=`/`href=` con `http(s)://` en `dist` → 0, `url(http…)` → 0, `@import` o `url(` no-data en el CSS → 0. |
| CA-3 | Contraste | `npx vitest run src/styles` | pasa: 36/36 (ver F-03 sobre su cobertura). |
| CA-4 | Smoke contra kind | `flock … bash scripts/kind/dashboard-smoke.sh \| grep -c '^FALLA'` | pasa: `0`, salida 0, ocho OK incluido `reporte-visible` (`/tmp/r1-smoke.txt`). Pero mis mediciones a 375 px contradicen la afirmación de «sin scroll horizontal» (F-01). |
| CA-5 | Alcance | los dos comandos de la tarea | pasa: `0` y `0`. |

## Hallazgos

### F-01 · ROJO · `src/styles.css:476-494` (`dl`) y `Inbox/InboxPage.tsx` (Detalle y diálogo) · Scroll horizontal a 375 px en el detalle y el diálogo de confirmación
La tarea exige «usable a 375 px de ancho sin scroll horizontal» en los dos temas. La bitácora midió solo la lista de Inbox, sin abrir un detalle ni el diálogo.

Contra kind (`dashboard-up.sh` bajo `flock`, Chromium headless, script `/tmp/r1v/chk.mjs`):
```
light/dark 375  inbox sin detalle   scrollWidth=375
light/dark 375  tras clic en "Ver"  → Detalle abierto: scrollWidth=445   (y con el diálogo abierto: 445)
light/dark 320  igual: 445
```
`/tmp/r1v/d2.mjs` lista los culpables: los `DD.|445` y `CODE.|417`, es decir el SHA completo (40 caracteres, `<code>`) y el texto del artefacto dentro de `dl`.

Causa probable: `grid-template-columns: max-content 1fr` deja el `1fr` con mínimo `auto`, y esos valores largos no se parten. La captura `/tmp/r1v/dialog.png` muestra el SHA cortado en el borde derecho del Detalle y del diálogo. Las rutas Run, Run con plan abierto, Mis corridas, Warm y Login sí miden 375 (y 320).

Solución esperada: `minmax(0,1fr)` más `overflow-wrap:anywhere` en `dd`/`code`.

Las capturas de la bitácora nunca muestran el Detalle de Inbox, el diálogo, un run fallido ni el plan abierto. Es el flujo central de Inbox, y ahí está el defecto.

### F-02 · NARANJA · `src/styles.css:562-575` (`.phase-actual::before` + `@keyframes pulso`) · Animación infinita fuera de alcance
La lista «Fuera» de la tarea prohíbe «animaciones más allá de transiciones cortas de estado». El punto pulsante con `animation: pulso 1.6s infinite` no es una transición y no aparece en el mockup: `grep animation mockups/swarm-mock.html` solo da transiciones de 0.12 s y 0.15 s. Mitigante: se anula bajo `prefers-reduced-motion`. Aun así es trabajo fuera de alcance dentro del diff. Se resuelve borrando esas reglas.

### F-03 · NARANJA · `src/styles.css:341, 509, 579-583` y `src/styles/tokens.test.ts` · Colores literales fuera de `tokens.css` y pares declarados sin cubrir
La tarea pide colores como variables CSS en un único archivo. En `styles.css` quedan literales: `#ffffff` (`.btn-primary`), `#fde8ea` y `#b00020` (`.status-failed`) y `rgba(0,0,0,.25)` (sombra del modal).

CA-3 exige «cada par texto/fondo declarado», pero la prueba usa una lista manual de 17 pares y no deriva nada de la hoja. Quedan fuera:
- `#b00020` sobre `#fde8ea`. Calculé 6.25:1, cumple.
- `--text-muted` sobre `--info-soft` (`.phase-actual`). Calculé 5.79 en claro y 7.26 en oscuro, cumple.
- `--text` sobre `--danger-soft` (`.notice-danger`). Calculé 12.75 y 14.72, cumple.
- `#ffffff` sobre `--accent-hover` (hover del botón primario). Calculé 7.52 en claro y **4.53** en oscuro, al límite.

Hoy no hay violación real de AA, pero la prueba no protegería un cambio futuro. La prueba de `#ffffff` sobre `--accent` está desacoplada del CSS, que usa un literal.

Solución esperada: tokens `--on-accent` y `--failed-fg`/`--failed-bg`, o sus equivalentes. El `#b00020` en el TSX se queda por el test.

### F-04 · AMARILLO (no bloquea) · varios
- `ProtectedLayout.tsx` usa `Link`, no `NavLink`. Por eso `a[aria-current="page"]` (`styles.css:171`) nunca se activa (medí 0 `aria-current` en la nav) y la pestaña activa del mockup no se ve.
- Las clases `meta-list` y `warm-card` no existen en el CSS.
- `--font-sans` nombra «IBM Plex Sans»/«Mono», pero no hay `@font-face` ni archivos de fuente en el build. Cae a la pila del sistema: no hay CDN, pero tampoco «fuentes servidas desde el propio build».
- `.modal` no tiene `max-height` ni `overflow:auto`. En pantallas bajas o en horizontal, el diálogo con más familias puede quedar inalcanzable.
- El CSS tiene un contraste de bordes de campos de 1.49:1 (`--border` sobre `--surface`). La tarea acota AA a texto e insignias, así que no es defecto aquí.
- Hay una línea sin sangría en TSX (`<div className="toolbar">` y `<div className="card table-wrap">`). No pasa de estilo.

## Decisiones del orquestador
1. CA-2 con 3 URLs: aceptada, verificada por mí (arriba).
2. Sin selector de tema manual: acepto. La tarea pide `prefers-color-scheme`.
3. Estilo inline `#b00020` de `RunPage.tsx`: aceptable.
   - CSP: el build no tiene `unsafe-inline`, la CSP es `style-src 'self'` y React aplica el `style` por CSSOM. Ese estilo ya existía en `origin/main`.
   - AA: `#b00020` sobre el fondo fijo `#fde8ea` da 6.25:1 en ambos temas.
   - Nota: `DESIGN.md §6.2` dice «nunca `style={{}}`», pero el test de `RunPage` lo fija.
4. Desbordamiento a 375 px y contraste AA: el contraste del conjunto de tokens se cumple (todos los pares calculados ≥ 4.5). El desbordamiento **no** se cumple en Detalle y diálogo (F-01).

## Tareas candidatas (fuera de alcance)
- Corregir el filtro de CA-2 para excluir las bases de parseo y el enlace del decodificador de React.
- Arreglar la nota de `DESIGN.md §6.2` frente al test de `RunPage`: mover el color fallido a una clase y adaptar el test (es un cambio de aserción).

## Transcripciones
- `/tmp/r1-vitest.txt`
- `/tmp/r1-smoke.txt`
- `/tmp/r1v/chk.mjs`
- `/tmp/r1v/d2.mjs`
- `/tmp/r1v/dialog.png`

VEREDICTO: NO-VERDE
ROJO|src/styles.css:476-494 (dl) + Inbox Detalle/diálogo|Scroll horizontal a 375 px (scrollWidth 445) con detalle de Inbox y diálogo abiertos, en ambos temas
NARANJA|src/styles.css:562-575|Animación infinita (pulso) fuera de alcance
NARANJA|src/styles.css:341,509,579-583; tokens.test.ts|Colores literales fuera de tokens.css y pares AA declarados sin cubrir en la prueba
