# U4-T03 — Autorización: deny-by-default, IDOR, roles server-side, CORS y validación de token en cada request

**Unidad:** U4 — Gobernanza & Identidad
**Historias que implementa:** US-M8.3 (NF-SEG-08: control de acceso)
**Depende de:** U4-T02 (`go-identity` con login/logout, sesiones, `IDENTITY_USERS_FILE`, `hash-password`).

---

## Alcance

**Dentro** (una línea, concreta):

> Añadir a `services/go-identity` la capa de autorización: registro de rutas **con política obligatoria** (deny-by-default), middleware que valida el token en **cada** petición, función `Authorize(principal, acción, recurso)` con propiedad por ID de recurso (IDOR), roles resueltos solo en el servidor, CORS por lista blanca y los endpoints `GET /auth/session`, `GET /auth/sessions/{id}` y `GET /auth/users`, con sus esquemas en el OpenAPI.

Detalle:

- **Rutas con política obligatoria.** Las rutas se registran con `Handle(patrón, Política, handler)`, donde `Política` es `Public`, `Authenticated` o `Roles(admin|user…)`. Registrar sin política **panica al arrancar**; una ruta no registrada responde `404`. Una prueba enumera todas las rutas del servidor y falla si alguna no declara política. `Public` solo para `POST /auth/login`.
- **Validación en cada request.** El middleware extrae `Authorization: Bearer <token>` de forma estricta (esquema sin distinguir mayúsculas, un único espacio, 43 caracteres base64url; cualquier otra forma → `401`), busca la sesión por SHA-256 del token y comprueba expiración absoluta, inactividad y revocación **en esa petición**. Sin caché de decisiones: un `logout` hace fallar el token en la siguiente petición. `401` uniforme: `Error{code: "unauthorized"}` con `WWW-Authenticate: Bearer` (sin distinguir ausente, mal formado, expirado o revocado).
- **Roles en el servidor.** El rol sale **solo** de la sesión. Cabeceras (`X-Role`, `X-User`), parámetros de consulta o campos del cuerpo que hablen de rol o de usuario se **ignoran** (hay prueba).
- **`Authorize(principal, action, resource)`** (paquete `authz`): `resource = {Type, ID, Owner}`. Deny-by-default: acción, tipo de recurso o rol desconocidos → `Deny`. Tabla (en código, con prueba por celda):

  | acción | recurso | `user` | `admin` |
  |---|---|---|---|
  | `session:read` | `session` | solo si es el propietario | sí |
  | `users:list` | `users` | no | sí |

- **Endpoints** (todos `Authenticated` salvo `GET /auth/users`, que es `Roles(admin)`):
  - `GET /auth/session` → `200 Session{principal_id, role, session_id, expires_at}` (es lo que `ui-api` llama en U1-T07); cuenta como uso para la inactividad.
  - `GET /auth/sessions/{id}` → `200 Session` del `id` indicado. **IDOR:** un `user` que pide una sesión que no es suya recibe `403 Error{code: "forbidden"}`; si el `id` **no existe**, un `user` también recibe `403` (no hay oráculo de existencia); un `admin` recibe `404` para un `id` inexistente.
  - `GET /auth/users` → `200 [{username, role}]` (**nunca** hashes ni secretos); `user` → `403`.
