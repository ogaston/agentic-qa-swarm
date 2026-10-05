# U1-T04 — `ui-api` inbox: `GET /notifications` + `POST /notifications/{id}/confirm`

**Unidad:** U1 — Ingesta & Inbox
**Historias que implementa:** US-M1 (inbox), US-M2 (confirmación persistida)
**Depende de:** U1-T01 (módulo `services/ui-api`, ejemplos REST), U1-T02 (forma de `notify.created` y del `outbox`). En paralelo con U1-T03.

---

## Alcance

**Dentro** (una línea, concreta):

> Convertir `services/ui-api` en un servicio ejecutable (`cmd/ui-api`, `Dockerfile`) que mantiene la proyección del inbox a partir de eventos `notify.created`, sirve `GET /notifications` y `POST /notifications/{id}/confirm` detrás de autenticación por token (puerto `TokenVerifier`, con un fake local), con rate limiting, security headers, CORS restringido y persistencia append-only de cada confirmación.

Detalle:

- **Entrada de notificaciones.** Puerto `EventSubscriber` con una implementación que **lee el `outbox` JSONL** de `go-intake` (`UIAPI_EVENTS_FILE`, tail desde el inicio, idempotente por `event_id`). Cada `notify.created` válido contra el esquema crea una entrada `pending` en la proyección; uno inválido se descarta y se cuenta. Es transición (candidata C-45), igual que en U1-T02.
- **`GET /notifications`** → `200` con `[]Notification` (forma del OpenAPI). Filtro opcional `?state=pending|confirmed|rejected`; valor fuera del enum → `400`. Orden: más reciente primero. Sin token → `401`.
- **`POST /notifications/{id}/confirm`** con `{"flows": [..]}` (`minItems: 1`):
  - `201` con `ConfirmationReceipt{run_id, notification_id, confirmed_by, confirmed_at}`. `confirmed_by` es el `Principal.ID` del token (nunca un campo del cuerpo). `run_id` único (formato documentado por el codificador).
  - `404` si la notificación no existe; `409` si ya estaba confirmada (la segunda confirmación **no** crea otro recibo); `400` si `flows` falta, está vacío o tiene elementos vacíos; `401` sin token.
  - La confirmación se persiste en un JSONL **solo-agregar** (`UIAPI_DATA_DIR`, `fsync` por escritura) y el estado de la notificación pasa a `confirmed` leyendo de ese registro (sobrevive a un reinicio). **Publicar `run.confirmed` es de U1-T07**, no de esta tarea.
- **Autenticación.** Puerto `TokenVerifier { Verify(ctx, bearer string) (Principal, error) }`, `Principal{ID string, Role string}` con roles `user` y `admin` (los de las personas; sin roles inventados). Esta tarea trae un `FakeTokenVerifier` (tokens fijos por configuración de prueba) y **ninguna** implementación real; la real es U1-T07 contra `go-identity`. Cualquier error del verificador → `401` (fail-closed). Sin esquema `Bearer` → `401`.
- **Rate limiting** por IP en los endpoints expuestos (token bucket en memoria; `UIAPI_RATE_LIMIT_RPS`, por defecto 10, y `BURST`, por defecto 20). Excedido → `429` con `Retry-After`. Detrás de un proxy, se usa `X-Forwarded-For` **solo** si `UIAPI_TRUST_PROXY=true`.
- **Security headers** en todas las respuestas (incluidas las de error): `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'`, `Strict-Transport-Security: max-age=63072000; includeSubDomains`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, `Cache-Control: no-store`.
- **CORS** restringido a la lista `UIAPI_ALLOWED_ORIGINS` (vacía = ninguno). Nunca `*`. Un `Origin` no listado no recibe `Access-Control-Allow-Origin`.
- **Límites.** Cuerpo máximo 64 KiB; timeouts de servidor explícitos; `Content-Type: application/json` obligatorio en el `POST`.
- **Dockerfile** como el de U1-T02 (tags fijados, no root, `:8080`).

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Verificación real de tokens, login o sesiones: **U4-T02/U4-T03**; integración en **U1-T07**.
- Autorización por propietario del recurso (IDOR) sobre notificaciones/corridas: no hay `owner` en el contrato; es la candidata C-47. Aquí solo autenticación y el rol es informativo.
- Publicar `run.confirmed` y cualquier llamada a `go-run-controller`: **U1-T07** y U2.
- UI HTML/estáticos. `ui-api` en este MVP es API JSON; si hay HTML en el futuro, los headers ya están.
- `/healthz`, `/readyz`, `/metrics`, logging estructurado completo: **U1-T06**.
- `GET /runs`, `/reports`, `/policies`, `/audit` (la fachada de lectura C6/C7/C9 llega con U2/U3/U4).
- Cambios en `contracts/**`, `deploy/**` o workflows.

