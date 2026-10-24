# Ronda 2 — U10-T03

VEREDICTO: VERDE

Tarea blob 5c1acbc; rama `tarea/U10-T03` sha 1cdefab; base cbf8a6b. Worktree limpio antes y después (`git status --short` vacío). Todo lo que escribí (build, copia del repo, árbol fusionado) está en el scratchpad.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | lint, typecheck y vitest sin fallos | `npm run -s lint && npm run -s typecheck && npx vitest run --no-cache \| grep -c -E 'failed\|FAIL'` (desde `web/dashboard/`) | pasa: `0`; 20 archivos y 201 pruebas OK en 6,85 s (no es una suite que se salta) |
| 2 | Pruebas por estado por rol y texto | incluido en CA-1: `RunPage.fidelidad.test.tsx` y `RunPage.resultados.test.tsx` | pasa; se añadió la prueba de insignias |
| 3 | Sin `style={{` en `src/pages/Run` | `grep -rn 'style={{' src/pages/Run \| grep -v test \| wc -l` | pasa: `0` |
| 4 | Fidelidad, capturas lado a lado | reproduje `capturar.cjs` sobre una copia de `git archive HEAD` en el scratchpad con `PLAYWRIGHT_PATH=/home/omarjayg/.hermes/hermes-agent/node_modules/playwright`; comparé con PIL las 16 `par-*.png` entregadas con las mías | pasa: las 16 son idénticas píxel a píxel; `desbordamiento.json` coincide (app 0 en las 16 combinaciones; mockup 94 px a 375 px, defecto del mockup) |
| 5 | CSP y alcance | `vite build --outDir <scratchpad>` (código 0); `grep -c -E 'unsafe-inline\|unsafe-eval' index.html`; `git diff --name-only cbf8a6b...tarea/U10-T03 \| grep -v -E '^(web/dashboard/\|tareas/U10-T03\|bitacoras/U10-T03\|revisiones/U10-T03/)' \| wc -l` | pasa: `0` y `0` |

## Cierre de los hallazgos de la ronda 1
- **F-01 (ROJO), cerrado.** Hay 16 pares `par-<estado>-<tema>-<ancho>.png` (bug, sin-hallazgos, en-curso y fallida, en claro y oscuro, a 1280 y 375 px), más las capturas sueltas `app-*` y `mock-*`. El script es reproducible: se ejecuta con `PLAYWRIGHT_PATH`, construye en `os.tmpdir()` y no contiene rutas de la máquina. Vi los pares `par-bug-light-1280`, `par-fallida-dark-375` y `par-fallida-dark-1280`.
- **F-02 (NARANJA), cerrado.** El diff de `styles.css` es de +6 líneas y solo en `dl.kv-list dt`: `font-size: inherit`, `text-transform: none`, `letter-spacing: normal`, `align-self: stretch`. La regla global `dl dt` no se toca. En `par-bug-light-1280` las etiquetas («Invariante violado», «Causa raíz», «Evidencia (copiable)») salen en minúscula, con el tamaño y la alineación del mockup.
- **F-03 (AMARILLO), cerrado.** La insignia dice «Veredicto: bug», «Veredicto: sin-hallazgos» y «Veredicto: inconcluso», con las pruebas actualizadas. Retiré la clase `phase phase-*` de los `li`: `grep` no encuentra `phase-hecha|actual|pendiente` ni `className="phase"` en `src`, y `ol.phase-timeline` y `data-fase-estado` se conservan.

## Coordinación con T04 (rama `tarea/U10-T04`, sha a829f0e)
- La prueba de T04 (`RunsPage.test.tsx:127-129`) comprueba por regex que `styles.css` no tenga ninguna regla `th code` ni `dl.kv-list dt code`. El bloque de T03 es `dl.kv-list dt { ... }` y no menciona `code`, así que no choca.
- `git merge-tree --write-tree tarea/U10-T03 tarea/U10-T04` termina sin conflictos (árbol e821db8). Sobre el árbol fusionado, extraído con `git archive` en el scratchpad y con `node_modules` enlazado, corrí vitest, typecheck y lint. Resultado: 20 archivos y 219 pruebas pasan, typecheck 0 y lint 0. Esas 219 incluyen la prueba de T04.
- Las regex de T04 sobre el `styles.css` fusionado dan `false` y `false`, así que la prueba pasa.

## Hallazgos
### F-04 · AMARILLO · `web/dashboard/src/styles.css:1073-1079` y `src/styles/tokens.css:39-40,76-77,109-110` · Banner de fallida en tema oscuro
En `par-fallida-dark-1280.png` y `par-fallida-dark-375.png` el banner de fallida sale con fondo rosa claro y texto rojo, porque `.status-failed` usa los tokens `--failed-bg` y `--failed-fg`, que son fijos y no cambian en oscuro. El mockup lo pinta en rojo oscuro. Los tokens fijos son una decisión de U9 y el contraste es legible, así que no bloquea. Es un desvío visible frente al mockup en oscuro y lo debí señalar en la ronda 1. Los pares fallida y oscuro de las capturas lo muestran.

### F-05 · AMARILLO · `RunPage.fidelidad.test.tsx`, prueba «ninguna insignia depende solo del color»
Solo comprueba que cada `span.badge` tenga texto no vacío. Es débil, pero con la de las 9 fases cubre la intención y no bloquea.

## Tareas candidatas (fuera de alcance)
- Ajustar `--failed-bg` y `--failed-fg` en el tema oscuro para que el banner de fallida coincida con el mockup (F-04).
- Los textos de «Fallida · última fase alcanzada: …» y «Detenida en rehearsing» del mockup dependen de que la API exponga la última fase. En las capturas la API simulada no la expone, y el código ya soporta el caso.
- Las candidatas de la ronda 1 siguen vigentes: chevron del plan, «Ver request» con modal, revisión de la `dl dt` global y criterio común de insignia de veredicto.

## Rutas
- Salida de vitest: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/65e5c329-227c-4891-af74-2c6042289597/scratchpad/v2.txt`
- Árbol fusionado: `.../scratchpad/merged`
- Reproducción de capturas: `.../scratchpad/repro`

VEREDICTO: VERDE
AMARILLO|web/dashboard/src/styles.css:1073|banner de fallida en tema oscuro (tokens --failed-* fijos) difiere del mockup; no bloquea
AMARILLO|RunPage.fidelidad.test.tsx|prueba de insignias solo comprueba texto no vacío; no bloquea
INFORME: revisiones/U10-T03/ronda-2.md
