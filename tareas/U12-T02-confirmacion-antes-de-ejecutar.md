# U12-T02 — Confirmación humana antes de ejecutar las pruebas

**Unidad:** U12 — Control humano de la ejecución
**Historias que implementa:** US-M2 (el humano decide), US-M5 (ensayo antes de ejecutar), S5.
**Depende de:** nada. **Ola 1**. **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea):

> Tras un ensayo aprobado, la corrida se queda en `rehearsing` esperando que su dueño (o un admin) pulse «Ejecutar pruebas» en `/runs/:id`; solo entonces avanza a `running`. Sin aprobación en 30 minutos, la corrida sale por la ruta de fallo normal (`resetting` y luego `failed`).

Detalle:

1. **Dominio (`services/go-run-controller/runctl`)**
   - `Run` gana `ExecutionApprovedBy string` (`json:"execution_approved_by,omitempty"`) y `AwaitingSince time.Time` (`json:"awaiting_since,omitzero"` o equivalente), persistidos por el `RunStore` existente.
   - `Config` gana `Now func() time.Time` (por defecto `time.Now`) y `ExecutionApprovalTTL time.Duration` (por defecto `30 * time.Minute`).
   - En `Drive`, fase `rehearse`, con `EnsayoPassed == True` y `ExecutionApprovedBy == ""`: si `AwaitingSince` es cero se fija a `Now()` y se persiste; si `Now() - AwaitingSince > ExecutionApprovalTTL`, `failRun(..., "ejecución no aprobada a tiempo")`; si no, devuelve `nil` sin llamar al gate. Con `ExecutionApprovedBy != ""` sigue el avance actual a `running` (el gate de U4 se sigue consultando igual).
   - El gate duro existente de `transition` (`rehearsing -> running` exige `EnsayoPassed == True`) se amplía: también exige `ExecutionApprovedBy != ""`; si falta, `ErrIllegal`.
   - Método nuevo `ApproveExecution(ctx, id, principalID string, admin bool) error`, serializado con `k.mu`: `ErrNotFound` si no existe; `ErrForbidden` (nuevo) si `!admin && principalID != r.ConfirmedBy`; `ErrNotAwaiting` (nuevo) si la corrida no está en `rehearsing` con `EnsayoPassed == True`, o ya está aprobada por otra persona. Aprobar dos veces con el mismo principal es idempotente (`nil`).
   - Función `AwaitingExecution(r Run) bool` = `r.State == Rehearsing && r.EnsayoPassed == True && r.ExecutionApprovedBy == ""`.
2. **HTTP del controlador (`services/go-run-controller/internal/httpapi/api.go`)**
   - `TokenVerifier.Verify` devuelve también el principal (`principal_id`, `role`) que ya trae `GET /auth/session`. Ajustar sus dobles en las pruebas.
   - `GET /runs/{id}` añade `awaiting_execution` (bool, siempre presente) a `RunView`.
   - `POST /runs/{id}/execute` (sin cuerpo): `401` sin token, `404 not_found`, `403 forbidden`, `409 not_awaiting`, `202` con el `RunView` actualizado.
   - El `main.go` cablea el handler con el `Controller` (no con el store a pelo) para el POST.
3. **Contrato (`contracts/openapi/control-plane.yaml`)**: `Run.awaiting_execution` (boolean, requerido) y la operación `executeRun` en `/runs/{id}/execute` con las respuestas anteriores. Regenerar `web/dashboard/src/api/schema.gen.ts` con `npm run gen:api`.
4. **Dashboard (`web/dashboard/src/pages/Run/RunPage.tsx`)**
   - Con `data.awaiting_execution === true`: botón primario «Ejecutar pruebas» (`className="btn btn-primary"`, `data-testid="run-execute-button"`) en la parte superior derecha de la cabecera (dentro de `az-header-meta` o junto a ella), y un aviso `role="status"`: «Las pruebas están listas. Revisa los flujos y pulsa «Ejecutar pruebas» para empezar.» La etiqueta de estado y de la línea de tiempo muestran «Esperando tu confirmación» en lugar de «Validando las pruebas».
   - Al pulsar: `POST /api/runs/{id}/execute`; durante la petición el botón queda `disabled`; al terminar se llama a `refrescar()` del sondeo. `409` se trata como éxito silencioso (otra pestaña ya aprobó) y refresca; `403` y otros errores muestran `RunErrorNotice`.
   - El sondeo no se detiene mientras se espera (no es estado terminal).

**Fuera** (rechazar aunque se vea roto):

- Nuevo valor en el enum `state` o cambios en las políticas Rego de `go-governance`.
- Eventos NATS nuevos.
- El modal del Inbox (es U12-T01) y la consulta de evidencias por flujo.
- Desplegar o reconstruir imágenes en el clúster.

---

## Archivos de contexto

