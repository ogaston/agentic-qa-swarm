# U7-T05 — Dashboard servido en local contra la plataforma en kind

**Unidad:** U7 — Plataforma en kind local (podman)
**Historias que implementa:** S5 (dashboard contra servicios desplegados), sobre US-M1, US-M7.1 y US-M8.3.
**Depende de:** U7-T04 y U6-T06 **fusionadas**. **Ola 5**. Cierra U7. **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea, concreta):

> Añadir `scripts/kind/dashboard-up.sh` / `dashboard-down.sh` y `scripts/kind/dashboard-smoke.sh`, que sirven el build de `web/dashboard` con `vite preview` en `127.0.0.1:18610` y su proxy `/api` apuntando a los `port-forward` de `go-identity`, `ui-api` y `go-run-controller` en kind, y recorren con `curl` contra el origen del dashboard login → inbox → warm → logout.

Detalle:

- Reutiliza los `port-forward` de U7-T04 (extraídos a `scripts/kind/lib.sh` si hace falta, sin cambiar su comportamiento) y las variables de proxy que define U6-T01/T06; **no** cambia `vite.config.ts`.
- Comprobaciones `OK|FALLA` **solo contra `:18610`**: `index-servido`, `csp-estricta` (misma política que U6-T06), `login`, `inbox`, `warm-estado`, `sin-token-401`, `logout-invalida-token`.
- Confirmar una notificación y seguir la corrida **no** se comprueba (depende de P1); se imprime `PENDIENTE confirmar-y-seguir-corrida (P1)`.
- `trap` que siempre para `vite preview` y los `port-forward`.
- Runbook `docs/operaciones/kind-local.md`: sección «Dashboard».

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Imagen del dashboard, Deployment, ingress o servidor estático en el clúster (C-95).
- Cambios en `web/dashboard/src`, servicios, manifiestos o políticas.

---

## Archivos de contexto

- `tareas/U7-T04-humo-y-aislamiento.md`, `tareas/U6-T06-recorrido-local-csp.md` y sus bitácoras
- `web/dashboard/README.md`, `web/dashboard/vite.config.ts` (variables del proxy)
- `docs/operaciones/kind-local.md`

---

## Criterios de aceptación

Desde la raíz del worktree, con la plataforma desplegada en kind y `npm ci` hecho en `web/dashboard`.

- [ ] **CA-1** — Recorrido.
  ```bash
  bash scripts/kind/dashboard-smoke.sh 2>&1 | grep -E '^(OK|FALLA|PENDIENTE)'; echo "rc=${PIPESTATUS[0]}"
  ```
  Esperado: las 7 comprobaciones en `OK`, `PENDIENTE confirmar-y-seguir-corrida (P1)`, ninguna `FALLA`, `rc=0`.

- [ ] **CA-2** — Limpieza.
  ```bash
  bash scripts/kind/dashboard-smoke.sh >/dev/null 2>&1; ss -ltn | grep -c -E '127\.0\.0\.1:(186(0[0-6])|18610)'
  ```
  Esperado: `0`.

- [ ] **CA-3** — Sin secretos en la salida.
  ```bash
  bash scripts/kind/dashboard-smoke.sh > "${TMPDIR:-/tmp}/u7t05.out" 2>&1; grep -c -E 'password_hash|\$argon2id\$|Bearer [A-Za-z0-9_-]{20,}' "${TMPDIR:-/tmp}/u7t05.out"
  ```
  Esperado: `0`.

- [ ] **CA-4** — Humo de U7-T04 sigue en verde.
  ```bash
  bash scripts/kind/kind-smoke.sh 2>&1 | grep -c '^FALLA'
  ```
  Esperado: `0`.

- [ ] **CA-5** — Alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(scripts/kind/|docs/operaciones/kind-local\.md|bitacoras/U7-T05\.md|revisiones/U7-T05/)' | wc -l
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

- **Rojo primero:** pegar la salida de `bash scripts/kind/dashboard-smoke.sh` (no existe).
- Al terminar: `bash scripts/kind/kind-down.sh`.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
