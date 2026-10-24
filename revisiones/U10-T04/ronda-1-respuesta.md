# U10-T04 — respuesta a la ronda 1

F-01 (NARANJA, enlace `run_id`): Corregido en la ronda 2. `RunsPage.tsx` usa la clase `enlace-corrida`, con `color: var(--accent-text)`, `font-weight: 700` y `text-decoration: underline` en `styles.css`. Medido con Chromium: el enlace del dashboard y el del mockup dan `rgb(62, 42, 219)`, peso 700 y subrayado. Prueba: `src/pages/Runs/RunsPage.test.tsx`, «el enlace run_id usa la clase de enlace de corrida…». La bitácora corrige su afirmación «coincide».

F-02 (AMARILLO, regla global `th code`): Corregido en la ronda 2. La regla global se elimina. Ahora se aplica solo a los elementos marcados con `campo-api` (cabeceras de Runs y etiquetas de Entorno). Prueba: «ninguna regla global de la hoja apunta a th code ni a dt code».

AMARILLO (rutas absolutas del script) y AMARILLO (375 en estados de error): sin cambio, según el arbitraje del orquestador.

Verificación de la ronda 2: lint y typecheck en 0; `npx vitest run` con 207 pruebas en verde; build con 0 `unsafe-inline`/`unsafe-eval`; 375 px sin scroll horizontal.
