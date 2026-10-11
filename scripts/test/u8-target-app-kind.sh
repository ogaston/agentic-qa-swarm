#!/usr/bin/env bash
# CA-3 de U8-T04: deploy de target-app sobre el warm de kind, superficie alcanzable y reset verificado
# que deja la base realmente limpia. Habla con go-warm-manager, go-reset y warm-app por port-forward
# (loopback). Tokens leidos del clúster, nunca impresos.
# Solo actua sobre el clúster kind `aqs` (guarda require_kind_context, sale 3 en otro contexto).
# Imprime "OK|FALLA|PENDIENTE <comprobacion>" y sale distinto de 0 si alguna FALLA.
#
# Modo rojo (U8-T04, F-04): U8_RED_SKIP_FINAL_RESET=1 omite el reset final. Con ese modo las
# comprobaciones de «base limpia tras el reset» deben FALLAR: prueba que discriminan.
#
# Errata E-1 (decidida por el humano, 2026-10-11): el check completo de POST /surface pasa a U8-T05
# (C-107: el usuario MinIO aqs-evidence no existe). Aqui se comprueba que la superficie es alcanzable.
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
# shellcheck disable=SC1091  # lib.sh se carga por ruta dinámica (relativa a root)
. "$root/scripts/kind/lib.sh"

require_kind_context # sale 3 si el contexto no es kind-aqs

NS_SYS=aqs-system
NS_TEST=aqs-test
IMAGE_REF="$IMAGE_PREFIX/target-app:$IMAGE_TAG"
RUN_ID="u8t04-$(date +%s)"
P_WM=18610
P_RESET=18611
P_APP=18612
P_BASE=18613
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

port_forward() { # <objeto> <puerto-local> <puerto-remoto> <ns> <ruta-sonda|->
  local target=$1 lp=$2 rp=$3 ns=$4 probe=$5
  kubectl port-forward --address 127.0.0.1 -n "$ns" "$target" "$lp:$rp" >"$work/pf-$lp.log" 2>&1 &
  pids+=("$!")
  for _ in $(seq 1 60); do
    if [ "$probe" = - ]; then
      (echo >"/dev/tcp/127.0.0.1/$lp") 2>/dev/null && return 0
    else
      curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$lp$probe" && return 0
    fi
    sleep 0.5
  done
  return 1
}

token_of() { # <secret> <ns> -> token (no se imprime)
  kubectl get secret "$1" -n "$2" -o jsonpath='{.data.token}' | base64 -d
}

WM_TOKEN=$(token_of go-warm-manager-service-token "$NS_SYS")
RESET_TOKEN=$(token_of go-reset-service-token "$NS_SYS")
BASE_TOKEN=$(token_of target-app-baseline-token "$NS_TEST")
WM="http://127.0.0.1:$P_WM"
RESET="http://127.0.0.1:$P_RESET"
APP="http://127.0.0.1:$P_APP"
BASE="http://127.0.0.1:$P_BASE"

# api <metodo> <url> <token-o-vacio> [json]: imprime el cuerpo; rc != 0 si el HTTP no es 2xx.
api() {
  local m=$1 url=$2 tok=$3 body=${4:-}
  local code auth=()
  [ -n "$tok" ] && auth=(-H "Authorization: Bearer $tok")
  if [ -n "$body" ]; then
    code=$(curl -s -o "$work/resp" -w '%{http_code}' --max-time 120 -X "$m" "${auth[@]}" \
      -H 'Content-Type: application/json' -d "$body" "$url") || code=000
  else
    code=$(curl -s -o "$work/resp" -w '%{http_code}' --max-time 120 -X "$m" "${auth[@]}" "$url") || code=000
  fi
  cat "$work/resp" 2>/dev/null
  echo
  [ "${code:0:1}" = 2 ] || { echo "HTTP $code" >&2; return 1; }
}

reset_ok() { # <run_id>: reset verificado en go-reset (reset_verified=true). Si no lo es, imprime el cuerpo.
  local body
  body=$(api POST "$RESET/resets" "$RESET_TOKEN" "{\"run_id\":\"$1\"}" 2>/dev/null)
  if printf '%s' "$body" | jq -e '.reset_verified == true' >/dev/null 2>&1; then
    return 0
  fi
  printf 'reset no verificado; cuerpo: %s\n' "$body" >&2
  return 1
}