- **Contrato (único cambio permitido en `contracts/`, aditivo).** En `contracts/openapi/control-plane.yaml`: esquema `Session`, las tres rutas anteriores con `security: bearerAuth` y respuestas `401`/`403`/`404`, y la propiedad **opcional** `session_id` en la respuesta de `/auth/login`. No se cambia ningún `required` existente. `redocly lint` debe seguir pasando.
- **CORS.** Lista blanca `IDENTITY_ALLOWED_ORIGINS` (coma-separada; vacía = ningún origen). Nunca `*`. Origen permitido → `Access-Control-Allow-Origin: <ese origen>`, `Vary: Origin`, y para el preflight `Access-Control-Allow-Methods` y `Access-Control-Allow-Headers: Authorization, Content-Type` limitados. Origen no permitido → sin cabeceras CORS. Nunca `Access-Control-Allow-Credentials` (se usan tokens `Bearer`, no cookies).
- **Cabeceras de seguridad** en todas las respuestas: `X-Content-Type-Options: nosniff`, `Cache-Control: no-store`, `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'`, `Strict-Transport-Security: max-age=63072000; includeSubDomains`.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Cambiar login, hashing, MFA, anti-brute-force o la duración de las sesiones (U4-T02), salvo añadir `session_id` y exponer el almacén de sesiones al middleware.
- Políticas de negocio, gates y auditoría (**U4-T04**); roles distintos de `user` y `admin`; permisos por repositorio.
- Autorización del inbox y de las corridas de `ui-api` (propiedad de notificaciones/corridas): candidata C-47, porque el contrato `Notification` no tiene `owner`.
- Alta/baja/edición de usuarios; revocar sesiones ajenas; refresco de tokens.
- `/healthz`, `/readyz`, `/metrics`, alertas: **U4-T07**.
- Modificar `deploy/**`, workflows o `contracts/**` fuera de lo descrito.

---

## Archivos de contexto

- `unidades-y-tareas.md` (sección U4)
- `aidlc-docs/inception/application-design/unit-task-plans/U4.md`
- `aidlc-docs/inception/application-design/components.md` (C8) y `component-methods.md`
- `aidlc-docs/inception/requirements/requirements.md` (NF-SEG-08: deny-by-default, IDOR, función, CORS, validación de token)
- `contracts/openapi/control-plane.yaml` (`/auth/*`, `Error`, `bearerAuth`)
- `services/go-identity/` (U4-T01 y U4-T02), `tareas/U4-T02-go-identity-autenticacion.md`
- `tareas/U1-T07-integracion-u4-run-confirmed.md` (consume `GET /auth/session` y `Session`)
- `tareas/candidatas.md` (C-47)

---

## Criterios de aceptación

Desde la raíz del worktree. Arranque común (reutiliza el de U4-T02 con un segundo usuario):

```bash
otpnow() { python3 - "$1" <<'PY'
import sys,hmac,hashlib,struct,time,base64
k=base64.b32decode(sys.argv[1]+'='*((8-len(sys.argv[1])%8)%8)); c=int(time.time())//30
d=hmac.new(k,struct.pack('>Q',c),hashlib.sha1).digest(); o=d[-1]&15
print('%06d'%((struct.unpack('>I',d[o:o+4])[0]&0x7fffffff)%1000000))
PY
}
up() { t=$(mktemp -d); (cd services/go-identity && go build -o "$t/gi" ./cmd/go-identity) || return 1
  p1=$(openssl rand -base64 18); p2=$(openssl rand -base64 18); pa=$(openssl rand -base64 18); sec=$(head -c 20 /dev/urandom | base32 | tr -d '=')
  H() { printf '%s' "$1" | "$t/gi" hash-password; }
  jq -n --arg a "$(H "$p1")" --arg b "$(H "$p2")" --arg c "$(H "$pa")" --arg s "$sec" '[{username:"marta",password_hash:$a,role:"user"},{username:"luis",password_hash:$b,role:"user"},{username:"julian",password_hash:$c,role:"admin",mfa_secret:$s}]' > "$t/users.json"
  IDENTITY_USERS_FILE="$t/users.json" IDENTITY_ALLOWED_ORIGINS='https://app.example' LISTEN_ADDR=127.0.0.1:$1 "$t/gi" > "$t/log" 2>&1 & pid=$!; sleep 1
  B=http://127.0.0.1:$1
  tok() { curl -s -X POST $B/auth/login -H 'Content-Type: application/json' -d "$1" | jq -r .token; }
  tm=$(tok "$(jq -n --arg p "$p1" '{username:"marta",password:$p}')"); tl=$(tok "$(jq -n --arg p "$p2" '{username:"luis",password:$p}')")
  ta=$(tok "$(jq -n --arg p "$pa" --arg o "$(otpnow "$sec")" '{username:"julian",password:$p,otp:$o}')"); }
code() { curl -s -o "$t/r.json" -w '%{http_code}' "$@"; }
```

