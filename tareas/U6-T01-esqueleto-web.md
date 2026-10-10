# U6-T01 — Esqueleto de `web/dashboard`: Vite + React + TypeScript, tipos desde el OpenAPI y CI

**Unidad:** U6 — Frontend web (S5)
**Historias que implementa:** S5 (dashboard inbox + estado del warm), habilitadora; US-M1 y US-M2 se exponen por primera vez en una UI.
**Depende de:** MVP cerrado (U1–U5 terminadas). **Ola 1**, en paralelo con U6-T02 (no comparten archivos). **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea, concreta):

> Crear la aplicación `web/dashboard/` (Vite + React + TypeScript estricto) con tipos generados desde `contracts/openapi/control-plane.yaml`, un cliente API tipado, una pantalla vacía con el armazón de navegación, y un workflow de CI que la compila y la prueba.

Detalle:

- **Stack fijado** (versiones **exactas** en `package.json`, sin `^` ni `~`; `package-lock.json` versionado): `react@18.3.1`, `react-dom@18.3.1`, `react-router-dom@6.28.0`; dev: `vite@5.4.11`, `@vitejs/plugin-react@4.3.4`, `typescript@5.6.3`, `vitest@2.1.8`, `jsdom@25.0.1`, `@testing-library/react@16.1.0`, `@testing-library/user-event@14.5.2`, `msw@2.6.8`, `openapi-typescript@7.4.4`, `eslint@9.16.0`, `typescript-eslint@8.18.0`. Si una versión no resuelve, usa la más cercana que exista y regístralo en la bitácora; nunca un rango. **Ninguna otra dependencia de ejecución** (sin librería de UI, de estado ni de fetch).
- **Scripts npm**: `dev`, `build` (`tsc --noEmit && vite build`), `preview`, `lint`, `typecheck`, `test` (`vitest run`), `gen:api` (`openapi-typescript ../../contracts/openapi/control-plane.yaml -o src/api/schema.gen.ts`), `check:api` (regenera a un archivo temporal y falla si difiere de `schema.gen.ts`).
- **Prefijo `/api`**: el frontend llama siempre a rutas relativas `/api/...`. El proxy de `vite.config.ts` (dev y preview) quita el prefijo y enruta por variable de entorno: `/api/auth/*` → `AQS_IDENTITY_URL`, `/api/runs/*` → `AQS_RUN_CONTROLLER_URL`, el resto de `/api/*` → `AQS_UI_API_URL`. Así las rutas del SPA (`/runs/:id`, …) no chocan con las de la API.
- **Cliente API** `src/api/client.ts`: `apiFetch<T>(path, init)` tipado con `schema.gen.ts`; añade `Authorization: Bearer <token>` si hay token (el token se le **inyecta**; esta tarea no lo guarda en ningún sitio), `Accept: application/json` y un `X-Request-Id` generado (`^[A-Za-z0-9._-]{8,64}$`). Una respuesta no 2xx lanza `ApiError {status, code, message, retryAfter?, requestId}` leyendo el cuerpo `Error` del contrato; un cuerpo no JSON produce `code: "unexpected_response"`. Sin reintentos.
- **Armazón**: `App.tsx` con router y rutas vacías `/login`, `/inbox`, `/runs/:id`, `/warm`, y un `<NotFound>`; título del documento `Agentic QA Swarm`. Sin estilos elaborados: un `src/styles.css` con tokens de color en `:root` y modo oscuro por `prefers-color-scheme`.
- **CI**: `.github/workflows/web.yml` (nuevo) que, con `paths` en `web/**` y `contracts/openapi/**`, corre `npm ci`, `lint`, `typecheck`, `check:api`, `test`, `build` y `npm audit --omit=dev --audit-level=high`. Acciones pineadas por SHA de 40 caracteres, como en `ci.yml`; `permissions: contents: read`.
- `README.md` raíz: añadir `web/` al layout del monorepo (una línea) y `web/dashboard/README.md` con cómo arrancarlo y las tres variables del proxy.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Login, inbox, corrida, warm: son U6-T03…T05. Esta tarea solo deja las rutas vacías.
- Cualquier cambio en `services/**`, `agents/**`, `contracts/**`, `deploy/**` o en los workflows existentes.
- Imagen de contenedor, servidor estático de producción, manifiestos Flux o CSP de producción (backlog).
- Tocar `mockups/**`: es un prototipo de diseño, no código de partida.

---

## Archivos de contexto

