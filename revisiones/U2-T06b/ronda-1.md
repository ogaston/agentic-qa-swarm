# Ronda 1 — U2-T06b

VEREDICTO: VERDE

Los 6 criterios de aceptación pasan con mi propia ejecución y no queda ningún ROJO ni NARANJA. Hay 4 hallazgos AMARILLO y 1 tarea candidata. El worktree quedó limpio (`git status --short | wc -l` = 0). No edité nada: todos los mutantes y las pruebas de reproducción viven en una copia en el scratchpad (`$S/base`, `$S/m_*`, `$S/repro`).

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Suite en verde con `-race`, ≥120 PASS, 0 FAIL | comando de la tarea | 178 PASS, 0 FAIL |
| 2 | `warm-state` ilegible no atasca el reset | `-run 'CorruptWarmState\|UnreadableState'` | 5 PASS: `TestSweepCorruptWarmStateStuck`, `...HousekeepingRepairsWithoutSessions`, `...IdleCheckDoesNotScale`, `...CountsMetric`, `...TransientAPIErrorIsNotUnreadable` |
| 3 | IdleCheck nunca deja `ready` con 0 réplicas | `-run 'IdleCheck(NeverReady...\|PutFailure\|Compensates)'` | 3 PASS |
| 4 | Propiedad de barrido en el repo | `-run SweepProperty` | 2 PASS. Rapid corre 2000 casos; la enumerada reporta `172800 escenarios sin violaciones` |
| 5 | Política y alerta de cuarentena | `-run 'WarmPolicyNonPositive\|QuarantinePutFails'` | 3 PASS |
| 6 | Higiene, imagen, alcance | `go vet`, `gofmt -l`, `docker build`, `git status`, diff contra el merge-base | `ok`, `65532:65532`, `0`, `0` |

La invariante central de U2-T06 sigue intacta: `TestResetPropertyEventIffClean` (rapid, 2000 casos) y `TestResetExhaustiveCombinations` pasan.

## Mutaciones que ejecuté yo (cada una sobre la copia, no sobre el worktree)
| Mutante | Resultado |
|---|---|
| M1: `Verify` = «los pasos devolvieron éxito» | rojo: `TestVerifyDirtyDB`, `TestVerifyDirtyCache`, `TestVerifyPodNotReady`, `TestResetPropertyEventIffClean`, `TestResetExhaustiveCombinations`, `TestOutboxIntegrationQuarantineWritesNoResetVerified`, `TestResetSlowRolloutBeyondTimeoutQuarantines` y otras |
| M2: ignorar el error del `Put` final | rojo: `TestResetFinalPutFailureNoEventNotReady`, `TestSweepPropertyRapid`, `TestSweepPropertyEnumerated` |
| M3: `IdleCheck` con el orden antiguo | rojo: 4 pruebas de `IdleCheck...` más los dos barridos |
| M4: `AppReady` ignora el status del Deployment | rojo en servicio: `TestResetSlowRolloutRequiresPollingBeforeAppReady` y `TestResetSlowRolloutBeyondTimeoutQuarantines`; además la unitaria `TestKubeAppReadyRequiresCompletedRollout` |
| Oráculo: Verify sin la comprobación de `DBSize` | rojo: ambos barridos, `TestVerifyDirtyCache`, la invariante central |
| Oráculo: Verify sin la comprobación de `Diff` | rojo en `TestSweepPropertyEnumerated` (el barrido rapid no lo detecta) |
| Oráculo: quitar `v == ""` en `attempt` | rojo en ambos barridos |
| Oráculo: `s.baseline = ""` | rojo en ambos barridos |
| Oráculo: Verify sin espera, da Ready de inmediato | rojo (`TestSweepPropertyEnumerated` y pruebas de servicio) |
| Oráculo: quitar el `Flush` | rojo en los barridos |
| Quitar el `st.State = StateDirty` inicial en `run` | rojo en ambos barridos |
| `kube.Get` devuelve error ante contenido ilegible (comportamiento antiguo) | rojo: `TestSweepCorruptWarmStateStuck`, `...HousekeepingRepairsWithoutSessions`, `...CountsMetric` |
| `kube.Get` ante error de la API lo trata como ilegible | rojo: `TestUnreadableStateTransientAPIErrorIsNotUnreadable` |
| `compensate` sin `AppReady` | rojo: `TestIdleCheckCompensatesWhenScaleFails` |
| Quitar el guard `<= 0` del core | rojo: `TestWarmPolicyNonPositiveCoreDoesNotScale` |
| Quitar el guard `<= 0` de kube | rojo: `TestWarmPolicyNonPositiveKubeRejectedAndNoScale` |
| No alertar si falla el `Put` de cuarentena | rojo: `TestQuarantinePutFailsStillAlerts` |

El oráculo del barrido es válido. Cada mutación del código pone rojo al menos un barrido y las pruebas puntuales.

## Puntos que me pidió mirar
1. **F-10.** Un `warm-state` vacío, ausente, no JSON, no objeto, `null` o con state desconocido ya no atasca `Reset`, `Rebuild`, `Teardown` ni `Housekeeping`, y `IdleCheck` no escala.
   - Un error de la API se propaga como error y no se sobrescribe nada (mutante `kube.Get` ante error de la API: rojo).
   - «Ilegible ⇒ dirty» no produce `ready` ni `reset_verified=true` sin verificación. `run` fuerza `ResetVerified=false` y `dirty` antes del `Put` inicial, y solo escribe `ready` tras `attempt` (Verify real) y un `Put` exitoso. Los barridos incluyen la semilla `ilegible`.
