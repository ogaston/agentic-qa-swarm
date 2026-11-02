#!/usr/bin/env bash
# Estado de toda la demo en una pantalla, solo lectura. Cada línea es OK o FALLA y, si falla, el comando
# que la arregla. No cambia nada, no lee valores de Secrets. Sale 0 solo si todo está OK.
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
CTX=kind-aqs
PUBLIC_URL=${AQS_DEMO_PUBLIC_URL:-https://aqs.ogaston.com}
PUBLIC_URL=${PUBLIC_URL%/}
REPO=${AQS_DEMO_REPO:-ogaston/demo-target-app-qa-swarm}
STATE=${AQS_KIND_STATE_DIR:-${XDG_STATE_HOME:-$HOME/.local/state}/aqs-kind}
bad=0

ok() { printf 'OK    %s\n' "$1"; }
ko() { printf 'FALLA %s\n      arreglo: %s\n' "$1" "$2"; bad=1; }
code_of() { curl -s -o /dev/null -w '%{http_code}' --max-time "${2:-3}" "$1" 2>/dev/null || true; }
k() { kubectl --context "$CTX" --request-timeout=5s "$@"; }

# Nodo y API.
if [ "$(podman inspect -f '{{.State.Running}}' aqs-control-plane 2>/dev/null)" != true ]; then
  ko "nodo kind aqs-control-plane apagado" "bash scripts/demo/up.sh"
  echo 'status: FALLA (el resto necesita el clúster)'
  exit 1
fi
ok "nodo kind aqs-control-plane encendido"
if ! k get --raw=/readyz >/dev/null 2>&1; then
  ko "API del clúster no responde" "espera 1 min y repite; si sigue: bash scripts/demo/up.sh"
  exit 1
fi

# Flux.
fx=$(k -n flux-system get kustomization aqs-kind -o jsonpath='{.status.conditions[?(@.type=="Ready")].status} {.status.lastAppliedRevision}' 2>/dev/null)
if [ "${fx%% *}" = True ]; then ok "Flux aqs-kind Ready en ${fx#* }"; else ko "Flux aqs-kind no está Ready ($fx)" "bash scripts/kind/flux-up.sh"; fi

# Pods del control plane (los Completed de los CronJobs no cuentan).
notready=$(k -n aqs-system get pods --no-headers 2>/dev/null | awk '$3 != "Completed" { split($2, r, "/"); if (r[1] != r[2] || $3 != "Running") print $1 " " $2 " " $3 }')
if [ -z "$notready" ]; then ok "pods de aqs-system Running y listos"; else ko "pods de aqs-system sin listo: $(echo "$notready" | tr '\n' ';')" "kubectl -n aqs-system describe pod <pod>; luego bash scripts/demo/up.sh"; fi

# Warm (se apaga solo tras 30 min sin corridas: idleScaleDownAfter en deploy/flux/base/warm.yaml).
wr=$(k -n aqs-test get deploy warm-app -o jsonpath='{.status.readyReplicas}' 2>/dev/null)
if [ "${wr:-0}" -ge 1 ] 2>/dev/null; then ok "warm-app arriba ($wr lista)"; else ko "warm-app en 0 réplicas (apagado por inactividad o caído)" "bash scripts/demo/up.sh --reset"; fi

# Dashboard: vite + proxy /api hasta go-identity.
if [ "$(code_of http://127.0.0.1:18610/)" = 200 ] && [ "$(code_of http://127.0.0.1:18610/api/healthz)" = 200 ] &&
  [ "$(code_of http://127.0.0.1:18600/healthz)" = 200 ]; then
  ok "dashboard http://127.0.0.1:18610 y su proxy a identity"
else
  ko "dashboard o sus port-forwards no responden (login da 500 sin cuerpo)" "bash scripts/demo/reconnect.sh"
fi

# go-intake local (lo que el túnel publica).
if [ "$(code_of http://127.0.0.1:18622/healthz)" = 200 ]; then ok "go-intake en 127.0.0.1:18622"; else ko "go-intake no responde en 127.0.0.1:18622" "bash scripts/demo/reconnect.sh"; fi

# Conector cloudflared (servicio de systemd) y URL pública.
if systemctl is-active --quiet cloudflared 2>/dev/null || pgrep -f 'cloudflared.*tunnel' >/dev/null 2>&1; then
  ok "conector cloudflared vivo"
else
  ko "conector cloudflared apagado" "sudo systemctl start cloudflared"
fi
pc=$(code_of "$PUBLIC_URL/healthz" 8)
case "$pc" in
  200) ok "túnel $PUBLIC_URL llega a go-intake" ;;
  502) ko "túnel $PUBLIC_URL da 502 (el conector vive pero go-intake local no responde)" "bash scripts/demo/reconnect.sh" ;;
  *) ko "túnel $PUBLIC_URL respondió $pc" "systemctl status cloudflared" ;;
esac

# Última entrega del webhook de GitHub (informativo: un 502 viejo no se arregla solo).
if [ -r "$STATE/demo-webhook-id" ] && command -v gh >/dev/null; then
  hid=$(sed -n '2p' "$STATE/demo-webhook-id")
  last=$(gh api "repos/$REPO/hooks/$hid/deliveries?per_page=1" --jq '.[0] | "\(.delivered_at) \(.event) \(.status_code)"' 2>/dev/null)
  case "${last##* }" in
    2??) ok "última entrega del webhook: $last" ;;
    "") echo "info  sin entregas del webhook (o gh sin acceso)" ;;
    401) echo "aviso última entrega del webhook: $last (firma inválida: el secreto en GitHub no es el que go-intake espera)"
         echo "      arreglo: bash scripts/demo/up.sh (reconcilia el secreto); luego otro push o Redeliver en GitHub" ;;
    *) echo "aviso última entrega del webhook: $last; ese commit no llegó: haz otro push o pulsa Redeliver en GitHub" ;;
  esac
fi

if [ "$bad" -eq 0 ]; then echo 'status: OK (listo para grabar)'; else echo 'status: FALLA (aplica los arreglos de arriba, en orden)'; fi
exit "$bad"