---

## Archivos de contexto

- `unidades-y-tareas.md` (sección U1)
- `aidlc-docs/inception/application-design/unit-task-plans/U1.md`
- `aidlc-docs/inception/application-design/components.md` (C1, C8)
- `aidlc-docs/inception/requirements/requirements.md` (NF-SEG-04, NF-SEG-08, rate limiting)
- `contracts/openapi/control-plane.yaml` (`/notifications`, `/notifications/{id}/confirm`, `Notification`, `ConfirmationReceipt`, `Error`)
- `contracts/openapi/examples/` y `contracts/events/notify.created.schema.json`
- `services/ui-api/` (U1-T01) y `services/go-intake/` (formato del `outbox` de U1-T02)
- `deploy/flux/base/control-plane.yaml` (puerto 8080)

---

## Criterios de aceptación

Desde la raíz del worktree. Alias y arranque común:

```bash
start() { t=$(mktemp -d); (cd services/ui-api && go build -o "$t/ui-api" ./cmd/ui-api)
  jq -c . contracts/events/examples/valid/notify.created.json > "$t/events.jsonl"
  UIAPI_EVENTS_FILE="$t/events.jsonl" UIAPI_DATA_DIR="$t/data" UIAPI_FAKE_TOKENS='tok-user=u1:user,tok-admin=a1:admin' UIAPI_ALLOWED_ORIGINS='https://app.example' UIAPI_RATE_LIMIT_RPS=5 UIAPI_RATE_LIMIT_BURST=5 LISTEN_ADDR=127.0.0.1:$1 "$t/ui-api" & pid=$!; sleep 1; }
```

(`UIAPI_FAKE_TOKENS` solo lo lee el `FakeTokenVerifier`, que se selecciona con `UIAPI_AUTH=fake`; el codificador fija el nombre exacto y lo documenta. En el binario de producción el fake no se puede activar sin esa variable explícita.)

- [ ] **CA-1** — Las pruebas unitarias pasan, con al menos 12 casos.
  ```bash
  cd services/ui-api && go test -v ./... | grep -c -E '^\s*--- PASS'; go test ./... 2>&1 | grep -c FAIL
  ```
  Esperado: ≥ `12` y `0`. Cubren: listar con/sin filtro, filtro inválido, sin token, token inválido, confirmar ok, confirmar inexistente, doble confirmación, `flows` vacío, cuerpo demasiado grande, `confirmed_by` tomado del token y no del cuerpo, verificador que falla→401, evento inválido descartado. Antes de la tarea: `no such file or directory` (rojo inicial).

- [ ] **CA-2** — Con token válido, el inbox devuelve la notificación y valida contra el contrato.
  ```bash
  start 18090
  curl -s -H 'Authorization: Bearer tok-user' http://127.0.0.1:18090/notifications > "$t/inbox.json"
  jq -r '.[0].id + " " + .[0].state' "$t/inbox.json"
  docker run --rm -i --security-opt label=disable mikefarah/yq:4.44.3 -N -o=json '.components.schemas.Notification' < contracts/openapi/control-plane.yaml > "$t/n.schema.json"
  jq '.[0]' "$t/inbox.json" > "$t/n0.json"; npx --yes -p ajv-cli@5.0.0 -p ajv-formats@3.0.1 ajv validate --spec=draft2020 -c ajv-formats -s "$t/n.schema.json" -d "$t/n0.json"
  kill $pid; rm -rf "$t"
  ```
  Esperado: `n-1 pending` y `…/n0.json valid`.

- [ ] **CA-3** — Autenticación fail-closed.
  ```bash
  start 18091
  for h in "" "Authorization: Bearer nope" "Authorization: Basic dG9rOnRvaw==" "Authorization: Bearer"; do curl -s -o /dev/null -w '%{http_code} ' ${h:+-H "$h"} http://127.0.0.1:18091/notifications; done; echo
  curl -s -o /dev/null -w '%{http_code}\n' -X POST -H 'Content-Type: application/json' -d '{"flows":["f1"]}' http://127.0.0.1:18091/notifications/n-1/confirm
  kill $pid; rm -rf "$t"
  ```
  Esperado: `401 401 401 401 ` y `401`.

