# U4-T02 — `go-identity`: autenticación (login/logout, hashing, MFA admin, sesiones, anti-brute-force)

**Unidad:** U4 — Gobernanza & Identidad
**Historias que implementa:** US-M8.3 (NF-SEG-08 y NF-SEG-12: autenticación sin credenciales hardcodeadas)
**Depende de:** U4-T01 (módulo `services/go-identity`, `Principal`, `Role`, `ParseRole`).

---

## Alcance

**Dentro** (una línea, concreta):

> Convertir `services/go-identity` en un servicio ejecutable (`cmd/go-identity`, `Dockerfile`) que implementa `POST /auth/login` y `POST /auth/logout` con hashing adaptativo (argon2id), MFA TOTP obligatorio para `admin`, sesiones con expiración absoluta e inactiva, anti-brute-force con bloqueo creciente y **cero credenciales en el código o en la imagen**, más el subcomando `go-identity hash-password`.

Comportamiento exacto:

- **Usuarios.** Se cargan de `IDENTITY_USERS_FILE` (JSON: `[{username, password_hash, role, mfa_secret?}]`). El servicio **no arranca** si el archivo falta o está vacío, si un `password_hash` no es un hash argon2id en formato PHC, si hay `username` duplicado (sin distinguir mayúsculas), si `role` no pasa `ParseRole`, o si un `admin` no tiene `mfa_secret` (base32 válido, ≥ 160 bits). No hay usuario por defecto ni «modo desarrollo» con credenciales.
- **Hashing.** argon2id (`golang.org/x/crypto/argon2`), parámetros mínimos `m=19 MiB, t=2, p=1` (se aceptan hashes con parámetros más altos), sal aleatoria de 16 bytes, comparación en tiempo constante. `go-identity hash-password` lee la contraseña de **stdin** (nunca de un argumento), exige 8 o más caracteres y como máximo 128, e imprime el hash PHC. Las contraseñas de más de 1024 bytes recibidas en `/auth/login` se rechazan con `400` antes de hashear.
- **`POST /auth/login`** `{username, password, otp?}`:
  - Correcto → `200 {token, expires_at}`. `token`: 32 bytes de `crypto/rand` en base64url; el servidor guarda **solo su SHA-256** y nunca lo loguea.
  - Credenciales incorrectas **o usuario inexistente** → `401 Error{code: "invalid_credentials"}`, mismo cuerpo y tiempo comparable (se hashea contra un hash señuelo cuando el usuario no existe). Nunca se distingue «usuario no existe».
  - `admin` con contraseña correcta y sin `otp` → `401 Error{code: "mfa_required"}` (solo **después** de verificar la contraseña, para no revelar quién es admin). `otp` incorrecto → `invalid_credentials`. TOTP RFC 6238 (SHA-1, 6 dígitos, 30 s, ventana ±1), con **rechazo de reutilización** de un código ya aceptado.
  - Bloqueado → `429` con `Retry-After`, **incluso con la contraseña correcta** (el bloqueo no es un oráculo).
  - Cuerpo malformado, campos ausentes, `Content-Type` incorrecto o cuerpo > 8 KiB → `400`/`415`/`413`.