- `unidades-y-tareas.md` (U6 y nota de S5), `specs/prd.md` (S5)
- `contracts/openapi/control-plane.yaml` (rutas y esquemas)
- `services/ui-api/README.md`, `services/go-identity/README.md` (CORS, tokens `Bearer`, cabeceras)
- `mockups/swarm-mock.html` (solo como referencia visual de las vistas)
- `.github/workflows/ci.yml` (forma de pinear acciones)

---

## Criterios de aceptación

Desde la raíz del worktree, con Node 20+ (`node --version`).

- [ ] **CA-1** — Instala, compila y pasa calidad.
  ```bash
  cd web/dashboard && npm ci --no-audit --no-fund >/dev/null && npm run lint && npm run typecheck && npm run build >/dev/null; echo "rc=$?"; ls dist/index.html
  ```
  Esperado: `rc=0` y `dist/index.html` existe.

- [ ] **CA-2** — Versiones exactas y dependencias acotadas.
  ```bash
  cd web/dashboard && node -e 'const p=require("./package.json");const all={...p.dependencies,...p.devDependencies};const bad=Object.entries(all).filter(([,v])=>/[\^~*x]|latest/.test(v));console.log(bad.length, Object.keys(p.dependencies).sort().join(","))'
  ```
  Esperado: `0 react,react-dom,react-router-dom`.

- [ ] **CA-3** — Tipos del contrato al día.
  ```bash
  cd web/dashboard && npm run -s check:api; echo "rc=$?"
  printf '\n# deriva\n' >> ../../contracts/openapi/control-plane.yaml; sed -i 's/operationId: getRun/operationId: getRunX/' ../../contracts/openapi/control-plane.yaml; npm run -s check:api >/dev/null 2>&1; echo "rc_deriva=$?"; git checkout -- ../../contracts/openapi/control-plane.yaml
  ```
  Esperado: `rc=0` y `rc_deriva` distinto de `0`.

- [ ] **CA-4** — El cliente API cumple su contrato.
  ```bash
  cd web/dashboard && npx vitest run src/api 2>&1 | tail -n 3
  ```
  Esperado: sin `failed` y al menos 6 pruebas que cubren (con MSW): cabecera `Authorization` solo si hay token; `X-Request-Id` con el patrón; 4xx con cuerpo `Error` → `ApiError` con `code`/`message`/`status`; `429` con `Retry-After` → `retryAfter` numérico; cuerpo no JSON → `unexpected_response`; exactamente 1 petición por llamada (sin reintentos).

- [ ] **CA-5** — El proxy enruta por prefijo.
  ```bash
  cd web/dashboard && npx vitest run src/proxy 2>&1 | tail -n 3
  ```
  Esperado: sin `failed`; las pruebas comprueban (sobre la función que construye la tabla de proxy) que `/api/auth/login` → identidad, `/api/runs/x` → controlador, `/api/notifications` y `/api/warm` → ui-api, todas sin el prefijo `/api`, y que sin una de las tres variables `vite` no arranca con un error que la nombra.

- [ ] **CA-6** — CI y alcance.
  ```bash
  grep -c -E 'uses: [^ ]+@[0-9a-f]{40}' .github/workflows/web.yml; grep -c -E 'uses: [^ ]+@v[0-9]' .github/workflows/web.yml
  for s in 'npm ci' 'run lint' 'run typecheck' 'check:api' 'run test' 'run build' 'npm audit'; do grep -c -- "$s" .github/workflows/web.yml; done | paste -sd' '
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(web/dashboard/|\.github/workflows/web\.yml|README\.md|bitacoras/U6-T01\.md|revisiones/U6-T01/)' | wc -l
  ```
  Esperado: al menos `1`, luego `0`; siete valores ≥ `1`; `0` y `0`.

---

## Plan de pruebas

- Unitarias de `client.ts` con MSW en Vitest (`jsdom`); la guarda de MSW con `onUnhandledRequest: 'error'` para que ninguna prueba salga a la red.
- La tabla de proxy se extrae a una función pura (`src/proxy/table.ts`) para probarla sin levantar Vite.

**Rojo primero:** pegar la salida de `npx vitest run src/api` antes de implementar el cliente.

---

## Notas

- El token se guarda en U6-T03 (en memoria). Aquí el cliente lo recibe por parámetro o por un `setToken` del módulo, sin persistirlo.
- Node lo instala `fnm`; si el revisor no tiene Node, `podman run --rm -v "$PWD":/w -w /w/web/dashboard docker.io/library/node:20.18.1 npm ci && …` es equivalente.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
