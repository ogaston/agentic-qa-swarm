# web/dashboard — frontend del Agentic QA Swarm

SPA en Vite + React + TypeScript estricto. Esta versión es el esqueleto: rutas vacías
`/login`, `/inbox`, `/runs/:id`, `/warm` y `NotFound`. Las pantallas llegan en U6-T03…T05.

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

## Scripts

`dev`, `build`, `preview`, `lint`, `typecheck`, `test`, `gen:api`, `check:api`.
