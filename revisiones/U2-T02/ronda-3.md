# Ronda 3 — U2-T02

VEREDICTO: NO-VERDE

Las correcciones de la ronda 2 (F-01 a F-05) están bien hechas y las sostienen pruebas que se ponen rojas con mutaciones. El barrido propio de la familia «efecto externo + fallo del almacén» no encontró ningún caso de más de 3 lanzamientos, handoff duplicado ni avance sin allow. Encontré tres NARANJAS nuevos en el borde del disco: dos fuera de `Drive` (el diario y la entrada de eventos) y uno de cobertura. Worktree limpio: `git status --short | wc -l` = 0, HEAD `062f531`.

## Criterios de aceptación, verificados por mí (frescos, `-count=1`)
| # | Comando que corrí | Resultado |
|---|---|---|
| 1 | los tres comandos de CA-1 | pasa: `59`, `0`, `4` |
| 2 | `go test -run 'Gate(Deny\|Unknown\|Error)\|Without(...)'` | pasa: 7 `--- PASS` |
| 3 | `up` (go-identity y go-governance reales) + `go test -tags contract -run Contract` | pasa: 4 `--- PASS`, sin `FAIL`/`SKIP` |
| 4 | `up`, curl, `ajv-cli` con el esquema `Run` extraído de `contracts/` (comparado con el YAML) | pasa: `run3.json valid`; `404 401 503` |
| 5 | `go test -run 'Resume\|Journal\|Idempotent'` | pasa: 6 pruebas |
| 6 | 4 comandos literales + configuración completa | `rc=1` ×4. Completa: «no se permite con RUN_ENV=prod», «exige RUN_ALLOW_FAKE_PHASES=true», «debe ser una URL http(s)». Control positivo con `staging`: vivo (rc=124) |
| 7 | `up`, `/metrics`, grep | pasa: 4 métricas (+ `aqs_persist_errors_total`) y `0 0 0` |
| 8 | `go list`, `docker build`, `list-services.sh` | pasa: `0`, `65532:65532`, `1` |
| 9 | vet (con y sin `-tags contract`), `-race`, gofmt, status, diff fuera de alcance | pasa: `ok`, `0`, `0` |

## Verificación de las correcciones de la ronda 2
Mutaciones mías sobre copias en `scratchpad/r3m/services/*` (no toqué el repo). Cada fila indica qué pruebas se ponen rojas.
| Mutación | Pruebas rojas |
|---|---|
| M5: `failRun` sin `ErrPersist` | `TestPersistErrorOnPendingExitIsNotADenial`, `TestHandoffAtMostOncePerPhaseWhenSaveFails` |
| M-adv: `ErrPersist` del avance normal va a `failRun` | `TestPersistErrorOnAdvanceDoesNotFailRun` |
| Sin `Save(Started)` antes de `Launch` | 4 pruebas, incluida la propiedad |
| Quitar `Started>=3` de `exhausted` | 3 pruebas |
| Handoff antes de guardar `HandedOff` | `TestHandoffAtMostOncePerPhaseWhenSaveFails` + propiedad |
| Quitar `Observer.PersistError()` | `TestPersistHealthAndMetric` |
| F-04 sin `Launched[rehearse]` | `TestRehearsalEventBeforeLaunchIsDropped` |
| F-05 (doble llamada al gate) | `TestResettingGateDownOneGateCallPerTick` |
| **Quitar `PersistHealthy` del chequeo `persist` de `/readyz` en `main.go`** | **ninguna: ver F-03** |

**F-01 de ronda 2 (`Started` + Save antes de `Launch`).**
- Proceso muerto entre Save y Launch: la fase NO queda «lanzada» sin lanzarse ni atascada. `Launched` sigue en false, el siguiente tick incrementa `Started` y lanza. Se pierde a lo sumo un intento. Si `Started` ya era 3, la corrida falla con handoff sin haber lanzado: es fail-closed y alerta.
- Proceso muerto entre Launch y el Save de `Launched`: la fase se relanza (a lo sumo 3 lanzamientos en total).
- El trade-off es coherente con fail-closed: nunca más de 3, nunca atasco. Exige que los lanzadores reales sean idempotentes (F-05, AMARILLO).

**F-02, caja negra con disco lleno.** Service real en tmpfs de 8k dentro de `unshare -rm` (`scratchpad/bb_disk.sh`).
- Con el disco lleno la corrida no avanza ni falla ni hace handoff, y reintenta cada tick.
- `aqs_persist_errors_total` sube a 9.
- `/readyz` responde `{"checks":{"persist":"fail"},"status":"unavailable"}`.
- El outbox queda con 1 sola línea de `run.done`.

