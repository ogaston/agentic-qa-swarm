# U1-T02 — `go-intake`: `POST /webhooks/github` (firma, notificación, `notify.created`)

**Unidad:** U1 — Ingesta & Inbox
**Historias que implementa:** US-M1 (y la parte de US-M8.3: «con auto-run apagado, un evento GitHub nunca crea Jobs»)
**Depende de:** U1-T01 (módulo `services/go-intake`, `githubsig`, payloads de `testdata/github/`, ejemplos REST).

---

## Alcance

**Dentro** (una línea, concreta):

> Convertir `services/go-intake` en un servicio ejecutable (`cmd/go-intake`, `Dockerfile`) que sirve `POST /webhooks/github`: verifica la firma HMAC-SHA256 **antes de parsear**, clasifica el evento, crea una `Notification` en estado `pending`, la persiste, publica `notify.created` y responde `202` con la `Notification`. Es idempotente por `X-GitHub-Delivery`. No crea Jobs y no importa ningún cliente de Kubernetes.

Comportamiento exacto:

- **Firma.** Implementación real de `githubsig.Verifier`: `hmac.Equal` sobre el HMAC-SHA256 del cuerpo crudo. Secreto en `GITHUB_WEBHOOK_SECRET` (el servicio **no arranca** si está vacío; nunca se loguea). Firma ausente, mal formada o incorrecta → `401` con `Error{code: "invalid_signature"}`, sin leer más del cuerpo ni tocar el almacén.
- **Límites.** Cuerpo máximo 1 MiB (`http.MaxBytesReader`; excedido → `413`). Solo `Content-Type: application/json`. Timeouts de servidor explícitos (`ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout`).
- **Clasificación** (`X-GitHub-Event` + `action`/`ref`) a `github_event`:
  - `push` a `refs/heads/*` → `commit`; `push` a `refs/tags/*` → `tag`.
  - `pull_request` con `action` en `opened`, `synchronize`, `reopened` → `pull_request` (SHA = `pull_request.head.sha`).
  - `release` con `action: published` → `tag`.
  - Cualquier otro evento o acción (incluido `ping`) → `400` con `Error{code: "unsupported_event"}`; no se crea notificación. (Decisión de esta tarea; si el humano prefiere `2xx` para `ping`, se abre candidata.)
- **Artefacto.** Hasta U1-T03 se usa un `ArtifactResolver` **stub** (interfaz + implementación fija: `build-from-repo`, ref `<owner/repo>@<sha>`). U1-T03 lo reemplaza; esta tarea solo define la interfaz y la usa en el handler.
- **Persistencia (`NotificationStore`).** Puerto con una implementación **JSONL en disco** (`INTAKE_DATA_DIR`, solo se agrega, `fsync` por escritura) que sobrevive a un reinicio. Mantiene el índice `delivery_id → notification_id`; la misma entrega recibida dos veces devuelve **la misma** notificación y no publica un segundo evento.
- **Publicación (`EventPublisher`).** Puerto con una implementación `outbox` que agrega líneas JSON a `INTAKE_EVENTS_FILE`. Es el **marcador de posición** del transporte de eventos mientras no se decida (candidata C-45); no inventa broker. El evento cumple `contracts/events/notify.created.schema.json`: `event_id` uuid v4, `version: 1`, `occurred_at` UTC, `trace_id` (se toma de `traceparent` si viene; si no, se genera de 32 hex) y `data` con `notification_id`, `github_event`, `repo`, `sha`, `artifact`.
- **Orden y fallo.** Primero se persiste y luego se publica. Si publicar falla, responde `503` y la notificación queda marcada `publish_pending`; un reintento de la misma entrega la publica sin duplicar la notificación.
- **Dockerfile.** Multi-etapa, imagen de build y de runtime con **tag fijado** (no `latest`), binario estático, usuario no root, `EXPOSE 8080`. Escucha en `:8080` (coincide con `deploy/flux/base/control-plane.yaml`).

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Resolución real de artefacto (V5): **U1-T03**. Aquí solo el stub.
- `GET /notifications` y `POST .../confirm`: viven en `ui-api` (**U1-T04**). `go-intake` no expone lectura.
- `/healthz`, `/readyz`, `/metrics`, logging estructurado completo, trazas: **U1-T06**. Aquí basta con un `log.Printf` sin secretos ni cuerpo.
- Broker, base de datos, cliente HTTP saliente: decisión pendiente (C-45). No se añade ninguno.
- Importar `k8s.io/*`, `client-go` o llamar a la API de Kubernetes.
- Manifiestos (`deploy/flux/**`), workflows, `contracts/**`. Si el `Dockerfile` hace fallar `ci.yml`, se arregla el `Dockerfile`, no el workflow.
- Rate limiting del webhook (el límite de GitHub y el del Gateway son de U1-T04 y de infraestructura).

