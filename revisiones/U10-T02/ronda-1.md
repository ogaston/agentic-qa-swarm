# Ronda 1 — U10-T02

VEREDICTO: NO-VERDE

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1a | lint + typecheck + suite sin fallos | `cd web/dashboard; npm run -s lint; npm run -s typecheck; npx vitest run 2>&1 \| grep -c -E 'failed\|FAIL'` | lint 0, typecheck 0, 19 files / 195 tests pasan, conteo `0`. Pasa. |
| 1b | el diff de `*.test.tsx` solo cambia selectores | `git diff cbf8a6b...tarea/U10-T02 -- '*.test.tsx'` | **No literal**: `session.test.tsx` cambia la ruta de la sonda (ver F-03). El resto son selectores y pruebas nuevas de CA-2. |
| 2 | prueba de estructura (encabezado, tablist 3 pestañas con contadores, 4 KPI, tabla, «Actualizar», sin «Nueva aplicación») | `npx vitest run` (incluye los bloques «estructura del mockup» y «diálogo modal») | pasa (28/28 en Inbox). Ver F-02 sobre «contadores». |
| 3 | fidelidad (T05 no existe; comparo capturas) | Chromium headless con playwright-core de la caché npx, `dist` servido localmente, API simulada con `page.route`; 1280 y 375 px, claro y oscuro, contra `mockups/swarm-mock.html`. Script: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/65e5c329-227c-4891-af74-2c6042289597/scratchpad/cap.mjs` | **falla**: F-01 y F-02. |
| 4 | CSP y alcance | `npm run -s build >/dev/null; echo $?` y `grep -c -E 'unsafe-inline\|unsafe-eval' dist/index.html`; `git diff --name-only` | build 0, conteo `0`. Diff solo en `web/dashboard/**` y `bitacoras/U10-T02.md`. Pasa. |

El worktree quedó limpio (`git status --short` vacío; `dist` está ignorado).

Capturas (todas en `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/65e5c329-227c-4891-af74-2c6042289597/scratchpad/cap/`): `app-{1280,375}-{light,dark}.png`, `mock-{1280,375}-{light,dark}.png`, `app-modal-1280-light.png`. En consola: sin desbordamiento horizontal (scrollW = ancho de ventana) en los 4 casos de la app.

Lo que sí es fiel: cabecera con franja de metadatos solo con datos reales, `az-tablist` con roles y flechas, tarjeta con tabla e insignias, «Nueva aplicación» ausente, tema oscuro coherente. A 375 px la tabla se apila en tarjetas sin desbordar, mejor que el propio mockup, que desborda horizontalmente. El foco inicial del diálogo cae en el h2 «Confirmar corrida» y las pruebas de Tab, Esc y retorno de foco pasan.

## Hallazgos
### F-01 · ROJO · `InboxPage.tsx:312-316` (`<Card … pad={false}>`) + `Card.tsx` + `styles.css:486` · La cabecera de la tarjeta del listado queda pegada al borde
Con `pad={false}`, `.card-head` no recibe relleno. En las capturas (1280 y 375, claro y oscuro) «Listado de notificaciones» y su subtítulo tocan el borde izquierdo de la tarjeta. En el mockup (líneas ~1196-1200) van dentro de `.card-pad` con relleno de 22 px, y el título queda a unos 22 px del borde. Es un defecto visible en todas las vistas, y el codificador declaró que no había medido nada en navegador. CA-3 no se cumple. La corrección está dentro de `web/dashboard/**`: envolver la cabecera en el relleno del mockup, o que `Card` aplique `.card-pad` a la cabecera cuando `pad={false}`. `Card` es de T01; si se toca, hay que mantener `Card.test.tsx` en verde. `pad={false}` tiene un solo uso en el repo, así que el efecto sobre otras vistas es nulo.

### F-02 · NARANJA · CA-2/CA-3, bitácora D-1 · Contadores de pestañas y KPI vacíos al montar, y KPI sin la insignia del mockup
- Al montar solo se pide `?state=pending`. «Confirmadas» y «Rechazadas» salen sin «(n)», y dos de los cuatro KPI muestran «—» (captura 1280 claro). El mockup muestra `(3)/(2)/(0)` y valores reales.
- Es un conflicto real entre especificaciones, no pereza. Pedir las tres listas al montar rompería `reg.listas` en `InboxPage.test.tsx` (`toEqual(['pending'])` en las líneas 139, 166 y 179-184), y CA-1 exige que esas pruebas cambien solo en selectores. Mi juicio sobre D-1: el desvío es honesto y está documentado, pero deja CA-2 («contadores») cumplido solo a medias, y no lo puede resolver el codificador. Pide una decisión del orquestador o del humano: autorizar pedidos extra para los conteos y ajustar esas aserciones, o aceptar el hueco y aplazarlo a un endpoint de conteo.
- Efecto añadido: los contadores de las pestañas ya visitadas quedan obsoletos. «Actualizar» y las recargas tras 409/404 solo refrescan la pestaña activa. Un número «(3)» puede ser falso sin que nada lo avise.
- Los KPI no llevan la insignia de apoyo del mockup («esperando confirm», «ready»). El componente `Kpi` de T01 no tiene prop para ella. La insignia «ready» se podría derivar de `warm.state`, que es un dato real. Las barras del mockup (60 %, 40 %) no se derivan de ningún dato y hacen bien en omitirse. D-2 (omitir «Familias activas») y el KPI «Rechazadas» en lugar de «Auto-ejecución = 0» son correctos según el alcance: acepto ambos.

### F-03 · NARANJA · `session.test.tsx:86-98,178-187` · El diff de pruebas no es solo de selectores
La sonda de 401 pasa de `/api/warm` a `/api/protegida`. La justificación es plausible: el inbox ahora pide `/api/warm` al montar y el 401 de la sonda caía antes de tiempo. Las aserciones no cambian. Aun así, CA-1 dice textualmente «solo cambia selectores», y esto es un cambio de fixture en un archivo ajeno al inbox, sin autorización explícita en la tarea. El codificador lo declara en la bitácora, pero no es suyo decidir que una excepción a CA-1 es aceptable. Lo mismo vale para el handler por defecto `GET /api/warm → 503` en `src/test/server.ts`, que no es `*.test.tsx` y es razonable. Pide aceptación del orquestador o del humano; no exige cambiar código.

### F-04 · AMARILLO · `InboxPage.tsx:~330-370` (D-3) · Fila con «Ver detalle» y bloque «Detalle» heredado
El mockup ofrece «Revisar y confirmar →» primario directo en la fila. Aquí hay «Ver detalle» secundario, luego un bloque «Detalle» con `card card-pad detail` y, de ahí, el diálogo. En la captura del modal el bloque «Detalle» sigue visible detrás. Entiendo que las pruebas de comportamiento fuerzan este flujo (CA-1), así que juzgo D-3 aceptable bajo la tarea. Es una desviación de fidelidad que conviene decidir con el humano. Además, en el bloque «Detalle» el botón queda pegado a la lista de datos sin separación (captura del modal).

### F-05 · AMARILLO · tabla del listado · Tipografía de celdas distinta al mockup
El mockup pone el evento como chip `code` (`commit`, `pull_request`) y el artefacto en mono. La app muestra el evento en texto plano y el artefacto en texto normal, con la insignia de estado correcta. El KPI «Confirmadas» difiere de «Corridas confirmadas» del mockup. No bloquea.

## Tareas candidatas (fuera de alcance)
- Endpoint o parámetro de conteo por estado, para que las tres pestañas y los KPI muestren «(n)» al montar sin pedir tres listas (ya lo anota el codificador como D-1).
- Revisar si `Card` debe aplicar relleno a su cabecera cuando `pad={false}`, o sumar una prop de cabecera. Afecta a T03/T04 si estas lo usan después.

## Rutas de transcripciones largas
- Script de capturas: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/65e5c329-227c-4891-af74-2c6042289597/scratchpad/cap.mjs`
- Capturas: carpeta `.../scratchpad/cap/` (ver arriba).

VEREDICTO: NO-VERDE
ROJO|InboxPage.tsx:312-316 (Card pad={false}) / CA-3|Cabecera de la tarjeta del listado pegada al borde (sin relleno), visible en 1280 y 375, claro y oscuro
NARANJA|CA-2/CA-3 + bitácora D-1|Contadores de pestañas y KPI vacíos al montar (conflicto con pruebas de CA-1, requiere decisión); contadores obsoletos tras Actualizar; KPI sin insignia de apoyo
NARANJA|session.test.tsx:86-98,178-187 / CA-1|El diff de pruebas no es solo de selectores (sonda cambiada de /api/warm a /api/protegida); requiere aceptación explícita

## Arbitraje del orquestador (añadido tras el informe)
- F-01 ROJO: se corrige en la ronda 2.
- F-02: (a) conteos al montar de las tres pestañas → **fuera de alcance**, candidata C-133 (conteo por estado en la API); no se tocan las aserciones `reg.listas`. (b) contadores obsoletos → **se corrige**: un contador no puede mostrarse si no se refrescó junto con su lista (refrescar o retirar los contadores de pestañas no activas al «Actualizar»/recargar). (c) insignia de apoyo en KPI → **fuera de alcance**, candidata C-134 (prop de insignia en `Kpi` de T01).
- F-03: **aceptado** por el orquestador: el cambio de fixture lo provoca el comportamiento en alcance (el inbox pide /api/warm al montar) y las aserciones no cambian. El humano puede revertirlo.
- F-04: D-3 aceptado (flujo forzado por las pruebas de CA-1); la falta de separación del botón en «Detalle» **se corrige**.
- F-05: **se corrige** (evento como chip `code`, artefacto en mono, etiqueta «Corridas confirmadas»).
