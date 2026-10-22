# Ronda 2 — U9-T04

VEREDICTO: VERDE

Worktree `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U9-T04`, sha `3025297`. Después de mis corridas `git status --short` está vacío. Dejé `dist/` generado, que está ignorado por git.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| CA-1 | Comportamiento intacto | lint, typecheck, check:api y `npx vitest run \| grep -c -E 'failed\|FAIL'` | pasa: `0`, 14 archivos y 170 tests pasan. `git diff --stat origin/main -- 'src/**/*.test.tsx'` sale vacío: no se tocó ningún `*.test.tsx`. Solo existe `tokens.test.ts`. |
| CA-2 | CSP intacta, sin externos | build y los greps de la tarea | `unsafe-inline` = `0`. URLs = 3, las mismas de la ronda 1: `http://localhost/`, `https://destino.invalid` y el enlace del decodificador de React. Son las mismas que en `origin/main` (decisión 1 del orquestador). `src=`/`href=` externos = 0, `@import` o `url(` no-data = 0. |
| CA-3 | Contraste | `npx vitest run src/styles` | pasa: 42/42. |
| CA-4 | Smoke contra kind | `flock … dashboard-smoke.sh \| grep -c '^FALLA'` | pasa: `0`, salida 0, 8 OK incluido `reporte-visible` (`/tmp/r2-smoke.txt`). Las capturas de la bitácora cubren login, inbox, detalle, diálogo, corrida con reporte, plan y warm, en claro y oscuro, a 375 y 320 px. |
| CA-5 | Alcance | los dos comandos de la tarea | pasa: `0` y `0`. |

## Estado de los hallazgos de la ronda 1
- **F-01 (ROJO) — resuelto.** Repetí `/tmp/r1v/chk.mjs`, `d2.mjs` y uno nuevo, `d3.mjs`, bajo `flock`, con `dashboard-up.sh` y luego `dashboard-down.sh`. Medí en claro y oscuro, a 375 y 320 px.
  - login, inbox, Detalle abierto, diálogo abierto, `/runs`, `/warm`, corrida y corrida con plan abierto: `scrollWidth == ancho` en todas las combinaciones. La ronda 1 daba 445 con el detalle y el diálogo.
  - `d2.mjs` ya no lista ningún elemento por fuera de 376 px.
  - El diálogo en un viewport de 350 px de alto queda contenido: top 17.5, bottom 332.4 y `max-height` aplicado.
- **F-02 (NARANJA) — resuelto.** `grep 'animation\|keyframes' styles.css` solo deja `animation: none !important` dentro de `prefers-reduced-motion`. El punto pulsante y `@keyframes pulso` ya no existen.
- **F-03 (NARANJA) — resuelto.**
  - `styles.css` ya no tiene hex ni `rgba` (grep vacío). Los tokens nuevos `--on-accent`, `--failed-fg`, `--failed-bg` y `--modal-shadow` están en `tokens.css`, en ambos temas.
  - La prueba ahora deriva los pares de `styles.css`: misma regla, componente más su `:hover`, texto sobre las superficies de la app y fondos con `--text`/`--text-muted`. Comprobé que no es vacua con mutaciones en una copia en `/tmp`, ya borrada:
    - `.badge-warn` con color `--warn-soft` hace fallar la prueba (1.00:1).
    - `.empty` con `--border` falla en ambos temas.
    - `--on-accent` oscuro en `#9999ff` falla (2.23:1).
  - `.status-failed` (`#b00020` sobre `#fde8ea`) queda cubierto por el par derivado. Su contraste real es 6.25:1.
  - El hover del botón primario en oscuro mide 4.53:1, al límite pero ≥ 4.5. La bitácora lo documenta como decisión de referencia visual.
- **F-04 (AMARILLO) — resuelto en lo sustancial.**
  - `NavLink` ya funciona: medí 1 `aria-current="page"` en la navegación, en los cuatro contextos.
  - Quitaron las clases muertas `meta-list` y `warm-card`.
  - `.modal` tiene `max-height` y `overflow:auto`.

## Los dos pendientes que declaró el coordinador
1. **Run fallido sin captura: juzgo que basta lo hecho, y además lo verifiqué.** Simulé `state=failed` con `page.route` sobre la API en kind, en ambos temas, a 375 y 320 px. `/tmp/r1v/d3.mjs` mostró `scrollWidth == ancho` en las cuatro combinaciones. El estilo calculado es `color rgb(176,0,32)` sobre `rgb(253,232,234)` y el texto «Fallida» está presente (no depende solo del color). El contraste de ese par está cubierto por la prueba derivada y por `RunPage.test`. La captura `/tmp/r1v/fallida-dark.png` se ve legible. No hace falta capturarlo en kind.
2. **Fuentes: verificado.** `grep -rn Plex web/dashboard/src web/dashboard/index.html` solo encuentra un comentario de `tokens.css`. `--font-sans` y `--font-mono` usan pila de sistema, sin `@font-face` ni nombre de fuente externa.

## Limpieza
- No quedó nada mío levantado: el `dashboard-down.sh` terminó con 0. Ya no hay procesos ni puertos 186xx míos.
- El listener `127.0.0.1:18622` (`kubectl port-forward svc/go-intake 18622:8080`, pid 65146) arrancó el 21 oct 19:01, antes de mi sesión de hoy. **No es mío** y no lo cerré.

## Hallazgos nuevos
- **F-05 · AMARILLO (no bloquea)** · `InboxPage.tsx` · La reindentación quedó solo en dos líneas (`tablist` y `<table>`). El resto del bloque queda con sangría desalineada. Es estilo, y el codificador ya lo declaró. Prettier o lint no fallan.

## Tareas candidatas (sin cambios respecto de la ronda 1)
- Corregir el filtro de CA-2 para excluir las bases de parseo y el enlace de React.
- Reconciliar `DESIGN.md §6.2` («nunca `style={{}}`») con `RunPage.test`, que fija `rgb(176,0,32)` en un estilo inline.
- Decidir si se vendorizan las fuentes (hoy pila de sistema).

## Transcripciones
- `/tmp/r2-vitest.txt`
- `/tmp/r2-smoke.txt`
- `/tmp/r1v/chk.mjs`, `/tmp/r1v/d2.mjs`, `/tmp/r1v/d3.mjs`
- `/tmp/r1v/fallida-dark.png`, `/tmp/r1v/fallida-light.png`

VEREDICTO: VERDE
AMARILLO|web/dashboard/src/pages/Inbox/InboxPage.tsx|Reindentación parcial del bloque de tabla/toolbar (estilo, no bloquea)
