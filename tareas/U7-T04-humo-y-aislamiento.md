# U7-T04 — Prueba de humo en kind: APIs reales y aislamiento de red aplicado

**Unidad:** U7 — Plataforma en kind local (podman)
**Historias que implementa:** US-M10, US-M8.1, US-M8.2 (aislamiento verificado en un clúster que aplica NetworkPolicy), US-M1/US-M8.3 (login e inbox reales).
**Depende de:** U7-T03 **fusionada**. **Ola 4**. **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea, concreta):

> Añadir `scripts/kind/kind-smoke.sh`, que sobre la plataforma desplegada en `kind-aqs` abre `kubectl port-forward` en loopback, comprueba salud, login, inbox y estado del warm con los servicios reales, y verifica con pods de prueba que las NetworkPolicy y el RBAC se aplican; imprime `OK|FALLA <comprobación>` y deja documentado qué parte del ciclo de corrida aún no funciona en kind.

Detalle:

- **Port-forward** a `127.0.0.1:18600-18606` (`go-identity`, `ui-api`, `go-intake`, `go-run-controller`, `go-warm-manager`, `go-reset`, `go-governance`), con PIDs propios y `trap` que los cierra siempre. Llama a `require_kind_context`.
- **Comprobaciones** (cada una un `OK|FALLA`):
  - `healthz-<svc>` y `readyz-<svc>` para los 7 servicios (`200`).
  - `login-demo`: `POST /auth/login` en `go-identity` con el usuario `demo` (contraseña leída de `${XDG_RUNTIME_DIR:-/tmp}/aqs-kind/`, U7-T03) → token; `GET /auth/session` → `role=user`.
  - `inbox-con-token` (`GET /notifications` en `ui-api` → `200` y JSON) y `inbox-sin-token-401`.
  - `warm-estado`: `GET /warm` en `ui-api` → `200` con un `state` del enum de `WarmState`.
  - `rbac-test-ns-only`: `kubectl auth can-i` con las ServiceAccounts de `go-warm-manager` y `go-reset` coincide con la matriz esperada de `scripts/ci/rbac-matrix.sh` (puede en `aqs-test`, no en `aqs-system`/`default`).
  - `netpol-sin-egress-test`: un pod efímero en `aqs-test` (imagen fijada, p. ej. `busybox:1.37.0`) **no** alcanza `1.1.1.1:443` ni un Service de `aqs-system` fuera de los permitidos (timeout de 5 s).
  - `netpol-control-positivo`: el mismo pod sí alcanza la app del warm dentro de `aqs-test` (demuestra que el fallo anterior es la política y no la red).
  - `secrets-no-en-logs`: `kubectl logs` de los 7 Deployments no contiene `password_hash`, `$argon2id$` ni tokens de servicio.
- **Ciclo de corrida** (documental, sin `FALLA`): una sección final `PENDIENTE <paso> (<motivo>)` para `webhook→notificación→confirm→run.confirmed→controlador` y `planner/reporter`, citando P1 y P2 del backlog. No se intenta forzar el ciclo.
- Runbook `docs/operaciones/kind-local.md`: `kind-up` → `build-images` → `load-images` → `deploy` → `kind-smoke` → `kind-down`, y la tabla de qué funciona y qué queda pendiente.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Transporte de eventos entre pods, Deployments de los agentes, MinIO en los agentes (P1–P3); dashboard contra kind (U7-T05).
- Cambios en servicios, agentes, manifiestos o políticas. Un defecto destapado se registra como candidata y su comprobación queda `FALLA` documentada solo con aprobación del orquestador.

---

## Archivos de contexto

- `tareas/U7-T03-overlay-kind-y-despliegue.md` y su bitácora
- `scripts/test/u6-demo-local.sh` (si existe) o `scripts/test/u3-demo-local.sh` (estilo `OK|FALLA`, `trap`)
- `scripts/ci/rbac-matrix.sh`, `deploy/flux/base/security/*.yaml`
- `contracts/openapi/control-plane.yaml`, `contracts/plans/` (`WarmState`)
- `unidades-y-tareas.md` (backlog P1–P3)

