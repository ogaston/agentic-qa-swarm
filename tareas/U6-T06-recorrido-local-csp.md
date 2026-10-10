# U6-T06 — Recorrido local del dashboard contra los servicios reales y CSP del build

**Unidad:** U6 — Frontend web (S5)
**Historias que implementa:** S5 (dashboard de extremo a extremo en local), sobre US-M1, US-M2 y US-M8.3.
**Depende de:** U6-T04 y U6-T05 **fusionadas**. **Ola 4**. Cierra U6. **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea, concreta):

> Añadir `scripts/test/u6-up.sh`, `u6-down.sh` y `u6-demo-local.sh`, que levantan en loopback `go-identity` y `ui-api` reales, simulan `go-warm-manager` y `go-run-controller`, sirven el build del dashboard con `vite preview` y recorren login → inbox → confirmación → corrida → warm a través del origen del dashboard; y fijar una CSP estricta en el build.

Detalle:

- **Arranque** (`u6-up.sh`), todo en `127.0.0.1` y en un directorio temporal propio (`mktemp -d`, permisos `700`):
  - `go-identity` en `:18500` con un archivo de usuarios generado al vuelo con `go-identity hash-password` (usuario `demo`, rol `user`, contraseña aleatoria guardada en el directorio temporal con permisos `600`; nunca en el repo ni impresa).
  - `ui-api` en `:18501` con `UIAPI_AUTH=identity`, `IDENTITY_URL` al anterior, `WARM_URL` al stub, `UIAPI_WARM_TOKEN_FILE` con un token aleatorio, y un `UIAPI_EVENTS_FILE` sembrado con 2 notificaciones `notify.created` válidas contra su esquema.
  - Stubs en Python estándar (`scripts/test/u6_stubs.py`): warm-manager en `:18502` (`GET /warm` → `ready` con `reset_verified=true`, exige el token de servicio) y run-controller en `:18503` (`GET /runs/{id}` valida el token de la persona contra `go-identity /auth/session` y devuelve una secuencia `deploying → running → done` en llamadas sucesivas). Ambos responden `200` en `/healthz`.
  - `vite preview` del build en `:18504` con las tres variables del proxy apuntando a lo anterior.
  - `u6-down.sh` para todos los procesos por PID y borra el directorio temporal.
- **Recorrido** (`u6-demo-local.sh`): `u6-up` → comprobaciones con `curl` **solo contra `:18504`** (el origen del navegador) → `u6-down`. Imprime `OK|FALLA <comprobación>` por cada una: `index-servido`, `csp-estricta`, `login`, `inbox-con-pendientes`, `confirmar-201`, `confirmar-otra-vez-409`, `corrida-hasta-done`, `warm-ready`, `mis-corridas`, `sin-token-401`, `logout-invalida-token`. Sale distinto de 0 si alguna falla y siempre llama a `u6-down` (trap).
- **CSP del build**: `index.html` del build sin scripts ni estilos en línea, y `vite preview` (y `dev`) sirven `Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'`, más `X-Content-Type-Options: nosniff` y `Referrer-Policy: no-referrer`. La app funciona con esa política (el recorrido lo demuestra).

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Navegador real o Playwright (candidata C-95), imagen de contenedor, servidor estático de producción, manifiestos o despliegue.
- Cambios en los servicios Go o en los agentes; si el recorrido destapa un defecto en ellos, se registra como candidata y el recorrido lo documenta como `FALLA` esperada solo con aprobación del orquestador.
- Clúster, proveedor LLM, NATS, MinIO.

---

## Archivos de contexto

- `tareas/U6-T01-esqueleto-web.md` … `tareas/U6-T05-corrida-y-warm.md` y sus bitácoras
- `scripts/test/u3-up.sh`, `u3-down.sh`, `u3-demo-local.sh` (forma de arranque, PID y `OK|FALLA`)
- `services/go-identity/README.md` (`hash-password`, usuarios), `services/ui-api/README.md`
- `contracts/events/` (esquema de `notify.created`)

---

## Criterios de aceptación

Desde la raíz del worktree, con Go, Python 3 y Node disponibles (`GOTOOLCHAIN=auto` si hace falta).

- [ ] **CA-1** — Lo anterior sigue en verde.
  ```bash
  (cd web/dashboard && npm ci --no-audit --no-fund >/dev/null && npm run -s lint && npm run -s typecheck && npx vitest run 2>&1 | grep -c -E 'failed|FAIL')
  (cd services/ui-api && go test -count=1 ./... 2>&1 | grep -c -E '^(FAIL|--- FAIL)')
  ```
  Esperado: `0` y `0`.

- [ ] **CA-2** — Arranque y parada.
  ```bash
  bash scripts/test/u6-up.sh && for p in 18500 18501 18502 18503; do curl -s -o /dev/null -w '%{http_code} ' http://127.0.0.1:$p/healthz; done; curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:18504/
  bash scripts/test/u6-down.sh; for p in 18500 18501 18502 18503 18504; do curl -s -o /dev/null -w '%{http_code} ' --max-time 2 http://127.0.0.1:$p/; done; echo
  ```
  Esperado: `200 200 200 200 200` y, tras parar, `000 000 000 000 000`.

- [ ] **CA-3** — Recorrido completo.
  ```bash
  bash scripts/test/u6-demo-local.sh 2>&1 | grep -E '^(OK|FALLA)'; echo "rc=${PIPESTATUS[0]}"
  ss -ltn | grep -c -E '127\.0\.0\.1:185(00|01|02|03|04)'
  ```
  Esperado: las 11 comprobaciones en `OK`, ninguna `FALLA`, `rc=0`, y `0` puertos escuchando al terminar.

- [ ] **CA-4** — CSP y build sin código en línea.
  ```bash
  (cd web/dashboard && npm run -s build >/dev/null && grep -c -E '<script>|<script [^>]*>[^<]+</script>|<style|style=' dist/index.html)
  bash scripts/test/u6-up.sh >/dev/null && curl -s -D - -o /dev/null http://127.0.0.1:18504/ | grep -i -c -E "^content-security-policy: default-src 'self'; script-src 'self'.*frame-ancestors 'none'"; bash scripts/test/u6-down.sh
  ```
  Esperado: `0` y `1`.

- [ ] **CA-5** — Ningún secreto en el repo ni en la salida.
  ```bash
  bash scripts/test/u6-demo-local.sh > /tmp/u6.out 2>&1; grep -c -E 'password_hash|\$argon2id\$|Bearer [A-Za-z0-9_-]{20,}' /tmp/u6.out
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(scripts/test/u6-(up|down|demo-local)\.sh|scripts/test/u6_stubs\.py|web/dashboard/|bitacoras/U6-T06\.md|revisiones/U6-T06/)' | wc -l
  ```
  Esperado: `0`, `0` y `0`.

---

## Plan de pruebas

- El recorrido es la prueba de integración. Los stubs se prueban de forma indirecta (el recorrido falla si el stub del controlador acepta un token inválido: `sin-token-401`).
- **Rojo primero:** pegar la salida de `bash scripts/test/u6-demo-local.sh` (no existe) antes de implementar.

---

## Notas

- `curl` contra `vite preview` prueba el cableado del proxy, la CSP y los servicios reales; el comportamiento de la UI lo prueban las suites de T03–T05. Un e2e con navegador queda como candidata (C-95).

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
