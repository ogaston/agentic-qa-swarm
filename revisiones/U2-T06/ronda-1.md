# Ronda 1 — U2-T06

VEREDICTO: NO-VERDE

Los 10 criterios pasan en mi ejecución fresca y no hay ROJO. Quedan dos NARANJA que bloquean (F-01 y F-02), más amarillos.

## Criterios de aceptación, verificados por mí (worktree 0e6f0b6, `-count=1`)
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | ≥25 casos, 0 FAIL, `-race` | comando de la tarea | 63 PASS y 0 FAIL; pasa |
| 2 | Sin verificación no hay evento ni `ready` | `-run 'Reset(Fails\|Quarantine\|NoEventWithoutVerification)'` | 6 PASS; pasa |
| 3 | Verify lee estado real | `-run 'Verify(DirtyDB\|DirtyCache\|PodNotReady)'` | 3 PASS; pasa |
| 4 | Housekeeping e `IncompleteSession` | `-run 'Housekeeping\|IncompleteSession'` | 7 PASS; pasa |
| 4b | Rebuild y teardown | `-run 'Rebuild\|Teardown'` | 6 PASS; pasa |
| 5 | Job y eventos | kubeconform, conftest, `Outbox\|Events`, ajv | `Valid: 1, Invalid: 0`; 23/23 conftest; PASS; ajv `valid` para `reset.verified` y `teardown.verified` (volcados con `EVENTS_DUMP_DIR`); pasa |
| 6 | API | `up`/`down` de la bitácora | `401 400 400`, `0`, `0`; pasa |
| 7 | Arranque fail-closed | comando de la tarea (más el caso sin `RESET_BASELINE_SCRIPT`) | `rc=1` en los tres (y en el extra); pasa |
| 8 | Imagen y dependencias | comando de la tarea | 132, `65532:65532`, 1; pasa |
| 9 | Higiene y alcance | comando de la tarea | `ok`, 0, 0; pasa |

- El diff contra el merge-base `4bcac0f` solo toca `services/go-reset/` y `bitacoras/U2-T06.md`.
- El último commit solo toca la bitácora, así que la evidencia sale del código actual.
- Worktree limpio después de mi revisión. Las mutaciones las hice en copias del scratchpad, no en el worktree.

## Mutaciones propias (lectura de vuelta)
- **Verify = «pasos devolvieron éxito»**: fallan `TestVerifyDirtyDB`, `TestVerifyDirtyCache`, `TestVerifyPodNotReady` y otras 5 pruebas (`TestResetNoEventWithoutVerification`, `TestHousekeepingQuarantineWhenResetFails`, `TestRebuildFailedVerificationQuarantinesWithoutEvent`, `TestTeardownFailedVerificationQuarantinesWithoutEvent`, `TestResetPropertyEventIffClean`). CA-3 se pone rojo.
- **Anular una sola comprobación** (`db-baseline`, `cache-empty` o `app-ready`): cada una pone rojas pruebas dirigidas.
- **Caminos a `ready`, `reset.verified` o `teardown.verified` sin verificación**: revisé `service.go` y no encontré ninguno.
  - `run()` es el único sitio que escribe `ready` y publica eventos, y solo tras `attempt()` sin error, que exige las 3 comprobaciones.
  - Se cubren el reintento único, `cuarentena→ready` solo vía un reset verificado, rebuild/teardown y housekeeping (que llama a `Reset`).
  - Un fallo de `Put` o de `Publish` no deja evento sin estado.
  - Un fallo de lectura de DB o Redis cuenta como comprobación fallida (la salida inválida de `verify` es error, no 0).

