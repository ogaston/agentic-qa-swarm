#!/bin/sh
# Script de baseline de la app de referencia para go-reset (U8-T04). Contrato (script.go):
#   clean   -> rc 0 si la app dejo la base en el baseline; sin salida
#   verify  -> imprime solo un entero: filas que difieren del baseline (0 = limpia)
#   version -> imprime la version del baseline (ref-v1)
# Solo usa sh y wget de busybox (la imagen de go-reset es alpine). El token viene de un archivo montado
# (TARGET_APP_BASELINE_TOKEN_FILE); nunca se imprime. Errores a stderr con rc distinto de 0.
set -eu

BASE_URL="${TARGET_APP_BASELINE_URL:-http://warm-app.aqs-test.svc:9090}"
TOKEN_FILE="${TARGET_APP_BASELINE_TOKEN_FILE:-/etc/aqs/target-app-baseline/token}"
# -T: sin timeout busybox espera indefinidamente y go-reset se cuelga (U8-T04, F-01).
TIMEOUT="${TARGET_APP_BASELINE_TIMEOUT:-10}"

fail() { echo "baseline: $*" >&2; exit 1; }

[ -r "$TOKEN_FILE" ] || fail "no se puede leer el token de baseline"
TOKEN=$(cat "$TOKEN_FILE")
[ -n "$TOKEN" ] || fail "token de baseline vacio"

# call <POST|GET> <ruta>: la respuesta del cuerpo queda en stdout; el error de HTTP sale con rc != 0.
call() {
  if [ "$1" = POST ]; then
    wget -q -T "$TIMEOUT" -O - --header="Authorization: Bearer $TOKEN" --post-data="" "$BASE_URL$2"
  else
    wget -q -T "$TIMEOUT" -O - --header="Authorization: Bearer $TOKEN" "$BASE_URL$2"
  fi
}

case "${1:-}" in
  clean)
    call POST /__aqs/baseline/clean >/dev/null || fail "clean fallo"
    ;;
  verify)
    out=$(call GET /__aqs/baseline/verify) || fail "verify fallo"
    out=$(printf '%s' "$out" | tr -d '[:space:]')
    case "$out" in
      ''|*[!0-9]*) fail "verify no devolvio un entero" ;;
    esac
    printf '%s\n' "$out"
    ;;
  version)
    out=$(call GET /__aqs/baseline/version) || fail "version fallo"
    printf '%s\n' "$(printf '%s' "$out" | tr -d '[:space:]')"
    ;;
  *)
    echo "uso: baseline.sh clean|verify|version" >&2
    exit 2
    ;;
esac
