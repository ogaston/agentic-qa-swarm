#!/usr/bin/env bash
# Recorrido del dashboard contra la plataforma en kind `aqs` (U7-T05), todo a través del origen del
# dashboard (vite preview en 127.0.0.1:18610). Las comprobaciones curl solo llegan a :18610.
# Imprime «OK|FALLA <comprobación>» por cada una y «PENDIENTE <paso> (<motivo>)» si el paso no se puede
# comprobar en este clúster (p. ej. sin LLM real). Sale 0 solo si ninguna falla. Siempre para la pila (trap).
# reporte-visible (U9-T03): notificación de prueba por webhook firmado (go-intake, port-forward propio de
# este script), confirmación por el origen del dashboard, seguimiento de la corrida hasta done y
# GET /api/reports/{run} por el origen con verdict.
# Las contraseñas y tokens viven en un directorio 700 propio y nunca se imprimen.
# shellcheck disable=SC2317  # las funciones de comprobación se invocan indirectamente (check)
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
# shellcheck disable=SC1091  # lib.sh se carga por ruta dinámica (relativa a root)
. "$root/scripts/kind/lib.sh"

require_kind_context # sale 3 si el contexto no es kind-aqs (antes de crear nada)

ORIGIN=http://127.0.0.1:18610
P_INTAKE=18604
NS_SYS=aqs-system
work=$(mktemp -d "${XDG_RUNTIME_DIR:-/tmp}/aqs-dash-smoke.XXXXXX") || exit 1
chmod 700 "$work"
fail=0
FAIL_SUFFIX=""

cleanup() {
  trap '' INT TERM
  if [ -s "$work/pf-intake.pid" ]; then kill "$(cat "$work/pf-intake.pid")" 2>/dev/null; fi
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

# Login del usuario demo (contraseña en $AQS_KIND_STATE_DIR/demo.txt, línea 2) a través del origen; token en $work/token.
login() {
  local pw code role
  pw=$(sed -n 2p "$AQS_KIND_STATE_DIR/demo.txt" 2>/dev/null) || pw=""
  [ -n "$pw" ] || { echo "no hay contraseña demo en $AQS_KIND_STATE_DIR/demo.txt (ver scripts/kind/secrets.sh)"; return 1; }
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

# fidelidad-visual (U10-T05): dashboard frente a mockups/swarm-mock.html (npm run e2e:fidelidad). No usa el
# clúster, pero va bajo el mismo cerrojo que el resto de comprobaciones de kind para no solaparse con ellas.
fidelidad_visual() {
  (cd "$root/web/dashboard" && flock "${XDG_RUNTIME_DIR:-/tmp}/aqs-kind.lock" npm run -s e2e:fidelidad)
}
check fidelidad-visual fidelidad_visual

# reporte-visible: solo si hay LLM real configurado; si no, PENDIENTE con su motivo.
# Notificación de prueba: webhook push firmado a go-intake (el origen del dashboard no lo expone).
webhook_de_prueba() {
  local secret sha delivery sig body code
  (kubectl port-forward --address 127.0.0.1 -n "$NS_SYS" svc/go-intake "$P_INTAKE:8080" \
    >"$work/pf-intake.log" 2>&1 & echo $! >"$work/pf-intake.pid")
  local i
  for i in $(seq 1 30); do
    curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$P_INTAKE/healthz" && break
    sleep 1
  done
  curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$P_INTAKE/healthz" || { echo "port-forward go-intake no responde /healthz"; return 1; }
  secret=$(kubectl -n "$NS_SYS" get secret go-intake-webhook -o jsonpath='{.data.secret}' | base64 -d) || return 1
  [ -n "$secret" ] || { echo "secret go-intake-webhook vacio"; return 1; }
  sha=$(printf '%s' "smoke-$(date +%s%N)-$$-$RANDOM" | sha1sum | cut -c1-40)
  delivery=$(cat /proc/sys/kernel/random/uuid 2>/dev/null || printf 'd-%s-%s' "$$" "$(date +%s%N)")
  jq -n --arg a "$sha" --arg r "$(target_branch_ref)" --arg repo "$TARGET_REPO_FULL" '{ref:$r, after:$a, repository:{full_name:$repo}}' >"$work/hook.json" || return 1
  body=$(cat "$work/hook.json")
  sig="sha256=$(printf '%s' "$body" | openssl dgst -sha256 -hmac "$secret" -hex | awk '{print $2}')"
  code=$(curl -sS -o "$work/hook.out" -w '%{http_code}' --max-time 15 \
    -H 'Content-Type: application/json' -H "X-Hub-Signature-256: $sig" -H "X-GitHub-Event: push" \
    -H "X-GitHub-Delivery: $delivery" --data "$body" "http://127.0.0.1:$P_INTAKE/webhooks/github") || code=000
  [ "$code" = 202 ] || { echo "POST /webhooks/github respondio $code (esperaba 202)"; return 1; }
  NOTIF_SMOKE=$(jq -r '.id // empty' "$work/hook.out")
  [ -n "$NOTIF_SMOKE" ] && [ "$NOTIF_SMOKE" != null ] || { echo "el 202 no trae notification.id"; return 1; }
}

# Sigue GET /api/runs/{run} por el origen hasta done (cada 10 s, 20 min como mucho). Un 404 en los
# primeros 2 min es normal: la corrida tarda un instante en aparecer en el controlador tras la confirmación.
seguir_hasta_done() {
  local run=$1 i st code
  for i in $(seq 1 120); do
    code=$(curl -s -o "$work/body" -w '%{http_code}' --max-time 10 \
      -H "Authorization: Bearer $(cat "$work/token")" "$ORIGIN/api/runs/$run") || code=000
    if [ "$code" = 404 ] && [ "$i" -le 12 ]; then
      sleep 10
      continue
    fi
    [ "$code" = 200 ] || { echo "GET /api/runs/$run -> $code: $(head -c 200 "$work/body")"; return 1; }
    st=$(jq -r '.state // empty' "$work/body")
    rm -f "$work/body"
    case "$st" in
      done) return 0 ;;
      failed) echo "la corrida $run quedo en failed"; return 1 ;;
    esac
    sleep 10
  done
  echo "la corrida $run no llego a done en 20 min (ultimo estado: ${st:-?})"
  return 1
}

