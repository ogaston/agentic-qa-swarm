# Ronda 1 — U2-T05b

VEREDICTO: NO-VERDE

Los 6 criterios pasan con mi ejecución y el diseño es correcto. Lo que bloquea es un NARANJA: las ramas fail-closed nuevas del marcador `started-at` no tienen prueba que las mate, y la tabla de la bitácora afirma lo contrario.

## Criterios de aceptación, verificados por mí
Todos los comandos se corrieron en el worktree, a sha 9aab47f. El worktree quedó limpio (`git status --short | wc -l` da 0).

| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Suite en verde con `-race` | `go test -race -count=1 ./... \| grep -c FAIL` y `... -v \| grep -c '--- PASS'` | pasa: `0` y `177` (se pedía ≥175) |
| 2 | Plazo por corrida | `-run 'Runner(RunTimeoutNotStretched\|ExpiredJobsKeptUntilSettled)'` | pasa, 2 PASS |
| 3 | Lanzamiento parcial | `-run 'Runner(InvalidFlowCreatesNoJobs\|PartialLaunchLeavesNoJobs)'` | pasa, 2 PASS |
| 4 | Pruebas de F-03 | `-run 'RunDone(...)\|EvidenceMarkerLast\|JobDeadline...\|ResultStatusPassed\|QuotaOncePerRun\|RealWiring(Log\|NS)'` | pasa, 9 PASS (≥8) |
| 5 | `logs_unavailable` y README | `-run LogsUnavailable`; `grep -c -i 'no implica' README.md` | pasa: PASS y `1` |
| 6 | Higiene, imagen y alcance | `go vet` (con y sin `-tags minio,contract`), `gofmt`, build de la imagen, `git status`, diff fuera de alcance | pasa: `ok`, `65532:65532`, `0`, `0` |

No corrí `-tags minio -run EvidenceS3`. Este criterio no figura en CA-1..CA-6, solo en el plan de pruebas.

**Rojo primero, reproducido por mí.** Copié el `launcher.go` de `main` junto a las pruebas nuevas. Fallan las 5 pruebas, con estos mensajes:
- `jobsCreados=6 con RunTimeout=3m y FlowTimeout=1m (máximo 3)`
- `el Job vencido se borró sin evidencia escrita`
- `jobs vivos 1, creados 1`
- `jobs vivos: 1`
- `result.json` sin `logs_unavailable`

**Mutaciones corridas por mí.** Usé una copia con `ulimit -v 4000000` y `go test -timeout 60s`. Para que no dieran falsos rojos copié `contracts/` y `policy/` junto a la copia. La mutación nula no falló, así que la línea base está verde. Cada mutación que menciono falla en la prueba indicada:

| Mutación | Prueba que la mata |
|---|---|
| M1 | `RunDonePartialEvidence` |
| M2 | `RunDonePublishErrorRetried` |
| M3a (quitar la guarda en `runnersStep`) | `RunDoneOncePerRun` |
| M3b (quitar la guarda en `publishDone`) | `RunDoneOncePerRun` y otras 4 pruebas |
| M4 | `EvidenceMarkerLast` y `EvidenceWriteFailure...` |
| M5 | `JobDeadlineEqualsFlowTimeout` y `TestRunnerProperty` |
| M6a | `ResultStatusPassed` |
| M6b | `QuotaOncePerRun` y `RunTimeoutNotStretched` |
| M6c | `RealWiringLog` |
| M6d | `RealWiringNS` |
| F01b (borrar el vencido aunque `settle` falle) | `ExpiredJobsKeptUntilSettled` |
| F02a (no limpiar los creados) | `PartialLaunchLeavesNoJobs` |
| F02b (crear mientras se construye) | `InvalidFlowCreatesNoJobs` y `PartialLaunchLeavesNoJobs` |
| LU1 | `LogsUnavailable` |
| X4 (ola sin tope) | `MaxParallelNeverExceeded` y `RunTimeoutNotStretched` |

Las 8 mutaciones exigidas de F-03 mueren.

## Hallazgos

### F-01 · NARANJA · `runner/launcher.go` (`readStart` y bloque `!started`) · Ramas fail-closed del marcador sin prueba, y la bitácora afirma lo contrario
Apliqué dos mutaciones sobre el código nuevo que arregla F-01. Con ambas pasa la suite completa de `runner`, `runctl` y `cmd`, es decir, sobreviven:
- **X2:** ignorar el fallo de `PutVerified(startKey)`. La mutación sustituye el `return errors.New("no se pudo registrar el inicio...")` por `_ = 0`.
- **X1:** que `readStart` trate un error de lectura distinto de `ErrNotFound` como «aún no empezó» (`return time.Time{}, false, nil`).

La rama «inicio ilegible» tampoco tiene prueba.

