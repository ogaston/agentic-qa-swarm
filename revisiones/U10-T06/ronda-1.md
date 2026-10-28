# Ronda 1 — U10-T06

VEREDICTO: NO-VERDE

## Verificaciones previas
- Hash de la tarea: `d414ebb…68ce` — **coincide** con el esperado. Worktree HEAD `42f4ec7`, limpio al inicio y al final (`git status --short` vacío).
- Ejecución en copia limpia: `git archive HEAD | tar -x -C /tmp/u10t06-rev` + `node_modules` enlazado del worktree. Sin `npm ci`, sin navegadores descargados. Nada se escribió en el worktree.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| CA-1 | 20 de 20, exit 0 | `npm run -s e2e:fidelidad; echo $?` | **falla**: «10 de 20 combinaciones OK (máx. 17.10 %)», exit 1 |
| CA-2 | lint+typecheck+check:api+vitest sin fallos | los cuatro comandos, frescos | pasa: exit 0 ×3; vitest 225/225 (20 archivos), `grep -c 'failed\|FAIL'` → 0 |
| CA-3 | compuerta no debilitada | `git diff -U0 173c245 -- …/e2e-fidelidad.mjs \| … \| grep -c -E 'TOLERANCIA\|UMBRAL\|compar\|VISTAS'` | pasa: 0. Además leí el diff completo: solo datos servidos (`FIJOS`), preparación del mockup (`OCULTAR_MOCK`, `CSS_DESBORDE_MOCK`) y campo `neutralizaciones` del informe |
| CA-4 | CSP intacta | `npm run -s build >/dev/null && grep -c -E 'unsafe-inline\|unsafe-eval' dist/index.html` | pasa: 0 |
| CA-5 | alcance | `git diff --name-only 173c245 \| grep -v -E '…' \| wc -l` | pasa: 0 (todo bajo `web/dashboard/`, `DESIGN.md`, `unidades-y-tareas.md`, `tareas/`, `bitacoras/`) |
| (texto) | `--control` sigue en 0 | `npm run -s e2e:fidelidad -- --control; echo $?` | pasa: «20 de 20 combinaciones OK (0.00 %)», exit 0 |

Las 10 FALLA de CA-1 reproducen exactamente la bitácora: inbox ×4 (6.70/15.91/6.72/16.82 %), run ×4 (6.18/16.06/6.70/17.10 %), warm-375 ×2 (6.30/6.98 %). Neutralizaciones registradas por combinación en `informe.json` (campo `neutralizaciones`, 3 ids demo + 2 ids desborde 375) — requisito de la tarea cumplido.

## Hallazgos

### F-01 · ROJO · CA-1 · La compuerta sigue en rojo: 10 de 20, exit 1
El criterio exige exit 0 y «20 de 20». Mi ejecución fresca da exit 1 con 10 combinaciones por encima del umbral de píxeles (5 %). El propio codificador lo declara NO CUMPLIDO. Bloquea la fusión; las candidatas para el humano son la vía correcta, pero el CA no se cumple.

### F-02 · NARANJA · `web/dashboard/src/pages/Warm/WarmPage.tsx:76` y `:116` · Literales incrustados contra la regla de datos de la tarea
Dos valores nuevos visibles como literal: (1) la descripción fija `aqs-test` en un `<span class="mono">` — ese dato **sí existe** (`data.warm_id`, interpolado en el título de la línea 113), así que la descripción mentirá en cuanto el warm_id real difiera del fixture; (2) el subtítulo fijo «Pre-desplegado (App + Postgres + Redis)…», composición sin equivalente en el contrato `WarmState`. La tarea (líneas 22–24) ordena que la «descripción del entorno» se reporte como candidata «**sin texto fijo inventado en la app**», y la decisión (c) del humano condiciona la copia del mockup a «cuando el dato real lo permite». El codificador la tenía como candidata 8 y la resolvió incrustándola (commit `c3dde9f`) invocando (c), que no la cubre. Invariante 8 (literal incrustado → NARANJA mínimo); además contradice una instrucción explícita de la tarea. Decisión para el humano: revertir el literal, o añadir el dato al contrato.