reporte_visible() {
  local notif run
  # El check de logout invalida el token anterior: se entra de nuevo con el usuario demo.
  login || return 1
  webhook_de_prueba || return 1
  notif=$NOTIF_SMOKE
  jq -n '{flows: ["happy-path"]}' >"$work/confirm.json" || return 1
  http 201 "$ORIGIN/api/notifications/$notif/confirm" -H "Authorization: Bearer $(cat "$work/token")" \
    -H 'Content-Type: application/json' --data @"$work/confirm.json" || return 1
  run=$(jq -r '.run_id // empty' "$work/body")
  rm -f "$work/body"
  [ -n "$run" ] && [ "$run" != null ] || { echo "la confirmacion no devuelve run_id"; return 1; }
  seguir_hasta_done "$run" || return 1
  http 200 "$ORIGIN/api/reports/$run" -H "Authorization: Bearer $(cat "$work/token")" || return 1
  jq -e '.verdict | IN("bug", "sin-hallazgos", "inconcluso")' "$work/body" >/dev/null \
    || { echo "GET /api/reports/$run sin verdict valido"; return 1; }
  rm -f "$work/body"
}

if kubectl -n "$NS_SYS" get secret agents-llm-key >/dev/null 2>&1; then
  check reporte-visible reporte_visible
else
  echo "PENDIENTE reporte-visible (sin LLM real: falta el secret $NS_SYS/agents-llm-key)"
fi

if [ "$fail" -eq 0 ]; then
  echo "dashboard-smoke: OK (sin FALLA; pendiente declarado arriba)"
  exit 0
fi
echo "dashboard-smoke: FALLA (ver las líneas FALLA)"
exit 1