- [ ] **CA-1** — Pruebas unitarias en verde (≥ 15 casos nuevos, `go test -run AuthZ` incluido), con `-race`.
  ```bash
  cd services/go-identity && go test -race -run AuthZ -v ./... | grep -c -E '^\s*--- PASS'; go test -race ./... 2>&1 | grep -c FAIL
  ```
  Esperado: ≥ `15` y `0`. Antes de la tarea: `no tests to run` (rojo inicial).

- [ ] **CA-2** — Sin token, mal formado o inválido → `401`; con token válido → `200`.
  ```bash
  up 18220
  for h in "" "Authorization: Bearer" "Authorization: Basic abc" "Authorization: Bearer x" "Authorization: bearer  $tm"; do echo -n "$(code ${h:+-H "$h"} $B/auth/session) "; done; echo
  code -H "Authorization: Bearer $tm" $B/auth/session; echo " $(jq -c '[.role, (.principal_id|length>0), (.session_id|length>0), .expires_at]' "$t/r.json")"
  curl -s -D - -o /dev/null $B/auth/session | grep -i -c -E '^www-authenticate: Bearer'
  kill $pid; rm -rf "$t"
  ```
  Esperado: `401 401 401 401 401 `, `200 ["user",true,true,"<fecha>"]` y `1`.

- [ ] **CA-3** — IDOR: el token de otro propietario recibe `403`, y el de un id inexistente también (sin oráculo); el admin ve `404`.
  ```bash
  up 18221
  sid=$(curl -s -H "Authorization: Bearer $tm" $B/auth/session | jq -r .session_id)
  echo "propia=$(code -H "Authorization: Bearer $tm" $B/auth/sessions/$sid) ajena=$(code -H "Authorization: Bearer $tl" $B/auth/sessions/$sid) inexistente_user=$(code -H "Authorization: Bearer $tl" $B/auth/sessions/00000000000000000000000000000000) admin_ajena=$(code -H "Authorization: Bearer $ta" $B/auth/sessions/$sid) admin_inexistente=$(code -H "Authorization: Bearer $ta" $B/auth/sessions/00000000000000000000000000000000) sin_token=$(code $B/auth/sessions/$sid)"
  kill $pid; rm -rf "$t"
  ```
  Esperado: `propia=200 ajena=403 inexistente_user=403 admin_ajena=200 admin_inexistente=404 sin_token=401`. Antes de la tarea: `404` en todas (la ruta no existe).

- [ ] **CA-4** — Función por rol: `GET /auth/users` solo para `admin`, y nunca expone hashes ni secretos.
  ```bash
  up 18222
  echo "user=$(code -H "Authorization: Bearer $tm" $B/auth/users) admin=$(code -H "Authorization: Bearer $ta" $B/auth/users) sin=$(code $B/auth/users)"
  curl -s -H "Authorization: Bearer $ta" $B/auth/users | jq -c 'map(keys) | add | unique'
  curl -s -H "Authorization: Bearer $ta" $B/auth/users | grep -c -i -E 'argon2|password|mfa|secret'
  kill $pid; rm -rf "$t"
  ```
  Esperado: `user=403 admin=200 sin=401`, `["role","username"]` y `0`.

