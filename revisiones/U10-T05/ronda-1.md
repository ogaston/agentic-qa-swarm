# Ronda 1 — U10-T05

VEREDICTO: NO-VERDE

Revisé HEAD f7a0bc8 con la tarea en el hash 7ff12ffa (coincide con `git rev-parse HEAD:tareas/U10-T05-...`). Ejecuté en una copia limpia (`git archive HEAD`, con `node_modules` enlazado). El worktree sigue limpio.

## Criterios de aceptación, verificados por el revisor
| # | Criterio | Comando | Resultado |
|---|---|---|---|
| 1 | Script existe y sale ≠0 mientras haya diferencias reales | `npm run -s e2e:fidelidad -- --control; echo $?` | pasa. EXIT 1, 0 de 20 OK |
| 2 | Informe lista estilo calculado y % de píxeles | `python3` sobre `informe.json` | pasa. 20 entradas con `estilos`, `estructura`, `pixeles.porcentaje` |
| 3 | Enganche en `dashboard-smoke.sh` bajo `flock` | `bash -n`; `grep -n flock` | pasa en forma. No se corrió el smoke completo (requiere kind). El `OK fidelidad-visual` no se ha visto nunca |
| 4 | Alcance | `git diff --name-only 505bcec HEAD` filtrado | pasa. Vacío |
| 5 | Control mockup-contra-sí-mismo: 0 y 0 % | `npm run -s e2e:fidelidad -- --control; echo $?` | **falla** (F-01) |

`npm run -s lint` sale con 0. El lockfile solo añade `playwright` y `playwright-core` 1.64.1 como `dev`. Sin cambios de CSP ni del build.

## Hallazgos

### F-01 · ROJO · CA-5 · El modo `--control` no existe
`web/dashboard/scripts/e2e-fidelidad.mjs` no lee `process.argv`. Con `-- --control` ignora el argumento y compara app contra mockup (EXIT 1, 0 de 20 OK, porcentajes idénticos a la ejecución normal). CA-5 exige código 0 y 0 % comparando el mockup consigo mismo. Sin este control los porcentajes no se separan de falsos positivos del harness (captura recortada a `clip`, tolerancia de 16 por canal).
La bitácora no se actualizó tras la enmienda: dice «4 criterios (CA-1..CA-4)», cita el hash a75a11a y no trae evidencia de CA-5.
Para cerrarlo: `--control` compara el mockup contra sí mismo en las 5 vistas × 2 temas × 2 anchos, con salida pegada que muestre EXIT 0 y 0 %, y bitácora actualizada. Un argumento desconocido no debe ignorarse en silencio.

### F-02 · NARANJA · CA-3 y bitácora · Enganche al smoke no verificado de punta a punta
Con el gate en rojo, `check fidelidad-visual` imprime `FALLA fidelidad-visual` y nunca `OK fidelidad-visual` (consecuencia aceptada de la enmienda de CA-1). Solo se probó la función aislada: falta ver si el fallo aborta o no el resto del smoke y si cambia el código final. Mínimo: una ejecución del smoke, o una prueba de que `check` aísla el fallo, más la anotación de que el `OK` solo aparecerá tras T06. Si no se puede correr por el clúster, declararlo no verificado y el orquestador lo arbitra.

### F-03 · AMARILLO · `e2e-fidelidad.mjs:360-361` · Escribe en el árbol versionado
Cada ejecución reescribe `bitacoras/U10-T05/informe.*` y las 40 capturas dentro del repo. Conviene un directorio de salida configurable, con ese valor por defecto en la ejecución manual.

### F-04 · AMARILLO · Limitaciones declaradas, no bloqueantes
- Cobertura del run solo en el estado `bug`.
- Alturas de tarjeta dependientes de los datos (fixture compartido, candidata de T06).
- Desviación MSW a `page.route`, aceptada en CA-1.
`UMBRAL_PIXELES`, `TOLERANCIA_CANAL` y `TOLERANCIA_PX` están nombradas y declaradas.

## Tareas candidatas
- T06: cerrar brechas reales (alturas de KPI y listado en inbox, encabezado «Flujos ejecutados», `kpi-strip` del warm, cabecera del login, fixture compartido).
- Decidir en `DESIGN.md` si el login lleva cabecera.

VEREDICTO: NO-VERDE
ROJO|CA-5 (web/dashboard/scripts/e2e-fidelidad.mjs)|El modo `--control` no existe: el argumento se ignora y se compara app contra mockup (EXIT 1, 0 de 20)
NARANJA|CA-3 / bitacoras/U10-T05.md|Enganche al smoke solo verificado en función aislada; bitácora sin actualizar con CA-5 ni el hash 7ff12ff
