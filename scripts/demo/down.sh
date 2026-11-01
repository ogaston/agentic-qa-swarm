#!/usr/bin/env bash
# Stop demo-owned processes and the kind node, preserving cluster state.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
bash "$root/scripts/kind/dashboard-down.sh"
for entry in 'aqs-demo-pf-intake.pid:kubectl' 'aqs-demo-cloudflared.pid:cloudflared'; do
  file="/tmp/${entry%%:*}"
  expected=${entry#*:}
  if [ -f "$file" ]; then
    pid=$(cat "$file")
    if [[ "$pid" =~ ^[0-9]+$ ]] && [ -r "/proc/$pid/cmdline" ] && tr '\0' ' ' < "/proc/$pid/cmdline" | grep -Fq "$expected"; then
      kill "$pid" 2>/dev/null || true
    fi
    rm -f "$file"
  fi
done
if command -v podman >/dev/null && podman container exists aqs-control-plane; then
  if [ "$(podman inspect -f '{{.State.Running}}' aqs-control-plane)" = true ]; then
    podman stop aqs-control-plane >/dev/null
  fi
fi
echo 'down: demo stopped; cluster data and webhook preserved'
