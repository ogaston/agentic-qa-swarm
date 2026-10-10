# U6-T02 — `ui-api`: lectura del estado del warm y de las confirmaciones para el dashboard

**Unidad:** U6 — Frontend web (S5)
**Historias que implementa:** S5 (estado del warm `ready`/`dirty`/`cuarentena`/`idle-escalado`; lista de corridas confirmadas), sobre US-M2 y US-M7.1.
**Depende de:** MVP cerrado. **Ola 1**, en paralelo con U6-T01 (solo Go y contrato). **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea, concreta):

> Añadir a `ui-api` dos rutas de solo lectura autenticadas con el token de la persona, `GET /warm` (lee `go-warm-manager` con el token de servicio) y `GET /confirmations` (recibos de confirmación con control de propiedad), y llevarlas al OpenAPI.

Detalle:

- **`GET /warm`** → `200 WarmState` (el de `contracts/plans/warm-state.schema.json`). `ui-api` llama a `GET {WARM_URL}/warm` con `Authorization: Bearer <token de servicio>`, leído de `UIAPI_WARM_TOKEN_FILE` (archivo, nunca variable con el valor). Plazo 2 s; circuito como el de identidad (5 fallos de transporte seguidos lo abren 10 s). La respuesta del warm-manager se valida contra el esquema antes de devolverla: inválida, error de transporte, 5xx o circuito abierto → `503 {code: "warm_unavailable"}` con `Retry-After`; un `401` del warm-manager (token de servicio mal configurado) → `503 warm_unavailable` y un log `error` sin el token. Nunca se reenvía el token de la persona al warm-manager ni el de servicio al navegador. Sin `WARM_URL` la ruta responde `503 warm_unavailable` (el servicio sigue arrancando: el inbox no depende del warm).
- **`GET /confirmations[?limit=N]`** → `200 []ConfirmationReceipt`, más reciente primero, `limit` 1–100 (por defecto 20; fuera de rango → `400`). Sale del almacén existente (`confirmations.jsonl`). **Propiedad:** rol `user` ve solo los recibos con `confirmed_by` igual a su `Principal.ID`; `admin` ve todos. El rol sale del verificador de tokens, nunca de cabeceras o parámetros.
- Las dos rutas pasan por el mismo token, limitador por IP, security headers y log de acceso que el inbox; `route` en las métricas es el patrón (`/warm`, `/confirmations`).
- **Contrato** (solo se añade): en `control-plane.yaml`, las rutas `/warm` (esquema `WarmState` en `components`, equivalente al de `contracts/plans/`) y `/confirmations`, con `401`, `400` y `503` donde aplica. `contracts/validate.sh` y el lint de Redocly siguen en verde.
- `services/ui-api/README.md`: las dos rutas y las variables `WARM_URL`, `UIAPI_WARM_TOKEN_FILE`.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Cambiar `go-warm-manager`, `go-run-controller` o `go-identity`. `GET /runs/{id}` ya lo sirve el controlador con el token de la persona; no se reexpone aquí.
- Escritura sobre el warm (ensure, deploy, reset) o cualquier acción que cambie estado.
- `GET /reports/{run}` (C-93), manifiestos, Secrets reales o cableado en `deploy/**`.
- El frontend (`web/**`).

---

## Archivos de contexto

- `services/ui-api/README.md`, `services/ui-api/internal/httpapi/`, `services/ui-api/internal/auth/` (verificador e identidad con circuito), `services/ui-api/inbox/store.go`
- `services/go-warm-manager/README.md` (API y token de servicio)
- `contracts/openapi/control-plane.yaml`, `contracts/plans/warm-state.schema.json`, `contracts/validate.sh`
- `.github/workflows/contracts.yml` (lint de Redocly)

---

## Criterios de aceptación

Desde la raíz del worktree (`GOTOOLCHAIN=auto` si hace falta).

- [ ] **CA-1** — Lo anterior sigue en verde.
  ```bash
  cd services/ui-api && go vet ./... && go test -count=1 -race ./... 2>&1 | grep -c -E '^(FAIL|--- FAIL)'
  ```
  Esperado: `0`.

- [ ] **CA-2** — `GET /warm` cumple su contrato.
  ```bash
  cd services/ui-api && go test -count=1 -race -run 'Warm' -v ./internal/httpapi/... 2>&1 | grep -c -- '--- PASS'
  ```
  Esperado: al menos `8`, que cubren (con un warm-manager simulado en loopback que registra cabeceras): `200` con un `WarmState` válido por cada uno de los 4 estados; el token de servicio del archivo va en `Authorization` y el de la persona **no**; sin token de persona → `401` y 0 peticiones al warm-manager; respuesta inválida contra el esquema → `503 warm_unavailable`; 5xx, `401` del warm-manager y timeout → `503` con `Retry-After`; tras 5 fallos de transporte el sexto no sale (circuito); el token de servicio (marcador único) no aparece en el log ni en ningún cuerpo de respuesta; sin `WARM_URL` → `503` y `/notifications` sigue en `200`.

- [ ] **CA-3** — `GET /confirmations` respeta la propiedad.
  ```bash
  cd services/ui-api && go test -count=1 -race -run 'Confirmations' -v ./internal/httpapi/... 2>&1 | grep -c -- '--- PASS'
  ```
  Esperado: al menos `6`, que cubren: `user` A ve solo sus recibos aunque existan de B; `admin` ve los de ambos; orden más reciente primero; `limit` por defecto 20, `limit=0`/`101`/`abc` → `400`; `X-Role: admin` enviado por un `user` no cambia el resultado; sin token → `401`.

- [ ] **CA-4** — Contrato válido y aditivo.
  ```bash
  bash contracts/validate.sh >/dev/null 2>&1; echo "validate rc=$?"
  npx --yes @redocly/cli@1.25.0 lint contracts/openapi/control-plane.yaml >/dev/null 2>&1; echo "lint rc=$?"
  b=$(git merge-base HEAD origin/main); git diff $b -- contracts/openapi/control-plane.yaml | grep -c '^-[^-]'
  grep -c -E '^  /(warm|confirmations):' contracts/openapi/control-plane.yaml
  ```
  Esperado: `validate rc=0`, `lint rc=0`, `0` líneas borradas y `2`.

- [ ] **CA-5** — Higiene y alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/ui-api/|contracts/openapi/control-plane\.yaml|contracts/openapi/examples/|bitacoras/U6-T02\.md|revisiones/U6-T02/)' | wc -l
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

- Pruebas de tabla en `internal/httpapi` con `httptest.Server` para el warm-manager y el verificador de tokens falso existente (roles `user`/`admin`).
- **Rojo primero:** pegar la salida de `go test -run 'Warm|Confirmations' ./internal/httpapi/...` antes de implementar.

---

## Notas

- El `WarmState` del OpenAPI se copia del esquema de `contracts/plans/` porque Redocly no resuelve bien `$ref` externos a JSON Schema 2020-12; registrar en la bitácora si se resuelve de otra forma.
- El cableado de `WARM_URL` y del Secret del token en `deploy/flux/**` queda para cuando se despliegue el dashboard (backlog).

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