- [ ] **CA-5** — El rol sale del servidor: cabeceras, parámetros y cuerpo que digan «admin» se ignoran.
  ```bash
  up 18223
  echo "hdr=$(code -H "Authorization: Bearer $tm" -H 'X-Role: admin' -H 'X-User: julian' $B/auth/users) qs=$(code -H "Authorization: Bearer $tm" "$B/auth/users?role=admin") body=$(code -X POST -H "Authorization: Bearer $tm" -H 'Content-Type: application/json' -d '{"role":"admin"}' $B/auth/users)"
  kill $pid; rm -rf "$t"
  ```
  Esperado: `hdr=403 qs=403 body=403` (o `405` en `body`, pero nunca `2xx`; el codificador documenta cuál).

- [ ] **CA-6** — La validación es en cada request: tras `logout`, el mismo token falla inmediatamente.
  ```bash
  up 18224
  echo -n "antes=$(code -H "Authorization: Bearer $tm" $B/auth/session) "
  curl -s -o /dev/null -X POST -H "Authorization: Bearer $tm" $B/auth/logout
  echo "despues=$(code -H "Authorization: Bearer $tm" $B/auth/session) users=$(code -H "Authorization: Bearer $tm" $B/auth/sessions/x)"
  kill $pid; rm -rf "$t"
  ```
  Esperado: `antes=200 despues=401 users=401`.

- [ ] **CA-7** — Deny-by-default: toda ruta declara política, y las desconocidas dan `404`.
  ```bash
  cd services/go-identity && go test -run 'AuthZ.*(AllRoutes|Unregistered|PanicsWithoutPolicy)' -v ./... | grep -E '^\s*--- (PASS|FAIL)'
  up 18225; echo "desconocida=$(code -H "Authorization: Bearer $tm" $B/admin) metodo=$(code -X DELETE -H "Authorization: Bearer $tm" $B/auth/users)"; kill $pid; rm -rf "$t"
  ```
  Esperado: al menos 3 `PASS` (enumeración de rutas con política, registro sin política que panica, ruta no registrada) y `desconocida=404 metodo=403` o `405`, nunca `2xx` (el codificador documenta cuál).

- [ ] **CA-8** — CORS restringido.
  ```bash
  up 18226
  curl -s -D - -o /dev/null -H "Authorization: Bearer $tm" -H 'Origin: https://app.example' $B/auth/session | grep -i -E '^(access-control-allow-origin|vary):'
  curl -s -D - -o /dev/null -H "Authorization: Bearer $tm" -H 'Origin: https://evil.example' $B/auth/session | grep -i -c '^access-control-'
  curl -s -D - -o /dev/null -X OPTIONS -H 'Origin: https://app.example' -H 'Access-Control-Request-Method: GET' -H 'Access-Control-Request-Headers: authorization' $B/auth/session | grep -i -E '^access-control-allow-(methods|headers|credentials):'
  curl -s -D - -o /dev/null -X OPTIONS -H 'Origin: https://evil.example' -H 'Access-Control-Request-Method: GET' $B/auth/session | grep -i -c -E 'access-control-allow-origin'
  kill $pid; rm -rf "$t"
  ```
  Esperado: `access-control-allow-origin: https://app.example` y `vary: Origin`; `0`; `access-control-allow-methods` y `access-control-allow-headers` sin `credentials` ni `*`; `0`.

- [ ] **CA-9** — Cabeceras de seguridad también en errores.
  ```bash
  up 18227
  curl -s -D - -o /dev/null $B/auth/session | grep -i -c -E '^(x-content-type-options: nosniff|cache-control: no-store|content-security-policy:|strict-transport-security:)'
  curl -s -D - -o /dev/null $B/no-existe | grep -i -c -E '^(x-content-type-options: nosniff|cache-control: no-store|content-security-policy:|strict-transport-security:)'
  kill $pid; rm -rf "$t"
  ```
  Esperado: `4` y `4`.

