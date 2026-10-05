# U1-T07 — Integración contra `go-identity` real y publicación de `run.confirmed`

**Unidad:** U1 — Ingesta & Inbox
**Historias que implementa:** US-M2 (la confirmación persistida produce `run.confirmed` consumible por U2), US-M8.3 (cada corrida referencia un registro de confirmación auditable)
**Depende de:** U1-T04 (inbox y puerto `TokenVerifier`), U1-T06 (logs/métricas de `ui-api`), **U4-T02** (`go-identity`: login/logout) y **U4-T03** (`GET /auth/session` y su esquema `Session` en el OpenAPI). Cierra U1.

---

## Alcance

**Dentro** (una línea, concreta):

> Sustituir el verificador fake de `ui-api` por un `HTTPTokenVerifier` que valida **cada** petición contra `go-identity` (`GET /auth/session`), publicar `run.confirmed` (válido contra su esquema) al confirmar una notificación, y añadir un workflow de CI que levanta `go-identity` real y ejecuta las pruebas de contrato U1↔U4.

Detalle:

- **`HTTPTokenVerifier`** (selección `UIAPI_AUTH=identity`, `IDENTITY_URL` obligatorio y solo `http(s)`): reenvía el `Authorization: Bearer` a `GET {IDENTITY_URL}/auth/session`. `200` con un cuerpo que valida contra `components.schemas.Session` → `Principal{ID, Role}`; `401` → no autenticado; cualquier otra respuesta, cuerpo inválido, rol desconocido, timeout (2 s), error de red o circuito abierto → **denegar**. Sin caché de validaciones: «validación de tokens en cada request» (NF-SEG-08).
- **Códigos.** Token inválido o expirado → `401`. `go-identity` caído, lento o con respuesta inválida → `503` con `Error{code: "identity_unavailable"}` y `Retry-After` (nunca `200`, nunca se cae a un modo abierto). Esto **cambia en su sitio** la regla de U1-T04 «cualquier error del verificador → 401»: ahora hay un error tipado `ErrUnavailable` → `503`; el resto sigue en `401`.
- **Circuito.** Tras 5 fallos de transporte consecutivos, el circuito se abre 10 s y las peticiones se rechazan con `503` sin llamar a identidad; luego una sonda medio-abierta. Contadores y estado en `/metrics` (`aqs_identity_calls_total{result}`, `aqs_identity_circuit_open`).
- **Fake acotado.** `UIAPI_AUTH=fake` pasa a exigir además `UIAPI_ALLOW_FAKE_AUTH=true` y **el servicio no arranca** si `UIAPI_ENV=prod`. Sin `UIAPI_AUTH` no arranca (como en U1-T04).
- **`run.confirmed`.** Al aceptar una confirmación (`201`), después de persistir el recibo, `ui-api` publica el evento `run.confirmed` con `data{run_id, notification_id, confirmed_by, flows}`, `trace_id` de la petición, `event_id` **determinista** (UUIDv5 sobre `run_id`) y `occurred_at` = `confirmed_at`. Transporte: `outbox` JSONL (`UIAPI_OUTBOX_FILE`), el mismo marcador de transición que U1-T02 (candidata C-45). Si publicar falla, la respuesta sigue siendo `201` (el recibo ya es un hecho auditable), el recibo queda `publish_pending` y un re-publicador (al arrancar y cada 5 s) lo envía; por el `event_id` determinista, no hay duplicados. `aqs_inbox_publish_pending` (gauge) lo expone.
- **Contrato U1↔U4** (`go test -tags contract`, solo corre con `IDENTITY_URL` definido): (a) la respuesta real de `/auth/session` valida contra `Session`; (b) `401` sin token, con token inválido, con token cerrado por `logout` y con token expirado; (c) el `run.confirmed` producido valida contra `contracts/events/run.confirmed.schema.json` y `confirmed_by` es el `Principal.ID` que devolvió identidad; (d) con identidad detenida, `503`.
- **Workflow nuevo** `.github/workflows/integration-u1-u4.yml` (en `push` a `main` y en `pull_request`; `permissions: contents: read`; acciones fijadas por SHA, **las mismas** que usa `ci.yml`): construye `go-identity` y `ui-api`, genera un usuario de prueba con `go-identity hash-password`, arranca `go-identity` real en `127.0.0.1`, ejecuta `go test -tags contract ./...` en `services/ui-api` y valida `run.confirmed` con `ajv`. Sin secretos del repositorio: los valores de prueba se generan en el job.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Implementar o modificar `go-identity` (**U4-T02/T03**). Si falta `/auth/session`, `hash-password` o el `Session` del OpenAPI, es un bloqueo: se reporta, no se parcha desde U1.
- Autorización por propietario del recurso (IDOR) sobre notificaciones y corridas: candidata C-47. Aquí `Role` se propaga y no se aplica.
- Un broker o cualquier transporte real de eventos (C-45) y que `go-run-controller` consuma `run.confirmed`: **U2**.
- Modificar `ci.yml`, `contracts.yml`, `policies.yml`, `contracts/**` o `deploy/**`. Los cambios al OpenAPI que requiera esta integración los hizo U4-T03.
- Caché de sesiones, refresco de tokens, SSO.
- Cualquier comando contra un clúster o la nube; el humano valida la integración en dev después.

