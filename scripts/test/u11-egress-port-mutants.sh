#!/usr/bin/env bash
# U11-T06 (CA-2): barrido de mutantes sobre llm_endpoint_host_port (scripts/kind/lib.sh).
# Por cada mutante: copia lib.sh a un directorio temporal, aplica UN cambio de texto exacto (el texto a
# mutar debe aparecer exactamente una vez; si no, el mutante «NO-APLICADO» y el guion falla) y ejecuta
# scripts/test/u11-egress-port-test.sh con LIB_SH apuntando a esa copia.
#   MUERTO <nombre>  la prueba FALLA con el mutante (bien).
#   VIVO <nombre>    la prueba PASA con el mutante (hueco de la prueba).
# Nunca escribe sobre scripts/kind/lib.sh. Sale 0 solo si todos los mutantes aplicados mueren.
# Uso: bash scripts/test/u11-egress-port-mutants.sh; echo "rc=$?"
# Los 19 primeros son los no equivalentes de F-05 (revisiones/U11-T05/ronda-3.md), reconstruidos por
# categoría (la lista concreta de 95 no está en el repo). Después, los de F-01 (puerto 0 y 65536), la regla
# vieja `u.port or 80`, una inversión del puerto por defecto y el tope de dígitos de F-06.
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
orig="$root/scripts/kind/lib.sh"
prueba="$root/scripts/test/u11-egress-port-test.sh"
tmp=$(mktemp -d) || exit 1
trap 'rm -rf "$tmp"' EXIT

# Sanidad: sin mutar, la prueba debe pasar; si no, MUERTO no significa nada.
if ! LIB_SH="$orig" bash "$prueba" >/dev/null 2>&1; then
  echo "FALLA: la prueba no pasa con el lib.sh original; el barrido no tiene sentido" >&2
  exit 1
fi

# mutar <archivo> <texto-viejo> <texto-nuevo>: reemplazo literal de UNA ocurrencia exacta. rc 3 si no hay una.
mutar() {
  python3 -I -c '
import sys
p, old, new = sys.argv[1:4]
s = open(p, encoding="utf-8").read()
if s.count(old) != 1:
    sys.exit(3)
open(p, "w", encoding="utf-8").write(s.replace(old, new, 1))
' "$1" "$2" "$3"
}

total=0 muertos=0 vivos=0 noaplica=0

# caso <nombre> <texto-viejo> <texto-nuevo>
caso() {
  local name=$1 old=$2 new=$3 lib="$tmp/lib.sh"
  total=$((total + 1))
  cp "$orig" "$lib" || exit 1
  if ! mutar "$lib" "$old" "$new"; then
    echo "NO-APLICADO $name (el texto a mutar no aparece exactamente una vez en lib.sh)"
    noaplica=$((noaplica + 1))
    return
  fi
  if LIB_SH="$lib" bash "$prueba" >/dev/null 2>&1; then
    echo "VIVO $name"
    vivos=$((vivos + 1))
  else
    echo "MUERTO $name"
    muertos=$((muertos + 1))
  fi
}

# --- 19 no equivalentes de F-05 -------------------------------------------------------------------
# Hosts reales: cada carácter de la clase de host.
caso host-sin-punto            '|[A-Za-z0-9._-]+)(:' '|[A-Za-z0-9_-]+)(:'
caso host-sin-guion            '|[A-Za-z0-9._-]+)(:' '|[A-Za-z0-9._]+)(:'
caso host-sin-guion-bajo       '|[A-Za-z0-9._-]+)(:' '|[A-Za-z0-9.-]+)(:'
caso host-sin-digitos          '|[A-Za-z0-9._-]+)(:' '|[A-Za-z._-]+)(:'
# IPv6: clase, mayúsculas/minúsculas, punto y dos puntos, corchetes.
caso ipv6-clase-ancha          '\[[0-9A-Fa-f:.]+\]|' '\[[^]]+\]|'
caso ipv6-sin-mayusculas       '0-9A-Fa-f:.' '0-9a-f:.'
caso ipv6-sin-minusculas       '0-9A-Fa-f:.' '0-9A-F:.'
caso ipv6-sin-punto            '0-9A-Fa-f:.' '0-9A-Fa-f:'
caso ipv6-sin-dospuntos        '0-9A-Fa-f:.' '0-9A-Fa-f.'
caso ipv6-sin-corchetes-acepta '|[A-Za-z0-9._-]+)(:' '|[A-Za-z0-9._:-]+)(:'
caso ipv6-sin-quitar-corchete  '  host=${host#[}' '  host=${host}'
# Ceros a la izquierda (F-05 y tope de F-06: la forma actual de la línea de dígitos).
caso ceros-sin-quitar          '  digits=${port#"${port%%[!0]*}"}' '  digits=$port'
caso ceros-quitar-un-cero      '  digits=${port#"${port%%[!0]*}"}' '  digits=${port#0}'
# Ancla del esquema.
caso esquema-sin-ancla         '^([Hh][Tt][Tt][Pp][Ss]?)://' '([Hh][Tt][Tt][Pp][Ss]?)://'
# Puerto vacío.
caso puerto-vacio-acepta       '(:([0-9]+))?$ ]]' '(:([0-9]*))?$ ]]'
# Fin del authority: ?, #, @ y ancla final.
caso authority-sin-interrog    '([/?#].*)?$' '([/#].*)?$'
caso authority-sin-almohadilla '([^/?#]*)' '([^/?]*)'
caso authority-arroba-corta    'hostport=${hostport##*@}' 'hostport=${hostport#*@}'
caso ancla-final-sin-dolar     '(:([0-9]+))?$ ]]' '(:([0-9]+))? ]]'

# --- Extras: F-01 (puerto 0 y 65536), regla vieja, puerto por defecto invertido, tope de dígitos F-06 ----
caso f01-puerto-cero           '(( 10#$digits >= 1 ' '(( 10#$digits >= 0 '
caso f01-puerto-65536          '10#$digits <= 65535 ' '10#$digits <= 65536 '
caso regla-vieja-u-port-or-80  'if [ "$scheme" = https ]; then port=443; else port=80; fi' 'port=${port:-80}'
caso regla-https-http-invertida 'if [ "$scheme" = https ]; then port=443; else port=80; fi' 'if [ "$scheme" = http ]; then port=443; else port=80; fi'
caso f06-sin-tope-digitos      '[ "${#digits}" -le 5 ] || return 1' 'true'
caso f06-tope-cuatro-digitos   '[ "${#digits}" -le 5 ] || return 1' '[ "${#digits}" -le 4 ] || return 1'

echo "---"
echo "mutantes: $total · muertos: $muertos · vivos: $vivos · no aplicados: $noaplica"
if [ "$vivos" -eq 0 ] && [ "$noaplica" -eq 0 ]; then
  echo "OK todos los mutantes mueren"
  exit 0
fi
echo "FALLA: quedan mutantes vivos o sin aplicar"
exit 1
