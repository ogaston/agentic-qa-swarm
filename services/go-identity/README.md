# go-identity

Servicio de autenticación (U4-T02): `POST /auth/login` y `POST /auth/logout`. La autorización, `GET /auth/session`, CORS y las sondas llegan en U4-T03 y U4-T07.

## Versión de Go

`go.mod` exige `go 1.24.13` (parche con la biblioteca estándar corregida frente a govulncheck) y el `Dockerfile` usa `golang:1.24.13-alpine3.22`. Al subir el parche en uno, subirlo en el otro: si la imagen es anterior a `go.mod`, Go intentaría descargar el toolchain.

## Configuración (variables de entorno)

| Variable | Defecto | Descripción |
|---|---|---|
| `IDENTITY_USERS_FILE` | (obligatoria) | Ruta del JSON `[{username, password_hash, role, mfa_secret?}]`. Sin archivo, vacío o inválido el servicio no arranca. No hay usuarios por defecto. |
| `LISTEN_ADDR` | `:8080` | Dirección de escucha. |
| `IDENTITY_MAX_FAILURES` | `5` | Fallos seguidos (por usuario y por IP) antes del bloqueo. |
| `IDENTITY_SESSION_TTL` | `30m` | Expiración absoluta de la sesión (duración Go). |
| `IDENTITY_IDLE_TTL` | `15m` | Expiración por inactividad. |
| `IDENTITY_MAX_SESSIONS` | `5` | Sesiones simultáneas por usuario; la más antigua se descarta. |
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

## Advertencias y límites de diseño

- **Proxy e IP.** Con `IDENTITY_TRUST_PROXY=false` detrás de un ingress, `RemoteAddr` es la IP del ingress: cinco fallos de cualquiera bloquean a todos, con escalada hasta 15 min. Detrás de un proxy de confianza, activar `IDENTITY_TRUST_PROXY=true`; nunca activarlo si el servicio es alcanzable sin pasar por él (el cliente forjaría la IP).
- **Estado en memoria, por réplica.** Sesiones y contadores del guard se pierden al reiniciar y no se comparten: con N réplicas el límite efectivo de intentos es N veces mayor y un token solo vale en la réplica que lo emitió (candidata C-49).
- **Intentos reservados por adelantado.** Cada intento cuenta como fallo al admitirse, para que los intentos concurrentes no superen el límite: 8 logins correctos paralelos del mismo usuario dan 5 `200` y 3 `429`.
- **Memoria de argon2.** Cada login hashea con ~19 MiB y no hay límite de concurrencia propio (los bloqueos recortan el abuso por usuario/IP). Dimensionar `resources.limits` del Deployment en consecuencia.
- **Hash señuelo.** Para usuarios inexistentes se verifica contra un hash señuelo que copia los parámetros más altos de los hashes cargados, de modo que el tiempo no distingue usuarios.
- **Registros.** `slog` JSON con `level` y `message`; jamás contraseñas, tokens, OTP, hashes ni nombres de usuario.
