# Ronda 1 — U8-T03

VEREDICTO: NO-VERDE

Resumen: los 5 criterios pasan con mi propia ejecución. Queda un NARANJA en pie, la documentación que sigue afirmando el «9 fijo» que la tarea elimina. No hay ROJO. El worktree quedó limpio tras mi revisión (`git status --short` vacío). No usé el clúster kind, así que no necesité `flock`.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Pruebas del binario | `(cd services/aqs-runner && go test ./... ; echo rc=$?)` | Literal: rc=1, porque el Go local es 1.26.8 y el go.mod pide 1.26.9 (ver nota). Con `GOTOOLCHAIN=auto go test -count=1 -v ./...`: 11 pruebas PASS, rc=0. `go vet` y `gofmt -l` sin salida. |
| 2 | Interfaz idéntica a los Jobs | `bash scripts/test/u8-runner-contract.sh; echo rc=$?` | pasa: `OK rehearse-args`, `OK http-steps-args`, `OK paso-fallido-rc`, rc=0 (corrí el script dos veces, con la imagen recién reconstruida la segunda) |
| 3 | Imagen construida, cargada y referenciada | `bash scripts/kind/build-images.sh >/dev/null && podman image exists …/aqs-runner:0.0.0 && echo imagen`; `kubectl kustomize deploy/flux/kind \| grep -A1 -E 'name: (REHEARSAL_IMAGE\|RUNNER_IMAGE)$' \| grep -c 'aqs-runner:0.0.0'`; `podman run --rm --user 65532 --read-only … --help` | pasa: `imagen`, `2`, `rc=0`. La imagen se creó 2 minutos antes de la comprobación y su usuario es 65532:65532. |
| 4 | Políticas y lista coherentes | `bash scripts/ci/policies.sh 2>&1 \| grep -c '^FALLA'`; `grep -c -E '\b9\b' scripts/kind/build-images.sh scripts/kind/lib.sh \| awk …` | pasa: `0` y `0` |
| 5 | Alcance | `git status --short \| wc -l`; `git diff --name-only $b \| grep -v -E '<lista>' \| wc -l` | pasa: `0` y `0`. El diff son 8 archivos, todos dentro de la lista permitida. |

Nota sobre CA-1: el fallo literal es del entorno. `services/go-identity` falla igual con este Go local (`go: go.mod requires go >= 1.26.9`). El codificador lo declaró en la bitácora, donde sus comandos llevan `GOTOOLCHAIN=auto`. No es un hallazgo contra el código.

También revisé a mano:
- **Interfaz de los Jobs:** `main.go` coincide con `rehearsal.BuildJob` y `HTTPStepsExecutor.Plan`: los mismos subcomandos, flags y variables (`REHEARSAL_STEPS`/`REHEARSAL_INVARIANT`, `RUNNER_STEPS`/`RUNNER_INVARIANT`), y el mismo tipo `plan.Step`.
- **Validación antes de peticiones:** valida todo (destino http(s), método, ruta con `/`, `expect_status` 100..599, mismo host) antes de la primera petición. No sigue redirecciones. Aplica timeout de 5 s por paso.
- **Sin cambios al controlador:** no hay diff en `go-run-controller`.
- **Dockerfile:** sigue el patrón de go-identity y corre no root con uid 65532.
- **CI:** descubre servicios con `list-services.sh`, así que construye la imagen nueva sola.
- **Pruebas:** las negativas usan un `countingDoer` que falla si hay alguna petición. Eso demuestra «sin hacer peticiones». La suite tarda 0,1 s sin depender de estado externo, así que no hay señal de pruebas saltadas.

## Hallazgos
### F-01 · NARANJA · deploy/kind/README.md:37, 41, 46 · La documentación sigue afirmando el «9 fijo» que la tarea elimina
El README sigue diciendo «Las 9 imágenes de la plataforma (7 de `services/*` y 2 de `agents/agent-*`)», «construye las 9» y «si no da 9, el script falla». Tras este diff hay 10 imágenes (8 en `services/*` y 2 en `agents/*`), y `lib.sh` ya no falla por «no da 9». La documentación contradice el comportamiento que esta tarea cambió. La propia CA-5 incluye `deploy/kind/README\.md` entre los archivos permitidos, así que se esperaba actualizarlo. Para detectarlo: `grep -n -E '\b9\b' deploy/kind/README.md`. El codificador no lo tocó ni lo justificó en la bitácora.

Arreglo esperado: actualizar los tres pasajes, sin un número fijo o con la cuenta actual, y explicar la nueva regla de `lib.sh`.

### F-02 · AMARILLO · scripts/test/u8-runner-contract.sh (caso `paso-fallido-rc`) · El caso de fallo acepta cualquier error
`run_job` devuelve distinto de 0 para cualquier fallo: imagen ausente, rc 125 de podman, pánico. El caso `OK paso-fallido-rc` pasa con cualquiera de ellos. Los dos casos positivos solo comprueban rc=0, y `run.out` no se lee de vuelta. CA-2 pide el código de salida, así que no bloquea. Sería más fuerte exigir rc==1 y que stdout tenga `"status":500,"ok":false`.

### F-03 · AMARILLO · services/aqs-runner/cmd/aqs-runner/main.go · Detalles menores
- `rehearse --help` devuelve 2 (error de flag), mientras que `--help` a secas devuelve 0.
- El User-Agent `aqs-runner/0.0.0` está incrustado, pero no es una decisión de usuario.
- La comprobación nueva de `platform_images` compara `list-services.sh` con un conteo de `ls -d services/*/Dockerfile agents/*/Dockerfile`, que es casi la misma fuente. Es una guarda débil, aunque coincide con lo que pide la tarea.

## Tareas candidatas (defectos reales fuera de alcance)
- Ninguna nueva.

## Rutas de transcripciones largas
- Bitácora del codificador: /home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U8-T03/bitacoras/U8-T03.md

VEREDICTO: NO-VERDE
NARANJA|deploy/kind/README.md:37,41,46|La documentación sigue afirmando el «9 fijo» (hay 10 imágenes)
INFORME: revisiones/U8-T03/ronda-1.md
