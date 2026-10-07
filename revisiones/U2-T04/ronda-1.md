# Ronda 1 — U2-T04

VEREDICTO: NO-VERDE (un único NARANJA, de arreglo corto; el resto es aceptable para fusionar en desarrollo)

Worktree f21b440 (rama tarea/U2-T04), limpio antes y después (`git status --short | wc -l` = 0). Todo el diff está dentro del alcance (CA-8: 0 archivos fuera). Mis pruebas y mutaciones están en el scratchpad, no en el repo.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | ≥45 PASS, 0 FAIL, con -race | `go test -race -count=1 -v ./... \| grep -c '--- PASS'` y `grep -c FAIL` | pasa: 129 y 0 |
| 2 | Plan roto / ensayo agotado | `go test -run 'Rehearsal(BrokenPlan\|Exhausted\|NeverSkipped)'` | pasa (3 PASS). Con plan roto hay 0 Jobs, no 3 (ver F-05) |
| 3 | Gate no omitible | `-run 'EnsayoGate\|NoBypass'` | pasa (6 PASS) |
| 4 | Job sin credenciales, kubeconform, conftest | render + grep + kubeconform v0.6.7 + conftest v0.56.0 | pasa con la salvedad D1: grep=1 (solo `automountServiceAccountToken: false`), `Valid: 1, Invalid: 0`, `23 tests, 23 passed, 0 failures` |
| 5 | Contratos contra servicios reales | `up`, `go test -tags contract -run 'Contract(Warm\|Reset)'`, `down` | pasa: 7 PASS, 0 FAIL/SKIP. `/warm/ensure` listo no es alcanzable (D3) |
| 6 | Recorrido hasta el ensayo | `-run 'Flow(ToRehearsal\|ResetOnFailure)'` | pasa (2 PASS) |
| 7 | Vallas de configuración | los tres `env ... rc` | los tres dan rc≠0 (ver F-04: el criterio es débil por sí mismo) |
| 8 | Imagen, deps, alcance | `go list`, `docker build`, vet (con y sin `contract`), gofmt, git | pasa: 132, usuario `65532:65532`, `ok`, 0, 0 |

Nota sobre CA-7: mis tres comandos fallan ya por `RUN_EVENTS_FILE es obligatorio`, no por la valla. La valla real solo la prueban las unitarias de `real_test.go`.

## Mutaciones propias (ulimit -v 4000000, `go test -timeout 60s -count=1 ./...` por mutante, todas revertidas)
Quedan **muertas** estas, las de las invariantes críticas:
- (1) quitar la guarda `rehearsing→running`: muere. Quitar la guarda de `Drive` en `running`: muere.
- (3) quitar el tope de 3 Jobs contado en el clúster: muere.
- (4) quitar la adopción del Job vivo, `AlreadyExists`=fallo, el nombre del intento: mueren.
- (5) error de lectura = `passed`: muere. Job con condición `Failed` = `passed`: muere. Sin bearer, sin `CheckRedirect`, `deploy failed`=éxito, no validar `reset_verified`/`state` en el reset, surface de otra corrida, `AlreadyExists`, dispatch de fase no implementada = éxito: mueren.
- (6) automount, rofs, nonroot, envFrom, deadline, resources, run-id, intento>3, plan roto, plan de otra corrida, plan vacío: mueren.
- (7) quitar el chequeo `RUN_ALLOW_FAKE_PHASES`: muere.

Sobreviven: ver F-01 (bloqueante) y F-02..F-04 (AMARILLO).

Repro propia fuera del repo: un almacén que falla el `Save` justo tras `Launch` (crash entre `Launch` y `Launched`) con todos los Jobs fallando termina en `failed` con **3** Jobs, `Started=3` y 1 handoff. El tope se mantiene.

## Hallazgos