---

## Archivos de contexto

- `unidades-y-tareas.md` (secciones U1 y U4)
- `aidlc-docs/inception/application-design/unit-task-plans/U1.md` y `U4.md`
- `aidlc-docs/inception/application-design/components.md` (C1, C8, C9)
- `contracts/openapi/control-plane.yaml` (`/auth/*`, `Session`, `ConfirmationReceipt`)
- `contracts/events/run.confirmed.schema.json` y `contracts/events/examples/valid/run.confirmed.json`
- `services/ui-api/` (U1-T04, U1-T06), `services/go-identity/` (U4-T02, U4-T03)
- `.github/workflows/ci.yml` (SHAs de acciones fijados) y `.github/workflows/policies.yml`
- `tareas/U4-T02-go-identity-autenticacion.md`, `tareas/U4-T03-autorizacion.md` (nombres de variables y CLI de `go-identity`)
- `tareas/candidatas.md` (C-45, C-47)

---

## Criterios de aceptación

Desde la raíz del worktree. Arranque común (usa los nombres de entorno de U4-T02):

```bash
up() { t=$(mktemp -d)
  (cd services/go-identity && go build -o "$t/go-identity" ./cmd/go-identity) && (cd services/ui-api && go build -o "$t/ui-api" ./cmd/ui-api) || return 1
  h=$(printf 'Cl4ve-de-Prueba-1!' | "$t/go-identity" hash-password)
  jq -n --arg h "$h" '[{username:"marta",password_hash:$h,role:"user"}]' > "$t/users.json"
  IDENTITY_USERS_FILE="$t/users.json" LISTEN_ADDR=127.0.0.1:18200 "$t/go-identity" > "$t/id.log" 2>&1 & idpid=$!
  jq -c . contracts/events/examples/valid/notify.created.json > "$t/events.jsonl"
  UIAPI_AUTH=identity IDENTITY_URL=http://127.0.0.1:18200 UIAPI_EVENTS_FILE="$t/events.jsonl" UIAPI_DATA_DIR="$t/ui" UIAPI_OUTBOX_FILE="$t/out.jsonl" LISTEN_ADDR=127.0.0.1:18201 "$t/ui-api" > "$t/ui.log" 2>&1 & pid=$!; sleep 2
  tok=$(curl -s -X POST http://127.0.0.1:18200/auth/login -H 'Content-Type: application/json' -d '{"username":"marta","password":"Cl4ve-de-Prueba-1!"}' | jq -r .token); }
down() { kill $pid $idpid 2>/dev/null; rm -rf "$t"; }
```