- **Anti-brute-force.** Contador de fallos consecutivos por `username` (normalizado a minúsculas, **también para usuarios inexistentes**, con tabla acotada a 10 000 entradas) y por IP de cliente. Tras `IDENTITY_MAX_FAILURES` (por defecto 5) fallos seguidos: bloqueo de 30 s que se duplica en cada reincidencia hasta 15 min. Un acierto fuera de bloqueo reinicia el contador. La IP sale de `RemoteAddr`; `X-Forwarded-For` solo si `IDENTITY_TRUST_PROXY=true`.
- **Sesiones.** Expiración absoluta `IDENTITY_SESSION_TTL` (por defecto 30 m) e inactividad `IDENTITY_IDLE_TTL` (por defecto 15 m). Almacén **en memoria** (reiniciar cierra todas las sesiones, que es seguro; con más de una réplica las sesiones no se comparten: candidata C-49). Máximo de sesiones por usuario configurable (por defecto 5; la más antigua se descarta).
- **`POST /auth/logout`** (con `Authorization: Bearer`) → `204` y la sesión queda revocada de inmediato; token inválido o ausente → `401`. Logout repetido → `401`.
- **Cero credenciales hardcodeadas.** Ninguna contraseña, hash, secreto TOTP ni clave en el código, las pruebas versionadas o la imagen: las pruebas **generan** sus contraseñas, hashes y secretos en tiempo de ejecución.
- **Contrato (único cambio permitido en `contracts/`).** En `contracts/openapi/control-plane.yaml`, en el cuerpo de `/auth/login`, añadir la propiedad **opcional** `otp` (`string`, patrón `^[0-9]{6}$`). Aditivo: no cambia `required` ni ningún otro endpoint. `redocly lint` debe seguir pasando.
- **Dockerfile** multi-etapa, imágenes con **tag fijado** (no `latest`), binario estático, usuario no root, `EXPOSE 8080`. Escucha en `:8080` (`LISTEN_ADDR`).
- **Registro mínimo.** `log/slog` en JSON con `level` y `message`; **jamás** contraseñas, tokens, OTP ni hashes. (El formato completo del contrato de observabilidad y las métricas llegan en U4-T07.)

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Autorización, `GET /auth/session`, IDOR, CORS, middleware de validación de tokens: **U4-T03**.
- Políticas, gates y auditoría: **U4-T04** (`go-governance`).
- Alta/baja de usuarios por API, recuperación de contraseña, SSO, OAuth, cookies de sesión.
- Persistencia de sesiones (base de datos, Redis): C-49.
- `/healthz`, `/readyz`, `/metrics` y alertas: **U4-T07**.
- Creación del Secret de usuarios en el clúster: es de operación (U5-T14 cubre el patrón de Secrets). Aquí solo se lee un archivo.
- Modificar `deploy/**`, workflows o cualquier parte de `contracts/` que no sea la propiedad `otp`.

---

## Archivos de contexto

- `unidades-y-tareas.md` (sección U4)
- `aidlc-docs/inception/application-design/unit-task-plans/U4.md`
- `aidlc-docs/inception/application-design/components.md` (C8) y `component-methods.md` (`login`)
- `aidlc-docs/inception/requirements/requirements.md` (NF-SEG-08, NF-SEG-12, política de contraseñas 8+)
- `contracts/openapi/control-plane.yaml` (`/auth/login`, `/auth/logout`, `Error`)
- `services/go-identity/` (U4-T01)
- `deploy/flux/base/control-plane.yaml` (puerto 8080), `docs/operaciones/secrets.md` (cómo llegan los Secrets)
- `.github/workflows/contracts.yml` (el lint de OpenAPI que debe seguir en verde)

---

## Criterios de aceptación

Desde la raíz del worktree. Arranque común:

```bash
up() { t=$(mktemp -d); (cd services/go-identity && go build -o "$t/go-identity" ./cmd/go-identity) || return 1
  pw=$(openssl rand -base64 18); admpw=$(openssl rand -base64 18); sec=$(head -c 20 /dev/urandom | base32 | tr -d '=')
  h=$(printf '%s' "$pw" | "$t/go-identity" hash-password); ah=$(printf '%s' "$admpw" | "$t/go-identity" hash-password)
  jq -n --arg h "$h" --arg ah "$ah" --arg s "$sec" '[{username:"marta",password_hash:$h,role:"user"},{username:"julian",password_hash:$ah,role:"admin",mfa_secret:$s}]' > "$t/users.json"
  IDENTITY_USERS_FILE="$t/users.json" LISTEN_ADDR=127.0.0.1:$1 "$t/go-identity" > "$t/log" 2>&1 & pid=$!; sleep 1; }
login() { curl -s -o "$t/r.json" -w '%{http_code}' -X POST http://127.0.0.1:$1/auth/login -H 'Content-Type: application/json' -d "$2"; }
```

