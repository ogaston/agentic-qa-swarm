# Ronda 1 — U2-T05

VEREDICTO: NO-VERDE

Los 8 criterios de aceptación pasan en mi ejecución fresca. Lo que bloquea son dos defectos funcionales y un grupo de pruebas que no matan mutantes de invariantes centrales. Los tres se arreglan con poco código y pocas pruebas.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | ≥65 PASS acumulado y 0 FAIL con `-race` | `go test -race -v ./...` (con `ulimit -v`) | pasa: 163 PASS, 0 FAIL. Solo se modificó `real_test.go`, y solo la firma de `buildConfig` y las variables de entorno. |
| 2 | Runner sin LLM ni egress | `render-runner-job`, `grep -v automount\|grep -c`, yq, kubeconform, conftest (podman) | pasa: `0`; etiquetas `app.kubernetes.io/name`, `aqs.io/{flow-id,phase,role,run-id}`; `Valid: 1, Invalid: 0`; `23 tests, 23 passed, 0 failures`; `RunnerNoSensitiveEnv` PASS |
| 3 | Sin ensayo no hay runners; el fallo termina en reset | `go test -run 'Runner(NeedsEnsayo\|Timeout\|QuotaDenied\|FailureToReset)'` | pasa |
| 4 | Evidencia a MinIO real | `go test -tags minio -run EvidenceS3` | pasa: `TestEvidenceS3RoundTripAndMissingBucket` (0.82s, contenedor real), sin SKIP |
| 5 | `run.done` válido | `go test -run RunDone` | pasa; el volcado con ajv-cli lo da la bitácora |
| 6 | Recorrido con fakes | `go test -run 'Flow(ToRunDone\|MultiFlow)'` | pasa |
| 7 | Secretos fuera de logs y /metrics | `EvidenceSecretsNotLogged`; binario con `EVIDENCE_ENDPOINT=ftp://x` | pasa: `rc=1` |
| 8 | Imagen, dependencias, alcance | `go list -deps\|grep -c minio-go`; build de imagen; vet y gofmt; `git status`; diff fuera de alcance | pasa: `15`, `65532:65532`, `ok`, `0`, `0`. Worktree limpio al terminar. |

## Hallazgos que bloquean

### F-01 · NARANJA · `runner/launcher.go` (`first` / `runExpired`) · el plazo por corrida se estira cuando vencen Jobs
El origen del plazo de la corrida es el `created-at` más antiguo entre los Jobs **vivos**, pero el controlador elimina los Jobs vencidos. Con eso el origen avanza cada vez. Mi repro con `MaxParallel=1`, `FlowTimeout=1m`, `RunTimeout=3m` y 6 flujos que vencen lanzó los 6 Jobs, hasta t=5m5s. El plazo de 3 minutos nunca actuó.
- El estado correcto era lanzar 3 como máximo; ocurrió `jobsCreados=6`.
- El peor caso queda acotado por `flujos × FlowTimeout`, no por `RUNNER_RUN_TIMEOUT_SECONDS`.
- `TestRunnerRunTimeoutBoundsPendingFlows` solo cubre el caso en que el primer Job sigue vivo.
- Arreglo simple: no borrar los Jobs vencidos hasta que la corrida termine, o derivar el origen de algo que persista, por ejemplo una marca en el almacén de evidencia.
- La misma raíz, estado derivado de Jobs que se borran, explica F-02-b (AMARILLO más abajo).

### F-02 · NARANJA · `runner/launcher.go` bucle de creación + `runctl.exhaust` · un fallo parcial de lanzamiento deja Jobs vivos al salir por reset
Si falla el 2.º `CreateJob`, `Plan` o `BuildJob`, el 1.er Job ya existe. `Launch` devuelve error y tras 3 intentos la corrida va a `resetting` y luego `failed`, con el runner vivo. Repro: `estado final: failed, jobs vivos: 1`.
- Rompe «nunca queda un Job colgado» y «ningún Job queda sin dueño» (CA-3).
- Además el runner puede seguir golpeando el warm después del reset verificado. Eso está acotado por `activeDeadlineSeconds`, pero es justo la higiene de reset que protege la plataforma.
- Causa determinista alcanzable: un `flow_id` inválido en el 2.º flujo (por ejemplo, con mayúsculas) hace fallar `BuildJob` tras haber creado el 1.º.
- Arreglo simple: construir y validar todos los Jobs (`Exec.Plan` + `BuildJob`, ambos puros) antes de crear ninguno. Además, ante un error de `CreateJob`, borrar de forma best-effort los Jobs creados en esa llamada.

