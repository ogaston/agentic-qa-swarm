#!/usr/bin/env bash
# Interactive entry point for the live demo. Existing scripts own each operation.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/aqs-demo-launcher.XXXXXX")
trap 'rm -rf "$work"' EXIT

read -r -p 'Deploy platform if needed? [y/N] ' deploy
export AQS_DEMO_PUBLIC_URL=${AQS_DEMO_PUBLIC_URL:-https://aqs.ogaston.com}
setup_args=()
case "$deploy" in y|Y|yes|YES) setup_args+=(--deploy) ;; esac
while :; do
  if bash "$root/scripts/demo/00-setup.sh" "${setup_args[@]}" 2>&1 | tee "$work/setup.log"; then
    break
  else
    setup_status=${PIPESTATUS[0]}
  fi
  if [ "$setup_status" -ne 2 ] || ! grep -q 'otro script de prueba corre' "$work/setup.log"; then
    exit "$setup_status"
  fi
  echo 'Stop the competing script shown above before restarting the demo setup.'
  read -r -p 'Restart demo setup? [y/N] ' restart
  case "$restart" in y|Y|yes|YES) ;; *) exit "$setup_status" ;; esac
done
bash "$root/scripts/kind/dashboard-up.sh"

state_dir=${AQS_KIND_STATE_DIR:-${XDG_STATE_HOME:-$HOME/.local/state}/aqs-kind}
credentials="$state_dir/demo.txt"
if [ ! -r "$credentials" ]; then
  echo "FALLA: demo credentials not readable: $credentials" >&2
  exit 1
fi
username=$(sed -n '1p' "$credentials")
password=$(sed -n '2p' "$credentials")
[ -n "$username" ] && [ -n "$password" ] || { echo 'FALLA: demo credentials incomplete' >&2; exit 1; }
printf '\nDashboard: http://127.0.0.1:18610\nUsername: %s\nPassword: %s\n' "$username" "$password"
unset password
webhook=$(sed -n 's/^URL pública del webhook: //p' "$work/setup.log" | tail -n 1)
printf 'Webhook: %s\n' "${webhook:-Unavailable; check setup output}"
printf '%s\n' 'Configure the GitHub push webhook shown above before continuing.'

read -r -p 'Paste a commit SHA, or choose sha/commit: ' choice
case "$choice" in
  sha)
    read -r -p 'Commit SHA (40 lowercase hex characters): ' sha
    [[ "$sha" =~ ^[0-9a-f]{40}$ ]] || { echo 'FALLA: invalid SHA' >&2; exit 1; }
    ;;
  commit)
    read -r -p 'Path to target repository clone: ' repo
    read -r -p 'Scenario [off/oversell]: ' bug
    case "$bug" in off|oversell) ;; *) echo 'FALLA: choose off or oversell' >&2; exit 1 ;; esac
    printf 'This creates a commit and pushes it to main in %s.\n' "$repo"
    read -r -p 'Continue? [y/N] ' confirm
    case "$confirm" in y|Y|yes|YES) ;; *) echo 'Cancelled.'; exit 0 ;; esac
    bash "$root/scripts/demo/10-push-commit.sh" --repo-dir "$repo" --bug "$bug" | tee "$work/push.log"
    sha=$(sed -n 's/^SHA=//p' "$work/push.log" | tail -n 1)
    [[ "$sha" =~ ^[0-9a-f]{40}$ ]] || { echo 'FALLA: commit script did not return a valid SHA' >&2; exit 1; }
    ;;
  *)
    if [[ "$choice" =~ ^[0-9a-f]{40}$ ]]; then sha=$choice
    else echo 'FALLA: enter a 40-character SHA, sha, or commit' >&2; exit 1; fi
    ;;
esac
printf '\nLog in to the dashboard and confirm the notification when it appears.\n'
bash "$root/scripts/demo/20-run-and-watch.sh" "$sha"
