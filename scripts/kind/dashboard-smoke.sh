#!/usr/bin/env bash
# Recorrido del dashboard contra la plataforma en kind `aqs` (U7-T05), todo a través del origen del
# dashboard (vite preview en 127.0.0.1:18610). Las comprobaciones curl solo llegan a :18610.
# Imprime «OK|FALLA <comprobación>» por cada una y «PENDIENTE <paso> (<motivo>)» para lo que depende de
# P1 (confirmar y seguir la corrida). Sale 0 solo si ninguna falla. Siempre para la pila (trap).
# Las contraseñas y tokens viven en un directorio 700 propio y nunca se imprimen.
# shellcheck disable=SC2317  # las funciones de comprobación se invocan indirectamente (check)
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
# shellcheck disable=SC1091  # lib.sh se carga por ruta dinámica (relativa a root)
. "$root/scripts/kind/lib.sh"

require_kind_context # sale 3 si el contexto no es kind-aqs (antes de crear nada)

ORIGIN=http://127.0.0.1:18610
STATE_DIR="${XDG_RUNTIME_DIR:-/tmp}/aqs-kind"
work=$(mktemp -d "${XDG_RUNTIME_DIR:-/tmp}/aqs-dash-smoke.XXXXXX") || exit 1
chmod 700 "$work"
fail=0
FAIL_SUFFIX=""

cleanup() {
  trap '' INT TERM
  bash "$root/scripts/kind/dashboard-down.sh" >/dev/null 2>&1
  rm -rf "$work"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

bash "$root/scripts/kind/dashboard-up.sh" >"$work/up.log" 2>&1 || {
  echo "FALLA dashboard-up"
  sed 's/^/    /' "$work/up.log" | grep -v -E 'token|password|Bearer' >&2
  exit 1
}

# check <nombre> <función...>: ejecuta la función en este shell; imprime OK o FALLA y su detalle.
check() {
  local name=$1
  shift
  FAIL_SUFFIX=""
  if "$@" >"$work/last.out" 2>&1; then
    echo "OK $name"
  else
    echo "FALLA $name$FAIL_SUFFIX"
    sed 's/^/    /' "$work/last.out"
    fail=1
  fi
}

# http <código esperado> <url> [curl...]: cuerpo en $work/body; 0 si el código coincide.
http() {
  local want=$1 url=$2 code
  shift 2
  code=$(curl -s -o "$work/body" -w '%{http_code}' --max-time 10 "$@" "$url") || code=000
  [ "$code" = "$want" ] && return 0
  echo "GET $url -> $code (se esperaba $want)"
  return 1
}

index_servido() {
  curl -s --max-time 10 -o "$work/index.html" "$ORIGIN/" || return 1
  grep -q '<div id="root">' "$work/index.html" || { echo "el index no contiene <div id=\"root\">"; return 1; }
}

# La misma política que fija vite.config.ts (preview) y que comprueba scripts/test/u6-demo-local.sh.
csp_estricta() {
  curl -s -D "$work/headers.txt" -o /dev/null --max-time 10 "$ORIGIN/" || return 1
  python3 -I - "$work/headers.txt" <<'PY'
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
if bad:
    print("cabeceras distintas de la política: " + ", ".join(bad))
    sys.exit(1)
PY
}

# Login del usuario demo (contraseña en $STATE_DIR/demo.txt, línea 2) a través del origen; token en $work/token.
login() {
  local pw code role
  pw=$(sed -n 2p "$STATE_DIR/demo.txt" 2>/dev/null) || pw=""
  [ -n "$pw" ] || { echo "no hay contraseña demo en $STATE_DIR/demo.txt (ver scripts/kind/secrets.sh)"; return 1; }
  jq -n --arg u demo --arg p "$pw" '{username: $u, password: $p}' >"$work/login.json" || return 1
  http 200 "$ORIGIN/api/auth/login" -H 'Content-Type: application/json' --data @"$work/login.json" || return 1
  jq -j '.token // empty' "$work/body" >"$work/token" || return 1
  [ -s "$work/token" ] || { echo "POST /api/auth/login sin token"; return 1; }
  chmod 600 "$work/token"
  rm -f "$work/body"
  http 200 "$ORIGIN/api/auth/session" -H "Authorization: Bearer $(cat "$work/token")" || return 1
  role=$(jq -r '.role // empty' "$work/body")
  [ "$role" = user ] || { echo "role='$role' (se esperaba user)"; return 1; }
  rm -f "$work/body"
}

inbox() {
  http 200 "$ORIGIN/api/notifications?state=pending" -H "Authorization: Bearer $(cat "$work/token")" || return 1
  jq -e 'type == "array"' "$work/body" >/dev/null || { echo "GET /api/notifications no devuelve un array JSON"; return 1; }
  rm -f "$work/body"
}

# Estado del warm a través del origen: un state del enum de WarmState (contracts/plans/warm-state.schema.json).
warm_estado() {
  http 200 "$ORIGIN/api/warm" -H "Authorization: Bearer $(cat "$work/token")" || return 1
  local st enum
  st=$(jq -r '.state // empty' "$work/body")
  enum=$(jq -r '.properties.state.enum[]' "$root/contracts/plans/warm-state.schema.json") || return 1
  grep -qxF "$st" <<<"$enum" || { echo "state='$st' no pertenece al enum WarmState"; return 1; }
  rm -f "$work/body"
}

sin_token_401() {
  http 401 "$ORIGIN/api/notifications?state=pending" || return 1
  rm -f "$work/body"
}

# Logout por el origen; después el mismo token debe dar 401 en session.
logout_invalida_token() {
  local code
  code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 -X POST \
    -H "Authorization: Bearer $(cat "$work/token")" "$ORIGIN/api/auth/logout") || code=000
  case "$code" in 200|204) ;; *) echo "POST /api/auth/logout respondió $code"; return 1 ;; esac
  http 401 "$ORIGIN/api/auth/session" -H "Authorization: Bearer $(cat "$work/token")" || return 1
}

check index-servido index_servido
check csp-estricta csp_estricta
check login login
check inbox inbox
check warm-estado warm_estado
check sin-token-401 sin_token_401
check logout-invalida-token logout_invalida_token

# Confirmar y seguir la corrida depende de P1 (los eventos viajan por archivos en el disco de cada pod).
echo "PENDIENTE confirmar-y-seguir-corrida (P1)"
echo "    motivo: P1, los eventos viajan por archivos en el disco de cada pod; no hay transporte entre pods"

if [ "$fail" -eq 0 ]; then
  echo "dashboard-smoke: OK (sin FALLA; pendiente declarado arriba)"
  exit 0
fi
echo "dashboard-smoke: FALLA (ver las líneas FALLA)"
exit 1