### F-03 · NARANJA · pruebas que no matan mutantes de invariantes declaradas (barrido de clase)
Apliqué mutaciones propias, siempre sobre una copia en el scratchpad, con `ulimit -v 4000000` y `go test -timeout 60s`. Las siguientes **sobreviven a las 163 pruebas** y todas corresponden a invariantes declaradas. Con mis repros, cada una pasa a rojo.

| Mutación | Invariante que no está probada |
|---|---|
| `controller.go`: `if out.FailReason != "" \|\| len(out.URIs)==0` → solo `len==0` | Con 2 flujos y uno sin evidencia, `run.done` se publicaría con evidencia parcial. Invariante (4). |
| `controller.go`: se traga el error de `Publish` en `runnersStep` | `run.done` se perdería al fallar el publicador. Tampoco hay prueba de reintento. |
| `controller.go`: se quita la guarda `if r.DonePublish {return true,nil}` de `runnersStep` | Publicación única tras reinicio entre `run.done` y la salida de running. |
| `launcher.go`: se escribe `result.json` ANTES de `logs.txt` | El marcador-al-final es el diseño de D3 y no tiene prueba que lo mate. Si `logs.txt` falla, el marcador existiría y apuntaría a evidencia inexistente. En `TestEvidenceWriteFailure…` el caso «hash distinto» está además exento de la comprobación de ausencia de `result.json`. |
| `launcher.go`: `cfg.DeadlineSeconds = 99999` | «`activeDeadlineSeconds` igual al timeout del workflow» no se comprueba a través del `Launcher`. |
| `launcher.go`: `ResPassed` → `ResFailed` | Ninguna prueba lee `"status":"passed"` de `result.json`. |
| `launcher.go`: `if len(jobs)==0` → `if true` | «La cuota se consulta una sola vez por corrida» no se prueba con varias olas. |
| `real.go`: `Log` omitido en el lanzador / `NS` de `ClientGo` cambiado | Cableado en `real` sin prueba. Sin `Log` se pierden los logs de runners; con otro `NS` los Jobs irían a otro namespace. |

- Repros reutilizables: `scratchpad/repros/rev_S1_S4_test.go` (publicación única, evidencia parcial, reintento de publish) y `rev_R1_R4_test.go` (F-01, F-02 y AMARILLOS).
- Mínimo exigible para cerrar F-03: pruebas que maten las 3 mutaciones de `controller.go`, la del orden `logs.txt`→`result.json`, y la del plazo del Job. El resto puede ir como AMARILLO.

## Cableado y mutantes que sí mueren
Estas cubren las invariantes 1, 2, 3 y 7 de tu lista.
- `ensayo_passed`, política ilegible, plan inválido, cuota denegada y gate caído.
- Tope de concurrencia, adopción por flujo y `AlreadyExists`.
- Lectura de vuelta y hash.
- `checkSpec`: regex, mayúsculas, nombre vacío y `envFrom`.
- Todos los campos de endurecimiento de `BuildJob`.
- `Runners`, `Run`, `Gate`, `MaxParallel`, `Policy`, `Exec`, `Evidence`, imagen, `Plans` y `Obs` en `buildConfig`.
- Las vallas de `fake` siguen intactas: `RUN_ENV=" Production "` se rechaza.
- Sin `Runners`, T02/T04 se comportan idéntico; las pruebas heredadas no se debilitaron.

## Decisiones del codificador
- **D2 (sin egress): aceptable, sin brecha para U2-T07.**
  - `default-deny` + `allow-same-namespace` + `allow-dns` aplican a todo pod de `aqs-test`.
  - Un runner solo sale a pods de `aqs-test` (el warm, intencional) y a DNS en `kube-system:53`. No alcanza MinIO en `aqs-system` ni la API.
  - `policy/egress.rego` prohíbe en `aqs-test` cualquier egress que no sea intra-namespace o DNS, e `isolation.rego` prohíbe bindings de SA.
  - Los logs los lee el controlador con `pods/log`, y el `Role aqs-test-operator` ya lo permite.
  - La comprobación depende de que el CNI haga cumplir la NetworkPolicy; eso es de la comprobación humana en U2-T08.
