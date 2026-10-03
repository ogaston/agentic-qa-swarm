# U1-T03 — Resolución de artefacto V5: build-from-repo vs imagen publicada

**Unidad:** U1 — Ingesta & Inbox
**Historias que implementa:** US-M2 (el artefacto que se desplegará sobre el warm queda fijado en la notificación)
**Depende de:** U1-T02 (interfaz `ArtifactResolver`, handler del webhook, payloads de `testdata/github/`).

---

## Alcance

**Dentro** (una línea, concreta):

> Implementar `internal/artifact` en `services/go-intake`: una función pura `Resolve(event)` que, a partir de un evento de GitHub ya clasificado, devuelve el `ArtifactRef` `{kind, ref}` (`build-from-repo` para commit/PR, `published-image` para tag/release) o un error tipado, y cablearla en el handler del webhook en lugar del stub de U1-T02.

Reglas (V5):

| Evento clasificado | `kind` | `ref` |
|---|---|---|
| `commit` (push a rama) | `build-from-repo` | `<owner>/<repo>@<sha 40 hex>` |
| `pull_request` (`opened`/`synchronize`/`reopened`, mismo repositorio) | `build-from-repo` | `<owner>/<repo>@<head sha>` |
| `tag` (push a `refs/tags/*` o `release` `published`) | `published-image` | `<registro>/<owner>/<repo>:<tag>` |

- `<registro>` sale de `ARTIFACT_REGISTRY` (por defecto `ghcr.io`), en minúsculas; el repositorio también se pasa a minúsculas en la imagen (los registros OCI lo exigen).
- **Fail-closed.** Se rechaza con error tipado (`ErrUnresolvableArtifact`) y el webhook responde `422` con `Error{code: "unresolvable_artifact"}` (el OpenAPI de U5 no lo lista; se documenta en la bitácora y se propone a C-11, **no se edita el contrato**) en estos casos:
  - PR **desde un fork** (`head.repo.full_name != base.repo.full_name`): el código no es del repositorio conectado. Es una decisión de seguridad conservadora; si el humano quiere permitirlos, es una candidata.
  - Tag `latest` o vacío, o que no cumpla `^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$` (la plataforma no despliega referencias móviles).
  - `owner/repo` que no cumpla `^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`.
  - SHA que no sea 40 hex en minúsculas.
- La notificación creada por U1-T02 lleva el `artifact` resuelto; el `notify.created` publicado lo repite (el esquema ya lo exige).
- La resolución **no** hace llamadas de red (no verifica que la imagen exista): eso lo hace `go-warm-manager` al desplegar (U2).

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Verificar que la imagen o el commit existan en el registro/GitHub, o cualquier cliente HTTP saliente.
- Construir imágenes o invocar un `ContainerRuntime`: es C3, **U2**.
- Modificar el esquema `notify.created`, el OpenAPI o los `Notification` ya persistidos (no hay migración: el almacén es de transición).
- Soportar fork PRs, `workflow_run`, `deployment` u otros eventos.
- Cambios en la verificación de firma, el almacén o el `outbox` más allá del cableado.

---

## Archivos de contexto

- `unidades-y-tareas.md` (sección U1)
- `aidlc-docs/inception/application-design/unit-task-plans/U1.md`
- `aidlc-docs/inception/application-design/components.md` (C1: puerto `ArtifactRef`; C3: quién despliega)
- `aidlc-docs/inception/requirements/requirements.md` (V5)
- `contracts/events/notify.created.schema.json`, `contracts/openapi/control-plane.yaml` (`Notification.artifact`)
- `services/go-intake/` (U1-T01 y U1-T02), `services/go-intake/testdata/github/`
- `tareas/candidatas.md` (C-11)

---

## Criterios de aceptación

Desde la raíz del worktree.

- [ ] **CA-1** — Las pruebas parametrizadas cubren cada tipo de evento y cada rechazo.
  ```bash
  cd services/go-intake && go test -run Artifact -v ./internal/artifact/ | grep -c -E '^\s+--- PASS'; go test ./... 2>&1 | grep -c FAIL
  ```
  Esperado: ≥ `12` subpruebas y `0`. Casos mínimos: commit, PR opened, PR synchronize, PR reopened, push tag, release published, PR de fork, tag `latest`, tag con espacios, tag vacío, repo con caracteres inválidos, SHA corto, SHA con mayúsculas, `ARTIFACT_REGISTRY` personalizado, repo con mayúsculas→imagen en minúsculas.

