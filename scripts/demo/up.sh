#!/usr/bin/env bash
# Start the local demo and reconcile its dedicated GitHub webhook.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
# Explicit opt-in: a reset clears the warm database and cache.
if [ "${1:-}" = --reset ]; then
  [ "$#" -eq 1 ] || { echo 'usage: up.sh [--reset]' >&2; exit 2; }
  for cmd in kubectl curl jq base64 openssl; do
    command -v "$cmd" >/dev/null || { echo "reset: missing $cmd" >&2; exit 1; }
  done
  work=$(mktemp -d "${TMPDIR:-/tmp}/aqs-reset.XXXXXX")
  chmod 700 "$work"
  pf=''
  cleanup_reset() {
    if [ -n "$pf" ]; then kill "$pf" 2>/dev/null || true; wait "$pf" 2>/dev/null || true; fi
    rm -rf "$work"
  }
  trap cleanup_reset EXIT
  token=$(kubectl --context kind-aqs -n aqs-system get secret go-reset-service-token -o jsonpath='{.data.token}' | base64 -d)
  [ -n "$token" ] || { echo 'reset: empty service token' >&2; exit 1; }
  # curl reads the authorization header from a private file, not process arguments.
  printf 'header = "Authorization: Bearer %s"\n' "$token" > "$work/curl.conf"
  chmod 600 "$work/curl.conf"
  unset token
  kubectl --context kind-aqs -n aqs-system port-forward --address 127.0.0.1 svc/go-reset :8080 >"$work/forward.log" 2>&1 &
  pf=$!
  port=''
  for _ in $(seq 1 30); do
    port=$(sed -n 's/^Forwarding from 127\.0\.0\.1:\([0-9]*\) -> 8080$/\1/p' "$work/forward.log" | head -n 1)
    [ -n "$port" ] && break
    kill -0 "$pf" 2>/dev/null || { echo 'reset: port-forward failed' >&2; cat "$work/forward.log" >&2; exit 1; }
    sleep 1
  done
  [ -n "$port" ] || { echo 'reset: port-forward timed out' >&2; exit 1; }
  rid="run-$(openssl rand -hex 8)"
  code=$(curl -sS --config "$work/curl.conf" --max-time "${AQS_RESET_TIMEOUT_S:-300}" -o "$work/response" -w '%{http_code}' \
    -H 'Content-Type: application/json' -d "{\"run_id\":\"$rid\"}" "http://127.0.0.1:$port/resets") || {
      echo 'reset: request failed or timed out; inspect warm state before retrying' >&2; exit 1;
    }
  if [ "$code" != 200 ] || ! jq -e '.state == "ready" and .reset_verified == true' "$work/response" >/dev/null; then
    echo "reset: not verified (HTTP $code)" >&2
    cat "$work/response" >&2
    exit 1
  fi
  echo 'reset: OK (state=ready, reset_verified=true)'
  exit 0
fi
[ "$#" -eq 0 ] || { echo 'usage: up.sh [--reset]' >&2; exit 2; }
state=${AQS_KIND_STATE_DIR:-${XDG_STATE_HOME:-$HOME/.local/state}/aqs-kind}
repo=${AQS_DEMO_REPO:-ogaston/demo-target-app-qa-swarm}
if [ "$repo" != "ogaston/demo-target-app-qa-swarm" ]; then
  echo 'up: alternate repositories require platform configuration; only the default target is supported' >&2
  exit 1
fi
export AQS_DEMO_PUBLIC_URL=${AQS_DEMO_PUBLIC_URL:-https://aqs.ogaston.com}
for cmd in kind podman kubectl gh jq curl base64; do
  command -v "$cmd" >/dev/null || { echo "up: missing $cmd" >&2; exit 1; }
done
fresh=0
if ! kind get clusters 2>/dev/null | grep -qx aqs; then fresh=1; fi
# Existing stopped nodes must be started before kind-up waits for readiness.
if podman container exists aqs-control-plane && [ "$(podman inspect -f '{{.State.Running}}' aqs-control-plane)" != true ]; then
  podman start aqs-control-plane >/dev/null
fi
bash "$root/scripts/kind/kind-up.sh"
if [ "$fresh" -eq 1 ]; then
  bash "$root/scripts/kind/build-images.sh"
  bash "$root/scripts/kind/load-images.sh"
fi
bash "$root/scripts/kind/demo-preflight.sh" --deploy
bash "$root/scripts/kind/dashboard-up.sh"
mkdir -p "$state"
chmod 700 "$state"
work=$(mktemp -d "${TMPDIR:-/tmp}/aqs-hook.XXXXXX")
chmod 700 "$work"
trap 'rm -rf "$work"' EXIT
# A stored ID identifies only the webhook created by this lifecycle.
idfile="$state/demo-webhook-id"
if [ -f "$idfile" ]; then
  saved_repo=$(sed -n '1p' "$idfile")
  id=$(sed -n '2p' "$idfile")
  [ "$saved_repo" = "$repo" ] || { echo "up: webhook state belongs to $saved_repo; use a separate AQS_KIND_STATE_DIR" >&2; exit 1; }
  [[ "$id" =~ ^[0-9]+$ ]] || { echo 'up: invalid webhook ID' >&2; exit 1; }
else
  id=''
fi
secret=$(kubectl --context kind-aqs -n aqs-system get secret go-intake-webhook -o jsonpath='{.data.secret}' | base64 -d)
[ -n "$secret" ] || { echo 'up: webhook secret is empty' >&2; exit 1; }
# Keep the secret out of process arguments and logs.
printf '%s' "$secret" > "$work/secret"
unset secret
jq -n --rawfile secret "$work/secret" --arg url "${AQS_DEMO_PUBLIC_URL%/}/webhooks/github" \
  '{name:"web",active:true,events:["push"],config:{url:$url,content_type:"json",secret:$secret,insecure_ssl:"0"}}' > "$work/request"
if [ -n "$id" ]; then
  gh api --method PATCH "repos/$repo/hooks/$id" --input "$work/request" > "$work/response"
else
  gh api --method POST "repos/$repo/hooks" --input "$work/request" > "$work/response"
  id=$(jq -r '.id // empty' "$work/response")
  [[ "$id" =~ ^[0-9]+$ ]] || { echo 'up: GitHub did not return a webhook ID' >&2; exit 1; }
  printf '%s\n%s\n' "$repo" "$id" > "$idfile"
  chmod 600 "$idfile"
fi
printf '\nDashboard: http://127.0.0.1:18610\nWebhook configured for %s\n' "$repo"
[ -r "$state/demo.txt" ] && { printf 'Demo credentials: '; printf '%s\n' "$state/demo.txt"; }
echo 'Create a target commit, wait for its image, then confirm the notification in the dashboard.'