- **D3 (plazos y gate): aceptable.**
  - `go-governance` cuenta como cuota todas las decisiones `allow` hacia running de ese workflow. Como la transición del controlador envía `Workflow` vacío, la consulta del lanzador es la única que cuenta: una vez por corrida.
  - Hay un re-consumo menor si `Launch` falla tras autorizar y se reintenta; es AMARILLO.
  - Los plazos por configuración son coherentes mientras U4 no los devuelva; ya está registrada la candidata.
- **`run.done` con flujos fallidos:** es correcto según la tarea. Un runner fallido es evidencia (`result.json.status`), y el esquema no lleva estado. Un consumidor aguas abajo no debe leer `run.done` como corrida exitosa; conviene documentarlo para U3. Si el gate de salida cae, la corrida termina `failed` después de haber publicado `run.done`; es heredado de T02.
- **Marcador `result.json` al final:** con el orden actual, si el marcador existe es legible y `logs.txt` ya estaba verificado. En un reinicio solo se relanza un flujo sin marcador. La excepción es R4, más abajo.

## AMARILLO (no bloquean; aceptable fusionar en dev tras F-01 a F-03)
- **Logs tragados:** `logs, _ := Kube.JobLogs` guarda `logs.txt` vacío con estado normal si `pods/log` falla. Un flujo `failed` queda sin su explicación. Conviene marcar `logs_unavailable` en `result.json`.
- **R4 (F-02-b):** si el Job vence y la escritura de evidencia falla, se borra el Job y `evFail` vive solo en memoria. Tras un reinicio el flujo se relanza y el origen del plazo se reinicia. Repro: `jobs=1 creados=2`. Conviene no borrar el Job hasta que `settle` tenga éxito.
- Si `DeleteJob` falla una vez, no se reintenta: el flujo ya está resuelto y el Job queda hasta que el `activeDeadlineSeconds` de k8s lo termine.
- Un fallo permanente de lectura de S3 a mitad de corrida deja la corrida en `running` sin tope ni alerta. Es el mismo patrón de T04; candidata para un vigilante en T07/T08.
- Mutantes que sobreviven sin consecuencias graves:
  - `RunID` del plan y `FlowID` del marcador sin prueba.
  - Un marcador ilegible se acepta como resuelto con la guarda quitada.
  - `capabilities` y `seccomp` no se aseveran en Go; sí los cubre conftest en CA-2.
  - `RealPhases` delega la fase `run` sin prueba en `adapters`, aunque `Progress` hace el trabajo igual.
  - `obs.NewRunnerMetrics` en `main.run`, que no tiene prueba desde T02.
- `RunDuration` puede observarse dos veces si falla el `save` posterior.
- Mutantes equivalentes que descarté: `active<max` externo, `Status.Failed>0`, error de lectura como no encontrado, error del gate ignorado.

## Tareas candidatas
- Imágenes `RUNNER_IMAGE`/ensayo (k6 u otro motor), ya registradas.
- Un vigilante de corridas atascadas en `running` con almacén caído.
- `ttlSecondsAfterFinished` para los Jobs terminados.
- Documentar para U3 que `run.done` no implica éxito (leer `result.json`).

VEREDICTO: NO-VERDE
NARANJA|runner/launcher.go (first/runExpired)|El plazo por corrida se estira al borrar Jobs vencidos (6 Jobs con RunTimeout=3m)
NARANJA|runner/launcher.go bucle de creación + runctl.exhaust|Un fallo parcial de lanzamiento deja Jobs vivos al salir por reset
NARANJA|runctl/controller.go runnersStep, launcher.go settle/step, real.go|Mutantes sobrevivientes en invariantes declaradas: evidencia parcial publicada, fallo de Publish tragado, republicación tras reinicio, orden logs→marcador, plazo del Job desde la política, Log/NS sin cablear
INFORME: revisiones/U2-T05/ronda-1.md
