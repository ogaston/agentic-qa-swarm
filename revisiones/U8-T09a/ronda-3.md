# Ronda 3 — U8-T09a

VEREDICTO: VERDE

Sha revisado: ee82bdf (rama tarea/U8-T09a). `AQS_PLANNER_PYTHON=/home/omarjayg/.cache/u8t09a-venv/bin/python`. Mutaciones en copias del scratchpad; worktree limpio.

## Criterios de aceptación, verificados por el revisor
| # | Criterio | Comando | Resultado |
|---|---|---|---|
| 1 | Esquemas iguales y `go test` | `diff` de las dos copias; `go test ./... -count=1` en go-warm-manager y go-run-controller | pasa (`IGUALES`, sin `FAIL`) |
| 2 | La inferencia conserva el cuerpo | `go test ./... -run 'OpenAPI.*(Body\|Cuerpo)' -v` en go-warm-manager | pasa (3 `--- PASS`: `DeTargetApp`, `AusenteOExternoNoRompe`, `RefRaizVersusAnidado`) |
| 3 | El planner usa el esquema | `pytest -q -k 'prompt or body or validate or plan'` y suite completa | pasa (79 passed; 200 passed) |
| 4 | Flujo sano contra la app real, con la variable | `AQS_PLANNER_PYTHON=<venv> go test ./... -count=1 -run 'Orders.*(Cuerpo\|Body)' -v` en aqs-runner | pasa (0.95 s; subtests `con_body_del_plan` y `sin_body`) |
| 4 | Sin la variable | mismo comando con `env -u AQS_PLANNER_PYTHON` | `--- SKIP`, `ok`. Un skip no cuenta como pasa |
| 5 | Alcance | `git status` y los dos `git diff --name-only` | `0`, `0`, `0` |

## Verificación de las respuestas de la ronda 2
- F-01 (NARANJA), resuelto. Sin variable, `go test ./... -count=1` en aqs-runner sale `ok` (0.113 s), `--- SKIP` con mensaje «CA-4 OMITIDA». Con `AQS_PLANNER_PYTHON=/usr/bin/python3` falla en 0.04 s con `no importa jsonschema: exit status 1`. Con el venv pasa. Se eliminó el default `python3`.
- F-02 (AMARILLO), resuelto. La bitácora tiene la sección «Desviaciones»: el plan es un fixture del LLM falso que escribe la prueba; dice qué cubre y qué no y señala `tests/test_body_contract.py`.
- F-03 (AMARILLO), resuelto. Con `TARGET_APP_STORE=memoria` y `DATABASE_URL` definida el binario sale con rc=2 y `TARGET_APP_STORE=memoria no admite DATABASE_URL (solo pruebas)`; en modo memoria sin `DATABASE_URL` emite el WARN `almacen en memoria (solo pruebas): Diff siempre 0, no verifica reset`; sin variables sigue saliendo con `falta DATABASE_URL`. `TestModoMemoriaRechazaDatabaseURL` y `TestModoMemoriaSinDatabaseURLArranca` pasan; quitando el rechazo en una copia la primera falla (`config_test.go:19`).

## Desviaciones declaradas (las reescribe el humano en el PR)
1. Python del planner: CA-4 solo se verifica con `AQS_PLANNER_PYTHON` apuntando a un Python con `jsonschema`; sin la variable se omite.
2. Almacén en memoria: `TARGET_APP_STORE=memoria`, no Postgres. Handler de rutas y validación reales; `Diff` siempre 0, no sirve para verificar reset.
3. Fixture del LLM falso: lo escribe la prueba a partir de `required` y `minimum` del esquema inferido; no cubre que un LLM real produzca el plan.
4. Superficie por subproceso: `go run ./cmd/openapi-endpoints`, porque el Dockerfile de aqs-runner solo copia su directorio.

## Hallazgos
Ninguno ROJO ni NARANJA.

### F-01 · AMARILLO · services/aqs-runner/cmd/aqs-runner/main_test.go · CA-4 no corre en el CI por defecto
`.github/workflows/ci.yml` no define `AQS_PLANNER_PYTHON`, así que en CI la prueba aparece omitida. Consecuencia aceptada de omitirla sin la variable. El humano debe saberlo al reescribir CA-4. No bloquea.

## Tareas candidatas
- Job de CI o etiqueta de build para pruebas de integración multi-proceso (Go, Python y `target-app`) con el Python del planner instalado.
- Prueba de `target-app` contra Postgres real (la recoge U8-T09).

VEREDICTO: VERDE
AMARILLO|services/aqs-runner/cmd/aqs-runner/main_test.go|CA-4 queda omitida en el CI por defecto (sin AQS_PLANNER_PYTHON); desviaciones de CA-4 a decidir por el humano en el PR
INFORME: revisiones/U8-T09a/ronda-3.md