**Gate caído, caja negra** (`scratchpad/bb_gate.sh`: gate real detenido 8 s y vuelto a levantar).
- La corrida se queda en `rehearsing`.
- 16 llamadas al gate en 8 s, una por tick.
- Diario: `started={deploy:1, infer:1, rehearse:1}`.
- Con el gate de vuelta pasa por `resetting` (un solo reset lanzado) y termina en `failed`.
- Sin handoff, y el reinicio posterior arranca bien.

## Barrido de clase propio
Harness en `scratchpad/r3/chaos_test.go`: 6000 semillas × 150 ticks, con el diario modelado como disco+memoria y reinicios del proceso en 6 puntos de crash:
- tras el gate;
- antes del efecto de cada fase;
- después del efecto de cada fase;
- antes de alertar;
- después de alertar;
- después de publicar.

Más fallos de Save (antes y después de persistir), gate caído o denegando, Launch fallido y Publish fallido.

Invariantes que se cumplen en las 6000 semillas (sin violaciones):
- ≤3 efectos de Launch por (corrida, fase);
- ≤1 handoff por (corrida, fase);
- todo cambio de estado en el disco va precedido de un allow del gate para ese par from→to;
- ninguna salida de un estado terminal;
- ningún Launch sin intención persistida ni con estado ajeno a la fase;
- `run.done` solo se publica en `done` sin `FailReason`;
- tras sanar, ninguna corrida queda atascada salvo `Halted`.

La única violación del harness es el handoff perdido (F-04). Dos artefactos de mi harness, descartados: un publisher no idempotente (el contrato del puerto lo exige) y la divergencia disco/memoria. Esa divergencia no es del controlador: es el defecto del diario de F-01.

## Hallazgos

### F-01 · NARANJA · `adapters/journal.go:117-133` (`Save`) · tras un Save fallido con escritura parcial, el siguiente Save exitoso deja el diario irrecuperable
`Save` hace `Write` y `Sync`, y solo actualiza `seq`/`last` si ambos salen bien. Si `Write` falla a medias (ENOSPC), quedan bytes huérfanos en el archivo. Si falla `Sync`, la línea queda en disco pero no en memoria. El reintento de la política de F-02 de ronda 2 («ErrPersist se reintenta») escribe a continuación o con el mismo `seq`. Resultado: el proceso sigue vivo y sano, pero el diario ya no encadena y el servicio **no vuelve a arrancar**.
- Prueba mía `TestJournalPartialWriteThenRecoveryCorrupts` (`scratchpad/r3/disk_test.go`, con el diario real sobre tmpfs):
  ```
  Save 24 falló: write .../runs.jsonl: no space left on device
  REPRODUCIDO: ...no puede arrancar tras un Save fallido y uno posterior exitoso: diario: línea 24 ilegible
  ```
- `TestControllerOnFillingDisk`: controlador real + diario real. Con el disco lleno `persistErr` ≠ nil; tras liberar, la corrida llega a `rehearsing`; al reabrir el diario: «línea 30 ilegible».
- Caja negra (`bb_disk.sh`): tras el episodio, el reinicio del servicio muere con `"error":"diario: última línea incompleta"`.
- Mismo origen: un `kill -9` en mitad de un `write` deja una cola truncada y el servicio se niega a arrancar para siempre. Esa cola nunca tuvo un `Save` exitoso detrás, así que descartarla sería seguro.

Corrección esperada: en cualquier error de `Write` o `Sync`, truncar el archivo al tamaño previo (o reabrirlo y revalidarlo) antes de aceptar otro `Save`; o marcar el diario como roto y rechazar los `Save` siguientes. Al arrancar, tolerar y descartar solo una cola sin `\n` final. Más una prueba que fuerce un `Write` parcial y compruebe `OpenJournal` después. La suite actual no tiene ninguna.

### F-02 · NARANJA · `cmd/go-run-controller/main.go:152-156` y `adapters/events.go:221-225` (`Poll`) · un evento aplicado con el disco caído se pierde hasta reiniciar
`Poll` avanza el offset al leer la línea. Si `Apply` devuelve `ErrPersist`, `loop` solo escribe un `warn`, y el evento no vuelve a entregarse. La política de ronda 2 («un error de disco no mata la corrida, se reintenta») cubre el avance normal pero no la entrada.
- Prueba `TestEventLostWhenSaveFailsDuringApply` (`scratchpad/r3/evloss_test.go`): `run.confirmed` con el disco caído, luego el disco vuelve y 30 ticks → `REPRODUCIDO: la corrida nunca se crea` (el archivo de eventos sigue teniéndolo).
- Para `rehearsal.passed` o `rehearsal.failed` perdidos, la corrida espera para siempre en `rehearsing`. Solo un reinicio los repone (el replay es idempotente por `Seen`).
- `/readyz` se recupera al siguiente Save exitoso, así que nada avisa del evento perdido.

