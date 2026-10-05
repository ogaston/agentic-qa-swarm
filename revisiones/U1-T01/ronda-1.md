# Ronda 1 — U1-T01

VEREDICTO: VERDE

Verifiqué el SHA 2e092b6adce039c8ba686a4315a089c2ef20a0af en rama `tarea/U1-T01`. El worktree quedó limpio después de la revisión (`git status --short` da 0 líneas).

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Pruebas de ambos módulos | `go test -count=1 ./...` en go-intake y ui-api | pasa: `ok` en `contract`, `githubsig` e `inbox`, sin `FAIL` |
| 2 | Ejemplos REST vs. OpenAPI con ajv | El comando literal, con la extracción de esquemas hecha con python3 + PyYAML | pasa: 6 `valid rc=0` y 5 `invalid rejected=0` |
| 3 | `FakeVerifier` y `Sign` | `go test -run 'FakeVerifier\|Sign' -v` y `openssl` | pasa: 6 `--- PASS`, incluida `TestFakeVerifierDefaultRejects`; el vector `sha256=5910e620…4f6d` de `openssl` es igual a la constante de `TestSignVector` |
| 4 | Payloads de GitHub | El bloque literal de `jq` | pasa: `7`, sin `ROTO`, `acme/shop`, `true`, `refs/heads/main`, `refs/tags/v1.2.0` |
| 5 | Higiene del módulo | `go vet`, `gofmt -l`, `go.sum` no vacío, sin `replace`, sin `go.work`, deps `k8s.io`, `list-services.sh` | pasa: `go-intake ok`, `ui-api ok`, `sin go.work`, `0`, `0` |
| 6 | Árbol limpio y alcance | `git status --short \| wc -l` y diff fuera de alcance contra `origin/main` | pasa: `0` y `0` |

**Sustitución de `yq` en CA-2.** Es cierta y válida.
- `/usr/bin/docker` existe, pero el daemon no responde: `Cannot connect to the Docker daemon`.
- `/usr/bin/yq` es un script Python (`yq 0.0.0`), no el de mikefarah, y no acepta `-o=json`.
- Extraje `components.schemas.Notification` y `ConfirmationReceipt` con `yaml.safe_load` (557 y 274 bytes) y corrí `ajv` con las mismas versiones fijadas (ajv-cli 5.0.0, ajv-formats 3.0.1, draft2020). Es equivalente.
- Volví a correr cada inválido y confirmé que el rechazo es por la razón prevista, no por esquema vacío: `state` y `artifact.kind` → `enum`; `artifact` con campo extra → `additionalProperties`; `confirmed_at` → `format date-time`; falta `run_id` → `required`.

**Alcance.** El diff solo toca `services/go-intake/`, `services/ui-api/`, `contracts/openapi/examples/` y `bitacoras/U1-T01.md`. No hay `Dockerfile`, `cmd/`, `go.work`, `replace`, `k8s.io` ni cambios en contratos, `validate.sh` o workflows. Ambos `go.mod` declaran `go 1.26.8`. `Sign` y `FakeVerifier` cumplen lo pedido: fail-closed, tres modos, y `AcceptOnly("")` no acepta encabezado vacío.

## Hallazgos
### F-01 · AMARILLO · services/go-intake/testdata/github/release-published.json · El payload de release no lleva SHA de 40 hex
La tarea pide «el SHA de 40 hex que corresponda». `release-published.json` solo trae `target_commitish: "main"`. Eso se parece al evento real de GitHub, y `ping.json` tampoco lleva SHA. CA-4 no lo comprueba, así que no bloquea. Conviene que U1-T03 decida cómo resuelve el SHA de un release.

### F-02 · AMARILLO · bitacoras/U1-T01.md:26-45 · Bloque de evidencia inválido dejado en la bitácora
El primer bloque de CA-2 muestra `rc=0` y `rejected=0` con esquemas vacíos, o sea evidencia que no demuestra nada. El codificador lo marcó como inválido y lo repitió con la extracción en Python. También dejó una corrección en línea sobre los conteos de ejemplos (línea 98). Es solo ruido; la evidencia buena existe y la reproduje.

### F-03 · AMARILLO · services/ui-api/inbox/inbox_test.go · Prueba `rapid` y dependencia directa añadidas para llenar `go.sum`
Es una prueba mínima que no estaba pedida, y la bitácora la justifica porque CA-5 exige `go.sum` no vacío. Es coherente con U1-T05 (`rapid`) y no desborda el alcance.

## Tareas candidatas (defectos reales fuera de alcance)
- Ninguna.

VEREDICTO: VERDE
AMARILLO|services/go-intake/testdata/github/release-published.json|Payload de release sin SHA de 40 hex (no bloquea)