- [ ] **CA-1** — Pruebas unitarias en verde, con al menos 20 casos y detector de carreras.
  ```bash
  cd services/go-identity && go test -race -v ./... | grep -c -E '^\s*--- PASS'; go test -race ./... 2>&1 | grep -c FAIL
  ```
  Esperado: ≥ `20` y `0`. Cubren: hash/verify, parámetros bajos rechazados, PHC inválido, usuarios duplicados, admin sin MFA, login ok, usuario inexistente igual que contraseña mala, `mfa_required` solo con contraseña buena, TOTP con vectores RFC 6238, reutilización de OTP, ventana ±1, bloqueo y backoff creciente, bloqueo con contraseña correcta, reinicio del contador, expiración absoluta, expiración por inactividad, logout, límite de sesiones, token guardado como hash. Antes de la tarea: no hay `cmd/` (rojo inicial).

- [ ] **CA-2** — Login correcto y logout, medidos con el binario real.
  ```bash
  up 18210
  login 18210 "$(jq -n --arg p "$pw" '{username:"marta",password:$p}')"; echo; jq -r '.token | length' "$t/r.json"; jq -r '.expires_at' "$t/r.json"
  tok=$(jq -r .token "$t/r.json")
  curl -s -o /dev/null -w '%{http_code} ' -X POST -H "Authorization: Bearer $tok" http://127.0.0.1:18210/auth/logout
  curl -s -o /dev/null -w '%{http_code}\n' -X POST -H "Authorization: Bearer $tok" http://127.0.0.1:18210/auth/logout
  kill $pid; rm -rf "$t"
  ```
  Esperado: `200`, `43` (32 bytes en base64url sin relleno), una fecha RFC 3339 futura, y `204 401`.

- [ ] **CA-3** — Usuario inexistente y contraseña mala son indistinguibles.
  ```bash
  up 18211
  login 18211 '{"username":"nadie","password":"x1x1x1x1x1"}'; echo " $(jq -S -c . "$t/r.json")"
  login 18211 "$(jq -n '{username:"marta",password:"mala-contrasena-1"}')"; echo " $(jq -S -c . "$t/r.json")"
  kill $pid; rm -rf "$t"
  ```
  Esperado: dos líneas idénticas: `401 {"code":"invalid_credentials","message":"…"}` con el mismo `message`.

- [ ] **CA-4** — Anti-brute-force: con 5 fallos, el sexto intento (incluso con la contraseña correcta) recibe `429` con `Retry-After`.
  ```bash
  up 18212
  for i in 1 2 3 4 5; do login 18212 '{"username":"marta","password":"incorrecta-123"}'; echo -n ' '; done; echo
  login 18212 "$(jq -n --arg p "$pw" '{username:"marta",password:$p}')"; echo
  curl -s -D - -o /dev/null -X POST http://127.0.0.1:18212/auth/login -H 'Content-Type: application/json' -d "$(jq -n --arg p "$pw" '{username:"marta",password:$p}')" | grep -i '^retry-after:'
  kill $pid; rm -rf "$t"
  ```
  Esperado: `401 401 401 401 401 `, `429` y una cabecera `Retry-After: <n>` con `n` entre 1 y 30.

