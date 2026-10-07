# Ronda 3 — U2-T06

VEREDICTO: NO-VERDE

F-07, F-08 y F-09 están corregidos de verdad. No hay ROJO. El barrido sistemático de la familia (Reset, Rebuild, Teardown, Housekeeping, IdleCheck, PUT /sessions) no encontró ninguna secuencia que publique `reset.verified` o `teardown.verified` sin que las tres comprobaciones pasen, ni que deje `ready` con `reset_verified=false` o `dirty` con `reset_verified=true`. Sí encontró dos defectos NARANJA de la misma familia (F-10, F-11).

## Criterios de aceptación, verificados por mí (HEAD 7926d63, `-count=1`, worktree limpio antes y después)
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | ≥25 casos, 0 FAIL, `-race` | comando de la tarea | 113 PASS y 0 FAIL; pasa |
| 2 | Sin verificación no hay evento ni `ready` | `-run 'Reset(Fails\|Quarantine\|NoEventWithoutVerification)'` | 6 PASS; pasa |
| 3 | Verify lee estado real | `-run 'Verify(DirtyDB\|DirtyCache\|PodNotReady)'` | 3 PASS; pasa |
| 4 | Housekeeping e `IncompleteSession` | `-run 'Housekeeping\|IncompleteSession'` | 7 PASS; pasa |
| 4b | Rebuild y teardown | `-run 'Rebuild\|Teardown'` | 4 PASS en `kube` y 5 en `core`; pasa |
| 5 | Job y eventos | kubeconform, conftest, `Outbox\|Events`, ajv (volcado con `EVENTS_DUMP_DIR`) | `Valid: 1, Invalid: 0`; 23/23; PASS; `reset.verified.json valid` y `teardown.verified.json valid` (ajv-cli 5.0.0 + ajv-formats 3.0.1); pasa |
| 6 | API | `up`/`down` de la bitácora | `401 400 400`, `0`, `0`; pasa |
| 7 | Arranque fail-closed | comando de la tarea, más kube sin `baseline_version` | tres `rc=1`; el caso extra también `rc=1`; pasa |
| 8 | Imagen y dependencias | comando de la tarea | 132, `65532:65532`, 1; pasa |
| 9 | Higiene y alcance | comando de la tarea | `ok`, 0, 0; pasa |

La evidencia del codificador sale del código actual: tras `7e0a56b` solo cambian tests, bitácora y respuesta.

## Verificación de F-07, F-08 y F-09 (mutaciones propias en copias bajo scratchpad)
- **F-07 corregido.**
  - `kube.Rebuild` sube a 1 réplica si hay 0.
  - Mutación «no subir 0→1» en `Rebuild`: ponen rojo `TestKubeRebuildScalesUpIdleWarm`, `TestRebuildPathsFromEveryWarmState/Rebuild/idle-escalado` y `TestIdleCheckThenRebuildCycle`.
  - Mutación en `restart()`: ponen rojo `Reset/idle-escalado` y `Housekeeping->Reset/idle-escalado`.
  - `Rebuild` no espera por sí mismo; espera `Verify` (sondeo de `AppReady` hasta `ReadyTimeout`), igual que `Reset`. Eso es correcto.
  - Sobre el reloj simulado: no oculta que falte la espera, porque sin sondeo la primera lectura da falso y hay cuarentena. Sí oculta otra cosa, ver F-12(c).
- **F-08 corregido.**
  - Quitar `DeletionTimestamp` hace rojo `TestKubeAppReadyGuards/pod_Ready_pero_terminando`.
  - Quitar `Phase==Running` hace rojo `/pod_Ready_no_Running`.
  - Quitar `want==0` hace rojo `/replicas=0_y_0_pods`.
  - Repetí también las mutaciones de rollout (ignorar `observedGeneration`, `updatedReplicas` y `availableReplicas`; ignorar el conteo de pods). Todas ponen rojo.
- **F-09 corregido.**
  - Ignorar el error del `Put` final y publicar igual hace rojo `TestResetFinalPutFailureNoEventNotReady`.
  - El arranque en modo kube con un script cuyo `version` no imprime nada y sin `RESET_BASELINE_VERSION` da `rc=1` con mensaje claro.
- **Mutación de Verify («pasos devolvieron éxito»).** Ponen rojo `TestVerifyDirtyDB`, `TestVerifyDirtyCache`, `TestVerifyPodNotReady`, `TestResetNoEventWithoutVerification`, `TestHousekeepingQuarantineWhenResetFails`, `TestRebuildFailedVerificationQuarantinesWithoutEvent`, `TestTeardownFailedVerificationQuarantinesWithoutEvent`, `TestResetPropertyEventIffClean`, `TestResetExhaustiveCombinations` y `TestOutboxIntegrationQuarantineWritesNoResetVerified`. CA-3 se pone rojo.

