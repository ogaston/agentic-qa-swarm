#!/usr/bin/env bash
# Para vite preview y los port-forwards que arrancó dashboard-up.sh (U7-T05) y borra su estado.
# Solo mata los pids que consta haber creado; no toca otros procesos ni clústeres. Idempotente.
set -uo pipefail

STATE_DIR="${XDG_RUNTIME_DIR:-/tmp}/aqs-dashboard"
[ -d "$STATE_DIR" ] || exit 0

for pidf in "$STATE_DIR"/*.pid; do
  [ -f "$pidf" ] || continue
  pid=$(cat "$pidf" 2>/dev/null) || continue
  [ -n "$pid" ] && kill "$pid" 2>/dev/null
done
# Espera breve a que los listeners se cierren antes de borrar el estado.
for _ in $(seq 1 20); do
  left=0
  for pidf in "$STATE_DIR"/*.pid; do
    [ -f "$pidf" ] || continue
    pid=$(cat "$pidf" 2>/dev/null) || continue
    [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null && left=1
  done
  [ "$left" -eq 0 ] && break
  sleep 0.2
done
rm -rf "$STATE_DIR"
exit 0