Corrección esperada: avanzar el offset (ack) solo cuando `Apply` sale bien, o reencolar los eventos con error, con prueba.

### F-03 · NARANJA · `cmd/go-run-controller/main.go:116` + pruebas · el cableado `PersistHealthy` → `/readyz` no lo cubre ninguna prueba
Mi mutación `Mready` (el chequeo `persist` devuelve `nil`) deja verde toda la suite (`cmd/`, `runctl`, `adapters`, `internal`). Funciona (lo comprobé en caja negra), pero la respuesta de ronda 2 dice que «`/readyz` no-listo mientras falle el Save» está sostenido por pruebas, y el cableado de `main.go` no lo está. `TestPersistHealthAndMetric` solo mira `Controller.PersistHealthy()`.

### F-04 · AMARILLO · `runctl/controller.go:251-268` (`handoffOnce`) · un handoff es «a lo sumo una vez»: un crash entre Save(`HandedOff`) y el Alerter lo pierde
Con puntos de crash en mi harness, 537/6000 semillas terminan con `HandedOff[fase]` en el diario y ningún aviso emitido (con probabilidad de crash de hasta 15% por punto; en la realidad la ventana es de microsegundos). Para `Halted` (handoff `gate`) el humano nunca se entera: `halted` no es visible en `GET /runs`. Es el reverso coherente del «no duplicar handoff». Conviene documentarlo; no bloquea.

### F-05 · AMARILLO · `runctl/ports.go` (`PhaseLauncher`) · el puerto no declara que `Launch` debe ser idempotente
Un crash entre `Launch` y `Save(Launched)` repite el efecto (hasta 3 veces). Un crash entre `Save(Started)` y `Launch` gasta un intento sin lanzar. Un `Save(Launched)` fallido en el tercer intento produce un agotamiento espurio con handoff. Nada de eso es defecto del controlador, pero T03 a T06 necesitan saberlo: documentar en el puerto que `Launch` es idempotente por (corrida, fase, `Started[fase]`) y que la corrida llega con `Started` para nombrar el Job de forma determinista.

### F-06 · AMARILLO (candidata ya aceptada en ronda 1) · `adapters/journal.go` · diario truncado en frontera de línea
`TestTruncatedJournalAtLineBoundary`: quitando las últimas 12 de 13 líneas, el servicio arranca sin error, la corrida retrocede y `deploy` acumula 6 lanzamientos (tope 3 reiniciado). Es la candidata «HMAC y ancla del último `seq`» de ronda 1 (F-05); no la exijo aquí. La anoto porque pediste explícitamente el caso «reanudación desde diario truncado».

## Tareas candidatas
- HMAC y ancla del último `seq` + validación de legalidad al reproducir el diario (ya registrada).
- Compactación o rotación del diario: con el tmpfs de 8k se llenó solo; el diario crece sin cota.
- Hacer visible `halted` en `GET /runs/{id}` y reanudación por operador (ya registrada).
- Tiempo máximo de espera de `rehearsal.passed` (ya registrada).

## Rutas de pruebas de reproducción (todas fuera del repo, en el scratchpad)
- `r3/disk_test.go`: F-01 (se ejecuta con `unshare -rm` y `run.sh`).
- `r3/evloss_test.go`: F-02.
- `r3/chaos_test.go`: barrido de 6000 semillas.
- `r3/trunc_test.go`: F-06.
- `r3m/services/{M5,Madv,Mlaunch,Mready,Mmetric,MF04,MF05,Mhoff,Mcap}`: mutaciones.
- `bb_disk.sh` y `bb_gate.sh`: caja negra con el servicio real.

VEREDICTO: NO-VERDE
NARANJA|services/go-run-controller/adapters/journal.go:117-133 (Save)|Tras un Save fallido con escritura parcial, el siguiente Save exitoso deja el diario irrecuperable y el servicio no vuelve a arrancar
NARANJA|services/go-run-controller/adapters/events.go:221 (Poll) y cmd/go-run-controller/main.go:152|Un evento aplicado con el disco caído se pierde (el offset ya avanzó): run.confirmed nunca crea la corrida hasta reiniciar
NARANJA|services/go-run-controller/cmd/go-run-controller/main.go:116|El cableado PersistHealthy -> /readyz no lo cubre ninguna prueba (la mutación sobrevive)
INFORME: revisiones/U2-T02/ronda-3.md