---

## Archivos de contexto

- `unidades-y-tareas.md` (sección U1)
- `aidlc-docs/inception/application-design/unit-task-plans/U1.md`
- `aidlc-docs/inception/application-design/components.md` (C1)
- `contracts/openapi/control-plane.yaml` (`/webhooks/github`, `Notification`, `Error`)
- `contracts/events/notify.created.schema.json` y su ejemplo en `contracts/events/examples/`
- `services/go-intake/` (lo que dejó U1-T01, incluidos `testdata/github/`)
- `deploy/flux/base/control-plane.yaml` (puerto 8080, nombre de imagen)
- `scripts/ci/list-services.sh`, `.github/workflows/ci.yml`
- `tareas/candidatas.md`

---

## Criterios de aceptación

Desde la raíz del worktree. Alias:

```bash
AJV=(npx --yes -p ajv-cli@5.0.0 -p ajv-formats@3.0.1 ajv)
sig() { printf '%s' "$2" | openssl dgst -sha256 -hmac "$1" | sed 's/^.* /sha256=/'; }   # sig <secreto> <cuerpo>
```

- [ ] **CA-1** — Las pruebas unitarias del webhook pasan, con al menos 9 casos.
  ```bash
  cd services/go-intake && go test -run Webhook -v ./... | grep -c -E '^\s*--- PASS'; go test ./... 2>&1 | grep -c FAIL
  ```
  Esperado: un número ≥ `9` y `0`. Cubren: firma válida→202; firma inválida→401; sin firma→401; firma válida sobre cuerpo alterado→401; JSON roto→400; evento no soportado→400; cuerpo >1 MiB→413; entrega repetida→misma notificación; `Content-Type` incorrecto→415 o 400.

- [ ] **CA-2** — De extremo a extremo con el binario real: un evento firmado crea **una** notificación y publica **un** evento válido contra el esquema.
  ```bash
  t=$(mktemp -d); (cd services/go-intake && go build -o "$t/go-intake" ./cmd/go-intake)
  GITHUB_WEBHOOK_SECRET=s3cret INTAKE_DATA_DIR="$t/data" INTAKE_EVENTS_FILE="$t/events.jsonl" LISTEN_ADDR=127.0.0.1:18080 "$t/go-intake" & pid=$!; sleep 1
  body=$(cat services/go-intake/testdata/github/pull-request-opened.json)
  curl -s -o "$t/resp.json" -w '%{http_code}\n' -X POST http://127.0.0.1:18080/webhooks/github -H 'Content-Type: application/json' -H 'X-GitHub-Event: pull_request' -H 'X-GitHub-Delivery: d-1' -H "X-Hub-Signature-256: $(sig s3cret "$body")" --data-binary "$body"
  wc -l < "$t/events.jsonl"; head -n1 "$t/events.jsonl" > "$t/ev.json"
  "${AJV[@]}" validate --spec=draft2020 -c ajv-formats -s contracts/events/notify.created.schema.json -d "$t/ev.json"
  jq -r '.state' "$t/resp.json"; kill $pid; rm -rf "$t"
  ```
  Esperado: `202`, `1`, `…/ev.json valid` y `pending`. Antes de la tarea: `no such file or directory` en el `go build` (rojo inicial).

- [ ] **CA-3** — Firma inválida: `401`, y ni almacén ni salida de eventos cambian.
  ```bash
  t=$(mktemp -d); (cd services/go-intake && go build -o "$t/go-intake" ./cmd/go-intake)
  GITHUB_WEBHOOK_SECRET=s3cret INTAKE_DATA_DIR="$t/data" INTAKE_EVENTS_FILE="$t/events.jsonl" LISTEN_ADDR=127.0.0.1:18081 "$t/go-intake" & pid=$!; sleep 1
  curl -s -o /dev/null -w '%{http_code}\n' -X POST http://127.0.0.1:18081/webhooks/github -H 'Content-Type: application/json' -H 'X-GitHub-Event: push' -H 'X-Hub-Signature-256: sha256=00' -d '{}'
  ls "$t/data" 2>/dev/null | wc -l; test -s "$t/events.jsonl" && echo "HAY EVENTOS" || echo "sin eventos"; kill $pid; rm -rf "$t"
  ```
  Esperado: `401`, `0` (o el directorio vacío) y `sin eventos`.

- [ ] **CA-4** — Idempotencia y persistencia: la misma entrega dos veces, y un reinicio entre medias, dan **una** notificación y **un** evento.
  ```bash
  cd services/go-intake && go test -run 'Idempotent|Restart' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS` en al menos una prueba `Idempotent…` y otra `Restart…`; ninguna `FAIL`.

