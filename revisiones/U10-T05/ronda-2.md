# Ronda 2 — U10-T05

VEREDICTO: VERDE

Revisé HEAD 9e9ee21 con la tarea en el hash 7ff12ffa. Todo se corrió en una copia limpia (`git archive HEAD`, con `node_modules` enlazado). El worktree quedó limpio.

## Criterios de aceptación, verificados por el revisor
| # | Criterio | Resultado |
|---|---|---|
| 1 | Script existe y sale ≠0 mientras haya diferencias reales | pasa. EXIT 1, 0 de 20 OK, máximo 39,29 % |
| 2 | Informe con estilos, estructura y % de píxeles | pasa. 20 entradas y 40 capturas |
| 3 | Enganche en `dashboard-smoke.sh` bajo `flock` | pasa en forma y aislado. Smoke de extremo a extremo no corrido |
| 4 | Alcance | pasa. `revisiones/U10-T05/ronda-1.md` en alcance por arbitraje |
| 5 | Control mockup contra sí mismo | pasa. EXIT 0, 20 de 20 OK, 0,00 %, 21 s |

`npm run -s lint` sale con 0. El control no es vacuo: la ejecución normal da hasta 39,29 % con los mismos umbrales.

## Hallazgos de la ronda 1
- F-01 (ROJO): corregido. `--control` existe; `--bogus`, `--out` sin valor, `--out --control` y `--control extra` salen con código 2. Bitácora actualizada; `control/informe.json` determinista (`cmp`).
- F-02 (NARANJA): cerrado según arbitraje. Arnés propio: `OK antes`, `FALLA fidelidad-visual`, `OK despues`, `fail=1`, EXIT 1. El smoke usa `set -uo pipefail`, sin `-e`. Bitácora declara el smoke de extremo a extremo «no verificado» y que el `OK` aparecerá tras T06.
- F-03 (AMARILLO): corregido. `--out` funciona; el smoke pasa `--out "$work/fidelidad"`.
- F-04 (AMARILLO): sin acción.

## Hallazgos nuevos
Ningún ROJO ni NARANJA.

### F-05 · AMARILLO · `scripts/kind/dashboard-smoke.sh:155`
`kind-e2e.sh:43-65` documenta que el cerrojo `aqs-kind.lock` se hereda por fd. Si el smoke completo se lanza bajo `flock aqs-kind.lock bash dashboard-smoke.sh`, el `flock` interno de `fidelidad_visual` abriría otro descriptor y esperaría para siempre. CA-3 pide ese `flock`; no se exige cambio. `docs/operaciones/kind-local.md:203` invoca el smoke sin `flock`. No probado en kind.

## Tareas candidatas
- T06: cerrar las brechas reales del gate.
- Revisar la semántica del `flock` del smoke frente al fd heredado (F-05) al correr el smoke completo.
- Cobertura del run solo en el estado `bug` (F-04).

VEREDICTO: VERDE
AMARILLO|scripts/kind/dashboard-smoke.sh:155|Posible bloqueo del flock interno si el smoke se lanza ya bajo el mismo cerrojo
