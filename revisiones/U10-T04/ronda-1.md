# Ronda 1 — U10-T04

VEREDICTO: NO-VERDE

Hay un hallazgo NARANJA que bloquea, sobre la fidelidad del enlace `run_id`. Los criterios CA-1, CA-2, CA-3 y CA-5 pasan en mi ejecución, y el alcance está limpio. CA-4 queda sin cumplir por ese enlace.

## Criterios de aceptación, verificados por mí
Todo se corrió sobre el sha 2ceae78 y con el worktree limpio antes y después (0 líneas en `git status --short`). Los artefactos de build y capturas están en el scratchpad, fuera del worktree. La suite se corrió con `--no-cache`.

| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | lint, typecheck y vitest sin fallos | `npm run -s lint && npm run -s typecheck && npx vitest run --no-cache \| grep -c -E 'failed\|FAIL'` | pasa: `0`; 19 ficheros, 205 pruebas, 6,8 s |
| 1b | El diff de pruebas solo cambia selectores | `git diff --numstat -- '*.test.tsx'` | pasa: 65, 35 y 74 líneas añadidas y 0 borradas |
| 2 | Pruebas de estructura de las tres vistas y de los 4 estados del warm | Leí los tests nuevos de Runs, Warm y Login | pasa: cubren `h1`/`h2` por rol y nombre, `dl.kv-list`, los 4 estados con texto más `svg[aria-hidden]`, y `alert-box` con variante |
| 3 | 375 px sin scroll horizontal | Copié `medir-u10t04.mjs` al scratchpad y lo corrí con Chromium de Playwright sobre un build de `vite build --outDir` en el scratchpad, con la CSP estricta | pasa: login, runs y warm con ancho de documento 375, `scrollHorizontal false`, sin errores de consola ni violaciones de CSP |
| 4 | Fidelidad por capturas contra el mockup | Capturas propias a 1280 px frente a `mockups/swarm-mock.html`, más estilos computados | **falla en parte**, ver F-01 |
| 5 | CSP y alcance | `grep -c -E 'unsafe-inline\|unsafe-eval' dist/index.html` y `grep -c 'style=' dist/index.html` sobre el build; archivos fuera de `web/dashboard/`, `tareas/U10-T04`, `bitacoras/U10-T04*` y `revisiones/U10-T04/` | pasa: 0, 0 y 0 archivos |

Evidencia del codificador: sus comandos de la bitácora coinciden con mis resultados. La bitácora declara el commit 4513b28 para el código, y el 2ceae78 es solo la bitácora.

## Hallazgos
### F-01 · NARANJA · `web/dashboard/src/pages/Runs/RunsPage.tsx` (enlace `run_id`) con `styles.css:54` · El enlace `run_id` no se ve como enlace
`<Link className="mono">` hereda `color: var(--text)` de la regla `.mono`, que gana al `a { color: var(--accent-text) }`. Medí los estilos computados en Chromium:
- Mockup: color `rgb(62,42,219)`, peso 700, subrayado.
- Dashboard: color `rgb(28,39,60)` (el mismo del texto), peso 500, sin subrayado.

El enlace es indistinguible del texto plano y la captura `1280-runs.png` lo confirma. El mockup lo pinta en acento y en negrita (`style="color:var(--accent-text); font-weight:700"`). Esto incumple CA-4 (fidelidad con el mockup de la columna principal) y la señal de enlace no depende solo del color (WCAG 1.4.1).

La bitácora declara «coincide» en Mis corridas, y eso es falso en este punto. La verificación fue a ojo, no por estilos computados. Falta además una prueba que fije que el enlace conserva su color de acento, por ejemplo una clase específica en `styles.css` que se pruebe por clase.

El arreglo es una clase aditiva, sin atributos `style` inline por la CSP.

## Dictamen de las decisiones declaradas
- **(a) `PageHeader`: compatible hacia atrás.**
  - `descripcion: string` pasa a `ReactNode`, que es un superconjunto.
  - `acciones` es opcional.
  - `hayMeta` solo cambia cuando se pasa `acciones`, así que un `PageHeader` con `meta` y sin `acciones` se pinta igual.
  - `componentes.test.tsx` pasa sin tocarlo.
  - Revisé las ramas `tarea/U10-T02` y `tarea/U10-T03`: ninguna modifica `components/`. T02 solo usa `PageHeader` y T03 no. No hay conflicto semántico.
  - `styles.css` sí es tocado por T03 (+213/-57), pero T04 es aditivo por bloque. Cualquier conflicto textual lo resuelve el orquestador al fusionar.
- **(b) `kv-list` frente a `kpi-strip`: aceptable.** La tarea ordena explícitamente `kv-list` en «Alcance dentro». La divergencia con el mockup (y la insignia en `card-head`) está anotada en el acuse y en las candidatas. La insignia con icono más texto cumple lo pedido. No la cuento como hallazgo.
- **(c) Título «Entorno» sin namespace: correcto.** La API no entrega el namespace y la tarea prohíbe mostrar datos que no entrega.
- **(d) `styles.css` solo aditivo: confirmado.** `git diff` no tiene ninguna línea borrada en `styles.css` y todas las clases nuevas están en español. Una regla es global: `th code, dl.kv-list dt code, .login-texto code` fuerza `text-transform: none`. Es coherente con el mockup, pero afecta a otras vistas que usen `<code>` en `th` (ver AMARILLO).
- **(e) PNGs en `bitacoras/`: dentro del alcance**, porque el prefijo `bitacoras/U10-T04*` está permitido. Pesan ~250 KB en total, no hay problema.

## Tareas candidatas (fuera de alcance)
- Las tres de la bitácora son razonables: componente demo para «Simular estado» y para los estados de error del login, `kpi-strip` en el warm y formato de hora del mockup.
- AMARILLO: `medir-u10t04.mjs` tiene rutas absolutas incrustadas (`~/.npm/_npx/e41f203b7505f1fb/...` y `chromium-1234`), así que no es reproducible en otro equipo. T05 lo sustituye por `e2e:fidelidad`.
- AMARILLO: la regla global `th code` merece una nota para T02/T03, por si afecta a sus tablas.
- AMARILLO: el 375 solo se midió en las tres vistas con datos felices. No se midió el warm en `dirty` ni el login con error 429 o 503 en móvil. No es un criterio de la tarea.

## Resumen
CA-1, CA-2, CA-3 y CA-5 pasan en mi ejecución y el alcance está limpio. CA-4 no se puede dar por cumplido hasta cerrar F-01.

VEREDICTO: NO-VERDE
NARANJA|web/dashboard/src/pages/Runs/RunsPage.tsx (enlace run_id) + styles.css:54 (.mono)|El enlace run_id hereda color de texto, sin subrayado ni negrita: no se ve como enlace y difiere del mockup (CA-4)
INFORME: revisiones/U10-T04/ronda-1.md

## Arbitraje del orquestador (añadido tras el informe)
- F-01 NARANJA: se corrige en la ronda 2 con una clase aditiva y una prueba que la fije; verificación por estilos computados, no a ojo.
- AMARILLO regla global `th code`: se acota a las vistas de T04 (T02 pondrá el evento como chip `code` en su tabla y no debe heredar efectos de otra tarea).
- AMARILLO rutas absolutas del script: aceptado; T05 lo sustituye. AMARILLO 375 en estados de error: no es criterio; sin cambio.
- Candidatas de la bitácora (componente demo, `kpi-strip` en el warm, formato de hora): se registran para el humano.