2. **F-11.**
   - Repetí el `Put` fallando en 0..3, el kill en cada llamada, y el orden antiguo (M3, rojo).
   - Agregué una reproducción propia: `ScaleApp` que **aplica y luego devuelve error** (timeout ambiguo). El resultado es `store=dirty`, `reps=0`, nunca `ready`.
   - La compensación solo escribe `ready` si lee de vuelta `Replicas ≥ 1` y `AppReady`. Si el `Put` de compensación falla, queda `idle-escalado`, que no es un `ready` falso.
   - No encontré un camino por el que la compensación deje un estado optimista. La única carrera que queda es que un escalado en vuelo aterrice después de la lectura de vuelta. No es modelable aquí y está dentro de la candidata de exclusión entre procesos.
3. **F-12 b y g.** Las dos mutaciones ponen rojo (ver tabla).
4. **F-12 c.** M4 repetida por mí pone rojas dos pruebas de servicio, no solo la unitaria.
5. **Propiedad de barrido.** Ver tabla de mutaciones: oráculo válido. Los 172 800 escenarios se ejecutan de verdad (el log lo imprime).
6. **Columnas «propuestas».** Las ejecuté yo.
   - «tratar el error como ilegible» y «restaurar el return» ponen rojo.
   - «quitar el manejo `Unreadable` en `current`» **no** pone rojo `TestSweepCorruptWarmStateStuck`. Solo falla `TestUnreadableStateCountsMetric`.
   - «quitar el return de `IdleCheck`» es equivalente, como la bitácora ya admite.
   - Es una imprecisión de la tabla, no un hueco de comportamiento: ver F-01 (AMARILLO). La bitácora lo declara abiertamente, así que no bloquea.
7. **Invariante central.** Intacta (ver arriba); M1 repetida pone rojas `TestResetPropertyEventIffClean` y `TestResetExhaustiveCombinations`.

## Hallazgos
### F-01 · AMARILLO · `bitacoras/U2-T06b.md` (tabla de barrido) · celdas de mutación que no son rojo real
- La celda «ilegible ⇒ quitar el manejo `Unreadable` en `current` → `TestSweepCorruptWarmStateStuck`» es inexacta: ese test sigue verde sin ese código. Solo lo detecta la prueba de la métrica, porque `kube.Get` ya devuelve `dirty`.
- Las defensas son de dos capas redundantes (adaptador y core). Ninguna prueba de nivel puerto fija el contrato «`Snapshot.Unreadable` ⇒ dirty» del core con un `State` fake.
- Prueba: mutante donde `kube.Get` devuelve `{ready, ResetVerified:true, Unreadable:true}`. Sigue verde, y es seguro solo gracias al flag del core.

### F-02 · AMARILLO · `internal/adapters/kube/unreadable_test.go:~100` · `TestUnreadableStateIdleCheckDoesNotScale` es en buena parte vacua
Con 3 de los 6 contenidos (`vacio`, `null`, `estado desconocido`) no hay `updated_at`, así que `IdleCheck` no escala por `UpdatedAt` cero aunque el `return` de `Unreadable` falte. Con los demás, `dirty` tampoco escala. No hay regresión posible, pero la prueba no ejercita lo que dice.

### F-03 · AMARILLO · `kube.go: parseWarmState` · estado desconocido no cuenta en la métrica
Mutante: cambiar el `switch` por `default:` sobrevive. Un `state` con valor desconocido o vacío (por ejemplo `{}`) pasa sin log ni `aqs_warm_state_unreadable_total`. Sigue siendo seguro: un estado que no es `ready` no escala, y `run` fuerza `dirty`. Pero el «6.º contenido» de la tarea solo se prueba por «Reset funciona», no por «se registra como ilegible».

### F-04 · AMARILLO · `idle_test.go` / `sweep_test.go` · huecos del modelo de fallo
- Ninguna prueba cubre `ScaleApp` que aplica y luego falla. Mi reproducción pasa, pero conviene fijarla.
- El mutante «quitar `n >= 1` en `compensate`» sobrevive: es redundante con `AppReady`.
- El barrido no modela errores transitorios de `Get` ni sesiones activas.
- `Housekeeping` llama a `current` y luego a `Reset`, que lo llama otra vez, así que cuenta dos veces la métrica de ilegible (trivial).

## Tareas candidatas (fuera de alcance)
- Añadir al barrido el fallo transitorio de `State.Get` y el escalado «aplicado y con error».
- Ya registradas por el codificador: `SessionDone`, exclusión entre procesos (Lease), `ScaleApp(1)` con nombre, `sessions.List` que ignora líneas JSONL corruptas, evento perdido tras el `Put` ready.

VEREDICTO: VERDE
AMARILLO|bitacoras/U2-T06b.md tabla de barrido|celdas de mutación inexactas (quitar manejo Unreadable en current no pone rojo el sweep; defensa redundante en dos capas)
AMARILLO|internal/adapters/kube/unreadable_test.go IdleCheckDoesNotScale|prueba parcialmente vacua (sin updated_at no escala de todos modos)
AMARILLO|internal/adapters/kube/kube.go parseWarmState|estado desconocido no cuenta en la métrica ni se loguea (mutante switch->default sobrevive)
AMARILLO|idle_test.go / sweep_test.go|falta ScaleApp aplicado-y-con-error y Get transitorio en el barrido (mi reproducción pasa)
INFORME: revisiones/U2-T06b/ronda-1.md
