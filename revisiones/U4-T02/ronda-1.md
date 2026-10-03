# Ronda 1 — U4-T02

VEREDICTO: NO-VERDE

Los 11 criterios de aceptación pasan con la ejecución propia del revisor. Quedan dos NARANJA: un hueco en el anti-brute-force por IP y la documentación de configuración. No hay ROJO.

## Arbitraje del orquestador, valorado
1. CA-9 tercer comando: con `keys | sort` da `username,password | otp,password,username`. Defecto de especificación, no hallazgo contra el codificador.
2. CA-10 `docker build` literal: falla con x509 en `go mod download` (el contenedor de build no confía en la CA del proxy). Repetida la estrategia del codificador en un contexto temporal fuera del repo (mismo Dockerfile más `COPY ca.crt` y `ENV SSL_CERT_FILE` en la etapa build); el diff contra el Dockerfile real muestra solo esas dos líneas y la CA no llega a la imagen final. Sustitución legítima. Humo de la imagen: `uid=65532(identity)`, `200` con token, `EXPOSE 8080/tcp`, log JSON con `level` y `message`, solo el binario en `/usr/local/bin`.
3. `golang.org/x/crypto` v0.31.0: govulncheck no pudo correrse (`vuln.go.dev` devuelve 403 en el proxy). El módulo solo importa `argon2`, `blake2b` y `x/sys/cpu`; los avisos conocidos de x/crypto están en `ssh`. Sin verificar contra la base de datos: debe confirmarse en la CI real.
4. `totp-code` (opcional) no implementado; CA-5 con Python pasa.

## Criterios de aceptación, verificados por el revisor
| # | Resultado |
|---|---|
| 1 | pasa: 90 PASS, 0 FAIL, 0 SKIP (server 12.8 s, guard 5.7 s) |
| 2 | pasa: `200`, `43`, fecha futura, `204 401` |
| 3 | pasa: dos líneas idénticas con el mismo `message` |
| 4 | pasa: `401×5`, `429`, `Retry-After: 30` |
| 5 | pasa: `401 mfa_required`, `200`, `401` por reuso |
| 6 | pasa: seis `rc=1`; el log de arranque no contiene el hash |
| 7 | pasa: gitleaks `0`, grep `0`, log `0` |
| 8 | pasa: 6 PASS |
| 9 | pasa (con `sort`): lint válido, 0 líneas eliminadas |
| 10 | pasa con la sustitución legítima: `65532:65532`, `0`, `1`, `go`, `0` |
| 11 | pasa: `ok`, `0`, `0` |

## Sondas adversariales (binario real)
- Enumeración: `401` idéntico para usuario inexistente y contraseña mala; tiempos del mismo orden (~31 ms); el bloqueo también aplica a inexistentes.
- OTP: con letras da `400`; 4 peticiones paralelas del mismo OTP → un `200` y tres `401` (reutilización atómica).
- Entradas: contraseña de 2000 bytes `400`; cuerpo de 9 KB `413`; campo extra o basura final `400`; `Content-Type` incorrecto `415`; GET `405`; logout sin cabecera `401`.
- Secretos: el log solo trae `outcome` e `ip`; el token se guarda como SHA-256; comparaciones con `subtle.ConstantTimeCompare`.
- Carreras: `-race` limpio. Cero credenciales: las pruebas generan sus secretos; los únicos literales son los vectores públicos de RFC 6238.

## Hallazgos
### F-01 · NARANJA · `internal/guard/guard.go:133` (`Success`) · Un login acertado de cualquier cuenta reinicia el contador por IP (password spraying sin bloqueo)
`Success(tk)` borra la entrada de la IP aunque el acierto sea de otra cuenta. Un atacante con una cuenta válida alterna un fallo contra una víctima con un login propio correcto: nunca llega a `IDENTITY_MAX_FAILURES` por IP y el contador por usuario ve un fallo por víctima. Evidencia (binario real, 12 víctimas con login de `marta` intercalado): `401 ok:200 401 ok:200 ...` ×12, ningún `429`. Sin los logins intercalados, las mismas víctimas dan `401×5` y luego `429`. El texto de la tarea («un acierto fuera de bloqueo reinicia el contador») es ambiguo; la lectura segura es reiniciar solo el contador del usuario que acertó, y que el de la IP decaiga por ventana de tiempo. Falta una prueba que fije el comportamiento.

### F-02 · NARANJA · Nota de la tarea («configurables y documentados») · La configuración no está documentada
`IDENTITY_USERS_FILE`, `LISTEN_ADDR`, `IDENTITY_MAX_FAILURES`, `IDENTITY_SESSION_TTL`, `IDENTITY_IDLE_TTL`, `IDENTITY_MAX_SESSIONS` e `IDENTITY_TRUST_PROXY` y sus defaults (5, 30 m, 15 m, 5, false) solo están como literales en `cmd/go-identity/main.go:106-127`. No hay README ni la bitácora los lista. Basta un `services/go-identity/README.md` (dentro de alcance) con variables, defaults y la advertencia de F-03.

### F-03 · AMARILLO · Límites de diseño que conviene dejar escritos
- Con `IDENTITY_TRUST_PROXY=false` tras un ingress, `RemoteAddr` es la IP del ingress: cinco fallos de cualquiera bloquean a todos (escalada hasta 15 min).
- Intentos reservados por adelantado: 8 logins correctos paralelos del mismo usuario → 5 `200` y 3 `429`. Diseño pedido por la tarea.
- El hash señuelo usa parámetros mínimos; un usuario con parámetros mayores se distingue por tiempo. Mejor que el señuelo copie los parámetros más altos cargados.
- Argon2 sin límite de concurrencia (~19 MiB por login). Fuera de este diff.

## Tareas candidatas (fuera de alcance)
- Limitar la concurrencia de argon2 y poner `resources.limits` de memoria al Deployment.
- C-49 ya cubre sesiones multi-réplica; añadir que el estado del guard también se pierde por réplica (límite efectivo ×N).
- Que la CI de imágenes confíe en la CA del entorno, o documentar la sustitución de CA-10.

VEREDICTO: NO-VERDE
NARANJA|services/go-identity/internal/guard/guard.go:133|Un login acertado de cualquier cuenta reinicia el contador por IP (password spraying sin bloqueo, 12 de 12 sin 429)
NARANJA|nota de la tarea (configurables y documentados)|Variables IDENTITY_*, valores por defecto y advertencia de proxy sin documentar en ningún sitio
INFORME: revisiones/U4-T02/ronda-1.md
