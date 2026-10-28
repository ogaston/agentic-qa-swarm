# Ronda 4 — U10-T06

VEREDICTO: VERDE

## Verificaciones previas

- HEAD: `3ba907e45361d9e4518f2a2c5e2a70061cb31769`, el commit del arreglo de F-05.
- Alcance del commit: exactamente los cuatro archivos autorizados (`web/dashboard/src/pages/Inbox/InboxPage.tsx`, `web/dashboard/src/styles.css`, `web/dashboard/src/pages/Inbox/InboxPage.test.tsx`, `revisiones/U10-T06/ronda-4-respuesta.md`). Nada más.
- Pruebas en copia limpia de HEAD: `/tmp/u10t06-review-r4-BGdgxmgV`, con `node_modules` enlazado y `contracts/`+`mockups/` copiados íntegros. (Nota: una primera ejecución de `typecheck` falló por artefacto de mi copia incompleta —faltaba `contracts/plans`—; tras copiar `contracts/` completo, exit 0. No es un defecto del código.)
- Worktree fuente conservado sin modificaciones de código; solo se añaden este informe y su bitácora.

## Resolución de F-05, verificada por mí en la fuente

- `TONO_ENTORNO` en `InboxPage.tsx` (`ready→ok`, `dirty→warn`, `cuarentena→warn`, `idle-escalado→neutral`) coincide exactamente con `COLOR_ESTADO` de `WarmPage.tsx:14-17` (la semántica de la vista Entorno exigida por la respuesta A).
- Cabecera: `meta-${tono}`; KPI: `badge-${tono}` y `colorBarra` `var(--ok)`/`var(--warn)`/`var(--text-muted)`. Las clases `badge-warn` y `badge-neutral` ya existían en `styles.css` (líneas 689 y 714); `meta-warn` y `meta-neutral` se añaden junto a `.meta-ok` con el mismo patrón y variables existentes de `styles/tokens.css`.
- Texto visible sin cambios: `ETIQUETA_ENTORNO` + `warm_id` en cabecera; etiqueta del estado en el KPI; la insignia conserva el `warm.state` crudo.
- `ready` renderiza igual que antes (`meta-ok`/`badge-ok`/`var(--ok)`): lo confirma la compuerta de fidelidad, 20/20 con el mismo máximo 4,20 % de la ronda 3.
- Rojo/verde propio: revertí `InboxPage.tsx` a `3ba907e^` **solo en la copia** (`git archive … | tar -x`, porque el proxy truncó `git show`); las tres pruebas de `dirty`, `cuarentena` e `idle-escalado` fallan y la de `ready` pasa (3 fallidos, 29 pasados). Restaurada la fuente, 32/32 en el archivo y copia idéntica al worktree.
- Las cargas de las pruebas (`warm_id`, `state`, `reset_verified`, `baseline_version`) reproducen la forma real de `WarmState` del contrato (`contracts/openapi/control-plane.yaml:438-446`); los tipos `Record<WarmState['state'], …>` hacen exhaustivo el mapeo (typecheck exit 0).

## Criterios de aceptación, verificados por mí

| Criterio | Comando ejecutado | Resultado |
|---|---|---|
| CA-1 | `npm run -s e2e:fidelidad -- --out /tmp/u10t06-review-r4-BGdgxmgV/official` | Pasa: **20/20**, máximo **4,20 %**, exit 0 |
| CA-2 | `npm run -s lint`, `npm run -s typecheck`, `npm run -s check:api`, `npx vitest run` | Pasan: exit 0 los cuatro; **230/230 pruebas**, 20 archivos (226 previas + 4 nuevas) |
| CA-3 | Diff del pipeline de fidelidad en el commit `3ba907e` | Pasa: 0 archivos; comparadores, tolerancias, anclas y VISTAS intactos (los cambios de `scripts/` son de commits previos ya revisados en rondas 1–3) |
| CA-4 | `npm run -s build` y búsqueda de `unsafe-inline\|unsafe-eval` en `dist/index.html` | Pasa: build exit 0, **cero coincidencias** |
| CA-5 | Diff de archivos fuera de alcance en `3ba907e`; `mockups/swarm-mock.html` contra `173c245` | Pasa: solo los cuatro archivos autorizados; mockup idéntico a `173c245`; sin cambios de contratos |
| Control | `npm run -s e2e:fidelidad -- --control --out /tmp/u10t06-review-r4-BGdgxmgV/control` | Pasa: **20/20**, **0,00 %**, exit 0 |

`git diff --check` también pasa (exit 0).

## Hallazgos

Ninguno. F-05 queda resuelto; la sonda de la ronda 3 queda cubierta por las cuatro pruebas de regresión incorporadas al propio `InboxPage.test.tsx` y verificadas en rojo y en verde por mí.

## Cumplimiento

Los cinco CA y el control pasan en copia limpia de HEAD. El commit se limita al arreglo autorizado de F-05: sin cambios de referencias ni tolerancias, sin otras funciones, sin tocar el mockup, el pipeline de fidelidad ni los contratos. Sin push ni merge.

## Evidencia

- `/tmp/u10t06-review-r4-BGdgxmgV/ca1.log`, `control.log`, `vitest.log`, `build.log`, `ca2a.log`, `typecheck.log`, `red.log`, `green.log`
- `/tmp/u10t06-review-r4-BGdgxmgV/official/informe.json`
- `/tmp/u10t06-review-r4-BGdgxmgV/control/control/informe.json`
- Copias en `bitacoras/U10-T06/revision-ronda-4/`

VEREDICTO: VERDE
INFORME: revisiones/U10-T06/ronda-4.md
