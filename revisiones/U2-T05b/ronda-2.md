# Ronda 2 — U2-T05b

VEREDICTO: VERDE

Corrí CA-1 a CA-6 y las mutaciones yo mismo, sobre el sha 7836c31. No queda ningún ROJO ni NARANJA. El worktree quedó limpio (`git status --short | wc -l` da 0).

## Criterios de aceptación, verificados por mí
| # | Criterio | Resultado |
|---|---|---|
| 1 | `go test -race -count=1 ./... \| grep -c FAIL` y conteo de `--- PASS` | pasa: `0` y `181` (se pedía ≥175) |
| 2 | `RunTimeoutNotStretched` y `ExpiredJobsKeptUntilSettled` | pasa, 2 PASS |
| 3 | `InvalidFlowCreatesNoJobs` y `PartialLaunchLeavesNoJobs` | pasa, 2 PASS |
| 4 | Pruebas de F-03 (`RunDone*`, `EvidenceMarkerLast`, `JobDeadlineEqualsFlowTimeout`, `ResultStatusPassed`, `QuotaOncePerRun`, `RealWiring(Log\|NS)`) | pasa, 9 PASS (≥8) |
| 5 | `LogsUnavailable` y `grep -c -i 'no implica' README.md` | pasa: PASS y `1` |
| 6 | `go vet` (con y sin `-tags minio,contract`), `gofmt`, imagen, `git status`, diff fuera de alcance | pasa: `ok`, `65532:65532`, `0`, `0` |

El diff de la ronda 2 toca solo `plazos_test.go`, `runner_test.go` (un renombrado), el README, la bitácora y los informes. No hay código de producción nuevo, así que no hay riesgo de desborde.

## Hallazgos de la ronda 1, verificados
Apliqué cada mutación sobre una copia de la ronda 2, con `ulimit -v 4000000` y `go test -timeout 60s`.

| Hallazgo | Mutación | Prueba que la mata |
|---|---|---|
| F-01 | X2 (ignorar el fallo de escritura de `started-at`) | `TestRunnerStartMarkerWriteFailure` |
| F-01 | X1 (error de lectura distinto de not-found tratado como «aún no empezó») | `TestRunnerStartMarkerReadError` |
| F-01 | X1b (marcador ilegible tratado como «aún no empezó») | `TestRunnerStartMarkerCorrupt` |
| F-03 | X3 (`AlreadyExists` contado como creado) | `TestRunnerPartialLaunchKeepsAdoptedJobs` |

- **F-01: resuelto.** Las tres pruebas nuevas exigen 0 Jobs creados y el marcador sin reescribir. Las dos de lectura exigen además que la cuota no se vuelva a consultar. Esa exigencia no la puse a prueba con una mutación propia de la cuota.
- **F-01, fila falsa de la bitácora:** corregida.
- **F-02:** la salvedad quedó documentada en el README y en la bitácora. El README anota también que la cuota ya consultada puede consultarse otra vez si falla la escritura del marcador. La verificación de que el reset borre esos Jobs queda como tarea candidata.
- **F-03: resuelto.** `notMarker` pasó a llamarse `isFlowEvidenceKey`. El Job adoptado de `PartialLaunchKeepsAdoptedJobs` se crea sin etiquetas a propósito, para forzar el camino de `AlreadyExists`.

## Barrido de la misma clase (marcador de inicio y cuota)
- **Y1** (quitar la consulta de cuota de la primera ola): muere con `QuotaOncePerRun` y `QuotaDenied`.
- **Y2** (reescribir el marcador en cada ola): muere con `RunTimeoutNotStretched` y `QuotaOncePerRun`.
- **Y3** (escribir el marcador con `Put` simple, sin leerlo de vuelta): sobrevive. Ver el AMARILLO de abajo.

## Hallazgos que quedan
### F-04 · AMARILLO · `runner/launcher.go`, escritura de `started-at` · La lectura de vuelta por hash del marcador no tiene prueba
La bitácora dice que el marcador se «lee de vuelta por hash» (usa `PutVerified`). Si se cambiara por un `Put` simple (mutación Y3), ninguna prueba falla. Falta un caso con `MemEvidence.Corrupt` aplicado solo a `started-at`. El fallo de escritura sí está cubierto; este caso es secundario y no bloquea.

## Tareas candidatas (fuera de alcance)
- Verificar que el reset de la corrida borre los Jobs vencidos que se conservan por evidencia caída.
- `evFail` sigue en memoria; tras un reinicio el flujo se re-liquida de forma idempotente.

VEREDICTO: VERDE
AMARILLO|runner/launcher.go (started-at)|La lectura de vuelta por hash del marcador de inicio no tiene prueba (la mutación Y3 sobrevive)