---

## Criterios de aceptación

Desde la raíz del worktree, con la plataforma desplegada (`kind-up`, `build-images`, `load-images`, `deploy`).

- [ ] **CA-1** — Humo completo.
  ```bash
  bash scripts/kind/kind-smoke.sh 2>&1 | grep -E '^(OK|FALLA)' | cut -d' ' -f1 | sort | uniq -c; echo "rc=${PIPESTATUS[0]}"
  ```
  Esperado: solo líneas `OK` (al menos 22: 14 de salud + login + 2 de inbox + warm + RBAC + 2 de red + logs), ninguna `FALLA`, `rc=0`.

- [ ] **CA-2** — El aislamiento es real (sensibilidad).
  ```bash
  kubectl get networkpolicy -n aqs-test -o yaml > "${TMPDIR:-/tmp}/np.yaml"; kubectl delete networkpolicy --all -n aqs-test >/dev/null
  bash scripts/kind/kind-smoke.sh 2>&1 | grep -c '^FALLA netpol-sin-egress-test'
  kubectl apply -f "${TMPDIR:-/tmp}/np.yaml" >/dev/null; bash scripts/kind/kind-smoke.sh 2>&1 | grep -c '^OK netpol-sin-egress-test'
  ```
  Esperado: `1` y `1` (sin políticas la comprobación falla; repuestas, pasa).

- [ ] **CA-3** — Sin procesos ni pods de prueba colgados.
  ```bash
  bash scripts/kind/kind-smoke.sh >/dev/null 2>&1; ss -ltn | grep -c -E '127\.0\.0\.1:186(0[0-6])'; kubectl get pods -n aqs-test --no-headers | grep -c -i -E 'smoke|probe'
  ```
  Esperado: `0` y `0`.

- [ ] **CA-4** — Pendientes declarados y runbook.
  ```bash
  bash scripts/kind/kind-smoke.sh 2>&1 | grep -c -E '^PENDIENTE .*\((P1|P2)'
  grep -c -E 'kind-up|build-images|load-images|deploy\.sh|kind-smoke|kind-down' docs/operaciones/kind-local.md
  ```
  Esperado: al menos `2` y al menos `6`.

- [ ] **CA-5** — Alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(scripts/kind/|deploy/flux/kind/|docs/operaciones/kind-local\.md|deploy/kind/README\.md|bitacoras/U7-T04\.md|revisiones/U7-T04/)' | wc -l
  ```
  Esperado: `0` y `0`.

---

## Errata (2026-10-10, ronda 1, decidida por el humano)

- **E-3 · `ui-api` sin configuración del warm (solo en kind).** `warm-estado` da `503 warm no configurado`. El Deployment de `ui-api` no define `WARM_URL` ni `UIAPI_WARM_TOKEN_FILE` (`services/ui-api/cmd/ui-api/main.go:18-19`); `base`, `dev` y `prod` tienen el mismo hueco (C-103). En kind se cubre **solo** desde `deploy/flux/kind/`, igual que E-2 de U7-T03:
  - `WARM_URL` apunta al Service de `go-warm-manager` dentro del clúster.
  - `UIAPI_WARM_TOKEN_FILE` apunta a un archivo montado desde el Secret existente `go-warm-manager-service-token`, clave `token`. Es el mismo token que ya acepta `go-warm-manager`.
  - Si la NetworkPolicy de `base` no deja pasar `ui-api` → `go-warm-manager`, se añade una de permiso acotada a ese par de pods, como en E-2.
  - `base`, `dev`, `prod` y `policy/` no cambian. `policies.sh` debe seguir sin `FALLA`.
  - CA-5 admite ahora `deploy/flux/kind/`. CA-1 no cambia.

## Plan de pruebas

- El script es la prueba de integración; CA-2 demuestra que la comprobación de red falla cuando debe.
- **Rojo primero:** pegar la salida de `bash scripts/kind/kind-smoke.sh` (no existe).
- Al terminar: `bash scripts/kind/kind-down.sh`.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
