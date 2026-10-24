#!/usr/bin/env bash
# Cuenta URLs absolutas http(s):// en dist/ que CARGAN un recurso: atributos src/href,
# url(...) y @import. Sale con 1 si el conteo es > 0 (el build no debe pedir nada externo).
#
# Fuera del conteo a propósito:
#  - w3.org (espacios de nombres SVG/MathML/XLink: no cargan nada).
#  - Bases de parseo del código (p. ej. new URL(x, 'http://localhost/') en src/api/client.ts,
#    'https://destino.invalid' en src/session/next.ts): no aparecen en contexto src/href/url/@import.
#  - El enlace del decodificador de errores de React (cadena de texto, no recurso).
set -uo pipefail

raiz="$(cd "$(dirname "$0")/.." && pwd)"
dist="$raiz/dist"

if [ ! -d "$dist" ]; then
  echo "check-dist-urls: no existe $dist (ejecuta npm run build)" >&2
  exit 2
fi

p_atributo='(src|href)[[:space:]]*=[[:space:]]*["'\'']?https?://[^"'\'' )>]*'
p_url='url\([[:space:]]*["'\'']?https?://[^"'\'' )]*'
p_import='@import[[:space:]]*(url\([[:space:]]*)?["'\'']?https?://[^"'\'' );]*'

conteo=$(grep -rhoE -e "$p_atributo" -e "$p_url" -e "$p_import" "$dist" 2>/dev/null \
  | grep -v 'w3\.org' \
  | wc -l)

echo "$conteo"
if [ "$conteo" -gt 0 ]; then
  exit 1
fi
exit 0
