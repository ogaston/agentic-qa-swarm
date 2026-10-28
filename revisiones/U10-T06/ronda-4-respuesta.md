# U10-T06 — Respuesta del codificador, ronda 4

- **F-05: Corregido.** La bandeja deriva ahora el tono semántico del estado real del warm con el mismo mapeo que la vista Entorno (`WarmPage.tsx`): ready→ok, dirty→warn, cuarentena→warn, idle-escalado→neutral. Se aplica a la insignia del KPI «Estado del entorno» (`badge-ok`/`badge-warn`/`badge-neutral`, clases ya existentes en `styles.css`), a su `colorBarra` (`var(--ok)`/`var(--warn)`/`var(--text-muted)`, tokens existentes en `styles/tokens.css`) y al span de la cabecera (`meta-ok`/`meta-warn`/`meta-neutral`; `meta-warn` y `meta-neutral` se añaden junto a `.meta-ok` en `styles.css` con el mismo patrón). El texto visible no cambia (`ETIQUETA_ENTORNO` + `warm_id`) y el estado `ready` renderiza exactamente igual que antes (verde), como exige la compuerta de fidelidad.
- **Prueba de regresión, rojo/verde.** Nuevo bloque `Inbox tonos del estado del entorno (U10-T06 F-05)` en `InboxPage.test.tsx` (4 pruebas, msw sobre `/api/warm`). Rojo con la fuente de HEAD (cambio de `InboxPage.tsx` retirado con `git stash`): 3 fallos — `dirty` con `reset_verified=false` y `cuarentena` reciben `badge badge-ok` y cabecera `meta-ok`, e `idle-escalado` igual (esperado `badge-warn`/`badge-neutral`); la cuarta prueba (`ready` conserva `badge-ok`/`meta-ok`) pasaba también en rojo, como guarda de fidelidad. Verde con la corrección: 4/4 pasan.

## Criterios de aceptación (ejecutados desde `web/dashboard/`)

| Criterio | Comando | Resultado |
|---|---|---|
| Lint / tipos / contrato | `npm run -s lint`, `npm run -s typecheck`, `npm run -s check:api` | Pasan: exit 0 los tres |
| Pruebas | `npx vitest run` | **230/230 pruebas**, 20 archivos (226 previas + 4 nuevas) |
| Build y CSP | `npm run -s build` y búsqueda de `unsafe-inline\|unsafe-eval` en `dist/index.html` | Pasa: exit 0, cero coincidencias |
| Fidelidad oficial | `npm run -s e2e:fidelidad -- --out /tmp/u10t06-r4-official` | **20/20**, máximo **4,20 %**, exit 0 |
| Control | `npm run -s e2e:fidelidad -- --control --out /tmp/u10t06-r4-control` | **20/20**, **0,00 %**, exit 0 |

`git diff --check` pasa. Alcance respetado: solo `web/dashboard/src/pages/Inbox/InboxPage.tsx`, `web/dashboard/src/pages/Inbox/InboxPage.test.tsx` y `web/dashboard/src/styles.css`, más esta respuesta. Sin tocar el mockup, comparadores, tolerancias, anclas ni VISTAS; ninguna otra página modificada. Sin push ni merge; pendiente revisión del revisor.

## Evidencia

- `/tmp/u10t06-r4-official/informe.json`
- `/tmp/u10t06-r4-control/control/informe.json`