- [ ] **CA-10** — Contrato: aditivo, válido y con los esquemas nuevos.
  ```bash
  npx --yes @redocly/cli@1.25.0 lint contracts/openapi/control-plane.yaml 2>&1 | tail -n 2
  b=$(git merge-base HEAD origin/main); git diff -U0 $b -- contracts | grep -E '^[+-][^+-]' | grep -c -E '^-'
  docker run --rm -i --security-opt label=disable mikefarah/yq:4.44.3 -N '[.paths | keys | map(select(test("^/auth/")))] | .[0] | join(",")' < contracts/openapi/control-plane.yaml
  docker run --rm -i --security-opt label=disable mikefarah/yq:4.44.3 -N '.components.schemas.Session.required | join(",")' < contracts/openapi/control-plane.yaml
  ```
  Esperado: lint sin errores; `0` líneas eliminadas (o, si U4-T02 ya tocó la misma zona, solo las que ese cambio añadió: el codificador lo explica); `/auth/login,/auth/logout,/auth/session,/auth/sessions/{id},/auth/users`; `principal_id,role,session_id,expires_at`.

- [ ] **CA-11** — La respuesta real valida contra `Session` (lo que usará U1-T07).
  ```bash
  up 18228
  curl -s -H "Authorization: Bearer $tm" $B/auth/session > "$t/s.json"
  docker run --rm -i --security-opt label=disable mikefarah/yq:4.44.3 -N -o=json '.components.schemas.Session' < contracts/openapi/control-plane.yaml > "$t/s.schema.json"
  npx --yes -p ajv-cli@5.0.0 -p ajv-formats@3.0.1 ajv validate --spec=draft2020 -c ajv-formats -s "$t/s.schema.json" -d "$t/s.json"
  kill $pid; rm -rf "$t"
  ```
  Esperado: `…/s.json valid`.

- [ ] **CA-12** — Higiene y alcance.
  ```bash
  (cd services/go-identity && go vet ./... && test -z "$(gofmt -l .)" && echo ok); git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/go-identity/|contracts/openapi/control-plane\.yaml|bitacoras/U4-T03\.md)' | wc -l
  go list -C services/go-identity -deps ./... | grep -c -E 'k8s.io|client-go'
  ```
  Esperado: `ok`, `0`, `0` y `0`.

---

## Plan de pruebas

- `authz.Authorize`: una prueba por celda de la tabla (rol × acción × propietario/no propietario), más acción desconocida, tipo de recurso desconocido, rol vacío, `Owner` vacío (un recurso sin propietario **no** es de nadie: solo `admin`).
- Middleware: formas del encabezado (vacío, `Bearer`, `bearer `, doble espacio, token de 42/44 caracteres, caracteres fuera de base64url, `Basic`), token expirado por reloj falso, revocado, inactivo.
- Rutas: prueba de enumeración (`TestAuthZ_AllRoutesDeclarePolicy`), registro sin política (`TestAuthZ_PanicsWithoutPolicy`), ruta no registrada (`TestAuthZ_UnregisteredRoute404`).
- Negativas de rol: `X-Role`, `?role=`, `{"role": ...}` → sin efecto.
- CORS y cabeceras: tabla de orígenes (permitido, no permitido, vacío, `null`, con puerto distinto, con subdominio).
- Todo bajo `-race`; sin `Sleep`.

**Rojo primero:** el codificador registra en su bitácora el `404` de `GET /auth/session` y de `GET /auth/users` con el binario de U4-T02 antes de tocar nada.

---

## Notas

- Archivos que se **modifican en su sitio**: `contracts/openapi/control-plane.yaml` (aditivo), `cmd/go-identity` y el servidor de U4-T02 (registro de rutas con política y exposición de sesiones). Nada de duplicados con sufijo.
- La fuente de verdad del modelo `Principal`/`Role` es la de `go-identity`; `go-governance` (U4-T04) la copia sin importarla (módulos separados).
- `session_id` es un identificador público distinto del token: conocerlo no da acceso, y la autorización sigue exigiendo un token válido y propiedad (o rol `admin`).
- Ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
