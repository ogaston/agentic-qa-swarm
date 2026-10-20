# Ronda 2 — U8-T09a

VEREDICTO: NO-VERDE

Sha revisado: 94dd583. El revisor usó `AQS_PLANNER_PYTHON=/home/omarjayg/.cache/u8t09a-venv/bin/python`. Mutaciones hechas en copias del scratchpad; worktree limpio.

## Criterios de aceptación, verificados por el revisor
| # | Criterio | Resultado |
|---|---|---|
| 1 | Esquema igual en ambas copias; `go test` de warm-manager | pasa (`IGUALES`, sin `FAIL`; go-run-controller también) |
| 2 | La inferencia conserva el cuerpo | pasa (3 `--- PASS`, incluida `RefRaizVersusAnidado`) |
| 3 | El planner usa el esquema | pasa (79 con filtro, 200 suite completa) |
| 4 | Flujo sano contra la app real, con `AQS_PLANNER_PYTHON` | pasa (0.86 s; subtests `con_body_del_plan` y `sin_body`) |
| 4 | Ídem, tal como está escrito (sin esa variable) | **falla**: `.../healthz no respondió 200 a tiempo` tras 20 s; el `python3` del sistema no tiene `jsonschema` |
| 5 | Alcance | pasa (`0`, `0`, `0`) |

## Verificación de F-01, F-02, F-03 de la ronda 1
- F-01 (ROJO): resuelto en lo sustancial. La prueba compila y arranca el binario real de `demo/target-app`; la superficie sale de `parseOpenAPI` vía `cmd/openapi-endpoints`; el planner real (`python -m agent_planner`, LLM falso) devuelve el plan por `POST /v1/plan`; el runner ejecuta esos pasos. Par 201/exit 0 y 400/exit 1 sin `body`.
- F-02: resuelto. `_check_body` captura `SchemaError` y `UnknownType`. Con `validate.py` revertido, 4 tests de `test_body_contract.py` fallan.
- F-03: resuelto. `TestOpenAPIRequestBodyRefRaizVersusAnidado` pasa; el comentario de `openAPIRequestBody` describe el `$ref` raíz frente al anidado.

## Punto 1: CA-4 con almacén en memoria
Desviación menor que debe decidir el humano. Se cumple: superficie inferida por `parseOpenAPI`, plan del planner con LLM falso, ejecución contra el handler real (`appMux`, `gate`, `baselineMux` sin cambios de lógica; solo cambió la firma de `*store` a la interfaz `appStore`). No se cumple al pie de la letra: el almacenamiento es `memStore` (`TARGET_APP_STORE=memoria`), no Postgres. Además, el fixture del LLM falso lo escribe la propia prueba (mecanismo de `U3_FAKE_SURFACES_DIR`), y el planner lo valida contra el `request_body` inferido. Recomendación: aceptar la desviación y reescribir CA-4 en la tarea para decir «almacén en memoria», «requiere `AQS_PLANNER_PYTHON`» y «fixture del LLM falso».

## Punto 2: sensibilidad
- Extracción de `request_body` anulada: la prueba falla (`request_body de POST /orders no interpretable`).
- Plan sin `body`: el planner responde `422 body_requerido:0/0`; la prueba falla.
- Planner revertido a la base y plan sin `body`: `con_body_del_plan` falla con `400` y `exit 1`.
- Planner revertido con fixture válido: la prueba pasa (esperable; el rechazo lo cubre `test_body_contract.py`).
- Revertir `service.go` entero: error de compilación.

## Punto 3: el almacén en memoria no se activa por defecto
Correcto. Sin variables el binario sale con `falta DATABASE_URL`; con `TARGET_APP_STORE=x` sale con `TARGET_APP_STORE desconocido: x`; solo `memoria` lo activa. Sin referencias en `deploy/`, `scripts/` ni el Dockerfile.

## Hallazgos

### F-01 · NARANJA · `services/aqs-runner/cmd/aqs-runner/main_test.go` (`TestOrdersCuerpoFlujoSanoContraTargetApp`) · La prueba rompe `go test ./...` sin el Python del planner y CA-4 tal como está escrito falla
- Sin `AQS_PLANNER_PYTHON` usa `python3` y falla tras 20 s esperando `/healthz`. `cd services/aqs-runner && go test ./... -count=1` sale `FAIL ... 20.8s`; en la base ese paquete pasaba.
- `.github/workflows/ci.yml:79` ejecuta `go test ./...` por servicio sin instalar `jsonschema` ni definir `AQS_PLANNER_PYTHON`. Riesgo: el CI de aqs-runner queda en rojo al fusionar.
- Falla duro, lo cual es mejor que saltarla, pero un prerrequisito oculto no puede romper la suite por defecto.
- Opciones dentro de alcance: (a) etiqueta de build o guarda explícita con `t.Skip` y aviso (un skip no es un pass; CA-4 se correría con la variable o `-tags`, y el criterio se reescribe para exigirlo); (b) instalar `jsonschema` en CI (toca `.github/`, fuera del cerco de CA-5); (c) reescribir CA-4 declarando el prerrequisito y haciendo que la prueba falle rápido con mensaje claro (comprobar `import jsonschema` antes de arrancar).
- Cualquier opción exige que el comando de CA-4 en la tarea sea el que realmente se corre.

### F-02 · AMARILLO · `main_test.go` · La prueba mide el plan fabricado por la propia prueba
El fixture se arma en la prueba a partir del `required`/`minimum` del esquema inferido. Está documentado en el código, pero la bitácora solo declara Postgres → memoria. Con el planner revertido y un fixture válido, la prueba pasa.

### F-03 · AMARILLO · `demo/target-app/store_memoria.go`, `main.go` · Modo memoria sin aviso y con `Diff` siempre `0`
No hay peligro por defecto; el riesgo es de configuración manual: si alguien fijara la variable en un warm, la verificación de reset pasaría de forma trivial. Convendría un log de aviso al arrancar o rechazar el modo cuando `DATABASE_URL` esté definido.

## Tareas candidatas
- Etiqueta de build o job de CI para pruebas de integración multi-proceso (`go run`, `go build` de otro módulo y Python), separadas de `go test ./...`.
- Prueba de `target-app` contra Postgres real (la recoge U8-T09 según la bitácora).

VEREDICTO: NO-VERDE
NARANJA|services/aqs-runner/cmd/aqs-runner/main_test.go (CA-4 sin AQS_PLANNER_PYTHON)|La prueba nueva falla en `go test ./...` sin el Python del planner y el comando de CA-4 tal como está escrito no pasa; rompería el CI del servicio
AMARILLO|main_test.go|El plan es un fixture fabricado por la prueba y la desviación no se declara completa en la bitácora
AMARILLO|demo/target-app/store_memoria.go|Modo memoria sin aviso y con Diff siempre 0; no está activo por defecto
