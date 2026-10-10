#!/usr/bin/env bash
# Parada de U3 local (U3-T07): termina planner y reporter arrancados por u3-up.sh y borra el estado.
set -uo pipefail

state="${U3_LOCAL_STATE:-${TMPDIR:-/tmp}/aqs-u3-local}"

for name in planner reporter; do
  pidf="$state/$name.pid"
  [ -f "$pidf" ] || continue
  pid=$(cat "$pidf")
  kill "$pid" 2>/dev/null
  for _ in $(seq 50); do
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.1
  done
  kill -9 "$pid" 2>/dev/null
done
rm -rf "$state"
exit 0