### F-01 · NARANJA · `cmd/go-run-controller/real.go:26-47` + `main.go:146-153` · `wireReal` no tiene ninguna prueba: el cableado `RUN_PHASES=real` sobrevive a cualquier mutación
`wireReal` exige `rest.InClusterConfig()`, así que ninguna prueba lo ejecuta (`grep wireReal` solo da la definición y la llamada). Mis mutaciones dejan toda la suite en verde:
- quitar la llamada a `wireReal` en `run()`: `RUN_PHASES=real` corre con `FakePhases` y `FakeWarm{Fact: True}`, o sea un `reset_verified` fabricado entrando al gate de U4;
- quitar `cfg.Warm = wc`: lo mismo con `FakeWarm`;
- quitar `cfg.Results = l`: el ensayo real se queda esperando eventos que nunca llegan;
- quitar `Rehearse: l`: el ensayo real falla siempre.

Es exactamente la clase «cableado sin prueba» / «fake sin barrera» del historial (U2-T02 F-03, U2-T02b F-03, U2-T03b). La bitácora y el README solo declaran que `run()` completo lo cubre la caja negra manual, y en `real` esa caja negra no existe sin clúster.

Arreglo corto: sacar `InClusterConfig` a `run()` y que `wireReal`/`buildPorts(cfg, c, cs kubernetes.Interface)` reciba el clientset. Hacer que `main` elija entre fakes y reales en una rama (no «fakes por defecto y luego sobrescribir»). Una prueba con `kubernetes/fake` que compruebe que `Warm` es `*WarmClient`, `Phases` es `*RealPhases` con `Rehearse` y `Results` no nulos, y que ningún `Fake*` queda en la config. Con eso mueren W1-W4.

### F-02 · AMARILLO · `rehearsal/launcher.go:jobState` · la rama de respaldo `Status.Failed > 0` no tiene prueba
`finish()` siempre pone la condición, así que cambiar `return true,false` por `true,true` en esa rama sobrevive. La rama hoy devuelve lo correcto; falta un caso (Job con `Failed=1` y sin condición, debe ser `failed`).

### F-03 · AMARILLO · `adapters/phases.go` · defensa en profundidad sin casos
Sobreviven: quitar solo el chequeo `State=="ready"` o solo el de `ResetVerified` en `Ensure` (los casos de prueba tienen ambos mal a la vez); quitar `d.RunID != runID` en la respuesta de `POST /deploys`; quitar `Timeout: PhaseTimeout` (las pruebas pisan el timeout a 150 ms, así que el valor de 5 s nunca se afirma); quitar la presencia de `reset_verified` en `decodeWarm` (el fallo quedaría como `False`, que igualmente cierra).

### F-04 · AMARILLO · `cmd/go-run-controller/main.go` · variantes de `RUN_ENV` sin prueba (heredado de T02)
Quitar `ToLower/TrimSpace`, o `production`, sobrevive: las pruebas solo usan `"prod"` (en `main` ocurre igual, no es nuevo de T04). El código sí lo implementa. Añadir `" Production "` y `PROD` a la tabla.

### F-05 · AMARILLO · D2 · plan roto = 0 Jobs, no 3
La bitácora cita mal el Alcance («antes de crear ningún Job»); el Alcance dice «antes de crear ningún Job de runner». Pero el resultado es estrictamente más seguro que lo que pide CA-2, y la prueba lee de vuelta el clientset falso: 0 Jobs, 3 intentos, 1 handoff, `resetting→failed`, 0 runners. Lo acepto. Debería reconciliarse el texto de CA-2 en el archivo de la tarea.

