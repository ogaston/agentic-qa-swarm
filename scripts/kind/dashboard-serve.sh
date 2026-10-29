#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
# shellcheck disable=SC1091
. "$root/scripts/kind/lib.sh"

export KUBECONFIG="${KUBECONFIG:-$HOME/.kube/config-self-service}"
export KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-aqs-self-service}"
export KIND_CONTEXT="${KIND_CONTEXT:-kind-$KIND_CLUSTER_NAME}"

NS_SYS=aqs-system
PORT_OFFSET="${AQS_PORT_OFFSET:-1000}"
STATE_DIR="${STATE_DIR:-${XDG_RUNTIME_DIR:-/tmp}/aqs-dashboard-offset-$PORT_OFFSET}"
P_IDENTITY=$((18600 + PORT_OFFSET))
P_UI=$((18601 + PORT_OFFSET))
P_RUN=$((18603 + PORT_OFFSET))
P_WEB=$((18610 + PORT_OFFSET))

mkdir -p "$STATE_DIR"

cleanup() {
  echo "Stopping dashboard..."
  [ -n "${PF_ID:-}" ] && kill "$PF_ID" 2>/dev/null || true
  [ -n "${PF_UI_PID:-}" ] && kill "$PF_UI_PID" 2>/dev/null || true
  [ -n "${PF_RUN_PID:-}" ] && kill "$PF_RUN_PID" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

kubectl port-forward --address 127.0.0.1 -n "$NS_SYS" svc/go-identity "$P_IDENTITY:8080" >"$STATE_DIR/pf-go-identity.log" 2>&1 &
PF_ID=$!

kubectl port-forward --address 127.0.0.1 -n "$NS_SYS" svc/ui-api "$P_UI:8080" >"$STATE_DIR/pf-ui-api.log" 2>&1 &
PF_UI_PID=$!

kubectl port-forward --address 127.0.0.1 -n "$NS_SYS" svc/go-run-controller "$P_RUN:8080" >"$STATE_DIR/pf-go-run-controller.log" 2>&1 &
PF_RUN_PID=$!

# Wait for services
for i in $(seq 1 30); do
  if curl -s -o /dev/null "http://127.0.0.1:$P_IDENTITY/healthz" && \
     curl -s -o /dev/null "http://127.0.0.1:$P_UI/healthz" && \
     curl -s -o /dev/null "http://127.0.0.1:$P_RUN/healthz"; then
    break
  fi
  sleep 1
done

cd "$root/web/dashboard"
export AQS_IDENTITY_URL="http://127.0.0.1:$P_IDENTITY"
export AQS_RUN_CONTROLLER_URL="http://127.0.0.1:$P_RUN"
export AQS_UI_API_URL="http://127.0.0.1:$P_UI"

echo "dashboard-serve: running on http://127.0.0.1:$P_WEB"
exec ./node_modules/.bin/vite preview --host 127.0.0.1 --port "$P_WEB" --strictPort
