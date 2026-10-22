# Ronda 2 — U10-T04

VEREDICTO: VERDE

Todo se corrió sobre a829f0e, con el worktree limpio antes y después (`git status --short` vacío). Build y capturas fueron al scratchpad, fuera del worktree.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | lint, typecheck y vitest sin fallos | `npm run -s lint && npm run -s typecheck && npx vitest run --no-cache \| grep -c -E 'failed\|FAIL'` | pasa: `0`; 19 ficheros, 207 pruebas, 7,2 s |
| 1b | El diff de pruebas solo cambia selectores | `git diff --numstat cbf8a6b..HEAD -- '*.test.tsx'` | pasa: 65, 66 y 74 líneas añadidas, 0 borradas |
| 2 | Estructura de las tres vistas y los 4 estados del warm | Leí los tests; no cambiaron en la ronda 2 salvo las 2 pruebas nuevas | pasa |
| 3 | 375 px sin scroll horizontal | `vite build --outDir` al scratchpad; copia del `medir-u10t04.mjs` corrida con Chromium (`~/.cache/ms-playwright/chromium-1234`) bajo la CSP estricta | pasa: login, runs y warm con ancho de documento 375, `scrollHorizontal false`, `errores: []` |
| 4 | Fidelidad frente al mockup | Mismo script (estilos computados) más captura propia `1280-runs.png` frente a `mockup-1280-runs.png` | pasa; ver el cierre de F-01 |
| 5 | CSP y alcance | `grep -c -E 'unsafe-inline\|unsafe-eval' dist/index.html` y `grep -c 'style=' dist/index.html`; `git diff --name-only cbf8a6b..HEAD` filtrado por las rutas permitidas | pasa: 0, 0 y 0 archivos fuera de alcance |

Evidencia del codificador: la bitácora y `ronda-1-respuesta.md` declaran 207 pruebas en verde y el mismo medido del enlace. Mi ejecución fresca coincide. Su `medicion.json` es del build de esta ronda.

## Cierre de hallazgos de la ronda 1
### F-01 (NARANJA, enlace `run_id`): cerrado
- Estilos computados del enlace en Chromium, con la corrida de hoy:
  - Dashboard (1280 px): `rgb(62, 42, 219)`, peso 700, `underline`.
  - Mockup: `rgb(62, 42, 219)`, peso 700, `underline`.
  - Antes (ronda 1): `rgb(28,39,60)`, peso 500, sin subrayado.
- La clase `.enlace-corrida` está en `styles.css:878`, después de `.mono` (línea 54), así que gana la cascada. No hay `style=` inline.
- La captura `1280-runs.png` muestra el enlace en acento, negrita y subrayado, igual que el mockup.
- La prueba nueva lee la hoja y exige la clase, el color de acento, el peso 700 y el subrayado. Es una prueba de la hoja, no de píxeles. El resultado computado lo cubre mi medición. La bitácora tiene el rojo previo documentado (la prueba falló antes del cambio, también por el subrayado).
- La afirmación «coincide» de la ronda 1 quedó corregida en la bitácora.

### F-02 (AMARILLO, regla global `th code`): cerrado
- La regla global se eliminó. Ahora el selector es `.campo-api, .login-texto code`.
- `campo-api` está en las cabeceras de `RunsPage` y en las etiquetas `dt` de `WarmPage`. En el `grep` de `styles.css` ya no hay `th code` ni `dt code`.
- La captura de Mis corridas confirma que `run_id` y `notification_id` en las cabeceras conservan su grafía en minúscula.

## Juicio sobre la prueba «ninguna regla global apunta a th code ni dt code»
Es razonable y no choca con lo que hay en T02 ni en T03.
- **Regex:** `/(^|[\s,])th code\s*[,{]/` y `/(^|[\s,])dl\.kv-list dt code\s*[,{]/`. Ninguna coincide con `dl.kv-list dt {`, que es la regla que T03 corrige.
- **Reglas de código en las ramas hermanas:**
  - T02 y T03 solo tienen `.data-table td code`, `code {`, `.code-block` y `.phase-code`. Ninguna casa con los regex.
  - Hice `git merge-tree --write-tree HEAD tarea/U10-T02` y lo mismo con `tarea/U10-T03`. Ambas fusiones salen limpias, sin conflictos.
- **Riesgo residual (AMARILLO):** el regex es más amplio de lo que dice el nombre. También casaría `.data-table th code {` o `.card dl.kv-list dt code {`, es decir, reglas legítimas y acotadas. Si T02, T03 o T05 añaden una así, la prueba fallará por un motivo ajeno a la regresión que vigila. Conviene que el orquestador lo sepa al fusionar. Una redacción más estricta sería comprobar solo el selector exacto desnudo (`^th code` / `^dl.kv-list dt code`).

## Hallazgos en pie
### F-03 · AMARILLO · `RunsPage.test.tsx` (prueba `th code`) · regex más amplio que su nombre
Casaría reglas acotadas legítimas (ver arriba). No bloquea.

## Tareas candidatas (fuera de alcance)
- Las del arbitraje siguen vigentes y no las repito:
  - componente demo para «Simular estado» y para los estados de error del login;
  - `kpi-strip` en el warm;
  - formato de hora de «Mis corridas». La captura la muestra como `10/21/2026, 3:41:12 PM`, frente a `21/10/2026, 20:41:12` del mockup.
- `medir-u10t04.mjs` tiene rutas absolutas incrustadas. Es AMARILLO aceptado, T05 lo sustituye.

Rutas relevantes:
- `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U10-T04/web/dashboard/src/styles.css`
- `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U10-T04/web/dashboard/src/pages/Runs/RunsPage.test.tsx`
- Capturas propias, en el scratchpad: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/65e5c329-227c-4891-af74-2c6042289597/scratchpad/out/`

VEREDICTO: VERDE
AMARILLO|web/dashboard/src/pages/Runs/RunsPage.test.tsx (prueba th code)|El regex de la prueba es más amplio que su nombre y podría casar reglas acotadas legítimas de T02/T03/T05
INFORME: revisiones/U10-T04/ronda-2.md
