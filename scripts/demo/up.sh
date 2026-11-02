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
for cmd in kind podman kubectl gh jq curl base64 python3; do
  command -v "$cmd" >/dev/null || { echo "up: missing $cmd" >&2; exit 1; }
done
fresh=0
if ! kind get clusters 2>/dev/null | grep -qx aqs; then fresh=1; fi
# Existing stopped nodes must be started before kind-up waits for readiness.
booted=''
if podman container exists aqs-control-plane && [ "$(podman inspect -f '{{.State.Running}}' aqs-control-plane)" != true ]; then
  booted=$(date -u +%FT%TZ)
  podman start aqs-control-plane >/dev/null
fi
bash "$root/scripts/kind/kind-up.sh"
# Tras encender el nodo, los pods muestran el estado de antes del apagado y el kubelet los recrea unos
# minutos después (SandboxChanged). Se espera a que cada pod de aqs-system haya arrancado DESPUÉS del
# encendido y esté listo; si no, el preflight ve servicios que se reinician a mitad del humo.
if [ -n "${booted:-}" ]; then
  settled=0
  for _ in $(seq 1 60); do
    if kubectl --context kind-aqs -n aqs-system get pods -o json 2>/dev/null | jq -e --arg t "$booted" '
        [.items[] | select(.status.phase != "Succeeded" and .status.phase != "Failed")] as $p
        | ($p | length) > 0 and all($p[]; (.status.containerStatuses // []) as $c
            | ($c | length) > 0 and all($c[]; .ready and ((.state.running.startedAt // "") >= $t)))' >/dev/null; then
      settled=1; break
    fi
    sleep 5
  done
  [ "$settled" -eq 1 ] || { echo 'up: aqs-system pods did not restart and become ready within 5 min of booting the node' >&2; exit 1; }
  echo 'up: node booted; aqs-system pods restarted and ready'
fi
if [ "$fresh" -eq 1 ]; then
  bash "$root/scripts/kind/build-images.sh"
  bash "$root/scripts/kind/load-images.sh"
fi
bash "$root/scripts/kind/demo-preflight.sh" --deploy
# Un clúster existente conserva las imágenes viejas con el tag fijo: se reconstruyen, cargan y
# reinician sus deployments, con lectura de vuelta del startTime. Si falla, up.sh se corta aquí.
if [ "$fresh" -eq 0 ]; then
  bash "$root/scripts/kind/refresh-images.sh"
fi
# Los port-forward se pegan a un pod: si refresh-images recreó los pods, el de go-intake que abrió el
# preflight ya no responde. reconnect.sh lo rehace, levanta el dashboard y comprueba el túnel.
bash "$root/scripts/demo/reconnect.sh"
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
# Si el repo está registrado en el asistente de proyectos (go-governance) con webhook_secret_version >= 1,
# go-intake verifica con el secreto DERIVADO de esa versión, no con el de plataforma: hay que poner ese en
# GitHub o cada push da 401 invalid_signature. Falla cerrado si no se puede leer el registro.
reg=$(kubectl --context kind-aqs -n aqs-system exec deploy/go-governance -- cat /data/repositories.json) || {
  echo 'up: could not read the go-governance repository registry' >&2; exit 1; }
version=$(jq -r --arg r "$repo" '[.[] | select((.repo | ascii_downcase) == $r)][0].webhook_secret_version // 0' <<<"$reg")
[[ "$version" =~ ^[0-9]+$ ]] || { echo 'up: invalid webhook_secret_version in the registry' >&2; exit 1; }
if [ "$version" -ge 1 ]; then
  python3 "$root/scripts/demo/webhook-secret.py" "$work/secret" "$repo" "$version" "$work/derived"
  mv "$work/derived" "$work/secret"
  echo "up: $repo is registered in the project wizard; webhook uses its derived secret v$version"
fi
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