- [ ] **CA-4** — Confirmar crea un único recibo persistido y lo lee de vuelta tras un reinicio.
  ```bash
  start 18092
  curl -s -o "$t/c1.json" -w '%{http_code} ' -X POST -H 'Authorization: Bearer tok-user' -H 'Content-Type: application/json' -d '{"flows":["checkout"]}' http://127.0.0.1:18092/notifications/n-1/confirm
  curl -s -o /dev/null -w '%{http_code} ' -X POST -H 'Authorization: Bearer tok-user' -H 'Content-Type: application/json' -d '{"flows":["checkout"]}' http://127.0.0.1:18092/notifications/n-1/confirm
  curl -s -o /dev/null -w '%{http_code}\n' -X POST -H 'Authorization: Bearer tok-user' -H 'Content-Type: application/json' -d '{"flows":["x"]}' http://127.0.0.1:18092/notifications/nope/confirm
  jq -r '.confirmed_by' "$t/c1.json"; kill $pid; sleep 1
  UIAPI_EVENTS_FILE="$t/events.jsonl" UIAPI_DATA_DIR="$t/data" UIAPI_FAKE_TOKENS='tok-user=u1:user' LISTEN_ADDR=127.0.0.1:18092 "$t/ui-api" & pid=$!; sleep 1
  curl -s -H 'Authorization: Bearer tok-user' 'http://127.0.0.1:18092/notifications?state=confirmed' | jq -r 'length'
  find "$t/data" -type f | xargs cat | grep -c '"notification_id"'; kill $pid; rm -rf "$t"
  ```
  Esperado: `201 409 404`, `u1`, `1` (sigue `confirmed` tras reiniciar) y `1` línea de recibo en disco.

- [ ] **CA-5** — Security headers en respuestas correctas **y** de error.
  ```bash
  start 18093
  curl -s -D - -o /dev/null -H 'Authorization: Bearer tok-user' http://127.0.0.1:18093/notifications | grep -i -E '^(content-security-policy|strict-transport-security|x-content-type-options|referrer-policy|cache-control):' | wc -l
  curl -s -D - -o /dev/null http://127.0.0.1:18093/notifications | grep -i -c -E '^(content-security-policy|strict-transport-security|x-content-type-options):'
  kill $pid; rm -rf "$t"
  ```
  Esperado: `5` y `3`. (`curl -I` envía `HEAD`; por eso se usa `-D -` sobre `GET`.)

- [ ] **CA-6** — CORS restringido.
  ```bash
  start 18094
  curl -s -D - -o /dev/null -H 'Authorization: Bearer tok-user' -H 'Origin: https://app.example' http://127.0.0.1:18094/notifications | grep -i '^access-control-allow-origin:'
  curl -s -D - -o /dev/null -H 'Authorization: Bearer tok-user' -H 'Origin: https://evil.example' http://127.0.0.1:18094/notifications | grep -i -c '^access-control-allow-origin:'
  curl -s -D - -o /dev/null -X OPTIONS -H 'Origin: https://evil.example' -H 'Access-Control-Request-Method: POST' http://127.0.0.1:18094/notifications/n-1/confirm | grep -i -c 'access-control-allow-origin: \*'
  kill $pid; rm -rf "$t"
  ```
  Esperado: `access-control-allow-origin: https://app.example`, `0` y `0`.

- [ ] **CA-7** — Rate limiting: con límite 5 rps y ráfaga 5, 30 peticiones seguidas producen `429`.
  ```bash
  start 18095
  for i in $(seq 30); do curl -s -o /dev/null -w '%{http_code}\n' -H 'Authorization: Bearer tok-user' http://127.0.0.1:18095/notifications; done | sort | uniq -c
  curl -s -D - -o /dev/null -H 'Authorization: Bearer tok-user' http://127.0.0.1:18095/notifications | grep -i -c '^retry-after:'
  kill $pid; rm -rf "$t"
  ```
  Esperado: una línea `200` con entre 5 y 12 respuestas y una línea `429` con el resto; `1` (la respuesta `429` lleva `Retry-After`). (Si el codificador prefiere `hey`/`k6` como en el plan de U1, lo documenta; el criterio mínimo es este.)