- [ ] **CA-2** — Cada payload de ejemplo produce el `ArtifactRef` esperado, medido con el binario real.
  ```bash
  t=$(mktemp -d); (cd services/go-intake && go build -o "$t/go-intake" ./cmd/go-intake)
  GITHUB_WEBHOOK_SECRET=s3cret INTAKE_DATA_DIR="$t/d" INTAKE_EVENTS_FILE="$t/e.jsonl" LISTEN_ADDR=127.0.0.1:18083 "$t/go-intake" & pid=$!; sleep 1
  n=0; for p in "push push-branch" "push push-tag" "pull_request pull-request-opened" "release release-published"; do set -- $p; n=$((n+1)); b=$(cat services/go-intake/testdata/github/$2.json); s=sha256=$(printf '%s' "$b" | openssl dgst -sha256 -hmac s3cret | sed 's/^.* //')
    curl -s -o /dev/null -X POST http://127.0.0.1:18083/webhooks/github -H 'Content-Type: application/json' -H "X-GitHub-Event: $1" -H "X-GitHub-Delivery: d-$n" -H "X-Hub-Signature-256: $s" --data-binary "$b"; done
  jq -r '.data.github_event + " " + .data.artifact.kind + " " + .data.artifact.ref' "$t/e.jsonl"; kill $pid; rm -rf "$t"
  ```
  Esperado: cuatro líneas, en este orden: `commit build-from-repo <owner/repo>@<40 hex>`, `tag published-image ghcr.io/<owner>/<repo>:v1.2.0`, `pull_request build-from-repo <owner/repo>@<head sha>`, `tag published-image ghcr.io/<owner>/<repo>:<tag del release>`. Antes de la tarea (con el stub): las cuatro con `build-from-repo` (rojo inicial para tag y release).

- [ ] **CA-3** — Un PR de fork no crea notificación ni evento y responde `422`.
  ```bash
  t=$(mktemp -d); (cd services/go-intake && go build -o "$t/go-intake" ./cmd/go-intake)
  GITHUB_WEBHOOK_SECRET=s3cret INTAKE_DATA_DIR="$t/d" INTAKE_EVENTS_FILE="$t/e.jsonl" LISTEN_ADDR=127.0.0.1:18084 "$t/go-intake" & pid=$!; sleep 1
  b=$(cat services/go-intake/testdata/github/pull-request-fork.json); s=sha256=$(printf '%s' "$b" | openssl dgst -sha256 -hmac s3cret | sed 's/^.* //')
  curl -s -o "$t/r.json" -w '%{http_code}\n' -X POST http://127.0.0.1:18084/webhooks/github -H 'Content-Type: application/json' -H 'X-GitHub-Event: pull_request' -H 'X-GitHub-Delivery: d-fork' -H "X-Hub-Signature-256: $s" --data-binary "$b"
  jq -r .code "$t/r.json"; test -s "$t/e.jsonl" && echo "HAY EVENTOS" || echo "sin eventos"; kill $pid; rm -rf "$t"
  ```
  Esperado: `422`, `unresolvable_artifact` y `sin eventos`.

- [ ] **CA-4** — La resolución es pura: sin red ni reloj.
  ```bash
  go list -C services/go-intake -deps ./internal/artifact | grep -c -E '^(net/http|net)$|k8s.io'; grep -c -E 'time\.Now|http\.' services/go-intake/internal/artifact/*.go
  ```
  Esperado: `0` y `0` (en los archivos que no son `_test.go`; el codificador ajusta el `grep` con `--exclude='*_test.go'` si hace falta y lo anota).

- [ ] **CA-5** — Higiene y alcance.
  ```bash
  cd services/go-intake && go vet ./... && test -z "$(gofmt -l .)" && echo ok; cd - >/dev/null; git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/go-intake/|bitacoras/U1-T03.md)' | wc -l; git diff --name-only $b -- contracts | wc -l
  ```
  Esperado: `ok`, `0`, `0` y `0`.

---

## Plan de pruebas

- Tabla de CA-1 (tipo de evento → `kind`/`ref`) y una segunda de rechazos con el error esperado (`errors.Is(err, ErrUnresolvableArtifact)`).
- Cada `ref` de salida se comprueba contra el esquema de `notify.created` (compilado en la prueba), no solo contra una cadena esperada.
- Negativa: un `ArtifactResolver` que devuelve error hace que el handler responda `422` y **no** persista ni publique (la misma garantía de U1-T02, ahora con la implementación real).

**Rojo primero:** el codificador registra en su bitácora el resultado de CA-2 con el stub de U1-T02: tag y release salen como `build-from-repo`.

---

## Notas

- Archivos que se **modifican en su sitio**: el handler/cableado de `cmd/go-intake` y de `internal/` que usaban el stub de U1-T02; se elimina el stub. Nada de duplicados con sufijo.
- El `422` no está en el OpenAPI de U5 para `/webhooks/github`. No se edita el contrato aquí; se anota en la bitácora para C-11.
- El mapa de eventos a `github_event` ya lo fijó U1-T02; esta tarea no lo cambia.
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
