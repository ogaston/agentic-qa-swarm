# U4-T04 — `go-governance`: políticas admin, `authorizeTransition` fail-closed y auditoría append-only

**Unidad:** U4 — Gobernanza & Identidad
**Historias que implementa:** US-M8.3 (confirm-required y cero staging/prod), con la base de US-M7.1 (`reset_verified` previo a cada corrida)
**Depende de:** U4-T01 (modelo `GateInput`/`Decision` y matriz `authorize_matrix.json`), U4-T03 (`GET /auth/session` y `Session` para autenticar a los administradores).

---

## Alcance

**Dentro** (una línea, concreta):

> Convertir `services/go-governance` en un servicio ejecutable (`cmd/go-governance`, `Dockerfile`) que guarda políticas versionadas escribibles solo por `admin` (`PUT /policies/{name}`), evalúa los gates de cada transición de una corrida (`POST /gates/authorize`, fail-closed) y registra **toda** decisión y todo cambio de política en un log de auditoría append-only con cadena de hashes (`GET /audit`), más el subcomando `go-governance verify-audit`.

Detalle:

- **Autenticación de personas.** Puerto `TokenVerifier` con un `HTTPTokenVerifier` que llama a `GET {IDENTITY_URL}/auth/session` en **cada** petición (sin caché; mismo comportamiento que `ui-api` en U1-T07: `401` si el token es inválido, `503` `identity_unavailable` si identidad falla; nunca se permite). `GOVERNANCE_AUTH=identity` (por defecto exigido). Un `FakeTokenVerifier` para pruebas solo arranca con `GOVERNANCE_AUTH=fake` **y** `GOVERNANCE_ALLOW_FAKE_AUTH=true` **y** `GOVERNANCE_ENV` distinto de `prod`.
- **Políticas** (almacén `PolicyStore` JSONL solo-agregar en `GOVERNANCE_DATA_DIR`, una versión nueva por cada `PUT`; `PUT` idéntico a la última versión no crea otra). Cada política tiene un esquema; valor inválido → `422` sin guardar:
  - `events`: `{enabled_events: [commit|pull_request|tag, …]}` (sin repetidos; vacío permitido = nada habilitado).
  - `confirm_required`: `{required: true}`. **En esta versión solo se acepta `true`** (principio #4: auto-run apagado; `false` → `422`). El gate exige confirmación siempre, esta política no puede relajarlo.
  - `warm_quotas`: `{max_runs_per_day: 1..1000, rebuild_cadence_hours: 1..720, idle_scale_down_minutes: 1..1440, housekeeping_grace_hours: 1..168}`.
  - `workflows`: `{workflows: [{name, complexity: low|medium|high, max_runs_per_day: 1..1000, timeout_seconds: 1..3600, requires_approval: bool}]}`; `name` único, `^[a-z][a-z0-9-]{0,62}$`.
  - Nombre de política desconocido → `404`. Cuerpo > 64 KiB → `413`.
- **`PUT /policies/{name}`** solo `admin` (`401` sin token, `403` para `user`, `422` valor inválido, `200 PolicyVersion{name, version}`). **`GET /policies/{name}`** para cualquier autenticado (`user` solo lectura): `200 Policy`, `404` si nunca se escribió (sin valores por defecto implícitos).
- **`POST /gates/authorize`** (consumido por `go-run-controller` en U2). Cuerpo `GateRequest`: `run_id`, `from`, `to`, `target_namespace`, `workflow` (opcional), y hechos tri-estado (`"true"|"false"|"unknown"`) `confirmed`, `reset_verified`, `ensayo_passed`, `workflow_allowed`, `approval_recorded`. Respuesta `GateDecision{allow, reason, audit_ref}`. Autenticación **servicio a servicio** por `Authorization: Bearer <GOVERNANCE_SERVICE_TOKEN>` (el servicio no arranca si falta o tiene menos de 32 caracteres; comparación en tiempo constante; distinto de los tokens de personas). Decisión de esta tarea y provisional: la candidata C-50 la reemplaza por una identidad de servicio real.
- **Reglas del gate** (todas deben cumplirse; cualquier hecho `unknown` cuenta como `false`):

  Transiciones legales: `confirmed→warm_ready→deploying→inferring→rehearsing→running→resetting→reporting→done`; desde `deploying`, `inferring`, `rehearsing` y `running` también `→resetting` (fallo o fin: el reset siempre se intenta); desde cualquier estado no terminal, `→failed`. Cualquier otra transición, un estado desconocido o un `run_id` vacío → `Deny`.

  | destino | hechos exigidos |
  |---|---|
  | `warm_ready` | `confirmed`, `reset_verified`, namespace de prueba |
  | `deploying` | `confirmed`, `reset_verified`, namespace de prueba, `workflow_allowed` |
  | `inferring` | `confirmed`, namespace de prueba |
  | `rehearsing` | `confirmed`, namespace de prueba, `workflow_allowed` |
  | `running` | `confirmed`, `ensayo_passed`, namespace de prueba, `workflow_allowed` |
  | `resetting`, `reporting` | namespace de prueba |
  | `done`, `failed` | (ninguno adicional; la transición debe ser legal) |

  «Namespace de prueba» = `target_namespace == GOVERNANCE_TEST_NAMESPACE` (por defecto `aqs-test`, comparación exacta; vacío, `staging`, `prod`, `default` o con espacios → `Deny`).
- **Workflow permitido por política.** Si `workflow` no está vacío: debe existir en la política `workflows` (si no existe la política o el nombre → `Deny`); para `running`, el número de decisiones `allow` hacia `running` de ese workflow en el día UTC actual (leído del log de auditoría) no puede alcanzar `max_runs_per_day`; si `requires_approval`, `approval_recorded` debe ser `true`. Si `workflow` está vacío, solo cuenta `workflow_allowed` reportado (compatibilidad con la matriz de U4-T01).
- **Fail-closed.** Cualquier error interno (política ilegible, almacén caído, JSON roto, **fallo al escribir la auditoría**) → `Deny` con `reason` explícito y `503`/`200 allow=false` según el caso (el codificador documenta: la regla es que **nunca** devuelve `allow=true` si no pudo auditar la decisión).
- **Auditoría append-only.** `AuditLog` JSONL abierto con `O_APPEND`, `fsync` por entrada; cada entrada: `at`, `actor` (principal o servicio), `action` (`gate.allow`, `gate.deny`, `gate.error`, `policy.set:<nombre>@v<N>`), `run_id`, `detail` (hechos, razón, política y versión usadas), `prev_hash` y `hash = sha256(prev_hash || entrada canónica)`. Se audita **cada** decisión (allow y deny) y cada `PUT` aceptado o rechazado (`policy.rejected`). No existe ninguna ruta, función ni subcomando que borre o reescriba entradas. `DELETE`/`PUT`/`POST /audit` → `405`. `GET /audit?run=&limit=` (autenticado, `user` y `admin` leen; `limit` 1..500, por defecto 100) devuelve `[]AuditEntry` con los campos del contrato (`at`, `actor`, `action`, `run_id`).
- **`go-governance verify-audit <archivo>`**: recorre la cadena y sale con `0` si es íntegra, `1` indicando la línea si una entrada fue alterada, borrada o reordenada.
- **Contrato (único cambio permitido en `contracts/`, aditivo).** En `contracts/openapi/control-plane.yaml`: `GET /policies/{name}`, `POST /gates/authorize` (con esquema de seguridad `serviceToken`, tipo `http`/`bearer`), y los esquemas `GateRequest` y `GateDecision`. No se cambia ningún `required` existente. `redocly lint` debe seguir pasando.
- **Dockerfile** multi-etapa, tags fijados (no `latest`), binario estático, usuario no root, `EXPOSE 8080`.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Quién llama al gate y qué estados reporta: es `go-run-controller` en **U2-T02**. Aquí solo el servidor y su matriz.
- Aplicar las políticas sobre Kubernetes (cuotas de namespace, `ResourceQuota`): U2-T07 y U5.
- RBAC/NetworkPolicy: **U4-T05**. Alertas y dashboard: **U4-T07**. Pruebas PBT: **U4-T06**.
- `confirm_required: false`, aprobaciones con flujo de UI, SSO, roles distintos de `user`/`admin`.
- Borrado o compactación de auditoría, rotación de archivos (la retención ≥ 90 días es de U4-T07 y de operación).
- Verificar en `go-governance` que la confirmación **existe** en el almacén de `ui-api`: el hecho `confirmed` lo reporta el llamante; la decisión queda auditada con ese hecho. (Es un límite de confianza documentado, no un defecto.)
- Modificar `deploy/**`, workflows o `contracts/**` fuera de lo descrito.

---

## Archivos de contexto

- `unidades-y-tareas.md` (sección U4)
- `aidlc-docs/inception/application-design/unit-task-plans/U4.md`
- `aidlc-docs/inception/application-design/components.md` (C7, C9) y `component-methods.md` (`setPolicy`, `authorizeTransition`, `appendAudit`)
- `aidlc-docs/inception/user-stories/stories.md` (US-M8.3, US-M7.1)
- `aidlc-docs/inception/requirements/requirements.md` (confirm-required, test-ns-only, auditoría)
- `contracts/openapi/control-plane.yaml` (`/policies/{name}`, `/audit`, `AuditEntry`, `Policy`, `PolicyVersion`, enum `Run.state`)
- `services/go-governance/` (U4-T01: modelo y `testdata/authorize_matrix.json`), `services/go-identity/` (para el contrato de `Session`)
- `tareas/U4-T03-autorizacion.md`, `tareas/candidatas.md`

---

## Criterios de aceptación

Desde la raíz del worktree. Arranque común (autenticación `fake` para la mayoría; CA-9 usa `go-identity` real):

```bash
upg() { t=$(mktemp -d); (cd services/go-governance && go build -o "$t/gg" ./cmd/go-governance) || return 1
  GOVERNANCE_AUTH=fake GOVERNANCE_ALLOW_FAKE_AUTH=true GOVERNANCE_FAKE_TOKENS='tok-admin=a1:admin,tok-user=u1:user' GOVERNANCE_SERVICE_TOKEN="$(openssl rand -hex 32)" GOVERNANCE_DATA_DIR="$t/data" LISTEN_ADDR=127.0.0.1:$1 "$t/gg" > "$t/log" 2>&1 & pid=$!; sleep 1
  B=http://127.0.0.1:$1; svc=$(tr '\0' '\n' < /proc/$pid/environ | sed -n 's/^GOVERNANCE_SERVICE_TOKEN=//p'); }
gate() { curl -s -X POST $B/gates/authorize -H "Authorization: Bearer $svc" -H 'Content-Type: application/json' -d "$1"; }
```

- [ ] **CA-1** — Pruebas unitarias en verde (≥ 30 casos, incluida toda la matriz de U4-T01), con `-race`.
  ```bash
  cd services/go-governance && go test -race -v ./... | grep -c -E '^\s*--- PASS'; go test -race ./... 2>&1 | grep -c FAIL; go test -race -run 'Matrix' -v ./... | grep -c -E '^\s*--- PASS'
  ```
  Esperado: ≥ `30`, `0` y un número igual al `jq length` de la matriz (cada fila de U4-T01 corre contra el evaluador **real**). Antes de la tarea: no hay `cmd/` ni evaluador real (rojo inicial).

- [ ] **CA-2** — Tabla de gates: sin confirmar, sin `reset_verified` y sin ensayo se deniega; con todo, se permite. Medido contra el binario real.
  ```bash
  upg 18230
  R='{"run_id":"r1","target_namespace":"aqs-test","workflow":"","workflow_allowed":"true"'
  echo "ok_warm=$(gate "$R,\"from\":\"confirmed\",\"to\":\"warm_ready\",\"confirmed\":\"true\",\"reset_verified\":\"true\"}" | jq -c '[.allow]')"
  echo "sin_confirm=$(gate "$R,\"from\":\"confirmed\",\"to\":\"warm_ready\",\"confirmed\":\"false\",\"reset_verified\":\"true\"}" | jq -c '[.allow,.reason]')"
  echo "sin_reset=$(gate "$R,\"from\":\"confirmed\",\"to\":\"warm_ready\",\"confirmed\":\"true\",\"reset_verified\":\"false\"}" | jq -c '[.allow,.reason]')"
  echo "sin_ensayo=$(gate "$R,\"from\":\"rehearsing\",\"to\":\"running\",\"confirmed\":\"true\",\"ensayo_passed\":\"false\"}" | jq -c '[.allow,.reason]')"
  echo "con_ensayo=$(gate "$R,\"from\":\"rehearsing\",\"to\":\"running\",\"confirmed\":\"true\",\"ensayo_passed\":\"true\"}" | jq -c '[.allow]')"
  echo "unknown=$(gate "$R,\"from\":\"confirmed\",\"to\":\"warm_ready\",\"confirmed\":\"unknown\",\"reset_verified\":\"true\"}" | jq -c '[.allow]')"
  kill $pid; rm -rf "$t"
  ```
  Esperado: `ok_warm=[true]`, `sin_confirm=[false,"…confirm…"]`, `sin_reset=[false,"…reset_verified…"]`, `sin_ensayo=[false,"…ensayo…"]`, `con_ensayo=[true]` y `unknown=[false]`.

- [ ] **CA-3** — Namespace de prueba y transiciones ilegales: `staging`, `prod`, vacío y `done→running` se deniegan.
  ```bash
  upg 18231
  for ns in staging prod "" default "aqs-test " AQS-TEST; do echo -n "$(gate "{\"run_id\":\"r1\",\"from\":\"inferring\",\"to\":\"rehearsing\",\"target_namespace\":\"$ns\",\"confirmed\":\"true\",\"workflow_allowed\":\"true\"}" | jq -r .allow) "; done; echo
  gate '{"run_id":"r1","from":"done","to":"running","target_namespace":"aqs-test","confirmed":"true","ensayo_passed":"true","workflow_allowed":"true"}' | jq -c '[.allow]'
  gate '{"run_id":"","from":"confirmed","to":"warm_ready","target_namespace":"aqs-test","confirmed":"true","reset_verified":"true"}' | jq -c '[.allow]'
  gate '{"run_id":"r1","from":"nope","to":"warm_ready","target_namespace":"aqs-test"}' | jq -c '[.allow]'
  kill $pid; rm -rf "$t"
  ```
  Esperado: seis `false`, `[false]`, `[false]` y `[false]`.

- [ ] **CA-4** — Autenticación del gate: sin token de servicio o con el token de una persona, `401`; JSON roto, `400`; el gate nunca devuelve `allow=true` en un error.
  ```bash
  upg 18232
  echo "sin=$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/gates/authorize -d '{}') persona=$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/gates/authorize -H 'Authorization: Bearer tok-admin' -d '{}') roto=$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/gates/authorize -H "Authorization: Bearer $svc" -d '{rota')"
  curl -s -X POST $B/gates/authorize -H "Authorization: Bearer $svc" -d '{rota' | jq -r '.allow // "sin-allow"'
  kill $pid; rm -rf "$t"
  ```
  Esperado: `sin=401 persona=401 roto=400` y `sin-allow`.

- [ ] **CA-5** — Políticas: solo `admin` escribe; `user` lee; valores inválidos → `422`; versionado.
  ```bash
  upg 18233
  p() { curl -s -o "$t/r.json" -w '%{http_code}' -X PUT "$B/policies/$1" -H "Authorization: Bearer $2" -H 'Content-Type: application/json' -d "$3"; }
  echo "user=$(p events tok-user '{"value":{"enabled_events":["tag"]}}') sin=$(p events x '{"value":{}}') admin=$(p events tok-admin '{"value":{"enabled_events":["tag","commit"]}}') v=$(jq -r .version "$t/r.json")"
  echo "igual=$(p events tok-admin '{"value":{"enabled_events":["tag","commit"]}}') v=$(jq -r .version "$t/r.json") cambia=$(p events tok-admin '{"value":{"enabled_events":["tag"]}}') v=$(jq -r .version "$t/r.json")"
  echo "conf_false=$(p confirm_required tok-admin '{"value":{"required":false}}') conf_true=$(p confirm_required tok-admin '{"value":{"required":true}}') evento_malo=$(p events tok-admin '{"value":{"enabled_events":["push"]}}') cuota_malo=$(p warm_quotas tok-admin '{"value":{"max_runs_per_day":0,"rebuild_cadence_hours":24,"idle_scale_down_minutes":10,"housekeeping_grace_hours":24}}') desconocida=$(p inventada tok-admin '{"value":{}}')"
  echo "lee_user=$(curl -s -o "$t/g.json" -w '%{http_code}' -H 'Authorization: Bearer tok-user' $B/policies/events) $(jq -c .value "$t/g.json") sin_politica=$(curl -s -o /dev/null -w '%{http_code}' -H 'Authorization: Bearer tok-user' $B/policies/workflows)"
  kill $pid; rm -rf "$t"
  ```
  Esperado: `user=403 sin=401 admin=200 v=1`; `igual=200 v=1 cambia=200 v=2`; `conf_false=422 conf_true=200 evento_malo=422 cuota_malo=422 desconocida=404`; `lee_user=200 {"enabled_events":["tag"]} sin_politica=404`.

- [ ] **CA-6** — Workflows: política, cuota diaria y aprobación se aplican al gate.
  ```bash
  upg 18234
  curl -s -o /dev/null -X PUT $B/policies/workflows -H 'Authorization: Bearer tok-admin' -H 'Content-Type: application/json' -d '{"value":{"workflows":[{"name":"checkout","complexity":"low","max_runs_per_day":2,"timeout_seconds":600,"requires_approval":false},{"name":"refund","complexity":"high","max_runs_per_day":5,"timeout_seconds":900,"requires_approval":true}]}}'
  g() { gate "{\"run_id\":\"r$2\",\"from\":\"rehearsing\",\"to\":\"running\",\"target_namespace\":\"aqs-test\",\"workflow\":\"$1\",\"confirmed\":\"true\",\"ensayo_passed\":\"true\",\"workflow_allowed\":\"true\",\"approval_recorded\":\"$3\"}" | jq -r .allow; }
  echo "checkout: $(g checkout 1 false) $(g checkout 2 false) $(g checkout 3 false)  refund_sin_aprob: $(g refund 4 false)  refund_con_aprob: $(g refund 5 true)  inexistente: $(g nada 6 true)"
  kill $pid; rm -rf "$t"
  ```
  Esperado: `checkout: true true false  refund_sin_aprob: false  refund_con_aprob: true  inexistente: false`.

- [ ] **CA-7** — Auditoría append-only: cada decisión y cada `PUT` deja entrada, no hay forma de borrar, y la cadena detecta la manipulación.
  ```bash
  upg 18235
  gate '{"run_id":"ra","from":"confirmed","to":"warm_ready","target_namespace":"aqs-test","confirmed":"false","reset_verified":"true"}' >/dev/null
  gate '{"run_id":"ra","from":"confirmed","to":"warm_ready","target_namespace":"aqs-test","confirmed":"true","reset_verified":"true"}' >/dev/null
  curl -s -o /dev/null -X PUT $B/policies/events -H 'Authorization: Bearer tok-admin' -H 'Content-Type: application/json' -d '{"value":{"enabled_events":["tag"]}}'
  curl -s -H 'Authorization: Bearer tok-user' "$B/audit?run=ra" | jq -c 'map(.action)'
  curl -s -H 'Authorization: Bearer tok-user' "$B/audit" | jq -c '[length >= 3, (.[0] | keys | join(","))]'
  echo "delete=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE -H 'Authorization: Bearer tok-admin' $B/audit) put=$(curl -s -o /dev/null -w '%{http_code}' -X PUT -H 'Authorization: Bearer tok-admin' $B/audit) sin_token=$(curl -s -o /dev/null -w '%{http_code}' $B/audit)"
  f=$(find "$t/data" -name '*audit*' -type f | head -n1); "$t/gg" verify-audit "$f"; echo "integra rc=$?"
  sed -i '2s/gate\.deny/gate.allow/;2s/"allow":false/"allow":true/' "$f"; "$t/gg" verify-audit "$f"; echo "alterada rc=$?"
  kill $pid; rm -rf "$t"
  ```
  Esperado: `["gate.deny","gate.allow"]`; `[true,"action,actor,at,run_id"]` (el `run_id` puede faltar en la entrada de política: el codificador ajusta el `keys` esperado y lo documenta); `delete=405 put=405 sin_token=401`; `integra rc=0`; y `alterada rc=1` con el número de línea 2 en la salida. Si el `sed` no cambia nada por el formato elegido, el codificador adapta la mutación y la registra.

- [ ] **CA-8** — Fail-closed ante auditoría imposible y ante almacén roto.
  ```bash
  upg 18236
  f=$(find "$t/data" -name '*audit*' -type f | head -n1); chmod 444 "$f" 2>/dev/null; chattr +i "$f" 2>/dev/null
  gate '{"run_id":"rf","from":"confirmed","to":"warm_ready","target_namespace":"aqs-test","confirmed":"true","reset_verified":"true"}' | jq -c '[.allow // false]'
  chattr -i "$f" 2>/dev/null; kill $pid; rm -rf "$t"
  cd services/go-governance && go test -run 'FailClosed' -v ./... | grep -E '^\s*--- (PASS|FAIL)'
  ```
  Esperado: `[false]` (o, si el entorno corre como root y `chmod` no impide escribir, la prueba Go lo cubre con un `AuditLog` inyectado que falla) y al menos 4 `PASS` en `FailClosed` (auditoría que falla, política ilegible, almacén que devuelve error, JSON de política corrupto), todas con `allow=false`.

- [ ] **CA-9** — Con `go-identity` real: sin token `401`, `user` `403`, `admin` `200` en `PUT /policies/events`; identidad caída → `503`.
  ```bash
  t=$(mktemp -d); (cd services/go-identity && go build -o "$t/gi" ./cmd/go-identity); (cd services/go-governance && go build -o "$t/gg" ./cmd/go-governance)
  pu=$(openssl rand -base64 18); pa=$(openssl rand -base64 18); sec=$(head -c 20 /dev/urandom | base32 | tr -d '=')
  H() { printf '%s' "$1" | "$t/gi" hash-password; }
  jq -n --arg a "$(H "$pu")" --arg b "$(H "$pa")" --arg s "$sec" '[{username:"marta",password_hash:$a,role:"user"},{username:"julian",password_hash:$b,role:"admin",mfa_secret:$s}]' > "$t/users.json"
  IDENTITY_USERS_FILE="$t/users.json" LISTEN_ADDR=127.0.0.1:18237 "$t/gi" >/dev/null 2>&1 & ip=$!
  GOVERNANCE_AUTH=identity IDENTITY_URL=http://127.0.0.1:18237 GOVERNANCE_SERVICE_TOKEN="$(openssl rand -hex 32)" GOVERNANCE_DATA_DIR="$t/data" LISTEN_ADDR=127.0.0.1:18238 "$t/gg" >/dev/null 2>&1 & gp=$!; sleep 2
  otp=$(python3 - "$sec" <<'PY'
import sys,hmac,hashlib,struct,time,base64
k=base64.b32decode(sys.argv[1]+'='*((8-len(sys.argv[1])%8)%8)); c=int(time.time())//30
d=hmac.new(k,struct.pack('>Q',c),hashlib.sha1).digest(); o=d[-1]&15
print('%06d'%((struct.unpack('>I',d[o:o+4])[0]&0x7fffffff)%1000000))
PY
)
  tu=$(curl -s -X POST http://127.0.0.1:18237/auth/login -H 'Content-Type: application/json' -d "$(jq -n --arg p "$pu" '{username:"marta",password:$p}')" | jq -r .token)
  ta=$(curl -s -X POST http://127.0.0.1:18237/auth/login -H 'Content-Type: application/json' -d "$(jq -n --arg p "$pa" --arg o "$otp" '{username:"julian",password:$p,otp:$o}')" | jq -r .token)
  P() { curl -s -o /dev/null -w '%{http_code}' -X PUT http://127.0.0.1:18238/policies/events ${1:+-H "Authorization: Bearer $1"} -H 'Content-Type: application/json' -d '{"value":{"enabled_events":["tag"]}}'; }
  echo "sin=$(P) user=$(P "$tu") admin=$(P "$ta")"; kill $ip; sleep 1; echo "identidad_caida=$(P "$ta")"; kill $gp; rm -rf "$t"
  ```
  Esperado: `sin=401 user=403 admin=200` e `identidad_caida=503`.

- [ ] **CA-10** — Arranque seguro: sin token de servicio, con token corto, o con `fake` en `prod`, no arranca.
  ```bash
  t=$(mktemp -d); (cd services/go-governance && go build -o "$t/gg" ./cmd/go-governance)
  env GOVERNANCE_AUTH=fake GOVERNANCE_ALLOW_FAKE_AUTH=true GOVERNANCE_FAKE_TOKENS='a=a:admin' GOVERNANCE_DATA_DIR="$t/d" "$t/gg" >/dev/null 2>&1; echo "sin_servicio rc=$?"
  env GOVERNANCE_AUTH=fake GOVERNANCE_ALLOW_FAKE_AUTH=true GOVERNANCE_FAKE_TOKENS='a=a:admin' GOVERNANCE_SERVICE_TOKEN=corto GOVERNANCE_DATA_DIR="$t/d" "$t/gg" >/dev/null 2>&1; echo "corto rc=$?"
  env GOVERNANCE_AUTH=fake GOVERNANCE_ALLOW_FAKE_AUTH=true GOVERNANCE_ENV=prod GOVERNANCE_FAKE_TOKENS='a=a:admin' GOVERNANCE_SERVICE_TOKEN="$(openssl rand -hex 32)" GOVERNANCE_DATA_DIR="$t/d" "$t/gg" >/dev/null 2>&1; echo "fake_prod rc=$?"
  env -u GOVERNANCE_AUTH GOVERNANCE_SERVICE_TOKEN="$(openssl rand -hex 32)" GOVERNANCE_DATA_DIR="$t/d" "$t/gg" >/dev/null 2>&1; echo "sin_auth rc=$?"
  rm -rf "$t"
  ```
  Esperado: cuatro `rc=` distintos de `0`.

- [ ] **CA-11** — El contrato sigue válido y solo creció.
  ```bash
  npx --yes @redocly/cli@1.25.0 lint contracts/openapi/control-plane.yaml 2>&1 | tail -n 2
  b=$(git merge-base HEAD origin/main); git diff -U0 $b -- contracts | grep -E '^[+-][^+-]' | grep -v -E '^\+' | wc -l
  docker run --rm -i --security-opt label=disable mikefarah/yq:4.44.3 -N '[.paths."/gates/authorize".post.security[0] | keys | .[0], (.paths."/policies/{name}" | keys | join(",")), (.components.schemas | keys | map(select(. == "GateRequest" or . == "GateDecision")) | join(","))] | join(" | ")' < contracts/openapi/control-plane.yaml
  ```
  Esperado: lint sin errores; `0` líneas eliminadas respecto de la base de esta rama (o solo las añadidas por U4-T02/T03 si estas ya están en la base); y `serviceToken | get,put | GateDecision,GateRequest`.

- [ ] **CA-12** — Imagen conforme a las reglas de U5 y sin dependencia de Kubernetes.
  ```bash
  docker build -q -t aqs-go-governance:ci services/go-governance >/dev/null && docker inspect aqs-go-governance:ci --format '{{.Config.User}}'
  grep -E '^FROM' services/go-governance/Dockerfile | grep -c -i -E ':latest|^FROM [a-z0-9./-]+$'
  bash scripts/ci/list-services.sh | grep -c 'services/go-governance'; bash scripts/ci/detect-lang.sh services/go-governance
  go list -C services/go-governance -deps ./... | grep -c -E 'k8s.io|client-go'
  ```
  Esperado: usuario no vacío distinto de `root`/`0`, `0`, `1`, `go` y `0`.

- [ ] **CA-13** — Higiene, sin secretos en código y alcance.
  ```bash
  (cd services/go-governance && go vet ./... && test -z "$(gofmt -l .)" && echo ok)
  docker run --rm -v "$PWD":/repo zricethezav/gitleaks:v8.18.4 detect --no-git --source /repo/services/go-governance --redact 2>&1 | grep -c -E 'leaks found: [1-9]'
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(services/go-governance/|contracts/openapi/control-plane\.yaml|bitacoras/U4-T04\.md)' | wc -l
  ```
  Esperado: `ok`, `0`, `0` y `0`.

---

## Plan de pruebas

- **Matriz real**: la de U4-T01 corre contra el evaluador real (cada fila, una subprueba). Más filas nuevas para cada celda de la tabla de gates (cada hecho exigido ausente, `false` y `unknown`), cada transición ilegal y cada estado terminal.
- Namespace: tabla con `aqs-test`, `AQS-TEST`, `aqs-test ` (espacio), `aqs-test\n`, `aqs-test.evil`, vacío, `staging`, `prod`, `default`, Unicode confundible.
- Políticas: esquema por política (un caso válido, uno por cada restricción violada), versionado, idempotencia, concurrencia (`-race`, 20 `PUT` simultáneos → versiones consecutivas sin huecos).
- Workflows: cuota diaria con reloj inyectado (cambio de día UTC reinicia), aprobación, nombre inexistente, política ausente.
- Auditoría: cadena íntegra tras N escrituras concurrentes; alteración, borrado y reordenamiento de una línea detectados por `verify-audit`; reapertura continúa la cadena; archivo con línea truncada → el servicio **no arranca** (no continúa sobre una cadena rota) y lo dice.
- Fail-closed: `AuditLog` que falla, `PolicyStore` que falla, JSON corrupto, contexto cancelado → `allow=false` siempre.
- Negativa de secretos: el token de servicio y los tokens de personas no aparecen en logs ni en errores.

**Rojo primero:** el codificador registra en su bitácora que `go build ./cmd/go-governance` falla y que la matriz de U4-T01 solo pasa contra el `FakeEvaluator` (no hay evaluador real).

---

## Notas

- Archivos que se **modifican en su sitio**: `contracts/openapi/control-plane.yaml` (aditivo) y los paquetes de U4-T01 (`GateInput` gana `Workflow` y `ApprovalRecorded`; la matriz de U4-T01 **no cambia de resultado**). Nada de duplicados con sufijo.
- U2-T02 (`go-run-controller`) se escribirá contra `POST /gates/authorize` tal como queda aquí; si el codificador cambia un nombre de campo, lo avisa en la bitácora.
- Los máximos y rangos de las políticas son valores por defecto razonables, no requisitos del PRD; el humano los confirma (como en C-08).
- **Versión de Go (aprendida en U4-T02).** `go.mod` de `go-governance` debe pasar a `go 1.26.8` (hoy `go 1.24`, sin soporte: el job `vuln` de la CI falla por avisos de la biblioteca estándar) y el Dockerfile usar `golang:1.26.8-alpine3.24` con runtime `alpine:3.24`, como `go-identity`. Verifica con `GOTOOLCHAIN=go1.26.8`. `govulncheck` no corre en el entorno del loop (403 de `vuln.go.dev`): la confirmación es el CI de GitHub tras el push. El `docker build` literal falla en el entorno del loop por la CA del proxy; se verifica con un contexto temporal que añade la CA.
- Ningún comando contra un clúster ni la nube. Los servidores se levantan en `127.0.0.1`.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