- `services/go-run-controller/runctl/{state.go,controller.go,ports.go,fakes.go}` y sus `*_test.go`
- `services/go-run-controller/internal/httpapi/api.go` y su prueba
- `services/go-run-controller/cmd/go-run-controller/{main.go,real.go}`
- `contracts/openapi/control-plane.yaml` (`/runs/{id}`, `components.schemas.Run`)
- `web/dashboard/src/pages/Run/RunPage.tsx`, `RunPage.test.tsx`, `src/api/client.ts`, `src/hooks/usePolling.ts`
- `web/dashboard/src/proxy/table.ts` (`/api/runs` va directo a go-run-controller)

---

## Criterios de aceptación

- [ ] **CA-1** — Go verde, sin carreras.
  ```bash
  cd services/go-run-controller && go vet ./... && go test -race -count=1 ./... 2>&1 | grep -c -E '^(FAIL|--- FAIL)'
  ```
  Esperado: `0`.

- [ ] **CA-2** — La corrida espera y no ejecuta sin aprobación. Prueba de dominio `TestEsperaAprobacionAntesDeRunning`: con ensayo aprobado, varios `Drive` dejan la corrida en `rehearsing`, el doble de gate no recibe ninguna petición `rehearsing -> running`, y la lectura de vuelta del `RunStore` muestra `AwaitingSince` fijado. Tras `ApproveExecution` con el dueño, un `Drive` lleva a `running` (lectura de vuelta del store).
  ```bash
  cd services/go-run-controller && go test -count=1 -run 'TestEsperaAprobacionAntesDeRunning' -v ./runctl 2>&1 | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `--- PASS` y `ok`.

- [ ] **CA-3** — Permisos e idempotencia. `TestApproveExecution` (tabla): no existe → `ErrNotFound`; otro usuario → `ErrForbidden`; admin ajeno → `nil`; antes del ensayo o en `running` → `ErrNotAwaiting`; dueño dos veces → `nil` y el store conserva un solo `ExecutionApprovedBy`.
  ```bash
  cd services/go-run-controller && go test -count=1 -run 'TestApproveExecution' -v ./runctl 2>&1 | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: todos `PASS`.

- [ ] **CA-4** — Caducidad. `TestAprobacionCaduca` con reloj falso: pasados 30 min + 1 s, `Drive` lleva la corrida a `resetting` y el store registra `FailReason` con «no aprobada»; a 29 min sigue en `rehearsing`.
  ```bash
  cd services/go-run-controller && go test -count=1 -run 'TestAprobacionCaduca' -v ./runctl 2>&1 | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: `PASS`.

- [ ] **CA-5** — HTTP. Prueba de `internal/httpapi` con `httptest`: `POST /runs/{id}/execute` devuelve `401`, `404`, `403`, `409` y `202` en sus casos; tras el `202`, `GET /runs/{id}` lee de vuelta `awaiting_execution:false`; antes, `true`.
  ```bash
  cd services/go-run-controller && go test -count=1 -run 'Execute' -v ./internal/httpapi 2>&1 | grep -E '^(--- |ok|FAIL)'
  ```
  Esperado: todos `PASS`.

- [ ] **CA-6** — Contrato y cliente sincronizados.
  ```bash
  bash contracts/validate.sh >/dev/null && echo contrato-ok; cd web/dashboard && npm run -s check:api && echo api-ok
  ```
  Esperado: `contrato-ok` y `api-ok`.

- [ ] **CA-7** — Dashboard. Pruebas en `RunPage.test.tsx`: con `awaiting_execution:true` se ve el botón «Ejecutar pruebas» y el texto «Esperando tu confirmación»; al pulsarlo el doble de `fetch` registra un `POST` a `/api/runs/<id>/execute` y se vuelve a pedir `GET /api/runs/<id>`; con `awaiting_execution:false` el botón no existe; un `403` muestra el aviso de acceso.
  ```bash
  cd web/dashboard && npm run -s lint && npm run -s typecheck && npx vitest run 2>&1 | grep -c -E '✗|×| failed|FAIL'
  ```
  Esperado: `0`, y `npx vitest run src/pages/Run -t 'Ejecutar pruebas'` lista las pruebas nuevas en verde.

- [ ] **CA-8** — Alcance.
  ```bash
  b=$(git merge-base HEAD main); git diff --name-only $b | grep -v -E '^(services/go-run-controller/|contracts/openapi/control-plane\.yaml|contracts/openapi/examples/|web/dashboard/src/(api/schema\.gen\.ts|pages/Run/|styles\.css)|bitacoras/U12-T02(\.md|/)|revisiones/U12-T02/)' | wc -l
  ```
  Esperado: `0`.

---

## Plan de pruebas

- **Rojo primero:** CA-2 debe fallar hoy (el controlador avanza a `running` sin aprobación); pegar el fallo.
- Las pruebas de dominio leen el estado de vuelta del `RunStore`, no de variables locales.