- [ ] **CA-5** — MFA del admin con TOTP verificable de forma independiente.
  ```bash
  up 18213
  login 18213 "$(jq -n --arg p "$admpw" '{username:"julian",password:$p}')"; echo " $(jq -r .code "$t/r.json")"
  otp=$(cd services/go-identity && go run ./cmd/go-identity totp-code "$sec" 2>/dev/null || python3 - "$sec" <<'PY'
import sys,hmac,hashlib,struct,time,base64
k=base64.b32decode(sys.argv[1]+'='*((8-len(sys.argv[1])%8)%8)); c=int(time.time())//30
d=hmac.new(k,struct.pack('>Q',c),hashlib.sha1).digest(); o=d[-1]&15
print('%06d'%((struct.unpack('>I',d[o:o+4])[0]&0x7fffffff)%1000000))
PY
)
  login 18213 "$(jq -n --arg p "$admpw" --arg o "$otp" '{username:"julian",password:$p,otp:$o}')"; echo
  login 18213 "$(jq -n --arg p "$admpw" --arg o "$otp" '{username:"julian",password:$p,otp:$o}')"; echo " (reuso del mismo OTP)"
  kill $pid; rm -rf "$t"
  ```
  Esperado: `401 mfa_required`, `200` (el OTP calculado por el script de Python, independiente del código del servicio) y `401 (reuso del mismo OTP)`. (`totp-code` es opcional: si el codificador no lo implementa, se usa el script.)

- [ ] **CA-6** — El servicio se niega a arrancar con configuración insegura.
  ```bash
  t=$(mktemp -d); (cd services/go-identity && go build -o "$t/gi" ./cmd/go-identity)
  h=$(printf 'una-clave-valida-1' | "$t/gi" hash-password)
  env -u IDENTITY_USERS_FILE "$t/gi" >/dev/null 2>&1; echo "sin archivo rc=$?"
  echo '[]' > "$t/v.json"; IDENTITY_USERS_FILE="$t/v.json" "$t/gi" >/dev/null 2>&1; echo "vacio rc=$?"
  jq -n '[{username:"a",password_hash:"plano",role:"user"}]' > "$t/p.json"; IDENTITY_USERS_FILE="$t/p.json" "$t/gi" >/dev/null 2>&1; echo "hash no argon2id rc=$?"
  jq -n --arg h "$h" '[{username:"a",password_hash:$h,role:"admin"}]' > "$t/a.json"; IDENTITY_USERS_FILE="$t/a.json" "$t/gi" >/dev/null 2>&1; echo "admin sin mfa rc=$?"
  jq -n --arg h "$h" '[{username:"a",password_hash:$h,role:"user"},{username:"A",password_hash:$h,role:"user"}]' > "$t/d.json"; IDENTITY_USERS_FILE="$t/d.json" "$t/gi" >/dev/null 2>&1; echo "duplicado rc=$?"
  printf 'corta' | "$t/gi" hash-password >/dev/null 2>&1; echo "contrasena corta rc=$?"; rm -rf "$t"
  ```
  Esperado: seis `rc=` distintos de `0`.

- [ ] **CA-7** — Cero credenciales hardcodeadas: ni en el código, ni en las pruebas, ni en la imagen, ni en los logs.
  ```bash
  docker run --rm -v "$PWD":/repo zricethezav/gitleaks:v8.18.4 detect --no-git --source /repo/services/go-identity --redact -v 2>&1 | grep -c -E 'Finding|leaks found: [1-9]'
  grep -r -n -i -E '(password|passwd|secret|token)[a-z_]*\s*(:=|=|:)\s*"[^"]{6,}"' services/go-identity --include='*.go' | grep -v -E '_test\.go|"(password|username|otp|token|password_hash|mfa_secret)"' | wc -l
  up 18214; for i in 1 2; do login 18214 "$(jq -n --arg p "$pw" '{username:"marta",password:$p}')" >/dev/null; login 18214 '{"username":"marta","password":"incorrecta-123"}' >/dev/null; done
  kill $pid; grep -c -F -e "$pw" -e "$sec" -e "$(jq -r '.[0].password_hash' "$t/users.json")" "$t/log"; rm -rf "$t"
  ```
  Esperado: `0`, `0` y `0`. (Si `gitleaks` marca un valor de prueba, se genera en tiempo de ejecución; no se añade `.gitleaksignore`.)

- [ ] **CA-8** — Sesiones con expiración (absoluta e inactiva) comprobadas con reloj inyectado.
  ```bash
  cd services/go-identity && go test -run 'Session(Expir|Idle|Absolute|Revok|Limit)' -v ./... | grep -E '^\s*--- (PASS|FAIL)'
  ```
  Esperado: al menos 4 `PASS` (expira por TTL absoluto, expira por inactividad, un uso reciente renueva solo la inactividad, revocada por logout, el límite de sesiones descarta la más antigua) y ningún `FAIL`. Las pruebas no duermen: usan un reloj falso.