## Barrido propio de la familia (pruebas fuera del repo)
Copia del código en `scratchpad/m3/sweep/`. El código es el mismo y solo añadí pruebas.

1. `internal/core/sweep_test.go`: **655 360 escenarios**.
   - Producto de 4 estados del warm, 5 operaciones y 12 bits de fallo: restart, no Ready, clean, DB sucia, `Version` error o vacía, flush, Redis sucio, `Replicas`, `AppReady`, `Diff` y `DBSize` con error.
   - Además `Put` fallando desde la llamada 0, 1, 2 o 3, y `Publish` fallando o no.
   - El oráculo comprueba el mundo real en el instante exacto del `Publish`: filas 0, claves 0, app Ready con ≥1 réplica y almacén `ready` verificado con `baseline_version` conocida.
   - También comprueba coherencia de cada `Put`, que no haya terminación sin error y sin cuarentena, y que tras retirar las fallas un `Reset` siempre llegue a `ready`.
   - Resultado: **0 violaciones de (a), (b) y (c)** salvo F-11. Validé el oráculo con dos mutantes, que lo ponen rojo de inmediato:
     - Verify mutado.
     - Put final ignorado.
2. `internal/core/crash_test.go`: kill (panic) en cada llamada a un puerto, en 4 estados × 4 operaciones, más recuperación con un `Reset`.
   - Nunca queda un estado incoherente, ningún evento sale con el mundo sucio, y siempre hay recuperación.
   - Observaciones: mismo hallazgo que F-11, y el caso de F-12(d).
3. `internal/adapters/kube/sweep_test.go`: `warm-state` existente pero ilegible, con `kube.Client` real sobre `kubernetes/fake`. Reproduce F-10.
4. `internal/core/idle_test.go` y `xproc_test.go`: reproducen F-12(a), F-12(b) y F-12(e).

## Hallazgos
### F-10 · NARANJA · `adapters/kube/kube.go` (`Get`, `Unmarshal`) y `core/service.go` (`current`, `run`) · Un `warm-state` ilegible deja Reset, Rebuild, Teardown y Housekeeping atascados para siempre
- `kube.Get` devuelve error si el ConfigMap `warm-state` existe pero `state` está vacío, ausente, no es JSON o no es un objeto. `current()` propaga el error y `run()` sale antes del primer `Put`.
- Ninguna operación llega a sobrescribir el estado, y no hay salida automática. Hace falta que un humano edite el ConfigMap.
- Reproducción: `go test -run TestSweepCorruptWarmStateStuck` en la copia. Los 4 casos dan, para Reset, Rebuild y Housekeeping, `err=warm-state ilegible: ...`, y el estado sigue ilegible.
- Causas plausibles: un ConfigMap creado vacío (`kubectl create configmap warm-state`) o escrito por T03 con otra forma. Es una causa evitable de cuarentena permanente. El propósito del reset es justamente fijar un estado conocido.
- Pido: distinguir error transitorio de la API (devolver error) de contenido ilegible (tratarlo como `dirty`/desconocido, que el reset sobrescribe). `IdleCheck` debe seguir sin escalar con estado ilegible. Y una prueba con `kube.Client` y ConfigMap corrupto que termine en `ready`.

### F-11 · NARANJA · `core/service.go` (`IdleCheck`, `ScaleApp` y luego `Put`) · Fallo del `Put` tras escalar deja `ready` con 0 réplicas
- `IdleCheck` escala primero y escribe `idle-escalado` después. Si el `Put` falla (conflicto de ConfigMap, kill del pod), el clúster queda con 0 réplicas y el almacén dice `ready`, `reset_verified=true`.
- Reproducción: `TestSweepFamily` da `ready/IdleCheck: (b') ready en el almacén pero 0 réplicas en el clúster` en 8 192 escenarios, el primero con `failPut:1`. `TestCrashSweep` lo da con un kill en la llamada 2.
- Efecto: un consumidor (U2-T03) que confíe en `ready` despliega sobre 0 réplicas hasta que un idle-check posterior lo repare. Es el estado optimista falso de esta familia.
- Pido: escribir `idle-escalado` antes de escalar, o compensar si el `Put` falla. Una prueba con `FailPutN` que falle el `Put` de `IdleCheck` y compruebe que el almacén no dice `ready` con réplicas 0.

