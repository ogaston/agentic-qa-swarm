# Ronda 1 — U4-T03

VEREDICTO: VERDE

CA-1..CA-12 corridos por el revisor con `GOTOOLCHAIN=go1.26.8` y `-count=1`. Todos pasan salvo el último caso de CA-2, defecto de especificación arbitrado por el orquestador. Sin ROJO ni NARANJA. Worktree limpio.

## Criterios de aceptación, verificados por el revisor
| # | Resultado |
|---|---|
| 1 | 76 PASS (`-run AuthZ`), 0 FAIL; 9 paquetes `ok`; sin SKIP ni Sleep; server 14 s |
| 2 | `401 401 401 401 401 200`, luego `200 ["user",true,true,"…"]` y `1` (el último `200` es el caso arbitrado) |
| 3 | `propia=200 ajena=403 inexistente_user=403 admin_ajena=200 admin_inexistente=404 sin_token=401` |
| 4 | `user=403 admin=200 sin=401`, `["role","username"]`, `0` |
| 5 | `hdr=403 qs=403 body=405` (nunca 2xx) |
| 6 | `antes=200 despues=401 users=401` |
| 7 | 3 PASS; `desconocida=404 metodo=405` |
| 8 | ACAO `https://app.example`, `Vary: Origin`, `0`, Allow-Headers `Authorization, Content-Type`, Allow-Methods `GET, POST`, sin credentials, `0` |
| 9 | `4` y `4` |
| 10 | lint válido; 0 líneas eliminadas; rutas y `required` de `Session` esperados |
| 11 | `s.json valid` |
| 12 | `ok`, `0`, `0`, `0` |

## Arbitraje del orquestador, valorado
1. CA-2, espacio final: la prueba `TestAuthZ_BearerParsing/espacio_final` inyecta `"Bearer "+tok+" "` con `Header.Set` sobre el handler completo y exige 401; el mutante con `strings.TrimSpace` la hace fallar. Por red, Go recorta el espacio (también tabulador y obs-fold) antes del middleware y el token sigue siendo el válido de 43 caracteres: no abre ningún bypass. Documentado en el README. No es hallazgo.
2. CA-5/CA-7 `405`: nunca 2xx (`TestAuthZ_WrongMethodIsNever2xx`: DELETE, PUT, POST, PATCH → 405 con `Allow`). Documentado.
3. Go 1.26.8, x/crypto v0.57.0 sin cambios. CI de GitHub sobre 2cda126: `ci`, `contracts` y `policies` en success; `vuln (services/go-identity)` ejecutó govulncheck con success.
4. No hizo falta construir ningún Dockerfile.

## Sondas adversariales (binario real)
- Cabecera `Authorization`: dos cabeceras → 401; token por query o `Cookie` → 401; `BEARER` en mayúsculas → 200; login público con `Bearer junk` → 400.
- IDOR: el 403 de una sesión ajena y el de una inexistente son idénticos byte a byte; `X-Role`/`?role=`/`owner` sin efecto; id en mayúsculas no coincide; el `session_id` no sirve como token.
- Rutas: `/auth/users/` y `/AUTH/users` → 404; `//auth/users` y `/auth/./users` → 307 (redirección canónica del mux); HEAD sin token → 401.
- CORS: solo el origen exacto recibe cabeceras; `https://app.example/`, `…evil.example`, `HTTPS://…`, `:443`, `null` no; sin `Allow-Credentials` ni `*`; `ParseOrigins` rechaza `*`, `null`, rutas, esquemas raros.
- Carrera logout/uso del token: 2450 peticiones, 16 hilos, 0 respuestas 200 iniciadas tras el logout; `Authenticate` y `Revoke` comparten mutex, sin caché.
- `/auth/users` solo devuelve `username` y `role`.
- Mutantes sobre copia: M1 (IDOR 404), M3 (CORS por sufijo), M5 (`TrimSpace`), M7 (sin `Cache-Control`) detectados; M4 (`X-Role` abre `Roles`) y M6 (`/auth/users` como `Authenticated`) sobreviven porque `listUsers` repite `authz.Authorize` (defensa en profundidad).
- Contrato: 60 líneas añadidas, 0 eliminadas; rutas nuevas con `security: bearerAuth`.
- Alcance: solo `services/go-identity/**`, `contracts/openapi/control-plane.yaml` y la bitácora.

## Hallazgos (AMARILLO, no bloquean)
### F-01 · `internal/server/authz_test.go` · Ninguna prueba aísla la capa `Roles(admin)` del middleware
M4 y M6 pasan todas las pruebas; hoy es inocuo por la doble comprobación del handler.

### F-02 · `internal/server/routes.go` (`secure`) · El preflight con origen permitido responde 204 para cualquier ruta
`OPTIONS /zzz` con origen permitido da 204 con `Allow-Methods: GET, POST`. No fuga datos ni salta la autenticación.

## Tareas candidatas (fuera de alcance)
- Corregir el CA-2 de la tarea (el último caso no es observable por red).
- Prueba de aislamiento de la política `Roles` en el middleware cuando exista una ruta que dependa solo de ella (U4-T04).

VEREDICTO: VERDE
AMARILLO|internal/server/authz_test.go|Ninguna prueba aísla la capa Roles(admin) del middleware (mutantes M4/M6 sobreviven por defensa en profundidad)
AMARILLO|internal/server/routes.go (secure)|El preflight con origen permitido responde 204 para cualquier ruta, incluso no registrada
INFORME: revisiones/U4-T03/ronda-1.md
