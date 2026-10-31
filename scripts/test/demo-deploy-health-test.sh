#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
sed -n '/^available_ok() {/,/^}/p' "$root/scripts/kind/deploy.sh" > "$work/function"
source "$work/function"
kubectl() {
  case "$*" in
    *spec.replicas*) echo 1 ;;
    *status.availableReplicas*|*status.readyReplicas*) echo 1 ;;
    *'get pods'*) printf '%s\n' "$PODS" ;;
    *) return 1 ;;
  esac
}
PODS='{"items":[{"metadata":{"name":"app-abc"},"status":{"containerStatuses":[{"restartCount":3,"state":{"running":{}}}]}}]}'
available_ok aqs-system deployment.apps/app
PODS='{"items":[{"metadata":{"name":"app-abc"},"status":{"containerStatuses":[{"restartCount":3,"state":{"waiting":{"reason":"CrashLoopBackOff"}}}]}}]}'
if available_ok aqs-system deployment.apps/app; then echo 'CrashLoop accepted' >&2; exit 1; fi
PODS='invalid'
if available_ok aqs-system deployment.apps/app 2>/dev/null; then echo 'invalid status accepted' >&2; exit 1; fi
echo 'demo deploy health: OK'
