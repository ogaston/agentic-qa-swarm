# Ronda 1 — U8-T09a

VEREDICTO: NO-VERDE

## Criterios de aceptación, verificados por el revisor
| # | Criterio | Comando que corrió | Resultado |
|---|---|---|---|
| 1 | Contrato acepta `request_body`, copias iguales, ejemplos valid/invalid, paridad de `ValidateSurface` | `diff ... && echo IGUALES`; `go test ./... -count=1` en go-warm-manager (y go-run-controller) | pasa (`IGUALES`, todo `ok`; `TestSurfaceRequestBodyEjemplos` y 3 casos nuevos de paridad presentes) |
| 2 | La inferencia conserva el cuerpo del OpenAPI | `go test -run 'OpenAPI.*(Body|Cuerpo)' -v` | pasa (2 `--- PASS`; usa `demo/target-app/openapi.json`, `POST /orders` exige `product_id,qty`, `GET /orders` sin cuerpo, `$ref` externo y sin `requestBody` no rompen) |
| 3 | El planner usa el esquema | `python -m pytest -q -k 'prompt or body or validate or plan'` (venv del codificador) | pasa (77 passed) |
| 4 | Flujo sano contra la app real con superficie inferida y plan del planner | `go test -run 'Orders.*(Cuerpo|Body)' -v` en aqs-runner | **falla como criterio**: el comando sale verde, pero la prueba no hace lo que CA-4 exige (ver F-01) |
| 5 | Alcance | los tres comandos de CA-5 | pasa (`0`, `0`, `0`) |

## Hallazgos

### F-01 · ROJO · CA-4 · La prueba verde no verifica el criterio tal como está escrito
CA-4 pide una prueba que (a) **tome la superficie inferida** del OpenAPI de `demo/target-app`, (b) use **el plan determinista del planner** con `LLM_PROVIDER` falso para esa superficie y (c) ejecute el flujo contra **el handler real** de `demo/target-app`. `TestOrdersCuerpoFlujoSanoContraTargetApp` (`services/aqs-runner/cmd/aqs-runner/main_test.go`, +76 líneas) no cumple ninguna de las tres:
- (a) `cuerpoDeEsquema` lee `openapi.json` directo con un struct local. No pasa por `parseOpenAPI` ni por `SurfaceArtifact`.
- (b) El steps JSON se arma a mano en la prueba. No interviene el planner ni su plan. La bitácora lo admite.
- (c) El servidor es `ordersHandler(false, nil)`, una réplica de ~30 líneas ya existente en el mismo `_test.go`, no `appMux` de `demo/target-app/main.go`. La bitácora lo admite.

Consecuencia sobre la evidencia: el par «con cuerpo → 201 / sin cuerpo → 400» solo demuestra que la réplica valida `product_id>0 && qty>0`. Eso no tiene relación con el diff de la tarea (el runner no cambió). La prueba pasaría igual con el diff revertido, salvo la lectura del JSON. Por tanto no cierra la cadena superficie → planner → ejecución, que es lo que CA-4 quería medir (el origen de la tarea es precisamente que el cuerpo generado deje pasar una app sana).

Sobre la excusa del Postgres: `appMux(st *store)` depende de un `*store` concreto (pgx). Aun así, hay vías dentro del alcance, porque `demo/target-app/` está dentro del cerco de CA-5. Una es extraer una interfaz del almacén y ejecutar `appMux` con un store en memoria. Otra es levantar Postgres con docker/podman, que están instalados. Que no haya Postgres local no vuelve la réplica un sustituto aceptable. Si el codificador considera inviable cualquiera de las dos, lo correcto era escalar el criterio, no declarar PASS. La bitácora presenta CA-4 como «Verde» y deja el hueco en «Desviaciones»: la prueba se llama «ContraTargetApp» pero no corre contra target-app.

Para cerrarlo: la prueba debe alimentarse de la salida de `parseOpenAPI`/`InferSurface` (o de un artefacto generado por ella). El plan debe salir del planner determinista (por ejemplo, un script Python que produzca el plan que luego consuma la prueba Go). El handler debe ser el real de `demo/target-app`. Debe conservarse el par falla-pasa. Si el humano acepta una versión reducida del criterio, hay que reescribir CA-4 en la tarea.

### F-02 · NARANJA · agents/agent-planner/src/agent_planner/validate.py:35-43 · Un `request_body` malformado provoca una excepción no controlada, no un rechazo
La tarea (Plan de pruebas) dice: «un esquema que el validador no soporta degrada a "no comprobar"… nunca a error de ejecución». `Draft202012Validator(request_body).iter_errors(...)` con un esquema inválido lanza `jsonschema.UnknownType` (que no es `PlanRejected`). Comprobado con la superficie `request_body={"type":"foo"}` y un POST con body: `UnknownType Unknown type 'foo' for validator with schema`. Un OpenAPI de una app cualquiera puede llevar eso, porque el lado Go solo exige que sea un objeto. Hace falta capturar `jsonschema.exceptions.SchemaError`, `UnknownType` y similares, degradar a «no comprobar» (o a un rechazo tipado) y añadir una prueba. Hoy no hay ninguna para este caso.

### F-03 · AMARILLO · services/go-warm-manager/service.go (`openAPIRequestBody`) y bitácora · Documentación imprecisa de `$ref` locales
La bitácora dice que los `$ref` locales dentro del requestBody «quedan sin cuerpo en la superficie». Solo es cierto para un `$ref` en la raíz del esquema. Un `$ref` anidado sí se conserva en `request_body`. El planner lo detecta con `_checkable` y exige `body` sin validarlo (comprobado: pasa). El comportamiento es razonable, pero no está documentado ni probado en Go. La tarea pedía documentar la degradación.

### Nota informativa
Ronda 1 de máximo 3. Las cadenas rojas de CA-1 a CA-3 están bien documentadas y los cambios son de buena calidad. El worktree quedó limpio tras la revisión. Las fixtures de `gen_agent_fixtures.py` quedan para U8-T09, como pedía la tarea.

## Tareas candidatas
- Extraer una interfaz del almacén en `demo/target-app` (`appMux(*store)` es concreto) para poder probar el handler real sin Postgres.

VEREDICTO: NO-VERDE
ROJO|CA-4 (services/aqs-runner/cmd/aqs-runner/main_test.go)|La prueba usa la réplica del handler, un JSON leído a mano y ningún plan del planner; no verifica la cadena que exige el criterio
NARANJA|agents/agent-planner/src/agent_planner/validate.py:35-43|Un request_body con esquema inválido lanza UnknownType (no PlanRejected) en vez de degradar a "no comprobar"
INFORME: revisiones/U8-T09a/ronda-1.md
