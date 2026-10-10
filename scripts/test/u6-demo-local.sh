#!/usr/bin/env bash
# Recorrido local del dashboard (U6-T06): u6-up -> login -> inbox -> confirmación -> corrida -> warm ->
# mis corridas -> 401 sin token -> logout, todo a través del origen del dashboard (vite preview, :18504).
# Las comprobaciones con curl solo llegan a :18504; el navegador no interviene.
# Imprime "OK|FALLA <comprobacion>" y sale distinto de 0 si alguna falla. Siempre para la pila (trap).
# Los secretos (contraseña, tokens) viven en el directorio de estado de u6-up y nunca se imprimen.
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
scripts="$root/scripts/test"
pointer="${TMPDIR:-/tmp}/aqs-u6-local.dir"
base="${U6_PORT_BASE:-18500}"
web="http://127.0.0.1:$((base + 4))"
fail=0

check() { # <comprobacion> <funcion> [args...]
  local name=$1
  shift
  if out=$("$@" 2>&1); then
    echo "OK $name"
  else
    echo "FALLA $name"
    [ -n "$out" ] && printf '%s\n' "$out" | sed 's/^/    /' >&2
    fail=1
  fi
}

st() { cat "$pointer"; }

token() { cat "$(st)/session.token"; }

# GET autenticado: deja el cuerpo en $st/resp.json y devuelve el código HTTP.
get_auth() { # <ruta>
  curl -s -o "$(st)/resp.json" -w '%{http_code}' -H "Authorization: Bearer $(token)" "$web$1"
}

# POST autenticado con JSON: cuerpo en $st/resp.json; código HTTP por stdout.
post_auth() { # <ruta> <json>
  printf '%s' "$2" | curl -s -o "$(st)/resp.json" -w '%{http_code}' -H "Authorization: Bearer $(token)" \
    -H 'Content-Type: application/json' --data-binary @- "$web$1"
}

json() { # <expresion python sobre la variable d> : lee $st/resp.json
  python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); sys.exit(0 if (eval(sys.argv[2])) else 1)' \
    "$(st)/resp.json" "$1"
}

index_servido() {
  curl -sf "$web/" | grep -q '<div id="root">'
}

csp_estricta() {
  curl -s -D "$(st)/headers.txt" -o /dev/null "$web/" || return 1
  python3 - "$(st)/headers.txt" <<'PY'
import sys
want = {
    "content-security-policy": "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; "
                               "connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'",
    "x-content-type-options": "nosniff",
    "referrer-policy": "no-referrer",
}
got = {}
for line in open(sys.argv[1], encoding="utf-8", errors="replace").read().splitlines():
    if ":" in line:
        k, v = line.split(":", 1)
        got[k.strip().lower()] = v.strip()
bad = [k for k, v in want.items() if got.get(k) != v]
sys.exit(1 if bad else 0)
PY
  local rc=$?
  rm -f "$(st)/headers.txt"
  return $rc
}

login() {
  local s pw code
  s=$(st)
  pw=$(cat "$s/demo.pw")
  code=$(printf '{"username":"demo","password":"%s"}' "$pw" |
    curl -s -o "$s/resp.json" -w '%{http_code}' -H 'Content-Type: application/json' \
      --data-binary @- "$web/api/auth/login")
  [ "$code" = 200 ] || { echo "login respondio $code"; return 1; }
  python3 -c 'import json,sys; sys.stdout.write(json.load(open(sys.argv[1]))["token"])' "$s/resp.json" \
    >"$s/session.token" || return 1
  chmod 600 "$s/session.token"
  rm -f "$s/resp.json"
}

inbox_con_pendientes() {
  local code
  code=$(get_auth '/api/notifications?state=pending') || return 1
  [ "$code" = 200 ] || { echo "GET /api/notifications respondio $code"; return 1; }
  json 'len(d) >= 2 and all(n["state"] == "pending" for n in d)' || return 1
  python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))[0]["id"])' "$(st)/resp.json" >"$(st)/notif.id"
}

confirmar_201() {
  local id code
  id=$(cat "$(st)/notif.id")
  code=$(post_auth "/api/notifications/$id/confirm" '{"flows":["checkout"]}') || return 1
  [ "$code" = 201 ] || { echo "confirmar respondio $code"; return 1; }
  python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["run_id"])' "$(st)/resp.json" >"$(st)/run.id"
  rm -f "$(st)/resp.json"
}

confirmar_otra_vez_409() {
  local id code
  id=$(cat "$(st)/notif.id")
  code=$(post_auth "/api/notifications/$id/confirm" '{"flows":["checkout"]}') || return 1
  [ "$code" = 409 ] || { echo "segunda confirmacion respondio $code"; return 1; }
}

corrida_hasta_done() {
  local run code i states=""
  run=$(cat "$(st)/run.id")
  for i in 1 2 3 4 5 6; do
    code=$(get_auth "/api/runs/$run") || return 1
    [ "$code" = 200 ] || { echo "GET /api/runs respondio $code"; return 1; }
    local s
    s=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["state"])' "$(st)/resp.json")
    states="$states $s"
    [ "$s" = done ] && break
  done
  case "$states" in
    *" deploying running done") return 0 ;;
    *) echo "secuencia de estados:$states"; return 1 ;;
  esac
}

warm_ready() {
  local code
  code=$(get_auth '/api/warm') || return 1
  [ "$code" = 200 ] || { echo "GET /api/warm respondio $code"; return 1; }
  json 'd["state"] == "ready" and d["reset_verified"] is True'
}

mis_corridas() {
  local code run
  run=$(cat "$(st)/run.id")
  code=$(get_auth '/api/confirmations?limit=20') || return 1
  [ "$code" = 200 ] || { echo "GET /api/confirmations respondio $code"; return 1; }
  json "any(r['run_id'] == '$run' for r in d)"
}

sin_token_401() {
  local run code
  run=$(cat "$(st)/run.id")
  code=$(curl -s -o /dev/null -w '%{http_code}' "$web/api/confirmations") || return 1
  [ "$code" = 401 ] || { echo "sin token en /api/confirmations: $code"; return 1; }
  # Un token con forma válida pero inexistente: el stub del controlador debe rechazarlo.
  code=$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $(python3 -c 'import secrets; print(secrets.token_urlsafe(32))')" \
    "$web/api/runs/$run") || return 1
  [ "$code" = 401 ] || { echo "token inventado en /api/runs: $code"; return 1; }
}

logout_invalida_token() {
  local code
  code=$(post_auth '/api/auth/logout' '') || return 1
  case "$code" in 200|204) ;; *) echo "logout respondio $code"; return 1 ;; esac
  code=$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $(token)" "$web/api/auth/session") || return 1
  [ "$code" = 401 ] || { echo "token tras logout: $code"; return 1; }
}

if ! bash "$scripts/u6-up.sh" >&2; then
  echo "FALLA u6-up"
  exit 1
fi
trap 'bash "$scripts/u6-down.sh" >&2' EXIT

check index-servido index_servido
check csp-estricta csp_estricta
check login login
check inbox-con-pendientes inbox_con_pendientes
check confirmar-201 confirmar_201
check confirmar-otra-vez-409 confirmar_otra_vez_409
check corrida-hasta-done corrida_hasta_done
check warm-ready warm_ready
check mis-corridas mis_corridas
check sin-token-401 sin_token_401
check logout-invalida-token logout_invalida_token

exit "$fail"
