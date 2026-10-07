# Ronda 2 — U2-T06

VEREDICTO: NO-VERDE

F-01, F-03, F-04 y F-06 están corregidos de verdad. F-02 está corregido para el caso que pedí (rollout completo con pod viejo Ready), pero al repetirlo encontré un defecto de la misma familia que no vi en la ronda 1 (F-07). Con él, el rebuild semanal de un warm idle termina en cuarentena. No hay ROJO.

## Criterios de aceptación, verificados por mí (HEAD c3ebcea, `-count=1`, worktree limpio antes y después)
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | ≥25 casos, 0 FAIL, `-race` | comando de la tarea | 89 PASS y 0 FAIL; pasa |
| 2 | Sin verificación no hay evento ni `ready` | `-run 'Reset(Fails\|Quarantine\|NoEventWithoutVerification)'` | 6 PASS; pasa |
| 3 | Verify lee estado real | `-run 'Verify(DirtyDB\|DirtyCache\|PodNotReady)'` | 3 PASS; pasa |
| 4 | Housekeeping e `IncompleteSession` | `-run 'Housekeeping\|IncompleteSession'` | 7 PASS; pasa |
| 4b | Rebuild y teardown | `-run 'Rebuild\|Teardown'` | 6 PASS; pasa |
| 5 | Job y eventos | kubeconform, conftest, `Outbox\|Events`, ajv | `Valid: 1, Invalid: 0`; 23/23; PASS; `reset.verified.json valid` y `teardown.verified.json valid` (ajv-cli@5.0.0 + ajv-formats@3.0.1, volcado con `EVENTS_DUMP_DIR`); pasa |
| 6 | API | `up`/`down` de la bitácora | `401 400 400`, `0`, `0`; pasa |
| 7 | Arranque fail-closed | comando de la tarea | tres `rc=1`; pasa |
| 8 | Imagen y dependencias | comando de la tarea | 132, `65532:65532`, 1; pasa |
| 9 | Higiene y alcance | comando de la tarea | `ok`, 0, 0; pasa |

La evidencia del codificador sale del código actual: `a67bd47` toca solo `services/go-reset/`, y los dos commits posteriores solo tocan bitácora y respuesta.

## Verificación de las correcciones
- **F-01 (valla del fake): corregido.**
  - Con el binario real, `RESET_BACKEND=fake` termina con `rc=1` en cada uno de estos casos:
    - sin `RESET_ALLOW_FAKE_BACKEND`;
    - con `RESET_ALLOW_FAKE_BACKEND=TRUE` (solo vale `true`);
    - con `KUBERNETES_SERVICE_HOST` puesto;
    - con `RESET_ENV=prod`, `Production` y `' prod '`.
  - Con permiso y `RESET_ENV=dev` o sin `RESET_ENV` arranca (el timeout me dio `rc=124`).
  - Las subcomandos one-shot pasan por el mismo `config.Load`.
  - Revisé todo el código no-test que escribe `StateReady`, `ResetVerified=true` o llama a `Publish`. Hay una sola vía, `service.go:144-155`, tras `attempt()` sin error.
  - `fakes` solo se importa desde `main.go`, detrás de la valla.
- **F-02 (rollout completo): corregido para lo pedido.** Repetí cada mutación sobre una copia en el scratchpad:

| Mutación en `kube.AppReady` | Resultado |
|---|---|
| quitar `observedGeneration` | `TestKubeAppReadyRequiresCompletedRollout/controlador_no_observó` roja |
| quitar `updatedReplicas` | `/réplicas_sin_actualizar` roja |
| quitar `availableReplicas` | `/réplicas_no_disponibles` roja |
| quitar el conteo de pods | `/pod_extra_del_rollout_viejo` y `TestKubeAppReadyReadsPods` rojas |
| quitar `podReady` | `TestKubeAppReadyReadsPods` roja |
| invertir `<` por `>` en observedGeneration | roja |
| quitar `DeletionTimestamp`, `Phase==Running` o `want==0` | **sobreviven** (ver F-08) |