# Los port-forward a warm-app apuntan a un pod concreto por el selector del Service: el deploy
# reemplaza el pod (nueva imagen), así que se reabren tras el deploy (U8-T04, ronda 2).
pf_warm_app() {
  for p in "${warm_pf[@]:-}"; do kill "$p" 2>/dev/null || true; done
  warm_pf=()
  kubectl port-forward --address 127.0.0.1 -n "$NS_TEST" svc/warm-app "$P_APP:80" >"$work/pf-app.log" 2>&1 &
  warm_pf+=("$!"); pids+=("$!")
  kubectl port-forward --address 127.0.0.1 -n "$NS_TEST" svc/warm-app "$P_BASE:9090" >"$work/pf-base.log" 2>&1 &
  warm_pf+=("$!"); pids+=("$!")
  for _ in $(seq 1 60); do
    if curl -s -o /dev/null --max-time 2 "$APP/" && (echo >"/dev/tcp/127.0.0.1/$P_BASE") 2>/dev/null; then return 0; fi
    sleep 0.5
  done
  return 1
}
warm_pf=()

port_forward svc/go-warm-manager "$P_WM" 8080 "$NS_SYS" /healthz || { echo "FALLA port-forward go-warm-manager"; exit 1; }
port_forward svc/go-reset "$P_RESET" 8080 "$NS_SYS" /healthz || { echo "FALLA port-forward go-reset"; exit 1; }
pf_warm_app || { echo "FALLA port-forward warm-app"; exit 1; }

# 0. Un warm recien instalado esta dirty: ensure no resetea (lo hace go-reset). Reset verificado previo.
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
kubectl -n "$NS_TEST" rollout status deploy/warm-app --timeout=120s >/dev/null 2>&1
pf_warm_app || { echo "FALLA port-forward warm-app tras el deploy"; exit 1; }

# 2. Errata E-1: la superficie es alcanzable desde go-warm-manager (POST /surface completo, en U8-T05).
superficie_alcanzable() {
  local wm_pod
  wm_pod=$(kubectl get pod -n "$NS_SYS" -l app.kubernetes.io/name=go-warm-manager -o jsonpath='{.items[0].metadata.name}') || return 1
  kubectl exec -n "$NS_SYS" "$wm_pod" -- wget -q -T 10 -O - http://warm-app.aqs-test.svc/openapi.json \
    | jq -e '.paths["/orders"] and .paths["/products"]' >/dev/null
}
check superficie-alcanzable superficie_alcanzable
echo "PENDIENTE superficie-con-orders (C-107, U8-T05)"

# 3. F-04: ensuciar la base, resetear y leer de vuelta que queda limpia. Sin ensuciar, el reset no prueba nada.
check ensuciar-base api POST "$APP/orders" "" '{"product_id":1,"qty":1}'
check sucio-antes-del-reset bash -c "[ \"\$(curl -fsS -H 'Authorization: Bearer $BASE_TOKEN' '$BASE/__aqs/baseline/verify' | tr -d '[:space:]')\" -gt 0 ]"

if [ "${U8_RED_SKIP_FINAL_RESET:-0}" = 1 ]; then
  echo "SALTADO reset-verificado (modo rojo U8_RED_SKIP_FINAL_RESET=1)"
else
  check reset-verificado reset_ok "$RUN_ID"
fi

# El reset puede reiniciar warm-app (rollout): los port-forward previos ya no sirven. Se esperan y reabren.
kubectl -n "$NS_TEST" rollout status deploy/warm-app --timeout=120s >/dev/null 2>&1
pf_warm_app || { echo "FALLA port-forward warm-app tras el reset"; fail=1; }
check orders-vacio-tras-reset bash -c "curl -fsS '$APP/orders' | jq -e 'length == 0' >/dev/null"
check verify-0-tras-reset bash -c "[ \"\$(curl -fsS -H 'Authorization: Bearer $BASE_TOKEN' '$BASE/__aqs/baseline/verify' | tr -d '[:space:]')\" = 0 ]"

if [ "$fail" -eq 0 ]; then echo "rc=0"; else echo "rc=1"; fi
exit "$fail"
