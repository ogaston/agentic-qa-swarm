# Ronda 1 — U1-T03

VEREDICTO: VERDE

SHA revisado: ab5b2ae5773caef054636809b4a06f2e5c0e6eda (base `main` local bcbc09a; el worktree no tiene `origin/main`). Nueve observaciones amarillas, ninguna bloquea. El worktree quedó limpio.

## Criterios de aceptación, verificados por el revisor
| # | Comando | Resultado |
|---|---|---|
| CA-1 | `go test -run Artifact -v ./internal/artifact/` contando PASS, y `go test ./... \| grep -c FAIL` | 23 subpruebas y 0 FAIL. Las pruebas son puras (milisegundos), sin señal de saltos. |
| CA-2 | binario real, 4 payloads firmados, `jq` | 4×202. Líneas: `commit build-from-repo acme/shop@aaaa…`, `tag published-image ghcr.io/acme/shop:v1.2.0`, `pull_request build-from-repo acme/shop@aaaa…`, `tag published-image ghcr.io/acme/shop:v1.2.0`. Orden esperado. |
| CA-3 | binario real con `pull-request-fork.json` | `422`, `unresolvable_artifact`, `sin eventos`. |
| CA-4 | `go list -deps … \| grep -c` y `grep -c` sobre `artifact.go` | `0` y `0`. Dependencias del paquete: `errors`, `fmt`, `regexp`, `strings`. |
| CA-5 | `go vet`, `gofmt`, `git status`, diff fuera de alcance, `contracts` | `ok`, `0`, `0`, `0`. |

## Puntos de atención
1. **Fixture `release-published.json` (`target_commitish` de `main` a SHA de 40 hex).** Permitido: C-75 dice que U1-T03 «debe decidir cómo resuelve el SHA de un release y, si hace falta, ajustar el fixture»; CA-2 exige la cuarta línea y la tarea prohíbe cliente HTTP. No oculta nada: la bitácora lo declara, el caso «release con `target_commitish` no SHA» sigue probando el 400 con cuerpo en línea. En producción un release cuyo `target_commitish` sea una rama sigue dando 400 `invalid_payload`; C-75 queda abierta.
2. **422 fuera del OpenAPI de U5.** Aceptable: la tarea lo ordena y manda proponerlo a C-11 sin editar el contrato. `git diff --name-only bcbc09a -- contracts` da 0.
3. **Seguridad.** Validación estricta y fail-closed. Rechaza tag `latest` (también `LATEST`), vacío, con espacios, con punto inicial o de 129 caracteres; repo inválido; SHA corto o en mayúsculas; evento desconocido; PR de fork o sin `head.repo`. El registro también se valida. Tablas parametrizadas con cada `ref` validado contra el esquema `notify.created`. Sin red ni k8s. El handler no persiste ni publica ante un rechazo (`TestWebhookResolverErrors`: 422 y 503; prueba del fork).
4. **Mutaciones** sobre una copia en el scratchpad: fallaron las pruebas al quitar la comprobación de fork, permitir `latest`, aceptar mayúsculas en el SHA, no pasar el repo ni el registro a minúsculas, cambiar el registro por defecto, aflojar las expresiones del tag y del repo, intercambiar los `kind`, comparar el fork con distinción de mayúsculas y cambiar el 422 por 503. Sobrevivió una (quitar `HeadRepo == ""`), equivalente: `EqualFold("", repo)` sigue rechazando.

## Hallazgos (sin ROJO ni NARANJA)
- **F-01 · AMARILLO · `artifact_test.go`:** «PR reopened» solo cambia `HeadRepo` a mayúsculas; ninguna prueba ejercita `reopened` de punta a punta (no hay fixture). El clasificador la acepta en `classify.go:79`.
- **F-02 · AMARILLO · `artifact_test.go`:** tags con `/`, `;`, `..` o saltos de línea no tienen caso propio; los cubre la expresión regular y la mutación que la aflojó fue detectada por otro caso.
- **F-03 · AMARILLO · `artifact.go`:** `repoRE` (definida por la tarea) acepta `../..` como `owner/repo`. Consecuencia de la especificación; conviene endurecerla.
- **F-04 · AMARILLO · `artifact.go`:** `regRE` acepta segmentos `..` en `ARTIFACT_REGISTRY`. Es configuración del operador, no entrada externa.
- **F-05 · AMARILLO · `artifact.go`:** tags con `/` se rechazan con 422 aunque son legales en git (`release/v1`). Decisión de producto.
- **F-06 · AMARILLO · `handler_test.go`:** «release con SHA» ya no pasa por el resolvedor; cubierto por `TestWebhookClassification`.
- **F-07 · AMARILLO · bitácora:** los scripts de CA-2/CA-3 se ejecutaron con `-w` y `2>/dev/null`; la bitácora lo declara.
- **F-08 · AMARILLO · bitácora:** el «rojo primero» de CA-2 dice que el release dio 400 con el stub, mientras la tarea decía «las cuatro con `build-from-repo`». Desviación explicada por C-75.
- **F-09 · AMARILLO · `go.mod`:** `govulncheck` no corre en el entorno del loop (`vuln.go.dev` da 403); lo confirma el job `vuln` de la CI.

## Tareas candidatas
- C-75 sigue abierta (release con `target_commitish` de rama → 400 en producción).
- C-11: documentar el 422 `unresolvable_artifact` en el OpenAPI.
- Endurecer `repoRE` y `regRE` contra segmentos `..` (F-03, F-04).
- Decisión de producto sobre tags con `/` (F-05).

VEREDICTO: VERDE
INFORME: revisiones/U1-T03/ronda-1.md