- **F-03 (literales y baseline): corregido.**
  - Mi barrido de literales en código no-test, `git grep` de duraciones, `1<<`, `warm-`, `baseline-` y puertos, deja solo:
    - los defaults con nombre de `config.go`, sobreescribibles por variable;
    - los nombres de objetos del warm (`warm-app`, `warm-policy`, `warm-state`);
    - cotas de buffer y de cuerpo;
    - `ReadHeaderTimeout 5s`;
    - los valores del backend fake;
    - las constantes con nombre y comentario del Job.
  - Eso es aceptable.
  - `baseline_version` desconocida (`Version()` vacío o con error) lleva a cuarentena, sin evento y sin `ready`: `TestResetUnknownBaselineVersionNeverPublishes`.
  - Mutaciones propias:
    - quitar la comprobación `err != nil || v == ""` hace rojo ese test;
    - fijar `"baseline-1"` en el estado hace rojo `TestResetStoresBaselineVersionFromCleaner`.
  - Los eventos no llevan `baseline_version` (el esquema no lo tiene), así que nunca llega vacío o inventado a un evento. Solo va a `WarmState`.
- **F-04: corregido.** `updated_at` ausente o ilegible da `UpdatedAt` cero, y `IdleCheck` no escala (`TestIdleCheckFailClosedWithoutUpdatedAt`, `TestKubeStateUnreadableUpdatedAtIsZero`).
- **F-06: corregido.**
  - Volví a mutar las tres comprobaciones, anulando cada una (db-baseline, cache-empty, app-ready), 3 veces cada una. `TestResetExhaustiveCombinations` falla siempre. `TestResetPropertyEventIffClean` falló en 5 de 9 corridas, así que ya no es la única red.
  - La métrica `verified` va tras `Publish` exitoso (`TestVerifiedMetricOnlyAfterSuccessfulPublish`).
  - Hay prueba de integración Service + Outbox real.
- **Mutación de Verify («pasos devolvieron éxito»).** Fallan `TestVerifyDirtyDB`, `TestVerifyDirtyCache`, `TestVerifyPodNotReady` y otras 6 pruebas, entre ellas `TestResetNoEventWithoutVerification`, `TestResetExhaustiveCombinations` y `TestResetPropertyEventIffClean`. CA-3 se pone rojo.
- **Timeouts y configuración inválida.** Probé `0`, `0s`, `-5s`, `-1h` y `-1` en `HOUSEKEEPING_GRACE`, `RESET_READY_TIMEOUT`, `RESET_READY_INTERVAL`, `RESET_SCRIPT_TIMEOUT` y `RESET_REDIS_TIMEOUT`, con el binario. Todos dan `rc=1`, y también faltan `WARM_ID` o `REDIS_ADDR` en modo kube.

## Hallazgos
### F-07 · NARANJA · `adapters/kube/kube.go:81-87,106-125` y `core/service.go:78-89` · El rebuild de un warm idle (0 réplicas) termina siempre en cuarentena
- `Service.Rebuild` llama a `Kube.Rebuild` directamente. Ese método solo hace patch de anotación y borra pods; **no** sube réplicas. Solo `restart()` (usado por `Reset`) hace `ScaleApp(1)` cuando hay 0 réplicas.
- Reproducción: copia del scratchpad, test `kube/fake` con `ScaleApp(0)`, luego `Rebuild`. Salida: `tras Rebuild de un warm idle: replicas=0 AppReady=false`.
- `AppReady` devuelve `false` con `want==0` (la guarda de F-02 es correcta). Tras `ReadyTimeout`, `Verify` falla, hay reintento y el warm queda en cuarentena sin `teardown.verified`.
- Contexto: `deploy/flux/base/warm.yaml` programa el CronJob `rebuild` el domingo 03:00 (`0 3 * * 0`), y el warm se escala a `minReplicasIdle=0` tras 30 min idle. El caso normal del rebuild periódico (US-M7.2) deja el warm en cuarentena cada semana.
- Falla hacia el lado seguro (cuarentena, no falso verificado), pero el rebuild periódico no funciona sobre un warm idle y ninguna prueba lo cubre. Fue un hueco mío de la ronda 1.
- Pido: que `Rebuild` suba a ≥1 réplica si hay 0 (igual que `restart()`), con una prueba `Service` + `kube/fake` que reconstruya desde `idle-escalado` y espere `teardown.verified`.

