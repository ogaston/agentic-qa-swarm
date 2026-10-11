#!/usr/bin/env bash
# CA-3 de U8-T04: deploy de target-app sobre el warm de kind, superficie leida de vuelta y reset verificado.
# Habla con go-warm-manager y go-reset por port-forward (loopback). Tokens leidos del clúster, nunca impresos.
# Solo actua sobre el clúster kind `aqs` (guarda require_kind_context, sale 3 en otro contexto).
# Imprime "OK|FALLA <comprobacion>" y sale distinto de 0 si alguna falla.
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
# shellcheck disable=SC1091  # lib.sh se carga por ruta dinámica (relativa a root)
. "$root/scripts/kind/lib.sh"

require_kind_context # sale 3 si el contexto no es kind-aqs

NS_SYS=aqs-system
IMAGE_REF="$IMAGE_PREFIX/target-app:$IMAGE_TAG"
RUN_ID="u8t04-$(date +%s)"
P_WM=18610
P_RESET=18611
fail=0
work=$(mktemp -d "${TMPDIR:-/tmp}/u8t04-kind.XXXXXX")
chmod 700 "$work"
pids=()

cleanup() {
  for p in "${pids[@]}"; do kill "$p" 2>/dev/null || true; done
  rm -rf "$work"
}
trap cleanup EXIT

check() { # <comprobacion> <comando...>
  local name=$1
  shift
  if out=$("$@" 2>&1); then
    echo "OK $name"
  else
    echo "FALLA $name"
    printf '%s\n' "$out" | sed 's/^/    /' >&2
    fail=1
  fi
}

port_forward() { # <svc> <puerto-local>
  kubectl port-forward --address 127.0.0.1 -n "$NS_SYS" "svc/$1" "$2:8080" >"$work/pf-$1.log" 2>&1 &
  pids+=("$!")
  for _ in $(seq 1 50); do
    curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$2/healthz" && return 0
    sleep 0.2
  done
  return 1
}

token_of() { # <secret> -> token (no se imprime)
  kubectl get secret "$1" -n "$NS_SYS" -o jsonpath='{.data.token}' | base64 -d
}

WM_TOKEN=$(token_of go-warm-manager-service-token)
RESET_TOKEN=$(token_of go-reset-service-token)
WM="http://127.0.0.1:$P_WM"
RESET="http://127.0.0.1:$P_RESET"

# POST/GET con token; imprime el cuerpo; rc != 0 si el codigo HTTP no es 2xx.
api() { # <metodo> <url> <token> [json]
  local m=$1 url=$2 tok=$3 body=${4:-}
  local code
  if [ -n "$body" ]; then
    code=$(curl -s -o "$work/resp" -w '%{http_code}' --max-time 120 -X "$m" -H "Authorization: Bearer $tok" \
      -H 'Content-Type: application/json' -d "$body" "$url")
  else
    code=$(curl -s -o "$work/resp" -w '%{http_code}' --max-time 30 -X "$m" -H "Authorization: Bearer $tok" "$url")
  fi
  cat "$work/resp"
  echo
  [ "${code:0:1}" = 2 ] || { echo "HTTP $code" >&2; return 1; }
}

port_forward go-warm-manager "$P_WM" || { echo "FALLA port-forward go-warm-manager"; exit 1; }
port_forward go-reset "$P_RESET" || { echo "FALLA port-forward go-reset"; exit 1; }

reset_ok() { # <run_id>: reset verificado en go-reset (reset_verified=true)
  api POST "$RESET/resets" "$RESET_TOKEN" "{\"run_id\":\"$1\"}" | jq -e '.reset_verified == true' >/dev/null
}

# 0. Un warm recien instalado esta dirty: ensure no resetea (lo hace go-reset). Reset verificado previo
#    con el baseline real para llevarlo a ready. No es la comprobacion final: esa es la del paso 3.
check reset-previo-warm-ready reset_ok "$RUN_ID-pre"

# 1. Warm listo y deploy de target-app.
check deploy-target-app-ensure api POST "$WM/warm/ensure" "$WM_TOKEN"
check deploy-target-app-aceptado api POST "$WM/deploys" "$WM_TOKEN" \
  "{\"run_id\":\"$RUN_ID\",\"artifact\":{\"kind\":\"published-image\",\"ref\":\"$IMAGE_REF\"}}"

state=""
for _ in $(seq 1 90); do
  state=$(api GET "$WM/deploys/$RUN_ID" "$WM_TOKEN" 2>/dev/null | jq -r '.state // empty' 2>/dev/null)
  case "$state" in done|failed) break ;; esac
  sleep 2
done
if [ "$state" = done ]; then echo "OK deploy-target-app"; else echo "FALLA deploy-target-app (estado: ${state:-sin estado})"; fail=1; fi

# 2. Superficie leida de vuelta: debe contener POST /orders.
surface_ok() {
  api POST "$WM/surface" "$WM_TOKEN" "{\"run_id\":\"$RUN_ID\"}" | jq -e '.endpoints[] | select(.method=="POST" and .path=="/orders")' >/dev/null
}
check superficie-con-orders surface_ok

# 3. Reset verificado tras el deploy: go-reset compara la base de target-app con el baseline real.
check reset-verificado reset_ok "$RUN_ID"

if [ "$fail" -eq 0 ]; then echo "rc=0"; else echo "rc=1"; fi
exit "$fail"
