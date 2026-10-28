# Ronda 2 — U10-T06

VEREDICTO: NO-VERDE

## Verificaciones previas

- HEAD verificado: `b123fd5aa52dc1da09196c2ae10ef1a515472bba`.
- Hash de tarea: `d414ebbdcb620ad0dacc67b3fec44a8fe02668ce`, coincide.
- Grafo consultado con `repo_root` explícito; no modela funciones del diff. Verificación continuada en fuente.
- Ejecución en archivo limpio de HEAD: `/tmp/u10t06-review-r2-vs3lgS`, con `node_modules` enlazado.
- Worktree fuente sin modificaciones de revisión; conserva únicamente `.codex/` ajeno sin seguimiento, igual al inicio.

## Criterios de aceptación, verificados por mí

| Criterio | Comando ejecutado | Resultado |
|---|---|---|
| CA-1 | `npm run -s e2e:fidelidad -- --out /tmp/u10t06-review-r2-vs3lgS/official` | **Falla**: 12/20, máximo 16,82 %, exit 1 |
| CA-2 | `npm run -s lint`, `npm run -s typecheck`, `npm run -s check:api`, `npx vitest run` | Pasan: exit 0 los cuatro; 226/226 pruebas, 20 archivos; cero coincidencias `failed\|FAIL` |
| CA-3 | Pipeline literal de diff contra `173c245` | Pasa: 0; leído el diff, no cambian comparadores, tolerancias ni VISTAS |
| CA-4 | `npm run -s build` y búsqueda de `unsafe-inline\|unsafe-eval` | Pasa: build exit 0, cero coincidencias |
| CA-5 | Pipeline literal de archivos fuera de alcance contra `173c245` | Pasa: 0 |
| Control | `npm run -s e2e:fidelidad -- --control --out /tmp/u10t06-review-r2-vs3lgS/official` | Pasa: 20/20, 0,00 %, exit 0 |

## Hallazgos

### F-01 · ROJO · CA-1 · La compuerta permanece en rojo: 12 de 20

Mi ejecución reproduce los resultados del codificador. Fallan:

- Inbox: cuatro combinaciones, 6,70/15,91/6,72/16,82 %.
- Corrida a 375 px: 14,65/15,04 %; tarjeta del informe de 756,188 px frente a 774,188 px.
- Entorno a 375 px: 9,71/11,24 %; tarjeta de 506,688 px frente a 525,688 px.

Corrida a 1280 px ahora pasa en ambos temas: 4,80/4,92 %. El avance de 10/20 a 12/20 es real; no satisface el criterio 20/20.

### F-04 · AMARILLO · `bitacoras/U10-T06/ronda-2-diagnostico/diagnostico.mjs:496` · La sonda exige un directorio de salida existente

La primera ejecución con `--out /tmp/u10t06-review-r2-vs3lgS/diagnostic` falla por `ENOENT` al escribir geometría. Tras crear ese directorio y repetir el mismo comando, termina en 0 y reproduce las mediciones. Conviene crear el directorio o documentar ese requisito. No afecta a la compuerta oficial.

## Resolución de hallazgos anteriores

**F-02: corregido.** Los literales `aqs-test`, `Postgres` y `Redis` desaparecen de WarmPage. Reproduje la prueba nueva contra WarmPage de `42f4ec7`: una prueba falla y 16 pasan. Restaurada la fuente de `b123fd5`, pasan las 17. El identificador mostrado procede del dato real.

**F-03: corregido mediante retractación, cambios y medición reproducible.** La conclusión anterior sobre ausencia de CSS corregible era incorrecta; la bitácora ahora lo reconoce. Los cambios contextualizados en `.run-page` corrigen herencia de código, tracking y tamaño de `.mono`.

Ejecuté la sonda diagnóstica y reproduje:

| Sonda | Base | Tras igualar contenido únicamente en la sonda |
|---|---:|---:|
| Inbox 375 | 16,04 % | 4,04 % |
| Corrida 1280 | 5,07 % | 0,36 % |
| Corrida 375 | 14,73 % | 0,31 % |

En Inbox, los cambios acumulados son retirar Familias activas, Nueva aplicación y contadores no disponibles. La envoltura de pestañas explica por qué los aportes individuales anteriores no eran aditivos.

En corrida, se igualan subtítulo, etiqueta de estado y presentación de hallazgos como texto plano. Reintroducir únicamente `.mono` a 12 px/500 en la sonda de app aumenta 0,31 % a 7,00 % y desplaza `gridY` de 405,59375 a 404,59375.

Estas sondas explican las causas investigadas; no son una ejecución aprobada de CA-1.

## Decisión de alcance pendiente

La autorización actual permite neutralizar demos y desbordamiento del mockup, pero prohíbe ocultar otras diferencias de datos. No autoriza trasladar los contrafactuales diagnósticos a la compuerta.

Se requiere decidir entre:

- Autorizar una referencia comparable con los datos y funcionalidades reales: sin Nueva aplicación ni metadatos inexistentes, con contadores disponibles, texto plano y subtítulo real del informe, y un único identificador del entorno.
- Especificar por separado las funcionalidades y campos de contrato necesarios para reproducir esos contenidos del mockup.

**Límite de evidencia sobre Entorno:** la ronda verifica la discrepancia de altura y el retiro correcto de literales falsos, pero la sonda nueva no incluye Entorno. No demuestra cuánto residuo quedaría al alinear su descripción, composición e identificador; tampoco permite prometer 20/20 tras una decisión de referencia. Hará falta medir nuevamente todas las combinaciones.

Persisten pequeños residuos geométricos, como los 4 px de tabla de Inbox reconocidos en la bitácora. No se certifica ausencia absoluta de CSS pendiente.

## Cumplimiento

Compuerta y CSP conservadas; sin cambios de infraestructura. Los requisitos de despliegue, persistencia y PBT de negocio no aplican a los cambios de esta ronda.

## Evidencia de esta revisión

- `/tmp/u10t06-review-r2-vs3lgS/ca1.log`
- `/tmp/u10t06-review-r2-vs3lgS/official/informe.json`
- `/tmp/u10t06-review-r2-vs3lgS/control.log`
- `/tmp/u10t06-review-r2-vs3lgS/vitest.log`
- `/tmp/u10t06-review-r2-vs3lgS/f02-red.log`
- `/tmp/u10t06-review-r2-vs3lgS/f02-green.log`
- `/tmp/u10t06-review-r2-vs3lgS/diagnostic-rerun.log`
- `/tmp/u10t06-review-r2-vs3lgS/diagnostic/aportes.json`

VEREDICTO: NO-VERDE
ROJO|CA-1|La compuerta permanece en 12/20, exit 1
AMARILLO|bitacoras/U10-T06/ronda-2-diagnostico/diagnostico.mjs:496|La sonda requiere crear previamente el directorio de salida
INFORME: revisiones/U10-T06/ronda-2.md
