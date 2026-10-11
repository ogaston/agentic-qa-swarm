# Ronda 2 — U8-T03

VEREDICTO: VERDE

Sha revisada: 5eefa987e1415b31fd00354e6220c56fe2b08ed4. Volví a correr yo mismo los 5 criterios completos, no solo los hallazgos. Todos pasan. No queda ningún ROJO ni NARANJA. El worktree quedó limpio (`git status --short` vacío). No usé el clúster kind, así que no hizo falta `flock`.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Pruebas del binario | `(cd services/aqs-runner && GOTOOLCHAIN=auto go test -count=1 ./...; echo rc=$?)` | pasa: `ok … 0.108s`, rc=0. `go vet` y `gofmt -l` sin salida. |
| 2 | Interfaz idéntica a los Jobs | `bash scripts/test/u8-runner-contract.sh; echo rc=$?` | pasa: `OK rehearse-args`, `OK http-steps-args`, `OK paso-fallido-rc`, rc=0 |
| 3 | Imagen construida, cargada y referenciada | `build-images.sh` más `podman image exists`; `kubectl kustomize … \| grep -c`; `podman run --user 65532 --read-only … --help` | pasa: `imagen`, `2`, `rc=0`. También `rehearse --help` en el contenedor sale con rc=0. |
| 4 | Políticas y lista coherentes | `policies.sh \| grep -c '^FALLA'`; `grep -c -E '\b9\b' build-images.sh lib.sh` | pasa: `0` y `0` |
| 5 | Alcance | `git status --short \| wc -l`; `git diff --name-only $b \| grep -v <lista> \| wc -l` | pasa: `0` y `0` |

Nota sobre CA-1: con el Go local 1.26.8 el comando literal da rc=1 porque `go.mod` pide 1.26.9. Es un problema del entorno y pasa igual con go-identity. Los comandos del codificador llevan `GOTOOLCHAIN=auto`, y con eso pasa.

## Verificación de las respuestas de la ronda 1, por identificador
- **F-01 (NARANJA): resuelto.**
  - `deploy/kind/README.md` ya no dice «9»: `grep -c -E '\b9\b'` da 0.
  - Ahora dice «hoy 10: 8 de `services/*` … y 2 de `agents/*`».
  - Explica la regla nueva de `lib.sh` (cruce con el conteo de Dockerfile; falla si no coinciden o la lista está vacía).
  - La afirmación coincide con el comportamiento real.
- **F-02 (AMARILLO): resuelto, y lo verifiqué.**
  - Los dos casos positivos del contrato exigen ahora `"ok":true` en stdout.
  - `paso-fallido-rc` exige rc==1 más `"status":500` y `"ok":false` en stdout.
  - Para comprobarlo puse un `podman` falso en el PATH (en el scratchpad) que sale 125 en `run`. Resultado: `FALLA rehearse-args`, `FALLA http-steps-args`, `FALLA paso-fallido-rc (rc=125)` y rc=1. En la ronda 1 ese mismo caso habría dado `OK`.
- **F-03 (AMARILLO): resuelto o justificado.**
  - `rehearse --help` y `http-steps -h` salen con 0. Lo cubre `TestRehearseHelpSaleCero`, y lo comprobé en el contenedor.
  - La justificación del User-Agent y de la guarda de `platform_images` la acepto: no bloquea y son observaciones menores.

## Diff nuevo (8a9e10d..HEAD) — regresiones
Cambia solo README, el script de contrato, `main.go` (3 líneas con `flag.ErrHelp`), una prueba nueva y la bitácora. No encontré regresiones: todo sigue dentro de la lista permitida y los criterios pasan con la imagen reconstruida.

## Tareas candidatas
- Ninguna.

VEREDICTO: VERDE
INFORME: revisiones/U8-T03/ronda-2.md
