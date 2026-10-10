# web/dashboard — frontend del Agentic QA Swarm

SPA en Vite + React + TypeScript estricto. Login, sesión en memoria y guarda de rutas (U6-T03). Las rutas
`/inbox`, `/runs/:id` y `/warm` son todavía placeholders; sus pantallas llegan en U6-T04 y U6-T05.

## Arranque

Requiere Node 20 o superior.

```bash
cd web/dashboard
npm ci
export AQS_IDENTITY_URL=http://localhost:8081      # go-identity
export AQS_RUN_CONTROLLER_URL=http://localhost:8082 # controlador de corridas
export AQS_UI_API_URL=http://localhost:8083         # ui-api
npm run dev        # servidor de desarrollo
npm run preview    # sirve dist/ con el mismo proxy
```

Sin una de las tres variables, `dev` y `preview` no arrancan y el error nombra la variable que falta.
`build`, `test` y `lint` no las necesitan.

## Proxy `/api`

El frontend llama siempre a rutas relativas `/api/...`. El proxy quita el prefijo y enruta:

| Prefijo | Destino | Variable |
|---|---|---|
| `/api/auth/*` | identidad | `AQS_IDENTITY_URL` |
| `/api/runs/*` | controlador de corridas | `AQS_RUN_CONTROLLER_URL` |
| resto de `/api/*` | ui-api | `AQS_UI_API_URL` |

La tabla vive en `src/proxy/table.ts`, función pura con pruebas.

## Contrato y tipos

- `src/api/schema.gen.ts` se genera desde `contracts/openapi/control-plane.yaml`: `npm run gen:api`.
- `npm run check:api` regenera a un archivo temporal y falla si difiere del commiteado. Lo corre CI.
- `src/api/client.ts`: `apiFetch<T>(path, init)`. Una sola petición por llamada, sin reintentos.
  Cualquier fallo no 2xx lanza `ApiError {status, code, message, retryAfter?, requestId}`.
  Un fallo de red (fetch rechazado) lanza `ApiError` con `code: "network_error"` y `status: 0`.
  Un cuerpo no JSON o no `Error` produce `code: "unexpected_response"`.
- El token se inyecta con `setAuthToken` y vive solo en memoria.

## Sesión y login

- `/login` pide usuario y contraseña. El campo de código de verificación (6 dígitos) aparece si
  el servidor responde `401` a un intento con usuario `admin`, o si la persona lo despliega.
  El código se envía solo si se rellenó.
- Tras entrar se llama a `GET /api/auth/session` para obtener `principal_id` y `role`, que se
  muestran en la cabecera junto al botón de salir.
- **El token vive solo en memoria** (estado de React y el módulo del cliente). No se usa
  `localStorage`, `sessionStorage`, `indexedDB` ni `document.cookie`. **Recargar la página
  obliga a iniciar sesión otra vez.** Es una decisión de seguridad: un XSS no puede leer lo
  que no se persiste, a cambio de pedir login al recargar. Cambiarlo es decisión del humano
  (candidata C-94).
- Un temporizador cierra la sesión local al llegar a `expires_at`. Cualquier `401` de una
  llamada autenticada borra la sesión y lleva a `/login?next=<ruta>`. Tras entrar se navega a
  `next` solo si es una ruta interna (empieza por `/`, no por `//` ni `/\`).
- Logout: `POST /api/auth/logout` y borrado local aunque la petición falle.
- Si el reloj del equipo va adelantado respecto al servidor, la sesión puede expirar antes de
  tiempo: el contrato no trae `expires_in`, así que el cliente compara con `expires_at`.
- Un `expires_at` que no sea una fecha válida rechaza el login y no deja sesión ni token.
- `/inbox`, `/runs/:id` y `/warm` exigen sesión: sin ella redirigen a `/login?next=…` sin
  llamar a la API.
- `src/api/errors.tsx` exporta `<ApiErrorNotice error={…} />`: `429` con cuenta atrás,
  `503`/red con «Servicio no disponible» y `Retry-After` si viene, y el resto con `code`,
  `message` y `requestId` para soporte. Nunca muestra el token.

## Scripts

`dev`, `build`, `preview`, `lint`, `typecheck`, `test`, `gen:api`, `check:api`.