## Desviaciones y preocupaciones, juzgadas
- **D1 (CA-4):** confirmado. Sin la línea `automountServiceAccountToken: false`, el grep da 0. El resto del Job solo lleva `REHEARSAL_STEPS` y `REHEARSAL_INVARIANT`, sin Secrets. Inconsistencia de la especificación: aceptada.
- **D3 (CA-5):** aceptable. `ensure` 200 no se alcanza con `WARM_KUBE=fake`. Pero las formas JSON que el cliente decodifica con `DisallowUnknownFields` (`WarmState`, `DeployStatus`, `SurfaceArtifact`) coinciden campo a campo con las del servicio real. El contrato real ejercita `GET /warm`, `POST /deploys` 202 + `GET` con `failed`, `409`, 401 y servicio caído; el reset real queda verificado.
- **Pruebas heredadas ajustadas:** no debilitan nada. Se siembra `EnsayoPassed=True` en `running`, que es el invariante nuevo. `TestGateWithoutEnsayoDenied` ahora es más fuerte: `ErrIllegal` y 0 llamadas al gate.
- **`RehearsalResults` opcional:** en `real` queda cableado (`cfg.Results = l`) pero eso no lo prueba nadie (F-01). Sin `Results` no hay `passed` por defecto: espera el evento `rehearsal.*`. Con `Results` fallando solo reintenta la lectura.
- **(a) deploy espera dentro de `Launch` (hasta 12 min), con `k.mu` tomado:** aceptable en una versión con una corrida a la vez. `/healthz`, `/readyz` (usa `healthMu` aparte) y `GET /runs/{id}` (leen del almacén) no se bloquean; solo se detienen eventos y otras corridas. Un SIGTERM cancela la espera y cuenta un intento fallido (menor). Candidata.
- **(b) `ensure` 5 s sin esperar `idle-escalado`:** el cliente falla en 5 s y cuenta un intento; con 3 intentos en ~1,5 s de tick, el primer run tras un warm escalado a cero fallará probablemente. Es lo que manda mi Alcance (5 s). Candidata con prioridad: dar a `ensure` un plazo mayor o esperar el estado.
- **(c) Job terminado no consumido tras un crash:** sin problema; consume un intento, el tope de 3 se mantiene (lo comprobé con mi repro).
- **(d) `RUN_ARTIFACT_REF` único:** aceptable, documentado en el README.
- **(e) `real` con `noFlows`:** ningún camino salta el ensayo. `noFlows` + validación del plan + guardas de `transition` y de `Drive` en `running` lo sostienen (mutaciones A, B, R, Q muertas).

## Tareas candidatas (fuera de alcance, no bloquean)
- `ensure` con plazo largo o espera de `idle-escalado` (ver (b)).
- `Deploy` reintenta contra un deploy `failed` pegajoso en warm-manager: los reintentos 2 y 3 de la fase fallan al instante.
- Limpieza o cancelación del Job de ensayo vivo cuando la corrida sale a `resetting` por otra causa (hoy el Job puede correr hasta 120 s durante el reset).
- Tiempo máximo de espera si el Job de ensayo desaparece (`Launched=true`, 0 Jobs: espera indefinida).
- Los errores de red de `callStatus` descartan la causa (`inalcanzable`), lo que dificulta el diagnóstico.

VEREDICTO: NO-VERDE
NARANJA|services/go-run-controller/cmd/go-run-controller/real.go:26 (wireReal) + main.go:146|El cableado de RUN_PHASES=real no lo cubre ninguna prueba: quitar la llamada a wireReal, cfg.Warm, cfg.Results o Rehearse sobrevive
AMARILLO|rehearsal/launcher.go:jobState|La rama de respaldo Status.Failed>0 no tiene prueba (la mutación a passed sobrevive)
AMARILLO|adapters/phases.go|Defensa en profundidad sin casos (State/ResetVerified de Ensure por separado, run_id de /deploys, timeout de 5 s no afirmado, presencia de reset_verified)
AMARILLO|cmd/go-run-controller/main.go|Variantes de RUN_ENV (mayúsculas/espacios/production) sin prueba, heredado de T02
AMARILLO|tareas/U2-T04 CA-2 / D2|Plan roto produce 0 Jobs de ensayo (no 3); aceptado, reconciliar el texto de la tarea
INFORME: revisiones/U2-T04/ronda-1.md