- [ ] **CA-9** — El contrato sigue válido tras añadir `otp` y el cambio es solo aditivo.
  ```bash
  npx --yes @redocly/cli@1.25.0 lint contracts/openapi/control-plane.yaml 2>&1 | tail -n 2
  b=$(git merge-base HEAD origin/main); git diff -U0 $b -- contracts | grep -E '^[+-][^+-]' | grep -c -E '^-'
  docker run --rm -i --security-opt label=disable mikefarah/yq:4.44.3 -N '.paths."/auth/login".post.requestBody.content."application/json".schema | [(.required | join(",")), (.properties | keys | sort | join(","))] | join(" | ")' < contracts/openapi/control-plane.yaml
  ```
  Esperado: el lint termina sin errores; `0` líneas eliminadas del diff de `contracts/`; y `username,password | otp,password,username`.

- [ ] **CA-10** — Imagen conforme a las reglas de U5 y detectada por la CI.
  ```bash
  docker build -q -t aqs-go-identity:ci services/go-identity >/dev/null && docker inspect aqs-go-identity:ci --format '{{.Config.User}}'
  grep -E '^FROM' services/go-identity/Dockerfile | grep -c -i -E ':latest|^FROM [a-z0-9./-]+$'
  bash scripts/ci/list-services.sh | grep -c 'services/go-identity'; bash scripts/ci/detect-lang.sh services/go-identity
  go list -C services/go-identity -deps ./... | grep -c -E 'k8s.io|client-go'
  ```
  Esperado: un usuario no vacío distinto de `root`/`0`, `0`, `1`, `go` y `0`.

- [ ] **CA-11** — Higiene y alcance.
  ```bash
  (cd services/go-identity && go vet ./... && test -z "$(gofmt -l .)" && echo ok); git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/go-identity/|contracts/openapi/control-plane\.yaml|bitacoras/U4-T02\.md)' | wc -l
  ```
  Esperado: `ok`, `0` y `0`.

---

## Plan de pruebas

- Unitarias con reloj inyectado y generador aleatorio inyectable (fijo en las pruebas): TOTP con los vectores de RFC 6238; hash con parámetros distintos; cada regla de arranque inseguro.
- Negativas: usuario inexistente vs contraseña mala (mismo cuerpo, mismo código, tiempos del mismo orden de magnitud con un umbral holgado, sin pruebas de tiempo frágiles); contraseña de 2000 bytes → `400` sin hashear; `otp` con letras; JSON con campos extra; `Content-Type` incorrecto.
- Concurrencia (`-race`): 100 logins simultáneos fallidos sobre el mismo usuario no pasan del límite sin bloquear; logout concurrente con uso del token.
- El token no aparece nunca en el log ni en el cuerpo de error; en memoria solo se guarda su SHA-256 (prueba que inspecciona el almacén).

**Rojo primero:** el codificador registra en su bitácora el fallo de `go build ./cmd/go-identity` y del CA-9 (el `otp` no existe en el contrato) antes de crear nada.

---

## Notas

- Archivos que se **modifican en su sitio**: `contracts/openapi/control-plane.yaml` (una propiedad añadida) y los paquetes de U4-T01 si hace falta exportar tipos. Nada de duplicados con sufijo.
- U1-T07 y U4-T03 dependen de los nombres de `IDENTITY_USERS_FILE`, `LISTEN_ADDR`, `hash-password` y del campo `otp`: **no se renombran** sin avisar.
- Los TTL y límites de arriba son valores por defecto razonables, no requisitos del PRD; el codificador los deja configurables y documentados, y el humano los confirma (como en C-08).
- Ningún comando contra un clúster ni la nube. Los servidores se levantan en `127.0.0.1`.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
