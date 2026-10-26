# U10-T06 — Cierre de las brechas que reporta la compuerta de fidelidad

**Unidad:** U10 — Fidelidad visual del dashboard al mockup
**Historias que implementa:** S5.
**Depende de:** U10-T05 **fusionada** (la compuerta existe y está en rojo a propósito). **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

## Por qué existe
U10-T05 entregó `npm run e2e:fidelidad` y, sobre T02–T04 ya fusionadas, sale en rojo: 0 de 20 combinaciones (5 vistas × claro/oscuro × 1280/375). El humano decidió que T05 queda como compuerta y que las vistas se corrigen aparte. Esta tarea baja esa cuenta a cero.

## Alcance
**Dentro:**
> Que `npm run e2e:fidelidad` salga con código 0 corrigiendo CSS y marcado de las vistas, sin tocar la compuerta ni sus tolerancias.

Brechas medidas por T05 (`bitacoras/U10-T05/informe.json`, a re-medir al empezar):
- **Inbox:** tarjetas de KPI 85 px frente a 101,6 px.
- **Corrida:** alturas de tarjeta; falta el encabezado «Flujos ejecutados» en el plan.
- **Mis corridas:** tarjeta de lista 83 px frente a 139,75 px.
- **Entorno:** 1 tarjeta frente a 5; falta el bloque `kpi-strip` (hoy `kv-list`).
- **Login:** el mockup muestra la cabecera con «Principal»; la app no. *Decisión por defecto al redactar (el humano puede revertirla): seguir al mockup.*
- **Píxeles:** 2,86 % a 39,29 % de diferencia según vista y ancho.

**Decisión del humano (2026-10-26), fixture compartido:** la compuerta alimenta hoy a la app con ejemplos de contrato (1 notificación) y el mockup pinta datos estáticos (3 filas, KPI distintas), lo que hace inalcanzable CA-1 solo con CSS. Se permite cambiar **únicamente los datos que `e2e-fidelidad.mjs` sirve a la app** (respuestas `page.route`) para que reproduzcan los datos del mockup. Si es posible, extráelos a `web/dashboard/scripts/fidelidad-datos.mjs`. Los datos deben ser válidos contra el OpenAPI (`check:api`). Las diferencias que sigan siendo de datos sin equivalente real (p. ej. KPI «Auto-ejecución sin humano») se reportan como candidatas, sin texto fijo inventado en la app.

**Decisión del humano (2026-10-26), neutralizar el demo en el mockup:** se permite cambiar la **preparación de la página del mockup** en `e2e-fidelidad.mjs` (la función que abre y fija la vista del mockup), y solo para: (a) ocultar al medir el mockup el conmutador «Previsualizar estado (Demo)» y los paneles «Previsualizar estados de error», que U10 excluye de la app; y (b) a 375 px, neutralizar el desbordamiento horizontal del mockup (que `DESIGN §4.4` prohíbe copiar) de la forma más pequeña posible. Cada neutralización debe quedar nombrada, comentada y registrada en el informe JSON. **No se toca** lo que se mide de la app, las tolerancias, `UMBRAL_PIXELES`, las funciones `comparar*` ni `VISTAS`; el modo `--control` debe seguir en 0. Lo que siga siendo de datos sin equivalente real (KPI «Auto-ejecución», «Familias activas», contadores de pestañas, descripción del entorno, locale de fechas) se reporta como candidata; si tras la neutralización quedan diferencias solo de esos datos, documenta con medición cuánto aportan y no inventes texto.

**Fuera:** subir tolerancias o añadir excepciones en la compuerta (si una diferencia es de datos y no de CSS, se reporta como candidata, no se oculta); endpoints nuevos; CDN; relajar la CSP; cambios fuera de `web/dashboard/**`; portar lo que `DESIGN.md §1.3` excluye.

## Archivos de contexto
`mockups/swarm-mock.html`, `DESIGN.md`, `web/dashboard/scripts/e2e-fidelidad.mjs`, `bitacoras/U10-T05.md`, `bitacoras/U10-T05/informe.json`, bitácoras de U10-T02..T04.

## Criterios de aceptación (desde `web/dashboard/`, tras `npm ci`)
- [ ] **CA-1** — `npm run -s e2e:fidelidad; echo $?` → `0` y el informe dice «20 de 20 combinaciones OK».
- [ ] **CA-2** — `npm run -s lint && npm run -s typecheck && npm run -s check:api && npx vitest run 2>&1 | grep -c -E 'failed|FAIL'` → `0`.
- [ ] **CA-3** — La compuerta no se debilitó: en `git diff 173c245 -- web/dashboard/scripts/e2e-fidelidad.mjs`, ninguna línea añadida o quitada fuera de la preparación de la página del mockup y de los datos servidos toca constantes (`TOLERANCIA_*`, `UMBRAL_PIXELES`), las funciones `comparar*` ni la lista de `VISTAS`; solo cambian los datos servidos a la app o su importación. Comando: `git diff -U0 173c245 -- web/dashboard/scripts/e2e-fidelidad.mjs | grep -E '^[+-]' | grep -v -E '^(\+\+\+|---)' | grep -c -E 'TOLERANCIA|UMBRAL|compar|VISTAS'` → `0`.
- [ ] **CA-4** — CSP intacta: `npm run -s build >/dev/null && grep -c -E 'unsafe-inline|unsafe-eval' dist/index.html` → `0`.
- [ ] **CA-5** — Alcance: `git diff --name-only 173c245 | grep -v -E '^(unidades-y-tareas.md|web/dashboard/|tareas/U10-T06|bitacoras/U10-T06|revisiones/U10-T06/)' | wc -l` → `0`.

**Rojo primero:** pegar la salida de `e2e:fidelidad` fallando antes de tocar las vistas.

_Creado con ❤️ por Luis Felipe Ariza Vesga._