- [ ] **CA-8** — El `ui-api` no depende de Kubernetes ni crea Jobs; la imagen cumple las reglas de U5.
  ```bash
  go list -C services/ui-api -deps ./... | grep -c -E 'k8s.io|client-go'
  docker build -q -t aqs-ui-api:ci services/ui-api >/dev/null && docker inspect aqs-ui-api:ci --format '{{.Config.User}}'
  grep -E '^FROM' services/ui-api/Dockerfile | grep -c -i -E ':latest|^FROM [a-z0-9./-]+$'
  bash scripts/ci/list-services.sh | grep -c 'services/ui-api'
  ```
  Esperado: `0`, un usuario no vacío y distinto de `root`/`0`, `0` y `1`.

- [ ] **CA-9** — Higiene y alcance.
  ```bash
  cd services/ui-api && go vet ./... && test -z "$(gofmt -l .)" && echo ok; cd - >/dev/null; git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/ui-api/|bitacoras/U1-T04.md)' | wc -l
  ```
  Esperado: `ok`, `0` y `0`.

---

## Plan de pruebas

- `httptest` con `FakeTokenVerifier`: tabla de CA-1; un verificador que devuelve error y otro que hace `panic` (recuperado → `500` sin filtrar el token).
- Persistencia: confirmar, destruir el servicio, reabrir sobre el mismo `UIAPI_DATA_DIR`, el estado sigue `confirmed`; un archivo de recibos con una línea truncada no impide arrancar.
- Concurrencia: dos `POST confirm` simultáneos sobre la misma notificación → exactamente un `201` y un `409` (`go test -race`).
- Headers: una prueba recorre **todas** las rutas y códigos (200/201/400/401/404/409/413/429/500) y exige los cinco headers.
- Rate limit: reloj inyectable; no se duerme en las pruebas.
- Negativa: el token **nunca** aparece en logs ni en respuestas de error (`grep` del buffer de log).

**Rojo primero:** el codificador registra en su bitácora la salida de `go build ./cmd/ui-api` antes de crear nada (falla: no existe `cmd/`).

---

## Notas

- **Go (aprendido en U4).** El módulo va en `go 1.26.8` (no `go 1.24`: con 1.24 el job `vuln` de la CI de GitHub falla por avisos de la biblioteca estándar), con las dependencias más recientes compatibles con esa versión. Patrón de referencia: `services/go-identity` y `services/go-governance`. `govulncheck` no corre en el entorno del loop (`vuln.go.dev` da 403): la confirmación es el job `vuln` de `ci` en el PR de GitHub (el orquestador lo abre como borrador para que corra).
- **Dockerfile (aprendido en U4).** Copia el patrón de `services/go-identity/Dockerfile` (`golang:1.26.8-alpine3.24`, runtime `alpine:3.24`, tags fijados, usuario 65532). El `docker build` literal falla en el entorno del loop por la CA del proxy (x509 en `go mod download`): se verifica con un contexto temporal fuera del repo que añade `/root/.ccr/ca-bundle.crt` (`COPY` + `ENV SSL_CERT_FILE` en la etapa build); es un límite del entorno, no un defecto.
- **Comandos con `yq` (aprendido en U4).** `keys` no ordena (usa `keys | sort`); `x // "y"` trata `false` como ausente; `if/then` de jq no parsea en yq; `yq -N` sobre un build imprime líneas en blanco (filtra con `grep -v '^$'`). Si un criterio no puede dar el esperado por esa causa, el codificador lo reporta con comando y salida; no rellena a ciegas.
- **Informes del loop.** El diff de `revisiones/<tarea>/` (informes del revisor) no cuenta como desborde en los criterios de alcance.
- Archivos que se **modifican en su sitio**: ninguno de U1-T01 salvo añadir paquetes. Nada de duplicados con sufijo.
- El fake de tokens es de prueba y de dev local: con `UIAPI_AUTH` sin definir, el servicio **no arranca** (no cae a un modo abierto).
- `Principal.Role` se propaga pero no se aplica en esta tarea (ver «Fuera»): no inventes reglas de rol.
- Ningún comando contra un clúster ni la nube. Los servidores se levantan en `127.0.0.1`.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
