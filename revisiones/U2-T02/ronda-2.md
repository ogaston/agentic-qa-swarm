# Ronda 2 — U2-T02

VEREDICTO: NO-VERDE

Los cuatro hallazgos de la ronda 1 que pedías (F-01, F-02, F-03, F-04) están corregidos de verdad. Hay dos NARANJAS nuevos, en el borde de los errores de disco (`ErrPersist`). El worktree quedó limpio (`git status --short | wc -l` = 0), en `03d1f7e`.

## Criterios de aceptación, verificados por mí (comandos frescos, `-count=1`)
| # | Qué corrí | Resultado |
|---|---|---|
| 1 | los tres comandos de CA-1 | pasa: `49`, `0`, `4` |
| 2 | `go test -run 'Gate(Deny\|Unknown\|Error)\|Without(...)'` | pasa: 7 `--- PASS` en `runctl` |
| 3 | mi `up` (`go-identity` y `go-governance` reales) y `go test -tags contract -run Contract` | pasa: 4 `--- PASS`, ningún `FAIL`/`SKIP` |
| 4 | `up`, curl, y `ajv-cli` con el esquema `Run` extraído de `contracts/` | pasa: `{"id":"r-1","state":"rehearsing","trace_id":"4bf9..."}` → `run.json valid`; luego `404 401 503` |
| 5 | `go test -run 'Resume\|Journal\|Idempotent'` | pasa: 6 pruebas, ningún `FAIL` |
| 6 | los 4 comandos literales | pasa: `rc=1` ×4 |
| 6b | configuración completa (todas las variables) | la valla discrimina. `RUN_ENV=prod`, `Prod`: "no se permite con RUN_ENV=prod" (rc=1). Fake sin `RUN_ALLOW_FAKE_PHASES`: "exige RUN_ALLOW_FAKE_PHASES=true" (rc=1). `ftp://`: "debe ser una URL http(s)" (rc=1). Control positivo con `staging`: sigue vivo (rc=124 por timeout) |
| 7 | `up`, curl a `/metrics`, grep | pasa: 4 nombres de métrica y `0 0 0` |
| 8 | `go list`, `docker build`, `list-services.sh` | pasa: `0`, `65532:65532`, `1` |
| 9 | vet (con y sin `-tags contract`), `-race`, gofmt, status, diff fuera de alcance | pasa: `ok`, `0`, `0` |

## Verificación de las correcciones
- **F-01 (reset sin tope).** Mi `TestResetExhaustGateDown` (`scratchpad/probe/p_test.go`) pasó de 28 lanzamientos y 26 handoffs a `reset launches=3 attempts=map[reset:3] handoffs=1`. Corregido.
- **F-02 (`rehearsal.*` fuera de `rehearsing`).** Mi `TestEarlyRehearsalPassed` pasó de `done` con `ensayo_passed=true` a `state=rehearsing`. Corregido.
- **F-04 (`trace_id` duplicado), caja negra.** Con el servicio real, 10 líneas de `rc.log` leídas con un parser que detecta claves duplicadas: 0 duplicadas y 0 con `trace_id` vacío dentro de una corrida. Corregido.
- **F-03 (pruebas), con mutaciones propias.** Las hice sobre copias en `scratchpad/m3/*`, sin tocar el repo. Dos fallos de la copia (`TestStatesMatchOpenAPIEnum` y `TestExhaustiveStateByState`) son artefactos de la ruta relativa a `contracts/`; también fallan en la copia sin mutar.
  - M1, revertir el tope de reset en `Drive`: ponen rojas `TestResetExhaustedWithGateDownNeverRelaunchesNorDuplicatesHandoff`, `TestEveryPhaseExhaustedWithGateDownLaunchesExactlyThree` y la propiedad.
  - M2, revertir el filtro `State != Rehearsing`: ponen rojas `TestRehearsalEventOutsideRehearsingIsDroppedAndNeverSetsFact` y `TestDroppedEventIsCounted`.
  - M4, quitar el `trace_id` del contexto en `obs`: pone roja `TestLogLinesHaveSingleTraceIDWithRunTrace`.
  - M6, reabrir el tope solo para `Reporting`: pone roja `TestEveryPhaseExhaustedWithGateDownLaunchesExactlyThree`.
  - M7, dejar de contar el intento de `rehearse`: ponen rojas `TestRehearsalFailedRetriesThenHandoff` y el barrido.
  - **M5, revertir el manejo de `ErrPersist` en `failRun`: no falla ninguna prueba nueva.** Ver F-02 abajo.
- **El barrido `TestEveryPhaseExhaustedWithGateDownLaunchesExactlyThree` sí cubre las seis fases.** Son `deploy`, `infer`, `run`, `reset` y `report`, cada una con el gate caído en su estado, más `rehearse` por `rehearsal.failed`. Cada caso exige exactamente 3 lanzamientos y un único handoff. M6 y M7 lo ponen rojo.
- **(c) la corrida no avanza con el gate roto.** Probé cada estado no terminal (de `confirmed` a `reporting`) con gate caído y con gate denegando, 30 ticks. Siempre se queda en su estado, nunca llama a un tercer lanzamiento y hace un solo handoff cuando deniega.

## Hallazgos

### F-01 · NARANJA · `runctl/controller.go` `Drive` (rama de lanzamiento) + `Store.Save` · fase lanzada y guardado fallido: relanzamiento sin tope
`Drive` llama a `Phases.Launch` y después a `Store.Save` para marcar `Launched`. Si el guardado falla, devuelve el error. Como `Launched` y `Attempts` no se persistieron, el siguiente tick vuelve a lanzar. No hay tope.

