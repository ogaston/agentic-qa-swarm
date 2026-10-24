#!/usr/bin/env bash
# Arranque local en un comando (sin clúster): compila go-identity, ui-api y el dashboard y levanta
# todo en 127.0.0.1 con stubs de warm-manager y run-controller (envuelve scripts/test/u6-up.sh).
# Imprime la URL, el usuario demo y la ruta del archivo con la contraseña; nunca la contraseña.
# Para parar: scripts/dev/dev-down.sh. Puerto base con U6_PORT_BASE (defecto 18500; web = base+4).
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
pointer="${TMPDIR:-/tmp}/aqs-u6-local.dir"
base="${U6_PORT_BASE:-18500}"

for bin in go npm python3 curl; do
  command -v "$bin" >/dev/null 2>&1 || { echo "dev-up: falta '$bin' en el PATH" >&2; exit 1; }
done

# Dependencias del dashboard solo si faltan (vite preview vive en node_modules).
if [ ! -x "$root/web/dashboard/node_modules/.bin/vite" ]; then
  echo "dev-up: instalando dependencias del dashboard (npm ci)..."
  (cd "$root/web/dashboard" && npm ci --silent) || { echo "dev-up: falló npm ci" >&2; exit 1; }
fi

echo "dev-up: compilando y arrancando (puede tardar un minuto)..."
bash "$root/scripts/test/u6-up.sh" || exit 1

state=$(cat "$pointer")
cat <<MSG

Listo.
  Dashboard : http://127.0.0.1:$((base + 4))
  Usuario   : demo
  Contraseña: cat $state/demo.pw
  Logs      : $state/*.log
  Parar     : scripts/dev/dev-down.sh
MSG