- [ ] **CA-1** — Pruebas unitarias de `ui-api` en verde (≥ 10 casos nuevos) y las de contrato se omiten sin `IDENTITY_URL`.
  ```bash
  cd services/ui-api && go test -v ./... | grep -c -E '^\s*--- PASS'; go test ./... 2>&1 | grep -c FAIL; go test -tags contract -run Contract -v ./... 2>&1 | grep -c -E 'SKIP|skipp'
  ```
  Esperado: ≥ `10` (acumulado con U1-T04/T06), `0` y ≥ `1`. Cubren: verificador `200` válido; `401`; cuerpo inválido; rol desconocido; timeout; circuito abierto/cerrado/medio-abierto; fake bloqueado en `prod`; `ErrUnavailable→503`; re-publicador sin duplicados.

- [ ] **CA-2** — De extremo a extremo con `go-identity` real: token válido ve el inbox; sin token, inválido y cerrado por `logout` reciben `401`.
  ```bash
  up
  curl -s -o /dev/null -w '%{http_code} ' -H "Authorization: Bearer $tok" http://127.0.0.1:18201/notifications
  curl -s -o /dev/null -w '%{http_code} ' http://127.0.0.1:18201/notifications
  curl -s -o /dev/null -w '%{http_code} ' -H 'Authorization: Bearer invalido' http://127.0.0.1:18201/notifications
  curl -s -o /dev/null -X POST -H "Authorization: Bearer $tok" http://127.0.0.1:18200/auth/logout
  curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $tok" http://127.0.0.1:18201/notifications
  down
  ```
  Esperado: `200 401 401 401`. Antes de la tarea: el `ui-api` no arranca con `UIAPI_AUTH=identity` (rojo inicial).

- [ ] **CA-3** — Confirmar publica **un** `run.confirmed` válido contra su esquema, con el `confirmed_by` que devolvió identidad.
  ```bash
  up
  curl -s -o "$t/c.json" -w '%{http_code}\n' -X POST -H "Authorization: Bearer $tok" -H 'Content-Type: application/json' -d '{"flows":["checkout","refund"]}' http://127.0.0.1:18201/notifications/n-1/confirm
  sleep 1; wc -l < "$t/out.jsonl"; head -n1 "$t/out.jsonl" > "$t/ev.json"
  npx --yes -p ajv-cli@5.0.0 -p ajv-formats@3.0.1 ajv validate --spec=draft2020 -c ajv-formats -s contracts/events/run.confirmed.schema.json -d "$t/ev.json"
  jq -r '.data.confirmed_by + " " + (.data.flows|join(","))' "$t/ev.json"; jq -r '.data.run_id' "$t/ev.json" | diff - <(jq -r .run_id "$t/c.json") && echo "run_id coincide"
  curl -s -H "Authorization: Bearer $tok" http://127.0.0.1:18200/auth/session | jq -r .principal_id
  down
  ```
  Esperado: `201`, `1`, `…/ev.json valid`, `marta checkout,refund` (o el `ID` que devuelva identidad; debe coincidir con la última línea), `run_id coincide` y el mismo identificador de principal.

- [ ] **CA-4** — Fail-closed ante caída de identidad: `503`, nunca `200`, y el circuito se abre.
  ```bash
  up
  kill $idpid; sleep 1
  for i in 1 2 3 4 5 6 7 8; do curl -s -o /dev/null -w '%{http_code} ' -H "Authorization: Bearer $tok" http://127.0.0.1:18201/notifications; done; echo
  curl -s -D - -o /dev/null -H "Authorization: Bearer $tok" http://127.0.0.1:18201/notifications | grep -i -c '^retry-after:'
  curl -s http://127.0.0.1:18201/metrics | grep -E '^aqs_identity_circuit_open [01]$'
  down
  ```
  Esperado: ocho `503 `, `1` y `aqs_identity_circuit_open 1`.