Prueba mía, `TestDiskDownLaunches` (`scratchpad/probe/p_test.go`): corrida en `inferring`, `Save` fallando, 20 ticks → `launches=map[deploy:1 infer:20]`.

Es el mismo riesgo que F-01 de la ronda 1 (un Job por tick), con otro disparador: el disco caído. Contradice «nunca hay reintento infinito ni un tercer lanzamiento». En las fases reales (T03 a T06) serían runners o resets creados una y otra vez contra el entorno warm.

Corrección esperada, a elección del codificador: persistir la intención de lanzar antes de lanzar, o contar el lanzamiento en memoria, o tratar el fallo de guardado tras un lanzamiento exitoso como un intento más. Más una prueba con un `RunStore` que falla.

### F-02 · NARANJA · `runctl/regress_test.go` `TestPersistErrorIsNotADenial` y `controller.go` (`failRun`, `Drive`) · la corrección de F-07 no está sostenida por ninguna prueba y no cubre el error intermitente
- **La prueba no sostiene la corrección.** M5 (quitar `|| errors.Is(err, ErrPersist)` en `failRun`) deja toda la suite verde, salvo los dos fallos de ruta de la copia. En la prueba el `Save` falla siempre, así que `failRun` sale por el primer `Save(FailReason)` y nunca llega a la rama de `ErrPersist`. La rama solo se alcanza cuando `FailReason` ya está fijada y falla el `Save` de `transition`.
- **El error de disco intermitente mata la corrida.** Si falla solo el `Save` de una transición permitida, `Drive` pasa el error a `failRun`, que guarda `FailReason` y lleva la corrida a `failed`. Mi `TestTransientDiskOnAdvance`: `confirmed` → `err=no se pudo persistir: disco state=failed failReason="no se pudo persistir: disco"`, y tras 5 ticks sigue `failed`. Es fail-closed, pero la respuesta del codificador dice que el error de disco «se reintenta», y eso solo vale dentro de `failRun`.

Corrección esperada: una prueba que falle con M5, y decidir qué hace `Drive` con `ErrPersist` en el avance normal.

### F-03 · AMARILLO · `runctl/regress_test.go` propiedad · cotas débiles
- La cota de handoffs `len(al.Calls) > 4*1+len(al.Calls)/(len(al.Calls)+1)` es casi `> 4` y no expresa «un handoff por fase agotada».
- La propiedad excluye `PhaseRehearse` del tope y no hace aleatorios sus fallos. Ese caso lo cubre el barrido determinista, así que no es grave.

### F-04 · AMARILLO · `Apply` (`rehearsal.*`) · se acepta el hecho en `rehearsing` aunque la fase aún no se lanzó
`Apply` solo comprueba `State == Rehearsing`, no `Launched[rehearse]`. Entre la transición y el primer `Drive` de ese estado un evento fijaría `ensayo_passed` antes de lanzar el ensayo. La ventana es de un tick. Mi petición de ronda 1 sugería «con la fase lanzada».

### F-05 · AMARILLO · `Drive` en `resetting` con gate caído · dos llamadas al gate por tick
`resetting` sin `FailReason` con el gate caído hace `transition(next)` y luego `failRun`, es decir, dos llamadas por tick (60 llamadas en 30 ticks). No avanza ni lanza nada; solo es ruido en `aqs_gate_calls_total`.

## Sobre (b): ¿se pierde un `rehearsal.passed` legítimo justo antes de `rehearsing`?
Mi `TestPassedJustBeforeRehearsing` lo reproduce: con `passed` en `inferring`, el evento se descarta (queda marcado como visto, sin reaplicarse en un replay) y la corrida llega a `rehearsing` y espera para siempre, sin handoff (`handoffs=0`).

No lo cuento como defecto de esta ronda. El productor legítimo de `rehearsal.passed` es la fase de ensayo, que el controlador lanza **dentro** de `rehearsing`, así que un `passed` anterior es un evento fuera de secuencia, no uno legítimo. Aun así deja la corrida esperando sin límite. Va como candidata (abajo). El descarte queda visible en el log y en `aqs_events_dropped_total`.

## Tareas candidatas (defectos reales fuera de alcance)
- Tiempo máximo de espera de `rehearsal.passed`, con handoff y salida segura. Ya la tenía anotada el codificador.
- HMAC y ancla del último `seq` en el diario, y validación de legalidad al reproducirlo (F-05 de la ronda 1).
- Hacer visible `halted` en `GET /runs/{id}`, con reanudación por operador (cambia el esquema `Run`).
- Redactar CA-6 con configuración completa: el literal de la tarea pasa por variables faltantes, no por la valla.

## Rutas de transcripciones largas
- `scratchpad/probe/p_test.go` (todas mis pruebas de reproducción, incluidas las nuevas de esta ronda)
- `scratchpad/m3/` (copias con las mutaciones m1, m2, m4, m5, m6, m7)

VEREDICTO: NO-VERDE
NARANJA|services/go-run-controller/runctl/controller.go (Drive, Launch seguido de Save fallido)|Con el guardado fallando, la fase se relanza en cada tick sin tope (infer: 20 lanzamientos en 20 ticks)
NARANJA|services/go-run-controller/runctl/regress_test.go (TestPersistErrorIsNotADenial) y controller.go (failRun)|La corrección de ErrPersist no la sostiene ninguna prueba (la mutación M5 sobrevive) y un error de disco intermitente deja la corrida en failed
INFORME: revisiones/U2-T02/ronda-2.md
