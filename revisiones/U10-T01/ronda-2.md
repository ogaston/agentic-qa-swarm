# Ronda 2 — U10-T01

VEREDICTO: VERDE

Worktree `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U10-T01`, rama `tarea/U10-T01`, c2eb0a3 sobre f2e4453. Al terminar, `git status` salió limpio. No escribí en el worktree. Mis copias y mutaciones están en el scratchpad.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | lint + typecheck + check:api + vitest | `npm ci`, y luego el comando literal de la tarea | `0`. 19 archivos y 189 pruebas pasan |
| 2 | Paridad de reglas | `npx vitest run src/styles/paridad 2>&1 \| grep -c failed` | `0` |
| 3 | Tema + contraste AA | Suite completa (incluye `tema.test.tsx`, `cabecera.test.tsx`, `tokens.test.ts`) | pasa |
| 4 | CSP intacta | `npm run -s build >/dev/null && grep -c -E 'unsafe-inline\|unsafe-eval' dist/index.html` | build 0, conteo `0` |
| 5 | Alcance | `git diff --name-only $(git merge-base HEAD origin/main) \| grep -v -E '^(web/dashboard/\|tareas/U10-T01\|bitacoras/U10-T01\|revisiones/U10-T01/)' \| wc -l` | `0` |

## Cierre de los hallazgos de la ronda 1
- **F-01 (NARANJA) — cerrado.**
  - `.card` ya no declara `padding` ni `margin-bottom` y coincide con el mockup. El relleno heredado pasó a `.card-vista`, que se aplica a las 9 tarjetas de vistas actuales: Login, Inbox ×3, Runs, Resultados ×3 y Warm. Se quitaron los `margin-bottom: 0` de compensación, que ya no hacían falta. El `@media (max-width: 520px)` pasó de `.card` a `.card-vista`.
  - Mutación mía en una copia del scratchpad:
    - Con `padding: 16px` en `.card`, `Card.test.tsx` falla (2 de 4 pruebas).
    - Con `margin-bottom: 16px` en `.card`, falla 1 prueba.
    - Revertido, pasan las 4.
  - Comparación en navegador del aspecto: construí f2e4453 (viejo) y c2eb0a3 (nuevo) en el scratchpad, con Chromium headless y API simulada. Probé 1280 px y 480 px, que es donde actúa el `@media` de 520. Vistas: login, inbox (lista), «mis corridas», entorno y corrida (veredicto, post-mortem y plan de flujos).
    - Los estilos computados de las 10 tarjetas medidas (`padding`, `margin-bottom`, alto, ancho y posición vertical) coinciden en ambas versiones.
    - Las 12 capturas de página completa son idénticas byte a byte.
    - No cubrí el detalle del Inbox ni el diálogo modal: la selección no se renderizó en mi simulación. Su cambio es solo añadir `card-vista` en la misma posición de la cascada, y la suite los cubre por comportamiento.
- **F-02 (AMARILLO) — cerrado.** `.btn:disabled` pasó a `opacity: 0.5`.
- **F-03 (AMARILLO) — cerrado.** `SelectorTema` tiene `aria-label` y `title` dinámicos («Cambiar a tema oscuro» / «Cambiar a tema claro»). `cabecera.test.tsx` es nuevo y cubre iniciales (AN, OD), «Rol: user», `nav` «Principal» con «Entorno», «Salir» y la etiqueta del tema.
- **F-04 y F-05** quedan fuera de alcance por arbitraje. Siguen como candidatas.

## Hallazgos nuevos
No hay ROJO ni NARANJA. No vi regresiones.

### F-06 · AMARILLO · `src/components/Card.test.tsx` · la prueba de `.card` es evadible
La prueba extrae con una regex la primera regla exactamente igual a `.card {`. En mutaciones mías, `padding: 16px` o `margin-bottom: 16px` dentro de esa regla sí se detectan. Pero una segunda regla `.card { padding: 16px; }` añadida al final del archivo, o un selector agrupado `.card, .xx { padding: 16px; }`, no se detecta y las 4 pruebas pasan. Ese segundo caso no lo detecta ni `paridad.test.ts`, que solo exige lo que el mockup declara. No bloquea: el caso directo sí falla. La candidata de T05 es la paridad bidireccional, que lo cubriría.

## Tareas candidatas (sin cambios respecto a la ronda 1)
- Paridad bidireccional y cobertura de `@media` y estados en T05 (ver F-06).
- Vendorizar IBM Plex Sans y Mono con `font-src 'self'` (F-04), idealmente en T05.
- `DESIGN.md` sigue nombrando «Warm» en la navegación.
- Retirar `.card-vista` al migrar las vistas en T02–T04. El comentario del CSS ya lo anota.

## Artefactos (scratchpad)
Todos están bajo `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/2b25eeb9-6a55-4813-b08e-b008f856522d/scratchpad/`:
- `cmpviews.cjs` — script de comparación viejo/nuevo.
- `old1.json` y `new1.json` — métricas de las tarjetas.
- `old1-*.png` y `new1-*.png` — capturas.
- `y/` — copia de la mutación.

VEREDICTO: VERDE
AMARILLO|web/dashboard/src/components/Card.test.tsx|La prueba de `.card` no detecta una segunda regla `.card` ni un selector agrupado (cubrir en la paridad bidireccional de T05)
