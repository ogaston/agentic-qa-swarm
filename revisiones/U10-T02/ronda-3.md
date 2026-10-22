# Ronda 3 — U10-T02

VEREDICTO: VERDE

## Criterios de aceptación, verificados por mí (HEAD 51853fb)
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Comportamiento intacto | `cd web/dashboard; npm run -s lint; npm run -s typecheck; npx vitest run --no-cache \| tail; npx vitest run 2>&1 \| grep -c -E 'failed\|FAIL'` | lint 0, typecheck 0, 19 archivos / 195 pruebas pasan, conteo `0`. El diff de `*.test.tsx` es el ya arbitrado: selectores, pruebas de CA-2 y la sonda de session.test.tsx. Pasa. |
| 2 | Estructura | incluida en `vitest run` (bloques «estructura del mockup» y «diálogo modal») | pasa |
| 3 | Fidelidad (T05 no existe) | Chromium headless (chromium-1234) sobre `dist` recién construido, API simulada con `page.route`. Ejecuté `cap.mjs` (copia en `r3inbox/`): 1280 y 375 px, claro y oscuro, contra `mockups/swarm-mock.html` | pasa. Distancia título→borde de la tarjeta: 23 px en app y mock, en los 4 casos. Desbordamiento horizontal 0 en los 4. Las capturas de 1280 claro, 1280 oscuro, 375 oscuro y el diálogo coinciden con el mockup en estructura. Las diferencias restantes son las ya arbitradas (D-3, C-133, C-134). |
| 4 | CSP y alcance | `npm run -s build; echo $?`; `grep -c -E 'unsafe-inline\|unsafe-eval' dist/index.html`; `git diff --name-only cbf8a6b HEAD` | build 0, conteo `0`. El diff queda en `web/dashboard/**`, `bitacoras/U10-T02.md` y `revisiones/U10-T02/*`. Pasa. |

## Verificación de los hallazgos de la ronda 2
- **F-01-R2 (NARANJA, hueco dl→botón): cerrado.** Lo medí yo con `det.mjs`, que ya usé en la ronda 2, y con una copia a 375 px. Resultado: `{gap: 18, pTag: 'DIV', mt: '0px', mb: '0px', dlMb: '0px'}` a 1280 y a 375 px. En la ronda 2 el hueco era 0 px. El código usa `div.stack` (clase existente, `gap: 18px` en `styles.css:562`) sin estilo inline. La captura `r3inbox/app-modal-1280-light.png` muestra el botón «Revisar y confirmar» separado de la fila «ESTADO / pending».
- **F-02-R2 (AMARILLO, contador anterior): cerrado.** Escribí `r3inbox/cnt.mjs`, que retrasa 1,5 s la respuesta de `/api/notifications` y lee las pestañas.
  - Inicial: `Pendientes (1), Confirmadas, Rechazadas`.
  - Durante la carga de Confirmadas: `Pendientes, Confirmadas, Rechazadas`. Ya no queda el «(1)» anterior.
  - Tras la carga: `Pendientes, Confirmadas (0), Rechazadas`.
  - Durante «Actualizar»: sin contadores; tras «Actualizar»: solo el de la pestaña vigente.
  - `setConteos({})` al empezar `cargar` en `InboxPage.tsx:~71`.

## Fusión
- `git merge-tree --write-tree tarea/U10-T02 tarea/U10-T03` → árbol `84970b71…`, salida 0, sin conflictos.
- `git merge-tree --write-tree tarea/U10-T02 tarea/U10-T04` → árbol `d06eb7e4…`, salida 0, sin conflictos.

## Hallazgos
Ninguno ROJO ni NARANJA en pie. Ninguno AMARILLO nuevo.

Sin cambios respecto a rondas previas, y ya arbitrados por el orquestador:
- C-133: conteo por estado en la API.
- C-134: prop de insignia en `Kpi`.
- D-3: sin botón primario directo ni insignia de apoyo por fila.

Worktree limpio tras mi revisión: `git status --short` vacío, `dist` ignorado.

## Rutas
- Capturas (app y mock, 1280/375, claro/oscuro, más el modal): `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/65e5c329-227c-4891-af74-2c6042289597/scratchpad/r3inbox/`
- Scripts: `.../scratchpad/r3inbox/cap.mjs`, `.../scratchpad/r3inbox/cnt.mjs`, `.../scratchpad/r3inbox/det375.mjs`, y `.../scratchpad/r2inbox/det.mjs` para 1280 px.

VEREDICTO: VERDE
INFORME: revisiones/U10-T02/ronda-3.md