La tabla del barrido de clase en la bitácora dice que «marcador de inicio no escribible» está cubierto por «EvidenceWriteFailure… («put falla») y wiring (fail-closed, sin Job)». Eso es falso. `notMarker` y el `FailPut` de esas pruebas dejan pasar `started-at` a propósito, y `TestEvidenceSecretsNotLogged` también. Ninguna prueba hace fallar la escritura del marcador. Es una afirmación sin evidencia en la bitácora.

X1 no es teórica. Un error transitorio al leer el marcador haría que el controlador:
- vuelva a consultar la cuota (viola «una vez por corrida»);
- reescriba `started-at` con `now`, moviendo el origen del plazo.

Es exactamente la clase de F-01: un origen de plazo que avanza.

Además, si `PutVerified` falla después de `authorize`, la cuota ya se consultó y se vuelve a consultar en el reintento.

**Barrido de clase.** Hay tres ramas de la misma clase, todas sin prueba:
1. fallo de escritura del marcador;
2. fallo de lectura distinto de not-found;
3. marcador con formato ilegible.

**Arreglo exigido.** Tres pruebas, con sus mutaciones pegadas en la bitácora. Cada una debe comprobar que no se crea ningún Job y que el marcador no se reescribe:
1. `FailPut` solo para `started-at`;
2. `GetErr` solo para `started-at`;
3. marcador con contenido corrupto.

Debe corregirse la fila de la tabla.

### F-02 · AMARILLO · `runner/runner_test.go` (`aliveWithinDeadline`, `TestRunnerProperty`) · Debilitamiento de la invariante: aceptable, con una salvedad
El cambio es necesario, y lo confirmé. Con `r.active(t)` como antes, `TestRunnerProperty` falla (`2 activos con máximo 1`). El caso: un Job vencido con evidencia caída se conserva por diseño, y la propiedad ya no podía contarlo.

Emular que Kubernetes mata el pod a `activeDeadlineSeconds` es defendible. El controlador usa el mismo criterio (`now-created > FlowTimeout`) y tampoco cuenta esos Jobs como activos.

La salvedad: un Job vencido por el plazo de la **corrida** (`expired = runExpired`) tiene un pod que puede seguir vivo hasta `FlowTimeout`. Si su `settle` falla, el Job se conserva sin que Kubernetes lo mate todavía. El comportamiento anterior era borrarlo siempre.

El acotamiento es suficiente por ahora: con `runExpired` no se lanza nada nuevo y la corrida termina en Done con `FailReason`, de modo que el reset limpia. No exijo cambio. Conviene dejarlo escrito en la bitácora y en el README, y no verifiqué que el reset borre ese Job.

### F-03 · AMARILLO · `runner/launcher.go` (limpieza de `created`) y `runner/plazos_test.go` (`notMarker`) · Dos menores
- **Mutación X3.** Si `created` incluyera los Jobs con `AlreadyExists`, la limpieza borraría Jobs adoptados que no creó esta llamada. Hoy el código es correcto, pero ninguna prueba lo protege.
- **Nombre `notMarker`.** En este repo «marcador» significa `result.json`, y `started-at` es el «marcador de inicio». Hoy `notMarker` filtra `logs.txt` y `result.json`, es decir, hace lo contrario de lo que sugiere su nombre.

## Puntos que pidió revisar el orquestador
- **Origen del plazo en `runs/<run>/started-at`:** decisión correcta y justificada. Se escribe una vez, se lee de vuelta por hash y arregla además la doble consulta de cuota cuando `len(jobs)==0`. La salvedad está en F-01.
- **Cuota por corrida («primera ola» = no hay marcador):** correcta y matada por M6b, salvo la ventana descrita en F-01.
- **Exención de «hash distinto» quitada:** correcto y más estricto. `EvidenceMarkerLast` y `EvidenceWriteFailure` pasan sin exención.
- **Conteo de activos en `TestRunnerProperty`:** ver F-02.
- **Lanzamiento parcial:** `Plan` y `BuildJob` se validan antes de crear nada, y la limpieza va después de un `CreateJob` fallido. Las mutaciones F02a y F02b mueren.

## Tareas candidatas (fuera de alcance)
- `evFail` sigue en memoria. Tras un reinicio, el flujo se re-liquida. La bitácora ya lo señala.
- Verificar que el reset de la corrida borre los Jobs vencidos que se conservan por evidencia caída.

VEREDICTO: NO-VERDE
NARANJA|runner/launcher.go (readStart y !started)|Ramas fail-closed del marcador started-at sin prueba; las mutaciones X1/X2 sobreviven y la tabla de la bitácora afirma cobertura inexistente
AMARILLO|runner/runner_test.go (aliveWithinDeadline)|Invariante de TestRunnerProperty relajada: aceptable, documentar el Job vencido por plazo de corrida conservado hasta el reset
AMARILLO|runner/launcher.go (created) y runner/plazos_test.go (notMarker)|AlreadyExists contado como creado (mutación X3 sobrevive) y nombre confuso notMarker
