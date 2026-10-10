# U6-T03 — Login, sesión en memoria y manejo uniforme de errores de la API

**Unidad:** U6 — Frontend web (S5)
**Historias que implementa:** S5 (acceso autenticado al dashboard), sobre US-M8.3 (confirm-required exige identidad).
**Depende de:** U6-T01 **fusionada**. **Ola 2**. **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea, concreta):

> Implementar en `web/dashboard` la pantalla de login contra `go-identity` (`POST /api/auth/login`, con OTP opcional), la sesión con el token **solo en memoria**, el logout, la guarda de rutas y el tratamiento común de `401`, `429` y `503`.

Detalle:

- **Login** (`/login`): usuario, contraseña y un campo OTP de 6 dígitos que se muestra si el servidor responde `401` a un intento con usuario `admin` o si la persona lo despliega (no se adivina el rol). Envío con `POST /api/auth/login`; durante la petición el botón queda deshabilitado (sin dobles envíos). Mensajes: `401` → «Usuario, contraseña o código incorrectos» (sin distinguir causa, igual que el servidor); `429` → «Demasiados intentos, espera N s» con la cuenta atrás de `Retry-After`; error de red/`503` → «Servicio de identidad no disponible».
- **Sesión** (`src/session/`): un contexto React con `{token, expiresAt, principal}`. Tras el login se llama a `GET /api/auth/session` para obtener `principal_id` y `role`. El token vive **solo en memoria** (estado de React / variable de módulo): **prohibido** `localStorage`, `sessionStorage`, `indexedDB` y `document.cookie`. Recargar la página obliga a iniciar sesión otra vez (documentado en el README del dashboard).
- **Expiración**: un temporizador cierra la sesión local al llegar a `expires_at`. Cualquier `401` de cualquier llamada (vía `ApiError`) borra la sesión y lleva a `/login?next=<ruta>`; tras volver a entrar se navega a `next` solo si es una ruta interna (empieza por `/` y no por `//`).
- **Logout**: `POST /api/auth/logout` y borrado local **aunque** la petición falle.
- **Guarda**: `/inbox`, `/runs/:id` y `/warm` exigen sesión; sin ella redirigen a `/login?next=…`. La cabecera muestra `principal_id` y rol, y el botón de salir.
- **Errores comunes** (`src/api/errors.tsx`): un componente `<ApiErrorNotice error>` que el resto de vistas reutiliza: `429` con cuenta atrás, `503` con «servicio no disponible» y el `Retry-After` si viene, otros con `code` y `message` del cuerpo y el `requestId` para soporte. Nunca muestra el token.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Inbox, corrida y warm (U6-T04, U6-T05). Solo la guarda y la cabecera.
- Registro de usuarios, cambio de contraseña, alta de MFA, «recordarme» o refresco de token.
- Cualquier cambio fuera de `web/dashboard/**`.

---

## Archivos de contexto

- `tareas/U6-T01-esqueleto-web.md` y `bitacoras/U6-T01.md` (cliente API, proxy `/api`)
- `services/go-identity/README.md` (login, OTP, anti-brute-force, `401` sin causa, `429` con `Retry-After`)
- `contracts/openapi/control-plane.yaml` (`/auth/login`, `/auth/logout`, `/auth/session`, `Session`)

---

## Criterios de aceptación

Desde `web/dashboard/`, con `npm ci` hecho.

- [ ] **CA-1** — Lo anterior sigue en verde.
  ```bash
  npm run -s lint && npm run -s typecheck && npm run -s check:api && npx vitest run 2>&1 | grep -c -E 'failed|FAIL'
  ```
  Esperado: `0`.

- [ ] **CA-2** — Flujo de login.
  ```bash
  npx vitest run src/session src/pages/Login 2>&1 | tail -n 3
  ```
  Esperado: sin `failed` y al menos 8 pruebas (Testing Library + MSW) que cubren: login correcto → `GET /auth/session` → navega a `/inbox` con `principal_id` visible; `401` → mensaje genérico, sin navegar; `429` con `Retry-After: 30` → cuenta atrás desde 30 y botón deshabilitado; `503`/red → mensaje de identidad no disponible; un solo `POST` aunque se pulse dos veces; el OTP se envía solo si se rellenó; `next=//evil.example` se ignora y `next=/warm` se respeta; logout borra la sesión aunque el servidor responda `500`.

- [ ] **CA-3** — Expiración y `401` global.
  ```bash
  npx vitest run src/session -t 'expira|401' 2>&1 | tail -n 3
  ```
  Esperado: sin `failed` y al menos 3 pruebas: con temporizadores falsos, al pasar `expires_at` se vuelve a `/login`; un `401` de cualquier llamada protegida borra la sesión y redirige con `next`; una ruta protegida sin sesión redirige sin hacer ninguna petición a la API.

- [ ] **CA-4** — El token nunca se persiste ni se filtra.
  ```bash
  grep -r -n -E 'localStorage|sessionStorage|indexedDB|document\.cookie' src | grep -v -E '\.test\.tsx?:' | wc -l
  npx vitest run -t 'no persiste' 2>&1 | tail -n 3
  npm run -s build >/dev/null && grep -r -l -E 'localStorage|sessionStorage' dist | wc -l
  ```
  Esperado: `0`; sin `failed` (una prueba que, tras login, comprueba `localStorage.length === 0`, `sessionStorage.length === 0`, `document.cookie === ''` y que el token —marcador único— no está en `document.body.innerHTML`); `0`.

- [ ] **CA-5** — Alcance.
  ```bash
  cd ../.. && git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(web/dashboard/|bitacoras/U6-T03\.md|revisiones/U6-T03/)' | wc -l
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

- Componentes con Testing Library + `user-event`, red con MSW (`onUnhandledRequest: 'error'`), temporizadores falsos de Vitest para la expiración y la cuenta atrás.
- **Rojo primero:** pegar la salida de `npx vitest run src/session src/pages/Login` antes de implementar.

---

## Notas

- Token en memoria es una decisión de seguridad (XSS no puede leer lo que no se persiste) a cambio de pedir login al recargar. Cambiarlo es decisión del humano (candidata C-94).

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
