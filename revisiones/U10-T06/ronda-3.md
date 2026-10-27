# Ronda 3 — U10-T06

VEREDICTO: NO-VERDE

## Verificaciones previas

- HEAD: `73b595783f8bbb4848e41c00f593d7c11f8d3755`.
- Hash de tarea: `71be525cdacbc458f24602252d372f5b72b68d3c`, coincide.
- Grafo consultado con raíz explícita; no modela funciones del diff. Se revisó la fuente.
- Pruebas en archivo limpio de HEAD: `/tmp/u10t06-review-r3-twhv8y3o`, con `node_modules` enlazado.
- Worktree fuente conservado sin modificaciones; únicamente `.codex/` ajeno sin seguimiento, igual al inicio.

## Criterios de aceptación, verificados por mí

| Criterio | Comando ejecutado | Resultado |
|---|---|---|
| CA-1 | `npm run -s e2e:fidelidad -- --out /tmp/u10t06-review-r3-twhv8y3o/official` | Pasa: **20/20**, máximo **4,20 %**, exit 0 |
| CA-2 | `npm run -s lint`, `npm run -s typecheck`, `npm run -s check:api`, `npx vitest run` | Pasan: exit 0 los cuatro; **226/226 pruebas**, 20 archivos |
| CA-3 | Pipeline literal de diff contra `173c245` | Pasa: 0; comparadores, tolerancias, anclas y VISTAS intactos |
| CA-4 | `npm run -s build` y búsqueda de `unsafe-inline\|unsafe-eval` en `dist/index.html` | Pasa: build exit 0, cero coincidencias |
| CA-5 | Pipeline literal de archivos fuera de alcance contra `173c245` | Pasa: 0 |
| Control | `npm run -s e2e:fidelidad -- --control --out /tmp/u10t06-review-r3-twhv8y3o/official` | Pasa: **20/20**, **0,00 %**, exit 0 |

`git diff --check` también pasa.

## Resolución de hallazgos anteriores

**F-01: corregido.** La compuerta oficial ahora aprueba las veinte combinaciones. El CSS contextual de Entorno corrige el desplazamiento del título sin cambiar la medición.

**F-04: corregido.** Ejecuté la sonda con `--out /tmp/u10t06-review-r3-twhv8y3o/diagnostic-new`, directorio inexistente. Termina en 0 y genera `aportes.json`, leído de vuelta. La creación recursiva resuelve el ENOENT.

## Revisión de la opción A

La preparación modifica exclusivamente el contenido autorizado del mockup en memoria. `mockups/swarm-mock.html` permanece intacto. No copia estilos de la aplicación ni elimina tarjetas o regiones completas.

El JSON registra por combinación nueve ajustes de Bandeja, cuatro de Corrida y tres de Entorno, con identificadores únicos. Mis corridas y Login no reciben ajustes adicionales de contenido. Los selectores ausentes producen error. El contenido del informe se asigna como texto, sin interpretar HTML arbitrario.

## Hallazgos

### F-05 · NARANJA · `web/dashboard/src/pages/Inbox/InboxPage.tsx:247` y `:331` · Estados no saludables conservan presentación de éxito

El diff completo introduce `meta-ok` para cualquier estado del entorno en la cabecera y `badge-ok` para cualquier `warm.state` en el KPI. Ambos usan `--ok`; no distinguen `ready` de `dirty`, `cuarentena` o `idle-escalado`.

Lo reproduje con una sonda Vitest añadida únicamente a la copia temporal, mediante las respuestas reales simuladas de `/api/warm`:

`npx vitest run src/pages/Inbox/InboxPage.test.tsx -t 'review probe'`

Con `state=dirty` y `reset_verified=false`, el DOM muestra valor «Sucio (requiere reset)» e insignia `badge badge-ok` con texto `dirty`. Con `state=cuarentena`, muestra «En cuarentena» e insignia `badge badge-ok` con texto `cuarentena`. La cabecera usa igualmente `meta-ok`.

El texto es correcto, pero la presentación semántica de éxito contradice el estado real. La vista Entorno ya dispone de tonos distintos para esos estados. Debe derivarse también aquí la variante del estado, conservando el texto real.

Barrido de clase: se revisaron los cuatro KPI y la cabecera. Las barras fijas 60/40/100 están declaradas decorativas en `styles.css`, sin rol de progreso ni texto de porcentaje; **no se añaden como otro hallazgo bloqueante**. El defecto descrito corresponde a los indicadores semánticos del estado.

## Cumplimiento

Los cinco CA y el control pasan; la preparación A permanece dentro del alcance aprobado y conserva la CSP. Sin cambios de infraestructura. Persiste F-05 en la implementación del propio diff; no se declara VERDE por el número de rondas.

## Evidencia

- `/tmp/u10t06-review-r3-twhv8y3o/results.json`
- `/tmp/u10t06-review-r3-twhv8y3o/ca1.log`
- `/tmp/u10t06-review-r3-twhv8y3o/official/informe.json`
- `/tmp/u10t06-review-r3-twhv8y3o/control.log`
- `/tmp/u10t06-review-r3-twhv8y3o/official/control/informe.json`
- `/tmp/u10t06-review-r3-twhv8y3o/vitest.log`
- `/tmp/u10t06-review-r3-twhv8y3o/f04.log`
- `/tmp/u10t06-review-r3-twhv8y3o/diagnostic-new/aportes.json`
- `/tmp/u10t06-review-r3-twhv8y3o/inbox-state-probe.log`

VEREDICTO: NO-VERDE
NARANJA|web/dashboard/src/pages/Inbox/InboxPage.tsx:247,331|Estados no saludables conservan indicadores semánticos de éxito
INFORME: revisiones/U10-T06/ronda-3.md
