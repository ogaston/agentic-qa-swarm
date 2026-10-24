# Ronda 2 — U10-T02

VEREDICTO: NO-VERDE

## Criterios de aceptación, verificados por mí (HEAD 85869f4)
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Comportamiento intacto | `cd web/dashboard; npm run -s lint; npm run -s typecheck; npx vitest run \| tail; npx vitest run 2>&1 \| grep -c -E 'failed\|FAIL'` | lint 0, typecheck 0, 19 files / 195 tests pasan, conteo `0`. Diff de `*.test.tsx`: selectores, pruebas nuevas de CA-2 y la sonda de session.test.tsx (F-03 aceptado por el orquestador). Pasa. |
| 2 | Estructura | incluida en `vitest run` (bloques «estructura del mockup» y «diálogo modal») | pasa. La aserción de contadores se ajustó a la regla de la última carga. |
| 3 | Fidelidad (T05 no existe) | Chromium headless (chromium-1234), `dist` recién construido servido localmente, API simulada con `page.route`; 1280 y 375 px, claro y oscuro, contra `mockups/swarm-mock.html`. Script: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/65e5c329-227c-4891-af74-2c6042289597/scratchpad/r2inbox/cap.mjs` (el `cap.mjs` que citaste ya no era el del inbox; escribí uno nuevo) | **pasa salvo F-01-R2** (ver abajo). |
| 4 | CSP y alcance | `npm run -s build; echo $?`; `grep -c -E 'unsafe-inline\|unsafe-eval' dist/index.html`; `git diff --name-only cbf8a6b..HEAD` | build 0, conteo `0`. Diff solo en `web/dashboard/**`, `bitacoras/U10-T02.md` y el informe de la ronda 1. Pasa. |

Worktree limpio tras mi revisión (`git status --short` vacío; `dist` está ignorado).

## Verificación de los hallazgos de la ronda 1
- F-01 (ROJO, cabecera de la tarjeta pegada al borde): **cerrado**. Medí la distancia del título «Listado de notificaciones» al borde de la tarjeta: app 23 px en los 4 casos (1280/375, claro/oscuro), igual que el mockup (23 px). Las capturas confirman el relleno. Sin desbordamiento horizontal (overflowX = 0) en los 4 casos.
- F-02 (b), contadores obsoletos: **cerrado**. `cargar` hace `setConteos({ [porEstado]: n })` y `setConteos({})` si falla. En vivo, tras cargar solo Pendientes, las pestañas son `["Pendientes (3)","Confirmadas","Rechazadas"]`. Respeto el arbitraje: F-02 (a) y (c) quedan fuera de alcance como C-133 y C-134, y no los cuento como defecto.
- F-03: aceptado por el orquestador, sin cambio.
- F-05: **cerrado**. Evento en `<code>`, artefacto en `span.mono`, KPI «Corridas confirmadas». Se ve en las capturas, también en oscuro y a 375 px.
- F-04 (separación del botón en «Detalle»): **no cerrado**, ver F-01-R2.

## Hallazgos
### F-01-R2 · NARANJA · `InboxPage.tsx:~404-410` (bloque «Detalle») · El botón «Revisar y confirmar» sigue pegado a la lista de datos
El arbitraje de la ronda 1 dijo «la falta de separación del botón en “Detalle” se corrige». La bitácora afirma que ahora va «dentro de un `<p>`, separado de la `dl.kv-list`». Lo medí en el navegador (`/inbox` con una notificación pendiente → «Ver detalle»): `{ gap: 0, pTag: 'P', marginTop: '0px', marginBottom: '8px', dl marginBottom: '0px' }`. La separación entre el borde inferior de la `dl` y el borde superior del botón es **0 px**. El `<p>` solo añade margen inferior, así que el cambio no produjo ningún efecto visible. En la captura `r2inbox/app-modal-1280-light.png` el botón sigue tocando la fila «ESTADO / pending». La afirmación de la bitácora está sin verificar y es falsa en medición. Es un defecto visible dentro del alcance, sin cambiar ningún criterio. Solución: dar margen superior al `<p>` (clase de utilidad existente o `.mt-*` del CSS del proyecto) o envolver el botón en un contenedor con separación, y medir gap > 0 en navegador. Mínimo, no afecta a pruebas.

### F-02-R2 · AMARILLO · `InboxPage.tsx:73-79` · El contador de la pestaña anterior sigue visible mientras carga la nueva
Al cambiar de pestaña, `conteos` conserva el valor de la pestaña previa hasta que llega la respuesta nueva (la lista se pone en `null`, el contador no). Es transitorio y no bloquea. Mismo caso en la ventana entre un 409/404 y su recarga. Solo informo.

## Tareas candidatas
- C-133 (conteo por estado en la API) y C-134 (prop de insignia en `Kpi`): ya registradas por el orquestador.
- Falta en el inbox la insignia de apoyo y el botón primario directo «Revisar y confirmar →» por fila: D-3 aceptado por el orquestador, sin acción.

## Rutas de capturas y scripts
- Capturas: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/65e5c329-227c-4891-af74-2c6042289597/scratchpad/r2inbox/` (`app-{1280,375}-{light,dark}.png`, `mock-{1280,375}-{light,dark}.png`, `app-modal-1280-light.png`)
- Script de capturas: `.../scratchpad/r2inbox/cap.mjs`; medición del hueco del botón: `.../scratchpad/r2inbox/det.mjs`

VEREDICTO: NO-VERDE
NARANJA|InboxPage.tsx:~404-410 (bloque «Detalle») / arbitraje F-04|El botón «Revisar y confirmar» sigue a 0 px de la lista de datos; la afirmación de la bitácora («separado de la dl») es falsa en medición
INFORME: revisiones/U10-T02/ronda-2.md

## Arbitraje del orquestador (añadido tras el informe)
- F-01-R2 NARANJA: se corrige en la ronda 3 (última según el tope de 3 rondas); separación medida en navegador > 0 y anotada en la bitácora.
- F-02-R2 AMARILLO: se corrige si es trivial (limpiar el contador al iniciar la carga de otra pestaña o la recarga); si no, queda como nota.
