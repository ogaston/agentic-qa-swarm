# Ronda 1 — U10-T03

VEREDICTO: NO-VERDE

Tarea blob 5c1acbc; rama `tarea/U10-T03` sha 91104d9; base cbf8a6b. Worktree limpio antes y después de mi revisión: `git status --short` vacío. El build lo hice con `--outDir` en el scratchpad.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí (desde `web/dashboard/`) | Resultado |
|---|---|---|---|
| 1 | lint, typecheck y vitest sin fallos | `npm run -s lint && npm run -s typecheck && npx vitest run --no-cache \| grep -c -E 'failed\|FAIL'` | pasa: `0`; 20 archivos y 200 pruebas OK, 6,7 s (no es una suite que se salta) |
| 2 | Pruebas por estado por rol y texto | incluidas en CA-1: `RunPage.fidelidad.test.tsx` (11 `it`) más `RunPage.resultados.test.tsx` adaptado | pasa; cubren los 6 estados, plan plegable y 9 fases con texto |
| 3 | Sin `style={{` en `src/pages/Run` | `grep -rn 'style={{' src/pages/Run \| grep -v test \| wc -l` | pasa: `0` |
| 4 | Fidelidad: `e2e:fidelidad` o capturas lado a lado en la bitácora | capturas propias con Chromium y Playwright sobre un build servido con API simulada con `page.route`; el mockup se carga con `showView('run'); applyRunDemo(...)` | **no cumple como se entrega** (F-01); la vista es en general fiel, con un desvío (F-02) |
| 5 | CSP y alcance | `npm run -s build` (código 0); `grep -c -E 'unsafe-inline\|unsafe-eval' dist/index.html`; `git diff --name-only cbf8a6b...tarea/U10-T03 \| grep -v -E '^(web/dashboard/\|tareas/U10-T03\|bitacoras/U10-T03\|revisiones/U10-T03/)' \| wc -l` | pasa: `0` y `0` |

## Barrido de clases CSS retiradas de `styles.css`
Las retiradas son `.verdict strong.badge`, `.phase`, `.phase-label`, `.phase-tag`, `.phase-hecha`, `.phase-actual`, `.finding-card`, `.details` y `.code-block`. `.findings-list` y `.flow-list` se redefinen, no se retiran.
- Las busqué en `src/**/*.tsx|ts`, excepto `pages/Run`: Inbox, Runs, Warm, Login, session y components no usan ninguna. Solo aparece `verdict` en `schema.gen.ts`, como campo de la API.
- Choque con T02 y T04: comparé los selectores nuevos de T03 con los del árbol de trabajo de cada una, y no hay selectores comunes. T02 no toca `styles.css`. T04 solo añade `.login-*` y `.btn-bloque`.
- Las ramas `tarea/U10-T02` y `tarea/U10-T04` no usan las clases retiradas.
- Conclusión: el retiro es seguro.

## Hallazgos
### F-01 · ROJO · CA-4 · Fidelidad sin evidencia entregada, aunque era verificable
La bitácora declara «no se intentó» y se apoya en que la vista exige sesión y no hay Playwright. Ninguna de las dos razones bloquea. Con un build servido y la API simulada con `page.route` por el flujo de login, las capturas salen sin escribir en el worktree. Con `playwright` global y Chromium en `~/.cache/ms-playwright` las hice yo. El CA permite capturas en la bitácora como sustituto de T05, y no están. Es un criterio no cumplido, no un bloqueo real.
Las hice por mi cuenta, en claro y oscuro, a 1280 y 375 px, para bug, sin-hallazgos, en curso y fallida; sin desbordamiento horizontal (`scrollWidth - clientWidth = 0` en las 16 combinaciones). Están en `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/65e5c329-227c-4891-af74-2c6042289597/scratchpad/shots/` como `impl-*` y `mock-*`.
Lo que vi:
- Cabecera, franja de metadatos, rejilla de dos columnas, tarjeta de post-mortem con `finding-item` y evidencias, plan con flujos plegados y las 9 fases con icono más texto coinciden con el mockup, tanto en claro como en oscuro.
- A 375 px la rejilla colapsa en una columna y la tabla de pasos queda legible.
- Con la corrida fallida se ve el banner rojo y la insignia «Detenida».
- Las diferencias que sí hay: el conmutador Demo y el banner de iniciar ejecución quedan fuera de alcance; «Ver request» no está (el cuerpo va en línea, y la bitácora ya lo anota como candidata); el estado esperado lleva insignia por familia, como pide la tarea. Estas no cuentan como defecto.

