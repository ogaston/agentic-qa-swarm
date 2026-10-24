# U10-T03 — Corrida (`/runs/:id`) fiel al mockup

**Unidad:** U10 — Fidelidad visual del dashboard al mockup
**Historias que implementa:** S5, US-M9, US-M4 (sin cambiar comportamiento).
**Depende de:** U10-T01 **fusionada**. **Ola 2** (en paralelo con T02 y T04). **Tope: 3 rondas**.

## Alcance
**Dentro:**
> Rehacer `RunPage`/`Resultados` con la «Vista de corrida» del mockup (líneas ~1232-1445).

- `PageHeader` con `Corrida <code>run-id</code>`, insignia de veredicto/estado, trace id y franja de metadatos (Estado de corrida, sondeo cada 3 s).
- Banner de fallida (`alert-box alert-danger`, `data-estado="failed"`) como en el mockup; el color pasa de estilo en línea a clase (cierra C-131; se adapta la aserción de `RunPage.test.tsx` a la clase/rol, no a `rgb(...)`).
- Rejilla `grid-main-side`: columna principal con tarjeta «Informe post-mortem» (resumen, `finding-item` con severidad/invariante/causa, `evidence-list` con URIs `s3://` copiables) y tarjeta plegable «Plan de flujos» (`flow-block`, método con insignia por verbo, esperado con insignia por familia de estado, `json-body`); columna lateral con la línea de tiempo de 9 fases (`phase-*`, hecha/actual/pendiente con icono + texto, no solo color).
- Estados: sin reporte aún, reporte con bug, sin hallazgos, inconcluso, fallida, plan no disponible.
**Fuera:** el conmutador «Previsualizar estado (Demo)» y el banner «Iniciar ejecución de flujos» (la ejecución no se inicia desde la UI: invariante de confirmación única); URLs prefirmadas; endpoints nuevos.

## Archivos de contexto
`mockups/swarm-mock.html`, `DESIGN.md`, `src/pages/Run/*`, `tareas/U9-T03-vista-de-resultados.md`.

## Criterios de aceptación
- [ ] **CA-1** — `npm run -s lint && npm run -s typecheck && npx vitest run 2>&1 | grep -c -E 'failed|FAIL'` → `0`.
- [ ] **CA-2** — Pruebas por estado (6 de arriba) verifican por rol/texto: veredicto, hallazgos, plan plegable, 9 fases con estado textual, y que ningún elemento usa solo color.
- [ ] **CA-3** — `grep -rn 'style={{' src/pages/Run | grep -v test | wc -l` → `0`.
- [ ] **CA-4** — Fidelidad: `npm run -s e2e:fidelidad -- run` (U10-T05) o, mientras no exista, capturas lado a lado en la bitácora (claro/oscuro, 1280 y 375 px).
- [ ] **CA-5** — CSP y alcance como en T02.

_Creado con ❤️ por Luis Felipe Ariza Vesga._