## Hallazgos
### F-01 · NARANJA · `cmd/go-reset/main.go:99-103`, `internal/config/config.go:50` · `RESET_BACKEND=fake` sin acotar
- El binario de producción acepta `RESET_BACKEND=fake` sin ninguna guarda: no hay build tag, ni se comprueba si corre en clúster, ni hay confirmación adicional.
- Con esa variable, `POST /resets` responde `reset_verified=true`, estado `ready` y publica `reset.verified` sin tocar pod, DB ni Redis. Lo comprobé con el `up` de CA-6.
- Una variable mal puesta en un Deployment produce el fallo que el KPI «reset verificado 100%» debe impedir: eventos de reset verificado falsos.
- Pido que el modo fake no sea alcanzable en la imagen de producción. Opciones: build tag `fakebackend` para `up`, o rechazar `fake` si existe `KUBERNETES_SERVICE_HOST`.

### F-02 · NARANJA · `adapters/kube/kube.go:106-120` y `core/service.go:87-96,161-172` · `app-ready` no confirma que el restart se haya aplicado
- `AppReady` solo lista pods (Running + Ready, sin `DeletionTimestamp`). No mira `observedGeneration`, `updatedReplicas` ni `availableReplicas` del Deployment.
- Tras `RestartApp` (un patch de anotación) la verificación puede leer el pod viejo, todavía Ready, antes de que el controlador cree el nuevo. DB y Redis se limpian en milisegundos y no dan margen.
- Entonces `app-ready` pasa sin que el pod haya reiniciado, con estado en memoria o conexiones de la corrida anterior (riesgo #1 del proyecto).
- `TestKubeAppReadyReadsPods` solo prueba la lectura de pods. Ninguna prueba cubre «patch hecho, rollout no completado».
- Pido que `AppReady` exija rollout completo: `observedGeneration>=generation`, `updatedReplicas==replicas==availableReplicas`, sin pods extra. Y una prueba con `kubernetes/fake` que lo cubra.

### F-03 · NARANJA · `cmd/go-reset/main.go:97-98`, `kube.go:28`, `jobspec/job.go:16-19,38-40`, `script.go:23`, `config.go:48` · Literales en puntos de decisión (barrido de clase)
- `warm_id` («warm-aqs-test») y `baseline_version` («baseline-1») están incrustados y se escriben en `WarmState` y en `reset.verified`. Un `baseline_version` fijo es información falsa si el baseline cambia.
- Tampoco son configurables `ReadyTimeout` 120 s ni `ReadyInterval` 2 s, ni el timeout 60 s del script, ni el 5 s de Redis.
- Tampoco lo son la dirección de Redis por defecto, la imagen por defecto `go-reset:0.0.0`, `activeDeadlineSeconds` 600, `ttlSecondsAfterFinished` 3600 ni los `resources` del Job.
- Pido sacar al menos `warm_id`, `baseline_version` y los timeouts a configuración. Un `WarmID` o una versión de baseline «desconocida» debería salir de `RESET_BASELINE_SCRIPT` o de variable, no de una constante.

### F-04 · AMARILLO · `adapters/kube/kube.go:160`, `core/service.go:239-248` · `updated_at` ignora el error de parseo
- Si `updated_at` falta o está corrupto, `at` queda en tiempo cero y `IdleCheck` escala de inmediato un warm `ready` recién creado.
- go-warm-manager (T03) escribe solo la clave `state`, así que tras sus escrituras `updated_at` queda viejo. Es un acoplamiento entre T03 y T06 a registrar.
- Pido tratar un `updated_at` ilegible como «no escalar» (fail-closed).

### F-05 · AMARILLO · `core/service.go:295` · Housekeeping resetea un `dirty` sin ninguna sesión registrada
- `needReset` se cumple si el warm está `dirty` y no hay sesión fresca. Hoy nadie llama a `PUT /sessions`, así que una corrida en curso que no registró sesión sería reseteada por el CronJob.
- Es ambiguo en la tarea («fin o abandono»). Anotar como riesgo de integración con U2-T02.

### F-06 · AMARILLO · Pruebas
- `TestResetPropertyEventIffClean` es probabilística. En una de mis mutaciones (`app-ready` anulado) no falló en la primera corrida; la reproduje fallando tras 71 casos. Subir `-rapid.checks` o añadir un caso fijo.
- `TestEventsDump` aparece como `--- SKIP` por defecto (es un ayudante de volcado, no una prueba prometida). Los eventos que valida ajv salen de los constructores, no de una corrida del `Service`.
- `aqs_reset_total{result="verified"}` se incrementa antes de `Publish`.

## Desviaciones del codificador
- **(a) Job no creado, reset en proceso: aceptable.**
  - La propia tarea es contradictoria: pide un Job «sin credenciales» y sin token montado, pero que ejecute pasos contra la API de Kubernetes.
  - El constructor y `render-reset-job` están y pasan las políticas. La bitácora documenta la decisión.
  - El Job renderizado, tal cual, no podría correr (sin token ni variables): candidata para U2-T07.
- **(b) `PUT /sessions`: aceptable, con reservas.**
  - Es necesario para «corrida activa» y para persistir sesiones, y es mínimo y con bearer. No es desborde.
  - Reabre como `activa` una sesión `incompleta` si se vuelve a enviar (¿es la reanudación? registrar).
- **(c) `warm-state` con clave `state` + `updated_at`: aceptable.** La clave `state` coincide con la que usa U2-T03 (leí su worktree). Ver F-04 sobre `updated_at`.
- **(d) Contrato `clean`/`verify` del script: aceptable.** Entrada inválida es error, no 0.
- **(e) `RESET_BACKEND` y `RESET_REDIS_ADDR`: la dirección de Redis es aceptable; `RESET_BACKEND=fake` no está acotado (F-01).**
- **(f) CA-2 contra fakes en memoria, no contra el outbox JSONL: tolerable.** El outbox real solo se prueba por idempotencia y forma. Sería mejor una prueba de integración `Service` + `Outbox` real que confirme que no hay línea `reset.verified` en cuarentena.

## Comprobaciones pedidas
- **Reset fuera de `aqs-test`:** imposible. `config.Load` rechaza otro namespace, `kube.New` rechaza `ns != aqs-test` (hay prueba) y el cuerpo de la API rechaza el campo `namespace` (400).
- **Token:** 0 apariciones en `gr.log` y en `/metrics`. Una petición con token distinto da 401, y un cuerpo con dos JSON da 400.
- **Grace de 24 h:** `>` estricto. 23 h 59 no cierra, 24 h exactas no cierra, 24 h 01 cierra. Una sesión fresca protege el warm. La sesión queda `incompleta` con plan y estado, releída de `SessionStore`.
- **`doc_test.go`:** eliminado; `git ls-files` solo muestra `doc.go`. Se usa `rapid`.

## Tareas candidatas
- Esquema de evento `warm.quarantined` (ya anotado en la tarea).
- Cómo se lanza el Job `reset-{run}`: token y variables para la ejecución en pod, o decidir que el reset es en proceso (U2-T07).
- Contrato de `warm-state` compartido T03/T06: `updated_at` en el esquema y quién lo escribe.
- API REST de `go-reset` al OpenAPI.
- Imagen sin script de baseline: `RESET_BASELINE_SCRIPT` debe montarse o empaquetarse (U2-T07).

VEREDICTO: NO-VERDE
NARANJA|services/go-reset/cmd/go-reset/main.go:99-103|RESET_BACKEND=fake sin acotar en el binario de producción (reset.verified sin verificar nada)
NARANJA|services/go-reset/internal/adapters/kube/kube.go:106-120|app-ready no confirma el rollout tras RestartApp (puede leer el pod viejo)
NARANJA|services/go-reset/cmd/go-reset/main.go:97-98 (y otros)|Literales en puntos de decisión: warm_id, baseline_version, timeouts, valores del Job
INFORME: revisiones/U2-T06/ronda-1.md