### F-12 · AMARILLO · barrido de clase (no bloquea por sí solo)
- (a) **La sesión de una corrida nunca se cierra con su propio reset.** Ninguna ruta usa `SessionDone` (`types.go:30`, solo declarada).
  - `TestIdleAfterNormalRunEnd`: `PUT /sessions r-1`, `Reset(r-1)` verificado, y a +1 h, +6 h y +23 h `IdleCheck` no escala. La sesión sigue `activa`.
  - El scale-down de 30 min es inalcanzable hasta que housekeeping cierra la sesión pasadas 24 h como `incompleta`, aunque la corrida terminó bien. Ese cierre además dispara un reset gratuito.
  - Depende del contrato con T02 (quién cierra la sesión). Dejarlo anotado o que `Reset(run)` cierre su sesión.
- (b) **`WarmPolicy` acepta `idleScaleDownAfter` ≤ 0.** Con `0s` o `-1h`, `IdleCheck` escala un warm recién verificado (`TestIdlePolicyNonPositive`). `config.dur` sí rechaza ≤ 0, pero `WarmPolicy` no. Debería ser fail-closed.
- (c) **El reloj simulado de la tabla de 16 combinaciones salta a la meta.** El controlador simulado deja todo Ready en el primer `Sleep`, y el campo `slow` no se usa.
  - Mutación «`AppReady` ignora el status del Deployment»: las pruebas de servicio de `service_test.go` siguen verdes y solo caen las unitarias de `kube_test.go` (`TestKubeAppReadyRequiresCompletedRollout`). La protección existe, pero a nivel unitario.
  - Falta un caso de servicio con transición (pod viejo Ready y status viejo durante N sleeps) que exija ≥N sondeos antes de verificar.
- (d) **Evento perdido.** Si el proceso muere o `Publish` falla tras el `Put` `ready`, el almacén queda `ready` verificado sin evento. El llamante lo reintenta (HTTP 500 o Job con reintento), así que no hay falso positivo. Es solo un hueco de entrega.
- (e) **Sin exclusión entre procesos.** `s.mu` es por proceso, pero `housekeeping` (cada hora) y `rebuild` (domingo 03:00) son CronJobs distintos con `concurrencyPolicy: Forbid` cada uno. A las 03:00 del domingo arrancan a la vez.
  - `TestCrossProcessRebuildVsReset`: el rebuild toca los pods tras un `reset.verified` ya publicado, sin coordinación.
  - Ni `Rebuild` ni `Teardown` miran si hay corrida activa. Es riesgo de integración, no un falso verificado.
- (f) **Literales.** `ScaleApp(ctx, 1)` está en `service.go`, `kube.go` (Rebuild y Teardown) y el valor por defecto 1 de `Replicas` y `AppReady`. Coincide con `deploy/` (`replicas: 1`) pero debería ser una constante con nombre o salir del Deployment.
- (g) **`Quarantine` no alerta si el `Put` de cuarentena falla.** El estado queda `dirty`, que es seguro, pero se pierde el aviso `warm.quarantined`.
- (h) **`sessions.List` ignora líneas JSONL corruptas sin avisar.** Una línea `activa` truncada hace invisible esa sesión para `IdleCheck`.

## Tareas candidatas (fuera de alcance)
- Contrato de fin de sesión con T02 (`SessionDone`).
- Exclusión entre procesos (Lease o ConfigMap) para `reset`, `rebuild` y `housekeeping`.
- Los CronJobs de `deploy/` lanzan `go-reset` sin variables ni volumen compartido para `RESET_DATA_DIR` y `RESET_OUTBOX_FILE`: sin eso no ven las sesiones del API ni sus eventos llegan a nadie (U2-T07).
- Esquema `warm.quarantined` y contrato `warm-state` compartido con T03 (ya anotadas).

## Rutas
- Pruebas de reproducción en `scratchpad/m3/sweep/`: `internal/core/{sweep,crash,idle,xproc}_test.go` e `internal/adapters/kube/sweep_test.go`.
- Mutantes: `scratchpad/m3/{w,sw2,sw3}/`. Script de mutación: `scratchpad/m3/mut.py`.
- Worktree `agentic-qa-swarm-wt-U2-T06`: limpio (`git status --short | wc -l` = 0), en 7926d63.

VEREDICTO: NO-VERDE
NARANJA|services/go-reset/internal/adapters/kube/kube.go (Get) + core/service.go (current/run)|warm-state ilegible o sin clave deja Reset/Rebuild/Teardown/Housekeeping atascados para siempre; sin prueba
NARANJA|services/go-reset/internal/core/service.go (IdleCheck: ScaleApp y luego Put)|Fallo del Put tras escalar deja el almacén en ready/reset_verified=true con 0 réplicas
INFORME: revisiones/U2-T06/ronda-3.md
