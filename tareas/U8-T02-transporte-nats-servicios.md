# U8-T02 — Transporte NATS en go-intake, ui-api y go-run-controller

**Unidad:** U8 — Producto demostrable (ciclo completo en kind)
**Historias que implementa:** US-M1 (notificación llega al inbox), US-M2 (la confirmación llega al controlador); decisión C-45.
**Depende de:** U8-T01 **fusionada**. **Ola 2**. **Tope: 3 rondas**.

---

## Alcance

**Dentro** (una línea, concreta):

> Añadir a `go-intake`, `ui-api` y `go-run-controller` un adaptador de eventos NATS JetStream seleccionable con `EVENTS_TRANSPORT=nats` (por defecto `file`, que no cambia): `go-intake` publica `notify.created`, `ui-api` lo consume (inbox) y publica `run.confirmed`, y `go-run-controller` lo consume y publica `run.done`; entrega al menos una vez, `Nats-Msg-Id = event_id` al publicar y deduplicación por `event_id` al consumir; y cablear `EVENTS_TRANSPORT=nats` y `NATS_URL` en `deploy/flux/base/control-plane.yaml`.

Detalle:

- Sujeto `aqs.events.<type>` y stream `AQS_EVENTS` (U8-T01). Cada consumidor es **durable** con nombre fijo (`ui-api-inbox`, `go-run-controller-confirmed`), `AckExplicit`, y hace `Ack` **solo** después de aplicar el evento con éxito (mismo contrato que hoy con el offset del archivo).
- El evento viaja con el mismo JSON que hoy y se valida contra `contracts/events/*.schema.json` al consumir; un evento inválido se registra, se hace `Term` y no se aplica.
- La lógica de dominio no cambia: el adaptador implementa los puertos existentes de publicación/lectura de cada servicio. Sin `EVENTS_TRANSPORT` el comportamiento y las pruebas actuales siguen idénticos.
- Cliente `github.com/nats-io/nats.go` con versión fijada en cada `go.mod`. Reconexión con espera acotada; `/readyz` da `nats: fail` si no hay conexión.

**Fuera**:

- Eventos de `go-warm-manager`, `go-reset` y agentes (siguen en su outbox; el controlador los consulta por HTTP).
- Eliminar el transporte `file` (sigue para pruebas locales y CI).
- Manifiestos de NATS (U8-T01).

---

## Archivos de contexto

- `tareas/U8-T01-nats-jetstream.md`, `deploy/flux/base/nats/README.md`
- `services/go-intake/` (publicación de `notify.created`), `services/ui-api/inbox/` (`event.go`, `publish.go`), `services/go-run-controller/adapters/events.go`, `services/go-run-controller/README.md` («Eventos de entrada/salida»)
- `contracts/events/` y `contracts/events/examples/`
- `deploy/flux/base/control-plane.yaml`, `scripts/test/u6-up.sh` (arranque local de servicios)

---

## Criterios de aceptación

Desde la raíz del worktree. Las pruebas de integración usan un `nats-server` local por podman (imagen fijada) que el propio script levanta y borra.

- [ ] **CA-1** — Pruebas existentes intactas y nuevas en verde.
  ```bash
  for s in go-intake ui-api go-run-controller; do (cd services/$s && go test ./... >/dev/null 2>&1; echo "$s rc=$?"); done
  ```
  Esperado: `rc=0` en los tres.

- [ ] **CA-2** — Recorrido por NATS con servicios reales (sin clúster).
  ```bash
  bash scripts/test/u8-nats-local.sh; echo "rc=$?"
  ```
  Esperado: `OK webhook-firmado-202`, `OK inbox-recibe-notificacion`, `OK confirm-202`, `OK controlador-recibe-run-confirmed` (leído de vuelta con `GET /runs/{id}` del controlador), `OK sin-duplicados` (reenviar el mismo `event_id` dos veces deja una sola notificación y una sola corrida), `rc=0`. El script es nuevo y deja puertos y contenedores limpios.

- [ ] **CA-3** — Al menos una vez: un consumidor caído no pierde eventos.
  ```bash
  bash scripts/test/u8-nats-local.sh --redelivery; echo "rc=$?"
  ```
  Esperado: `OK reentrega-tras-reinicio` (se para `ui-api`, se publica, se arranca: la notificación aparece una vez) y `rc=0`.

- [ ] **CA-4** — Manifiestos cableados y políticas en verde.
  ```bash
  kubectl kustomize deploy/flux/kind | grep -c -E 'name: EVENTS_TRANSPORT'; bash scripts/ci/policies.sh 2>&1 | grep -c '^FALLA'
  ```
  Esperado: `3` y `0`.

- [ ] **CA-5** — Alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/(go-intake|ui-api|go-run-controller)/|deploy/flux/(base|kind)/|scripts/test/u8-nats-local\.sh|bitacoras/U8-T02\.md|revisiones/U8-T02/)' | wc -l
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

- Pruebas unitarias del adaptador con un servidor NATS embebido o local; prueba negativa de evento inválido (`Term`, no se aplica).
- **Rojo primero:** `EVENTS_TRANSPORT=nats` hoy no arranca o se ignora; pegar la salida.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
