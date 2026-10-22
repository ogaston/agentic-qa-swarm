# chore/go-1.26.10 — bitácora (fuera del loop de tareas)

Objetivo: subir toolchain Go 1.26.9 → 1.26.10 para cerrar GO-2026-6809 (crypto/x509, fijado en go1.26.10).

## Verificación de la etiqueta de imagen
$ skopeo inspect docker://docker.io/library/golang:1.26.10-alpine3.24  → exit 0 (existe; RepoTags contiene 1.26.10)

## Cambios (20 archivos, 20 líneas)
- services/*/go.mod (9) y demo/target-app/go.mod: `go 1.26.9` → `go 1.26.10`. Ninguna otra línea (sin toolchain, sin dependencias).
- services/*/Dockerfile (8) y demo/target-app/Dockerfile: `golang:1.26.9-alpine3.24` → `golang:1.26.10-alpine3.24`.
- scripts/test/u8-runner-contract.sh y scripts/test/u8-target-app-local.sh: comentarios que citaban 1.26.9.
- .github/workflows: sin cambios. Leen go-version-file: <servicio>/go.mod, así que ya toman 1.26.10.

## Grep de residuos
$ git grep -n "1\.26\.9" -- . ':!*.md' ':!bitacoras' ':!revisiones' | wc -l
0

## Build / vet / test (GOTOOLCHAIN=go1.26.10, -count=1)
Por módulo: `go version` → go1.26.10 linux/amd64; `go build ./...` OK; `go vet ./...` OK; `go test -count=1 ./...` sin FAIL (EXIT=0):
aqs-runner, go-governance, go-identity, go-intake, go-reset, go-run-controller, go-warm-manager, ui-api, demo/target-app.

## govulncheck (invocación de CI: `go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...`, GOTOOLCHAIN=go1.26.10)
- services/aqs-runner: "No vulnerabilities found." EXIT=0
- services/go-governance: "No vulnerabilities found." EXIT=0
- services/go-reset: "No vulnerabilities found." EXIT=0
- services/go-identity, go-intake, go-run-controller, go-warm-manager, ui-api: "Your code is affected by 0 vulnerabilities." EXIT=0 (hay 1 vulnerabilidad en módulos requeridos, no alcanzada por el código; no bloquea).
- demo/target-app: EXIT=1 — GO-2026-6629 y GO-2026-5970 en golang.org/x/text@v0.29.0 (fijado en go.mod sin cambios; corregido en v0.41.0 / v0.39.0). Preexistente, no lo causa este cambio. demo/target-app NO está en el alcance de CI (scripts/ci/list-services.sh solo lista services/ y agents/), por eso no bloquea el job vuln. Tarea candidata.

## Imagen
$ cd services/ui-api && podman build -t qa-swarm-ui-api-go1.26.10:test -f Dockerfile .
→ "Successfully tagged ...", BUILD_EXIT=0 (imagen de prueba eliminada después).
