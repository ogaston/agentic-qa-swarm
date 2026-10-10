#!/usr/bin/env bash
# Parada de U6 local (U6-T06): termina por pid los procesos de u6-up.sh y borra su directorio de estado.
# Sin arranque previo (no hay puntero) no hace nada y sale con 0.
set -uo pipefail

pointer="${TMPDIR:-/tmp}/aqs-u6-local.dir"
[ -f "$pointer" ] || exit 0
state=$(cat "$pointer")

if [ -d "$state" ]; then
  for name in web run warm ui-api go-identity; do
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
fi
rm -f "$pointer"
exit 0