- [ ] **CA-5** — «Nunca crea Jobs»: el binario no depende de ninguna librería de Kubernetes y el servicio no pide permisos de API.
  ```bash
  go list -C services/go-intake -deps ./... | grep -c -E 'k8s.io|client-go'; grep -r -c -E 'k8s.io|batchv1|clientset' services/go-intake --include='*.go' | awk -F: '{s+=$2} END {print s}'
  ```
  Esperado: `0` y `0`. (La comprobación en un clúster, `kubectl get jobs -n <test-ns>` vacío, la hace el humano en dev y se **documenta** en la bitácora; ningún comando contra un clúster en esta tarea.)

- [ ] **CA-6** — El secreto es obligatorio y no se filtra.
  ```bash
  t=$(mktemp -d); (cd services/go-intake && go build -o "$t/go-intake" ./cmd/go-intake)
  env -u GITHUB_WEBHOOK_SECRET INTAKE_DATA_DIR="$t/d" INTAKE_EVENTS_FILE="$t/e" "$t/go-intake" >/dev/null 2>&1; echo "rc=$?"
  GITHUB_WEBHOOK_SECRET=topsecretvalue INTAKE_DATA_DIR="$t/d" INTAKE_EVENTS_FILE="$t/e" LISTEN_ADDR=127.0.0.1:18082 "$t/go-intake" > "$t/log" 2>&1 & pid=$!; sleep 1
  curl -s -o /dev/null -X POST http://127.0.0.1:18082/webhooks/github -H 'X-Hub-Signature-256: sha256=00' -d '{}'; kill $pid
  grep -c topsecretvalue "$t/log"; rm -rf "$t"
  ```
  Esperado: `rc=` distinto de `0` y `0`.

- [ ] **CA-7** — Imagen: construye, usa tags fijados, corre sin root y la CI la detecta.
  ```bash
  docker build -q -t aqs-go-intake:ci services/go-intake >/dev/null && docker run --rm --entrypoint id aqs-go-intake:ci -u 2>/dev/null || docker inspect aqs-go-intake:ci --format '{{.Config.User}}'
  grep -E '^FROM' services/go-intake/Dockerfile | grep -c -i -E ':latest|^FROM [a-z0-9./-]+$'
  bash scripts/ci/list-services.sh | grep -c 'services/go-intake'
  bash scripts/ci/detect-lang.sh services/go-intake
  ```
  Esperado: un UID distinto de `0` (o un `User` no vacío y no `root`); `0` (ningún `FROM` sin tag ni con `latest`); `1`; `go`.

- [ ] **CA-8** — Higiene.
  ```bash
  cd services/go-intake && go vet ./... && test -z "$(gofmt -l .)" && echo ok; cd - >/dev/null; git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/go-intake/|bitacoras/U1-T02.md)' | wc -l
  ```
  Esperado: `ok`, `0` y `0`.

---

## Plan de pruebas

- Tabla de casos de CA-1 con `httptest` y el `FakeVerifier` de U1-T01 donde convenga, **y** una prueba con la verificación real y el vector fijo de U1-T01.
- Negativa de firma: cuerpo alterado en un byte con la firma original; secreto distinto; cabecera sin `sha256=`; hex impar.
- Persistencia: escribir, reiniciar el `NotificationStore` sobre el mismo directorio y leer; archivo truncado a media línea no impide arrancar (la línea rota se ignora y se registra).
- Fallo de publicación: `EventPublisher` que falla → `503`; segundo intento de la misma entrega publica una vez.
- `notify.created` generado por el servicio valida contra el esquema (prueba Go **y** `ajv` en CA-2).

**Rojo primero:** el codificador registra en su bitácora la salida del `go build ./cmd/go-intake` antes de crear nada (falla: no existe `cmd/`).

---

## Notas

- Archivos que se **modifican en su sitio**: ninguno de U1-T01 salvo para añadir interfaces o constructores en `githubsig`. Nada de duplicados con sufijo.
- El `outbox` y el JSONL son soluciones de transición: están detrás de puertos precisamente para que C-45 (transporte y persistencia del control plane) las reemplace sin tocar el handler. No las vendas como decisión de arquitectura en la bitácora.
- `notification_id`: ULID o `n-<uuid>`; lo fija el codificador y lo documenta. Debe pasar `minLength: 1` y no contener `/`.
- La firma se verifica sobre el **cuerpo crudo**, no sobre el JSON reserializado.
- Ningún comando contra un clúster ni la nube. El único servidor que se levanta es local, en `127.0.0.1`.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
