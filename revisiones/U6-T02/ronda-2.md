# Ronda 2 — U6-T02

VEREDICTO: VERDE

Worktree `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U6-T02-web`, rama `tarea/U6-T02`, sha 5adbeb3. Está limpio (`git status --short | wc -l` = 0). Las mutaciones y mi reproducción las hice en copias del scratchpad (`.../scratchpad/ui2`), no en el worktree.

## Criterios de aceptación, verificados por mí
| # | Criterio | Resultado |
|---|---|---|
| 1 | `go vet ./... && go test -count=1 -race ./... \| grep -c -E '^(FAIL\|--- FAIL)'` | `0`. Los 6 paquetes dan `ok`; `inbox` tarda 9,5 s, así que las pruebas se ejecutan de verdad. Salida en `.../scratchpad/ca1r2.txt`. |
| 2 | `-run 'Warm'`, conteo de `--- PASS` | `13` (mínimo 8). Pasa. |
| 3 | `-run 'Confirmations'`, conteo de `--- PASS` | `8` (mínimo 6). Pasa. |
| 4 | `validate.sh`, lint de Redocly 1.25.0, líneas borradas y rutas | `validate rc=0`, `lint rc=0`, `0` líneas borradas, `2` rutas. Pasa. |
| 5 | Higiene y alcance | `0` y `0` con la base real `docs/u6-tareas` (con el merge-base de origin/main da `9`, los archivos del commit base ya aclarados). Pasa. |

## Cierre de los hallazgos de la ronda 1
- **F-01 (ROJO), cerrado.** El diff de código de la ronda 2 es una sola línea: `s.rorder = append(s.rorder, r.NotificationID)` en `OpenStore`, dentro del `if !dup` (sin duplicados por `notification_id`). Volví a correr mi reproducción de reinicio contra el código nuevo y `TestZZReceiptsTrasReinicio` pasa (antes daba `0 recibos (esperado 1)`). Las tres pruebas nuevas cubren el reinicio: `TestReceiptsTrasReinicioConservanOrden`, `TestReceiptsTrasReinicioIgnoraDuplicados` y `TestConfirmationsTrasReinicioSigueListando`. La última comprueba el nivel HTTP, con admin viendo 2 y user viendo 1. La bitácora ya corrige la afirmación falsa de la ronda 1.
- **F-02 (NARANJA), cerrado.** `TestWarmArranqueRechazaConfigIncompleta` cubre cinco casos: sin archivo de token, sin esquema, ftp, sin host y con credenciales. `TestWarmCincoCincoxxNoAbreElCircuito` comprueba que seis llamadas con 5xx llegan las seis al warm.
- **F-03 (AMARILLO), cerrado.** `TestWarmTokenIlegibleOVacioDa503SinLlamar` cubre archivo inexistente y vacío: 503, 0 llamadas al warm y log `ERROR`. `TestWarmCircuitoMedioAbiertoConRelojInyectable` usa reloj inyectado y cubre la ventana abierta, la sonda fallida que reabre y la sonda sana que cierra.
- **F-04**, fuera de alcance por arbitraje. Queda como tarea candidata: unificar el circuito de `internal/auth/identity.go` y `internal/httpapi/warm.go`.

## Verificación de la afirmación de los mutantes
Las pruebas de F-02 y F-03 pasan desde el primer momento porque el comportamiento ya existía. Para comprobar que detectan regresiones apliqué yo mismo mutantes en la copia del scratchpad. Cada uno hace fallar la prueba esperada y la copia original queda intacta:
| Mutante | Prueba que falla |
|---|---|
| Quitar el `append` a `rorder` en `OpenStore` | las 3 pruebas de reinicio y mi reproducción |
| `settle(probe, resp.StatusCode < 500)` (el 5xx abre el circuito) | `TestWarmCincoCincoxxNoAbreElCircuito` |
| Ignorar la ventana abierta (`if c.probing {`) | `TestWarmCircuitoTrasCincoFallosDeTransporte` y `TestWarmCircuitoMedioAbiertoConRelojInyectable` |
| Aceptar el token vacío (`if false {`) | `TestWarmTokenIlegibleOVacioDa503SinLlamar` |
| Quitar la validación del esquema de `WARM_URL` | `TestWarmArranqueRechazaConfigIncompleta` |
| No exigir el archivo de token | `TestWarmArranqueRechazaConfigIncompleta` |

La afirmación del codificador queda confirmada, y además comprobé dos mutantes más que él no declaró (los dos últimos de la tabla).

## Hallazgos en pie
Ninguno ROJO ni NARANJA. Tampoco encontré nada nuevo en el diff de la ronda 2: solo una línea de código y pruebas.

## Tareas candidatas
- Unificar el circuito de identidad y el del warm en un paquete común (F-04).

VEREDICTO: VERDE
INFORME: revisiones/U6-T02/ronda-2.md