### F-08 · AMARILLO · `kube_test.go` · Guardas de `AppReady` sin prueba
- Quitar `p.DeletionTimestamp != nil`, `Phase != Running` o `want == 0` deja la suite verde.
- Falta un caso con pod Ready pero terminando, uno con pod Ready no Running, y uno con `replicas=0`. Este último sale naturalmente de F-07.
- No existe la prueba de punta a punta «RestartApp y luego AppReady con el pod viejo Ready» sobre `kube.Client` real + `Service`. `kubernetes/fake` no sube `generation`. La matriz con `setStatus` cubre el caso a nivel unitario, y `TestVerifyPodNotReady` lo cubre con el puerto fake.

### F-09 · AMARILLO · `core/service.go:144-146` y `config.go:80-84` · Detalles menores
- Ninguna prueba cubre el fallo de `Put` justo después de verificar. `TestResetStatePutErrorNoEvent` solo falla el primer `Put`. Mutación: ignorar ese error y publicar igualmente deja la suite verde. Hoy el código es correcto.
- En modo kube, una `baseline_version` imposible de obtener (script sin `version` y sin `RESET_BASELINE_VERSION`) se descubre en el primer reset, que va a cuarentena, y no al arrancar. Es fail-closed, pero sería mejor validarlo en el arranque.
- La valla del fake depende de variables de entorno. Un pod que ponga `KUBERNETES_SERVICE_HOST=""` y el permiso podría saltarla. Es una configuración deliberada, no un accidente. Defensa en profundidad opcional: comprobar también el token montado del ServiceAccount.
- La imagen por defecto del Job sigue siendo `go-reset:0.0.0` (coherente con `deploy/`, sobreescribible por `RESET_JOB_IMAGE` o `--image`). Aceptable.

## Tareas candidatas
- Esquema de evento `warm.quarantined` (ya anotado en la tarea).
- Contrato `warm-state` compartido T03/T06 (`updated_at` y quién lo escribe).
- Cómo se lanza el Job `reset-{run}` (token y variables), el montaje de `RESET_BASELINE_SCRIPT` y la API REST al OpenAPI (U2-T07).
- Que el controlador de T02 llame a `PUT /sessions` (F-05 de la ronda 1; el codificador lo declaró fuera de alcance, lo acepto).

## Desviaciones del codificador
- (a) Job no creado y reset en proceso, (b) `PUT /sessions` que reabre la sesión como `activa`, (c) `warm-state` con `updated_at`, (d) contrato `clean`/`verify`/`version` del script y (e) `RESET_BACKEND`: siguen siendo aceptables y están documentadas en la bitácora. La reapertura de sesiones (b) la documentan como intencional.

## Rutas
- Mis mutaciones están en copias bajo `scratchpad/m/`. El worktree quedó limpio (`git status --short | wc -l` = 0) y en c3ebcea.

VEREDICTO: NO-VERDE
NARANJA|services/go-reset/internal/adapters/kube/kube.go:81-87 y core/service.go:78-89|Rebuild de un warm idle (0 réplicas) no sube réplicas: AppReady=false y cuarentena en cada rebuild semanal; sin prueba
INFORME: revisiones/U2-T06/ronda-2.md