- [ ] **CA-5** — Sin duplicados al republicar y recibo no perdido si falla la publicación.
  ```bash
  cd services/ui-api && go test -run 'Republish|PublishPending' -v ./... | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS` en al menos dos pruebas: una con `EventPublisher` que falla (la respuesta es `201`, el recibo existe y está `publish_pending`) y otra donde el re-publicador la envía exactamente una vez aunque se invoque dos veces.

- [ ] **CA-6** — El fake no es una puerta abierta.
  ```bash
  t=$(mktemp -d); (cd services/ui-api && go build -o "$t/ui-api" ./cmd/ui-api)
  env UIAPI_AUTH=fake UIAPI_ENV=prod UIAPI_ALLOW_FAKE_AUTH=true UIAPI_DATA_DIR="$t/d" UIAPI_EVENTS_FILE="$t/e" "$t/ui-api" >/dev/null 2>&1; echo "prod+fake rc=$?"
  env UIAPI_AUTH=fake UIAPI_DATA_DIR="$t/d" UIAPI_EVENTS_FILE="$t/e" "$t/ui-api" >/dev/null 2>&1; echo "fake sin permiso rc=$?"
  env -u UIAPI_AUTH UIAPI_DATA_DIR="$t/d" UIAPI_EVENTS_FILE="$t/e" "$t/ui-api" >/dev/null 2>&1; echo "sin auth rc=$?"
  env UIAPI_AUTH=identity UIAPI_DATA_DIR="$t/d" UIAPI_EVENTS_FILE="$t/e" "$t/ui-api" >/dev/null 2>&1; echo "identity sin URL rc=$?"; rm -rf "$t"
  ```
  Esperado: cuatro `rc=` distintos de `0`.

- [ ] **CA-7** — Las pruebas de contrato U1↔U4 pasan contra `go-identity` real.
  ```bash
  up
  (cd services/ui-api && IDENTITY_URL=http://127.0.0.1:18200 IDENTITY_TEST_USER=marta IDENTITY_TEST_PASSWORD='Cl4ve-de-Prueba-1!' go test -tags contract -run Contract -v ./... | grep -E '^\s*--- (PASS|FAIL|SKIP)|^(ok|FAIL)')
  down
  ```
  Esperado: al menos 4 `--- PASS` (sesión válida contra `Session`, `401` en los tres casos de token, `run.confirmed` válido, `503` con identidad caída) y ningún `FAIL`/`SKIP`. (La prueba de «identidad caída» detiene el proceso que ella misma arranca o usa un `IDENTITY_URL` sin servidor; el codificador la documenta.)

- [ ] **CA-8** — El workflow existe, es válido, pinea acciones por SHA y no usa secretos.
  ```bash
  docker run --rm -v "$PWD":/repo -w /repo rhysd/actionlint:1.7.1 .github/workflows/integration-u1-u4.yml; echo "actionlint rc=$?"
  grep -E '^\s+(- )?uses:' .github/workflows/integration-u1-u4.yml | grep -v -E '@[0-9a-f]{40}' | wc -l
  grep -c -E 'secrets\.' .github/workflows/integration-u1-u4.yml
  grep -E 'uses:' .github/workflows/integration-u1-u4.yml | sed -E 's/.*uses: *//; s/ *#.*//' | sort -u | while read -r u; do grep -q -F "$u" .github/workflows/ci.yml || echo "ACCION NUEVA: $u"; done
  ```
  Esperado: `actionlint rc=0`, `0`, `0` y ninguna línea `ACCION NUEVA` (reutiliza las acciones ya fijadas en `ci.yml`; si necesita una nueva, la fija por SHA, lo justifica en la bitácora y este último comando se documenta como excepción).

