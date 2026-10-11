# Ronda 1 — U8-T06

VEREDICTO: VERDE

Worktree en `7c89ec7`. Corrí los cuatro criterios yo mismo. Al terminar el worktree sigue limpio (`git status --short` da 0 líneas). No usé clúster: la tarea se prueba con stubs y con el reporter en loopback.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Pruebas unitarias y fail-closed | `(cd services/go-run-controller && GOTOOLCHAIN=auto go test -count=1 ./...)` | pasa, rc=0. Los 9 paquetes con pruebas dan `ok`; `adapters` tarda 5,3 s. `go vet ./...` sin salida. |
| 2 | Contrato contra el reporter real | `bash scripts/test/u8-report-contract.sh` | pasa, rc=0. Salida: `OK reporter-arranque`, `OK contrato-reporter-valido`, `OK contrato-reporter-caido`, `OK reporter-detenido-limpio`. El puerto 18402 queda libre (`ss -ltn` no lo muestra). |
| 2b | Contrato con el stub local | `go test -count=1 -tags contract -run ContractReporter -v ./adapters` | pasa: `TestContractReporterValido` y `TestContractReporterCaido`. |
| 3 | Manifiestos y contratos | `kubectl kustomize deploy/flux/kind \| grep -A1 -E 'name: (RUN_FLOW_SOURCE\|U3_URL\|REPORTER_URL)$' \| grep -c ...` / `policies.sh \| grep -c '^FALLA'` / `contracts/validate.sh` | pasa: `3`, `0`, `contracts rc=0`. |
| 4 | Alcance | `git status --short \| wc -l` y el `git diff --name-only … \| grep -v … \| wc -l` | `0` y **4**. Los 4 son los archivos que E-1 autoriza: `agents/agent-reporter/src/agent_reporter/reporter.py`, `agents/agent-reporter/src/agent_reporter/server.py`, `agents/agent-reporter/tests/test_server.py` y `contracts/plans/report.schema.json`. Con el alcance ampliado por E-1, el desborde es 0. |

Extra: `agents/agent-reporter/.venv/bin/python -m pytest -q` da `275 passed, 2 deselected`.

## Comprobaciones específicas
- **Compatibilidad del contrato:** `report_uri` se añade como propiedad opcional en `contracts/plans/report.schema.json` (`minLength 1`, sin entrar en `required`). Se añaden `verdict` y `report_uri` opcionales en `Run` de `control-plane.yaml`. Ningún consumidor existente se rompe. La paridad de `report_model.py` y la validación de ejemplos pasan.
- **`schema.gen.ts`:** la diferencia son solo +7 líneas, exactamente las de `verdict` y `report_uri`, y coincide con el OpenAPI. No regeneré el archivo (no tengo `Write`); el codificador declara `npm run gen:api`.
- **Fail-closed:** `ReporterClient` cubre todos los casos que pide la tarea y los prueba:
  - Fallos de transporte y estado: reporter caído, 500, 422 y redirección (sin seguirla).
  - Respuesta inválida: cuerpo inválido, bug sin hallazgos, campo extra, otra corrida, `report_uri` ausente o vacío.
  - Otros: circuito, plazo, solo http(s), sin evidencia y sin cliente.
- **Máquina de estados:** `report` va después de `reset`, como la máquina existente. El controlador usa `ReportLauncher` solo para report, conserva la evidencia de la fase run y guarda `Verdict` y `ReportURI` únicamente si hay reporte válido. `TestReportErrorNoDejaVeredicto` lee de vuelta el store: no hay veredicto, `Launched[report]` es falso y el estado no es `Done`.
- **Lectura de vuelta:** `GET /runs/{id}` se prueba con un JSON real (`TestGetRunExponeVeredicto`, y omite los campos si no hay reporte). El test del reporter lee `report_uri` de la respuesta y comprueba que existe en `svc.store.objects` con el mismo reporte.
- **Cableado:** `deploy/flux/base/control-plane.yaml` añade solo las 4 variables. Las `NetworkPolicy` de `aqs-system` son de ingreso con `allow-same-namespace`, así que no bloquean la llamada.
- **Literales:** plazo 60 s, umbral 5 y apertura 10 s son constantes con nombre (`ReporterTimeout`, `ReporterCircuitThresh`, `ReporterCircuitOpenFor`).
- **Rojo primero:** la bitácora pega el rojo de `RealPhases.Launch(report)` y el rojo de E-1 (`KeyError: 'report_uri'`).

## Hallazgos
No hay ROJO ni NARANJA.

### F-01 · AMARILLO · `services/go-run-controller/README.md:52` y `agents/agent-reporter/README.md:36-38` · Documentación desfasada
- El README del controlador lista lo que exige `RUN_PHASES=real` y no incluye `REPORTER_URL`, que ahora es obligatoria y válida. Sin ella el arranque falla.
- El README del reporter documenta `200 Report`, pero la respuesta ahora lleva `report_uri`.
- Ambos archivos están dentro del alcance.

### F-02 · AMARILLO · `agents/agent-reporter/src/agent_reporter/server.py:54` · Rama de 500 sin prueba
La rama `uri is None → 500 {"error":"internal"}` (store ausente) no tiene prueba. Es un caso de borde secundario, porque en la práctica `Service` siempre recibe un store.

### F-03 · AMARILLO · `contracts/plans/examples` · Sin ejemplo de contrato con `report_uri`
Los ejemplos de `report` no cubren el campo nuevo, ni válido ni inválido (por ejemplo `report_uri` vacío). El contrato lo cubre solo `reporter_contract_test.go`.

## Tareas candidatas
- Idempotencia de `POST /v1/report`: un fallo entre la llamada y el guardado de `Launched` repetiría el reporte, y el reporter publicaría `report.ready` en cada llamada. El codificador lo dejó registrado en la bitácora.
- Publicar `report.ready` por NATS (ya listada como candidata en la tarea).
- La prueba de contrato válida con `REPORTER_URL` no comprueba que el `report_uri` coincida con el del store real. Hoy solo exige que no esté vacío.

## Rutas de transcripciones largas
Ninguna; las salidas caben en el informe.

VEREDICTO: VERDE
AMARILLO|services/go-run-controller/README.md:52, agents/agent-reporter/README.md:36|Documentación desfasada (REPORTER_URL obligatoria; respuesta con report_uri)
INFORME: revisiones/U8-T06/ronda-1.md