### F-03 · NARANJA · bitácora cont. 5 (línea 537) · La conclusión «el residuo no es CSS ni copia» está sobre-vendida
La afirmación de inalcanzabilidad no está verificada en dos frentes que el propio codificador deja abiertos: (1) **inbox-375** (~15,9 %): los aportes de datos medidos suman ≈4,3 pp («Nueva aplicación» −0,41, «Familias activas» −2,74, contadores −1,16); quedan ≈11 pp con «No lo localicé a un elemento. Pendiente.»; (2) **run-1280** (~5,2 % tras datos): «sin causa única identificada» y la sospecha badge-dot queda «no verificado con sonda». Mientras esos residuos no tengan causa localizada, no se puede certificar que no esconden CSS/marcado corregible en alcance. Invariante 3: afirmación sin evidencia completa.

## ¿Sostiene la evidencia la afirmación de inalcanzabilidad?
**Parcialmente.** Verifiqué contra el mockup y la app que las causas **medidas** son genuinas y fuera del alcance permitido: «Nueva aplicación» es un `<a href="onboarding-mock.html">` (`swarm-mock.html:1158-1161`, feature inexistente; `InboxPage.test.tsx:437-438` fija su ausencia); «Familias Activas: happy-path» (`:1138-1139`), «Auto-ejecución sin humano» (`:1196`), «sí (100%)» (`:1135`) no tienen dato en la API (tests los fijan como ausentes); el identificador `env-aqs-test-01` choca con un contrato de un solo campo; neutralizar esos elementos en el mockup no está autorizado (la tarea solo permite ocultar demos y el desborde a 375). En cambio: (a) quedan ≈11 pp (inbox-375) y ≈5,2 % (run-1280) **sin causa localizada** — puede haber CSS corregible ahí; (b) la candidata 10 (`badge-dot` vs iconos SVG) es marcado/CSS de vistas — literalmente dentro del alcance escrito, aunque es una decisión de sistema de diseño (DESIGN.md:406 avala SVG) y su aporte no está medido; (c) F-02 muestra que una «candidata de datos» se resolvió incrustando el literal, lo que contamina el argumento. Conclusión: el grueso del residuo medido sí requiere al humano, pero la certificación «nada corregible en alcance» es prematura.

## Tareas candidatas (defectos reales fuera de alcance)
- Enlace «Nueva aplicación» del mockup: feature/ruta inexistente en la app (decisión de producto).
- Datos sin equivalente: «Familias activas», «Auto-ejecución sin humano», contadores de pestañas (exige cargar 3 listas, contradice la carga perezosa de U10-T02), identificador `env-aqs-test-01` (contrato `WarmState` con un solo campo), subtítulo «Generado por agent-reporter…», causa raíz en `<code>`, copia «Completada (done)».
- Decisión de diseño transversal: `badge-dot` del mockup frente a iconos SVG de la app (aporte sin medir).
- Diagnóstico pendiente: localizar la causa de los ≈11 pp de inbox-375 y el ≈5,2 % de run-1280; hasta cerrarlo no puede afirmarse que el residuo es solo datos.

## Rutas de transcripciones largas
- `/tmp/u10t06-rev/ca1.log` — CA-1 completo (10 de 20, exit 1)
- `/tmp/u10t06-rev/ctrl.log` — `--control` (20 de 20, 0.00 %, exit 0)
- `/tmp/u10t06-rev/bitacoras/U10-T05/informe.json` — informe JSON con `neutralizaciones` por combinación

VEREDICTO: NO-VERDE
ROJO|CA-1|La compuerta sigue en rojo: 10 de 20, exit 1
NARANJA|web/dashboard/src/pages/Warm/WarmPage.tsx:76|Literales incrustados (aqs-test y «App + Postgres + Redis») contra la regla de datos de la tarea
NARANJA|bitacoras/U10-T06.md cont. 5|«El residuo no es CSS ni copia» sobre-vendido: ~11 pp (inbox-375) y ~5,2 % (run-1280) sin causa localizada
