# Ronda 2 — U2-T04

VEREDICTO: VERDE

Worktree sha 51cadb6 (rama tarea/U2-T04), limpio antes y después (`git status --short | wc -l` = 0). Diff de la ronda dentro de `services/go-run-controller/` y de los informes. Corrí todo yo, con `-count=1`.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | ≥45 PASS, 0 FAIL, con -race | `go test -race -count=1 -v ./...` | pasa: 135 PASS, 0 FAIL |
| 2 | Plan roto / agotado / nunca omitido | `-run 'Rehearsal(BrokenPlan\|Exhausted\|NeverSkipped)'` | pasa (3 PASS; plan roto = 0 Jobs, aceptado en la ronda 1) |
| 3 | Gate no omitible | `-run 'EnsayoGate\|NoBypass'` | pasa (6 PASS) |
| 4 | Job sin credenciales + políticas | render, grep, kubeconform v0.6.7, conftest v0.56.0 | pasa: grep=1 (solo `automountServiceAccountToken: false`, D1 ya aceptada), `Valid: 1, Invalid: 0`, `23 tests, 23 passed, 0 failures` |
| 5 | Contratos contra servicios reales | `up`, `go test -tags contract`, `down` | pasa: 7 PASS, 0 FAIL/SKIP |
| 6 | Recorrido hasta el ensayo | `-run 'Flow(ToRehearsal\|ResetOnFailure)'` | pasa (2 PASS) |
| 7 | Vallas | los 3 comandos `env ... rc` | los tres dan rc=1. Además, `real` con entorno completo fuera del clúster sale con el error `RUN_PHASES=real requiere ejecutarse en el clúster`, o sea falla cerrado |
| 8 | Imagen, deps, alcance | `go list`, `docker build`, vet (con y sin `contract`), gofmt, git | pasa: 132, `65532:65532`, `ok`, 0, 0 |

## Verificación de lo pedido
- **(1) F-01 corregido.** `inClusterClient()` salió a `run()`, y `buildConfig` elige en una sola rama `fake` (FakeWarm/FakePhases) o `real` (WarmClient, RealPhases con Warm/Reset/Rehearse/Artifact, `Results` = el mismo lanzador); sin sobrescritura posterior. Mis mutaciones muertas, con el nombre del test que las mata:
  - sin rama real (`if true || !c.real`): muere (`TestBuildConfigRealWiresEveryRealPort` y `...FakeStaysFake`);
  - sin `base.Warm` real: muere;
  - sin `base.Results`: muere;
  - sin `Rehearse`: muere;
  - sin `Warm` dentro de `RealPhases`: muere;
  - `Results` apuntando a otro lanzador: muere;
  - `FakeWarm` por defecto en real: muere;
  - `cs==nil` aceptado: muere.
  - W5 (sin `Reset`) y W7 (`Artifact` nil) solo dieron fallo de compilación, así que no cuentan como muerte probada. Los campos `rp.Reset` y `rp.Artifact` sí se afirman distintos de nil en la prueba.
  - Sobrevive solo que `run()` no cree el clientset en real. Es equivalente: `buildConfig` rechaza `cs==nil` y falla cerrado (`W9`, muerta).
  - **La prueba no es vacua.** Afirma tipos concretos y no nulos, y la identidad `Results == Rehearse`. La comprobación por nombre «Fake» es redundante con las aserciones de tipo, pero no estorba. En `real` no queda ningún camino con un `Fake*` de fase/warm ni un `reset_verified` fabricado entrando al gate (el gate es `HTTPGate` desde `run()`). Sin prueba unitaria quedan `inClusterClient` y `ListenAndServe`/señales, declarado y aceptable: probé en caja negra que `real` fuera de clúster falla cerrado.
- **(2) `ensure` con `EnsureTimeout`=130 s.** El contexto propio sustituye al `Timeout` del `http.Client`. La lectura del cuerpo ocurre dentro de la misma llamada (el `cancel` va al final), así que el plazo cubre todo. Mis mutaciones: quitar el contexto, que `ensure` use el plazo de fase, defecto de 5 s, defecto de 60 s, fase en 50 s y reset en 1 h: todas mueren (`TestDefaultTimeoutsAndEnsureOwnDeadline`). 130 s queda por encima de `WARM_READY_TIMEOUT` (120 s) y por debajo del `WriteTimeout` del servidor (150 s). `Ensure` solo se llama dentro de `Deploy`/`Launch`, que ya bloqueaba el lazo (hasta 12 min, preocupación (a) aceptada en la ronda 1); suma como mucho 130 s al mismo bloqueo. `/healthz`, `/readyz` y `GET /runs/{id}` siguen sin bloquearse.
- **(3) F-02/F-03/F-04.** Mueren: `Status.Failed>0` sin condición = passed; `ensure` sin chequear `State`; `ensure` sin chequear `ResetVerified`; `POST /deploys` sin comprobar `run_id`; `RUN_ENV` sin `ToLower`; sin `TrimSpace`; sin `production`. Plazos afirmados (ver (2)).
- **(4) Invariantes de la ronda 1 intactas.** Mutaciones repetidas y muertas:
  - guarda `rehearsing→running`;
  - guarda de `Drive` en `running`;
  - tope de 3 Jobs contado en el clúster;
  - adopción del Job vivo;
  - error de lectura = passed.
  - También sigue muerto el que sin redirecciones.

  Las 3 pruebas heredadas ajustadas no se tocaron en esta ronda; siguen más fuertes que en `main`.
- **Barrido final** (`main.go`, `real.go`, `rehearsal/`, `adapters/phases.go`): sin hallazgos nuevos. Cableado probado, `fake` solo con RUN_ALLOW_FAKE_PHASES y fuera de prod (incluye `PROD`/` Production `), efectos externos con fallo cerrado, 3 Jobs como tope incluso tras fallos del almacén, y un error de lectura del Job nunca es passed.

## Hallazgos
Ninguno bloqueante: no queda ROJO ni NARANJA.

### AMARILLO, no bloquea
- `inClusterClient`, `ListenAndServe` y el manejo de señales sin prueba unitaria. Aceptado: la caja negra fuera de clúster falla cerrado, y la prueba en dev es U2-T08.
- La presencia de `reset_verified` en `decodeWarm` sigue sin caso propio. Cierra igual (falla cerrado).
- `build failed` en W5/W7 de mis mutantes es un defecto de mi mutación, no del código.

## Tareas candidatas (fuera de alcance)
- `Deploy` reintenta contra un deploy `failed` pegajoso en warm-manager: los reintentos 2 y 3 de la fase fallan al instante.
- Cancelar o limpiar el Job de ensayo vivo cuando la corrida sale a `resetting` por otra causa.
- Tiempo máximo de espera si el Job de ensayo desaparece (`Launched=true`, 0 Jobs).
- Los errores de red de `callStatusT` descartan la causa (`inalcanzable`).
- Un solo `RUN_ARTIFACT_REF` para todas las corridas.
- Reconciliar en `tareas/U2-T04-ensayo.md` el CA-2 (plan roto = 0 Jobs de ensayo) y el CA-4 (grep TOKEN frente a `automountServiceAccountToken`).

Lo que queda es aceptable para fusionar en desarrollo.

VEREDICTO: VERDE
AMARILLO|cmd/go-run-controller/real.go (inClusterClient) + main.go (ListenAndServe/señales)|Sin prueba unitaria; real fuera de clúster falla cerrado (comprobado en caja negra)
AMARILLO|adapters/phases.go (decodeWarm)|La presencia de reset_verified sin caso propio; cierra igual
INFORME: revisiones/U2-T04/ronda-2.md