### F-02 · NARANJA · `web/dashboard/src/styles.css:897-926` (`dl dt` global y `dl.kv-list dt`) · Las etiquetas de la lista de pares salen en mayúsculas pequeñas; el mockup las pone en minúscula normal
En el mockup, los `dt` del hallazgo («Invariante violado», «Causa raíz (negocio)», «Evidencia (copiable)») van en tamaño de 13 px, peso 600 y sin transformar. En la vista, la regla global heredada `dl dt` (`0.625rem`, `text-transform: uppercase`, `letter-spacing: 0.06em`, `align-self: center`) no se anula en `dl.kv-list dt`, que solo fija color, peso y `padding-top`. Se ve como «INVARIANTE VIOLADO», más pequeño y centrado en vertical, en `impl-bug-light-1280.png` frente a `mock-bug-light-1280.png`, y también a 375 px y en oscuro.
`dl.kv-list` es una primitiva de T01 que sale sin uso real, y esta es su primera vista; por eso el desvío se manifiesta aquí. T04 (Entorno) usará la misma lista y heredará el mismo desvío. Hay que corregirlo en `dl.kv-list dt`, restableciendo `text-transform`, `letter-spacing`, `font-size` y `align-self`, y anotar el cambio en la bitácora, porque toca una primitiva de T01. Aviso de coordinación: T04 toca el mismo archivo.

### F-03 · AMARILLO · `web/dashboard/src/pages/Run/RunPage.tsx:~253` · Clases sin regla
`phase phase-${estado}` queda como gancho sin CSS tras retirar `.phase`, `.phase-hecha` y las demás. Es inocuo; las pruebas pueden depender de él. El aviso de veredicto dice «Bug» a secas, y el mockup dice «Veredicto: bug». La prueba «ningún elemento usa solo color» comprueba las fases, no las insignias de método y de esperado, que sí llevan texto.

## Tareas candidatas (fuera de alcance)
- Indicador de pliegue (chevron) propio en la tarjeta «Plan de flujos» y «Ver request» con modal: ya están en la bitácora; la captura no pide el chevron de la tarjeta.
- Revisar la `dl dt` global de `styles.css` y retirarla si el resto de vistas (Inbox usa `<dl>`) no la necesita así. Este es el origen de F-02.
- Convenir la insignia de veredicto de T02, T03 y T04 («Veredicto: bug» frente a «Bug»).

## Rutas de transcripciones largas
- Capturas y script: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/65e5c329-227c-4891-af74-2c6042289597/scratchpad/shots/` y `.../scratchpad/cap.mjs`
- Salida de vitest: `.../scratchpad/v.txt`

VEREDICTO: NO-VERDE
ROJO|CA-4|Fidelidad sin capturas en la bitácora aunque eran verificables (yo las hice: la vista es fiel salvo F-02)
NARANJA|web/dashboard/src/styles.css:897-926|`dl.kv-list dt` hereda mayúsculas, tamaño y alineación de `dl dt`; el mockup no
AMARILLO|web/dashboard/src/pages/Run/RunPage.tsx|clases `phase phase-*` sin CSS e insignia de veredicto sin prefijo «Veredicto:»
INFORME: revisiones/U10-T03/ronda-1.md

## Arbitraje del orquestador (añadido tras el informe)
- F-01 ROJO: se corrige en la ronda 2: capturas lado a lado (claro/oscuro, 1280 y 375 px) en `bitacoras/U10-T03-capturas/` y referencia en la bitácora.
- F-02 NARANJA: se corrige en T03, solo en el bloque `dl.kv-list dt` (cambio mínimo a la primitiva de T01, anotado). T04 no lo toca; hereda el arreglo al fusionar.
- F-03 AMARILLO: el texto «Veredicto: bug» se corrige; los ganchos `phase-*` se conservan si las pruebas los usan.
- Candidatas: chevron del plan, «Ver request» con modal, revisión de `dl dt` global y criterio común de insignia de veredicto: se registran para el humano.
