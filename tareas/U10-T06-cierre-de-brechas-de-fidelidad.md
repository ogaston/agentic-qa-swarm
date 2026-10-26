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

**Fuera:** subir tolerancias o añadir excepciones en la compuerta (si una diferencia es de datos y no de CSS, se reporta como candidata, no se oculta); endpoints nuevos; CDN; relajar la CSP; cambios fuera de `web/dashboard/**`; portar lo que `DESIGN.md §1.3` excluye.

## Archivos de contexto
`mockups/swarm-mock.html`, `DESIGN.md`, `web/dashboard/scripts/e2e-fidelidad.mjs`, `bitacoras/U10-T05.md`, `bitacoras/U10-T05/informe.json`, bitácoras de U10-T02..T04.

## Criterios de aceptación (desde `web/dashboard/`, tras `npm ci`)
- [ ] **CA-1** — `npm run -s e2e:fidelidad; echo $?` → `0` y el informe dice «20 de 20 combinaciones OK».
- [ ] **CA-2** — `npm run -s lint && npm run -s typecheck && npm run -s check:api && npx vitest run 2>&1 | grep -c -E 'failed|FAIL'` → `0`.
- [ ] **CA-3** — La compuerta no se debilitó: `git diff $(git merge-base HEAD origin/main) -- scripts/e2e-fidelidad.mjs | wc -l` → `0`.
- [ ] **CA-4** — CSP intacta: `npm run -s build >/dev/null && grep -c -E 'unsafe-inline|unsafe-eval' dist/index.html` → `0`.
- [ ] **CA-5** — Alcance: `git diff --name-only $(git merge-base HEAD origin/main) | grep -v -E '^(web/dashboard/|tareas/U10-T06|bitacoras/U10-T06|revisiones/U10-T06/)' | wc -l` → `0`.

**Rojo primero:** pegar la salida de `e2e:fidelidad` fallando antes de tocar las vistas.

_Creado con ❤️ por Luis Felipe Ariza Vesga._
