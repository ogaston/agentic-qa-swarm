# go-identity

Servicio de autenticación y autorización (U4-T02, U4-T03): `POST /auth/login`, `POST /auth/logout`, `GET /auth/session`, `GET /auth/sessions/{id}` y `GET /auth/users`. Las sondas (`/healthz`, `/readyz`, `/metrics`) llegan en U4-T07.

## Versión de Go

`go.mod` exige `go 1.26.8` (línea soportada con la biblioteca estándar corregida frente a govulncheck; 1.24 ya no recibe parches) y el `Dockerfile` usa `golang:1.26.8-alpine3.24`. Al subir el parche en uno, subirlo en el otro: si la imagen es anterior a `go.mod`, Go intentaría descargar el toolchain.

## Configuración (variables de entorno)

| Variable | Defecto | Descripción |
|---|---|---|
| `IDENTITY_USERS_FILE` | (obligatoria) | Ruta del JSON `[{username, password_hash, role, mfa_secret?}]`. Sin archivo, vacío o inválido el servicio no arranca. No hay usuarios por defecto. |
| `LISTEN_ADDR` | `:8080` | Dirección de escucha. |
| `IDENTITY_MAX_FAILURES` | `5` | Fallos seguidos (por usuario y por IP) antes del bloqueo. |
| `IDENTITY_SESSION_TTL` | `30m` | Expiración absoluta de la sesión (duración Go). |
| `IDENTITY_IDLE_TTL` | `15m` | Expiración por inactividad. |
| `IDENTITY_MAX_SESSIONS` | `5` | Sesiones simultáneas por usuario; la más antigua se descarta. |
| `IDENTITY_ALLOWED_ORIGINS` | (vacía) | Lista blanca CORS, coma-separada, de orígenes exactos `http(s)://host[:puerto]`. Vacía = ningún origen. `*`, `null` o entradas con ruta impiden arrancar. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` o `error`. |
| `IDENTITY_TRUST_PROXY` | `false` | Si es `true`, la IP del cliente sale del último valor de `X-Forwarded-For`. |

Estos valores son defaults razonables, no requisitos del PRD: el humano los confirma.

## Usuarios y contraseñas

```bash
printf '%s' "$CONTRASENA" | go-identity hash-password   # lee de stdin, 8 a 128 caracteres, imprime el hash PHC
```

Hash argon2id con parámetros mínimos `m=19 MiB, t=2, p=1` (se aceptan más altos). Los usuarios con rol `admin` exigen `mfa_secret` (base32, al menos 160 bits) y TOTP (RFC 6238, SHA-1, 6 dígitos, 30 s, ventana +-1, sin reutilización). El Secret con el archivo de usuarios lo crea la operación (ver `docs/operaciones/secrets.md`).

## Anti-brute-force

- Contador de fallos consecutivos por `username` (minúsculas, también inexistentes) y por IP; tablas acotadas a 10 000 entradas.
- Al llegar a `IDENTITY_MAX_FAILURES`: bloqueo de 30 s que se duplica en cada reincidencia hasta 15 min. Bloqueado responde `429` con `Retry-After`, incluso con la contraseña correcta.
- Un acierto reinicia **solo el contador del usuario que acertó**. El contador por IP cuenta únicamente fallos (el intento acertado se le devuelve) y decae por tiempo: una serie de fallos con más de 15 minutos (`guard.IPWindow`, fija) expira. Así un acierto propio no borra los fallos acumulados contra otras cuentas (password spraying).

## Autorización (U4-T03)

- **Deny-by-default.** Las rutas se registran con `Handle(patrón, Política, handler)`; `Public()` (solo `POST /auth/login`), `Authenticated()` o `Roles(...)`. Registrar sin política panica al arrancar; lo no registrado da `404` JSON y un método no registrado en una ruta existente da `405` (nunca `2xx`). `Server.Routes()` enumera las rutas y una prueba falla si alguna no declara política.
- **Token en cada request.** `Authorization: Bearer <43 caracteres base64url>` (esquema sin distinguir mayúsculas, un solo espacio, una sola cabecera). Se valida contra el almacén (SHA-256, expiración absoluta, inactividad, revocación) sin caché: tras `logout` el token falla en la siguiente petición. Todo fallo es `401 {code: unauthorized}` con `WWW-Authenticate: Bearer`, sin distinguir la causa.
- **Roles solo del servidor.** El rol sale de la sesión; `X-Role`, `X-User`, parámetros y cuerpo se ignoran.
- **`authz.Authorize(principal, action, resource)`.** `session:read` (user solo si es propietario; un recurso sin propietario no es de nadie; admin sí) y `users:list` (solo admin). Todo lo demás, `Deny`.
- **IDOR.** `GET /auth/sessions/{id}`: `user` recibe `403` para una sesión ajena y también para un `id` inexistente (sin oráculo de existencia); `admin` recibe `404` si no existe. `session_id` (128 bits, derivado del token con separación de dominio) es público y no sirve como token.
- **CORS.** Solo orígenes de la lista blanca: `Access-Control-Allow-Origin` con ese origen y `Vary: Origin`; preflight con `Allow-Methods: GET, POST` y `Allow-Headers: Authorization, Content-Type`. Nunca `*` ni `Allow-Credentials` (tokens `Bearer`, no cookies).
- **Cabeceras de seguridad** en todas las respuestas: `nosniff`, `Cache-Control: no-store`, CSP `default-src 'none'; frame-ancestors 'none'`, HSTS.
- **Límite conocido.** El servidor HTTP de Go recorta los espacios finales de los valores de cabecera antes de llegar al middleware, así que `Bearer <token> ` (espacio final) por red equivale a la forma válida; el middleware lo rechaza si lo recibiera tal cual (prueba unitaria).

## Observabilidad (U4-T07)

- Log JSON (`timestamp`, `level` en minúsculas, `message`, `request_id`, `trace_id`, `service`); nunca contraseñas, hashes, tokens, OTP ni cuerpos de petición.
- `GET /healthz` (siempre 200), `GET /readyz` (archivo de usuarios legible y almacén de sesiones operativo; 503 si falla) y `GET /metrics` (Prometheus) no requieren token ni pasan por el anti-brute-force.
- Métricas de seguridad: `aqs_auth_login_total{result}`, `aqs_auth_lockouts_total`, `aqs_auth_active_sessions`, `aqs_authz_denied_total{reason}` y `aqs_privilege_escalation_attempts_total{endpoint}` (patrón de ruta). Los contadores arrancan en 0.

## Advertencias y límites de diseño

- **Proxy e IP.** Con `IDENTITY_TRUST_PROXY=false` detrás de un ingress, `RemoteAddr` es la IP del ingress: cinco fallos de cualquiera bloquean a todos, con escalada hasta 15 min. Detrás de un proxy de confianza, activar `IDENTITY_TRUST_PROXY=true`; nunca activarlo si el servicio es alcanzable sin pasar por él (el cliente forjaría la IP).
- **Estado en memoria, por réplica.** Sesiones y contadores del guard se pierden al reiniciar y no se comparten: con N réplicas el límite efectivo de intentos es N veces mayor y un token solo vale en la réplica que lo emitió (candidata C-49).
- **Intentos reservados por adelantado.** Cada intento cuenta como fallo al admitirse, para que los intentos concurrentes no superen el límite: 8 logins correctos paralelos del mismo usuario dan 5 `200` y 3 `429`.
- **Memoria de argon2.** Cada login hashea con ~19 MiB y no hay límite de concurrencia propio (los bloqueos recortan el abuso por usuario/IP). Dimensionar `resources.limits` del Deployment en consecuencia.
- **Hash señuelo.** Para usuarios inexistentes se verifica contra un hash señuelo que copia los parámetros más altos de los hashes cargados, de modo que el tiempo no distingue usuarios.
- **Registros.** `slog` JSON con `level` y `message`; jamás contraseñas, tokens, OTP, hashes ni nombres de usuario.
