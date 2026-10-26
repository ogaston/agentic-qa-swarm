#!/usr/bin/env bash
# U11-T05 (ronda 2-3): prueba SIN clúster de la regla que deriva host y puerto de LLM_BASE_URL para el
# control positivo llm-egress-acotado de scripts/kind/kind-smoke.sh.
# La regla vive en UN solo sitio: llm_endpoint_host_port (scripts/kind/lib.sh).
#   https://h        -> h 443     (la regla vieja `u.port or 80` daba 80: es el defecto del control)
#   http://h         -> h 80
#   https://h:8443/v1 -> h 8443
#   http://h:4000/v1  -> h 4000
#   https://h/v1     -> h 443
#   https://u:p@h/  -> h 443 (las credenciales no salen)
#   https://[::1]:8443/ -> ::1 8443
#   https://h:1 -> h 1; https://h:65535 -> h 65535   (límites inferior y superior del puerto)
#   HTTPS://H       -> H 443 (esquema sin distinguir mayúsculas)
# Errores (sale 1 sin salida): ftp://h, cadena vacía, sin host (https:///v1), puerto no numérico,
# puerto 0, 65536 y 99999; host con espacio, «;», «$()», «()» o salto de línea.
# Sensibilidad: la regla vieja (`u.port or 80`) debe dar EXACTAMENTE «h 80» en https://h (par falla-pasa).
# Si python3 no existe, la prueba FALLA (no omite en silencio).
# Uso: bash scripts/test/u11-egress-port-test.sh   → imprime OK/FALLA por caso y termina con OK o FALLA.
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
# shellcheck disable=SC1091  # lib.sh se carga por ruta dinámica (relativa a root)
. "$root/scripts/kind/lib.sh"

fail=0
ok() { echo "OK $1"; }
falla() { echo "FALLA $1"; fail=1; }

# Regla vieja (mutante): la que había en kind-smoke.sh. Solo para demostrar que la prueba la detecta.
regla_vieja() {
  python3 -c 'import sys;from urllib.parse import urlsplit;u=urlsplit(sys.argv[1]);print(u.hostname, u.port or 80)' "$1"
}

# caso_ok <regla> <url> <host> <puerto>: la regla debe dar exactamente «host puerto».
caso_ok() {
  local regla=$1 url=$2 want="$3 $4" got rc=0
  got=$($regla "$url" 2>/dev/null) || rc=$?
  if [ "$rc" -eq 0 ] && [ "$got" = "$want" ]; then
    ok "$url -> $want"
  else
    falla "$url: esperado «$want», obtenido «${got:-(ninguno)}» (rc=$rc)"
  fi
}

# caso_err <regla> <url>: la regla debe fallar (rc == 1) y no imprimir nada en stdout.
caso_err() {
  local regla=$1 url=$2 got rc=0
  got=$($regla "$url" 2>/dev/null) || rc=$?
  if [ "$rc" -eq 1 ] && [ -z "$got" ]; then
    ok "error para «${url}» (rc=$rc)"
  else
    falla "«${url}» debía fallar; salió «${got}» rc=$rc"
  fi
}

# Regla real (lib.sh).
R=llm_endpoint_host_port
caso_ok "$R" "https://h" h 443
caso_ok "$R" "http://h" h 80
caso_ok "$R" "https://h:8443/v1" h 8443
caso_ok "$R" "http://h:4000/v1" h 4000
caso_ok "$R" "https://h/v1" h 443
caso_ok "$R" "https://u:p@h/" h 443
caso_ok "$R" "https://[::1]:8443/" ::1 8443
caso_ok "$R" "https://h:1" h 1
caso_ok "$R" "https://h:65535" h 65535
caso_ok "$R" "HTTPS://H" H 443
caso_err "$R" "ftp://h"
caso_err "$R" ""
caso_err "$R" "https:///v1"
caso_err "$R" "https://h:notnum"
caso_err "$R" "https://h:0"
caso_err "$R" "https://h:65536"
caso_err "$R" "https://h:99999"
caso_err "$R" "https://h
evil"
caso_err "$R" "https://h evil"
caso_err "$R" "https://h;x"
caso_err "$R" 'https://$(x)'
caso_err "$R" "https://h(x)"

# Sensibilidad: el par falla-pasa. La regla vieja debe dar EXACTAMENTE «h 80» en https://h.
# Sin python3 no hay forma de comprobarla: se falla explícitamente.
if ! command -v python3 >/dev/null 2>&1; then
  falla "sensibilidad: falta python3; no se puede comprobar la regla vieja (la prueba no se omite)"
else
  got_viejo=$(regla_vieja "https://h" 2>/dev/null || true)
  if [ "$got_viejo" = "h 80" ]; then
    ok "sensibilidad: la regla vieja da «h 80» en https://h (≠ h 443): la prueba la detecta"
  else
    falla "sensibilidad: la regla vieja da «${got_viejo:-(vacío)}» en https://h, se esperaba «h 80»; la prueba no la distingue"
  fi
fi
# Y la regla nueva no imprime credenciales en ningún caso.
if $R "https://u:p@h/" 2>/dev/null | grep -q 'u:p'; then
  falla "la regla imprime credenciales"
else
  ok "las credenciales de la URL no salen"
fi

if [ "$fail" -eq 0 ]; then echo "OK"; exit 0; fi
echo "FALLA"; exit 1