- [ ] **CA-9** — La imagen sigue cumpliendo las reglas de U5 y U1 no creó Jobs.
  ```bash
  go list -C services/ui-api -deps ./... | grep -c -E 'k8s.io|client-go'
  docker build -q -t aqs-ui-api:ci services/ui-api >/dev/null && docker inspect aqs-ui-api:ci --format '{{.Config.User}}'
  go list -C services/go-intake -deps ./... | grep -c -E 'k8s.io|client-go'
  ```
  Esperado: `0`, un usuario no vacío distinto de `root`/`0`, y `0`.

- [ ] **CA-10** — Higiene y alcance.
  ```bash
  (cd services/ui-api && go vet ./... && go vet -tags contract ./... && test -z "$(gofmt -l .)" && echo ok); git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/ui-api/|\.github/workflows/integration-u1-u4\.yml|bitacoras/U1-T07\.md)' | wc -l
  ```
  Esperado: `ok`, `0` y `0`.

---

## Plan de pruebas

- Unitarias con `httptest` simulando `go-identity` (`200`, `401`, `500`, cuerpo roto, rol desconocido, respuesta lenta, conexión rechazada) y reloj inyectable para el circuito.
- Contrato con la etiqueta `contract` (solo en el workflow y en local con `IDENTITY_URL`): CA-7.
- Publicación: `EventPublisher` que falla N veces y luego funciona; `event_id` determinista (misma entrada → mismo UUID); dos confirmaciones concurrentes de la misma notificación → un solo evento.
- Negativa: el token reenviado a identidad **no** aparece en logs ni en métricas (`grep` del log de `ui-api` y de `/metrics`).

**Rojo primero:** el codificador registra en su bitácora que `UIAPI_AUTH=identity` hace fallar el arranque de `ui-api` (selección no implementada) y que `out.jsonl` no existe tras una confirmación.

---

## Notas

- **Go (aprendido en U4).** El módulo va en `go 1.26.8` (no `go 1.24`: con 1.24 el job `vuln` de la CI de GitHub falla por avisos de la biblioteca estándar), con las dependencias más recientes compatibles con esa versión. Patrón de referencia: `services/go-identity` y `services/go-governance`. `govulncheck` no corre en el entorno del loop (`vuln.go.dev` da 403): la confirmación es el job `vuln` de `ci` en el PR de GitHub (el orquestador lo abre como borrador para que corra).
- **Dockerfile (aprendido en U4).** Copia el patrón de `services/go-identity/Dockerfile` (`golang:1.26.8-alpine3.24`, runtime `alpine:3.24`, tags fijados, usuario 65532). El `docker build` literal falla en el entorno del loop por la CA del proxy (x509 en `go mod download`): se verifica con un contexto temporal fuera del repo que añade `/root/.ccr/ca-bundle.crt` (`COPY` + `ENV SSL_CERT_FILE` en la etapa build); es un límite del entorno, no un defecto.
- **Comandos con `yq` (aprendido en U4).** `keys` no ordena (usa `keys | sort`); `x // "y"` trata `false` como ausente; `if/then` de jq no parsea en yq; `yq -N` sobre un build imprime líneas en blanco (filtra con `grep -v '^$'`). Si un criterio no puede dar el esperado por esa causa, el codificador lo reporta con comando y salida; no rellena a ciegas.
- **Informes del loop.** El diff de `revisiones/<tarea>/` (informes del revisor) no cuenta como desborde en los criterios de alcance.
- Archivos que se **modifican en su sitio**: el cableado de `cmd/ui-api`, el verificador y el handler de `confirm` de U1-T04 (error tipado `ErrUnavailable` y publicación). Nada de duplicados con sufijo.
- Orden de dependencias: esta tarea **no puede empezar** hasta que U4-T02 y U4-T03 estén fusionadas. Si el orquestador las ve sin fusionar, reporta el hueco al humano y no despacha.
- `run_id`: lo sigue generando `ui-api` (U1-T04). U2 lo tratará como identificador opaco.
- Ningún comando contra un clúster ni la nube. La validación en dev la hace el humano y se anota en la bitácora.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
