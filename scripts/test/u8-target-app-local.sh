#!/usr/bin/env bash
# Pruebas locales de demo/target-app (U8-T04, CA-1 y CA-2) con Postgres por podman.
#   bash scripts/test/u8-target-app-local.sh                  CA-1: API, invariante, defecto, baseline HTTP
#   bash scripts/test/u8-target-app-local.sh --baseline-script CA-2: baseline.sh dentro de la imagen go-reset
# Imprime "OK|FALLA <comprobacion>" y sale distinto de 0 si alguna falla. No toca ningun clúster.
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
mode=full
case "${1:-}" in
  "") ;;
  --baseline-script) mode=baseline ;;
  *) echo "uso: $0 [--baseline-script]" >&2; exit 2 ;;
esac

pg_img=docker.io/library/postgres:16.4
reset_img=ghcr.io/ogaston/agentic-qa-swarm/go-reset:0.0.0
pg_name="aqs-u8t04-pg-$$"
pg_port=55432
app_port=18080
base_port=19090
app_url="http://127.0.0.1:$app_port"
base_url="http://127.0.0.1:$base_port"
pg_pass=u8t04-local
fail=0
app_pid=""

work=$(mktemp -d "${TMPDIR:-/tmp}/u8t04-local.XXXXXX")
chmod 700 "$work"

cleanup() {
  stop_app
  podman rm -f "$pg_name" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT

check() { # <comprobacion> <comando...>
  local name=$1
  shift
  if out=$("$@" 2>&1); then
    echo "OK $name"
  else
    echo "FALLA $name"
    printf '%s\n' "$out" | sed 's/^/    /' >&2
    fail=1
  fi
}

# Arranca la app (binario construido aqui) con o sin el defecto sembrado; espera a /health.
start_app() { # <bug: "" | oversell>
  stop_app
  env -i PATH="$PATH" HOME="$HOME" \
    DATABASE_URL="postgres://postgres:$pg_pass@127.0.0.1:$pg_port/postgres?sslmode=disable" \
    LISTEN_ADDR="127.0.0.1:$app_port" \
    BASELINE_LISTEN_ADDR="127.0.0.1:$base_port" \
    BASELINE_TOKEN_FILE="$work/token" \
    TARGET_APP_BUG="$1" \
    "$work/target-app" >"$work/app.log" 2>&1 &
  app_pid=$!
  for _ in $(seq 1 100); do
    if curl -fsS "$app_url/health" >/dev/null 2>&1; then return 0; fi
    kill -0 "$app_pid" 2>/dev/null || break
    sleep 0.2
  done
  echo "la app no arranco; log:" >&2
  cat "$work/app.log" >&2
  return 1
}

stop_app() {
  if [ -n "$app_pid" ]; then
    kill "$app_pid" 2>/dev/null || true
    wait "$app_pid" 2>/dev/null || true
    app_pid=""
  fi
}

# Llamadas al listener de baseline (solo dentro del clúster en la realidad; aqui, loopback).
bl() { # <metodo> <ruta> [token]
  curl -fsS -X "$1" -H "Authorization: Bearer ${3:-$(cat "$work/token")}" "$base_url$2"
}

# --- preparacion comun -----------------------------------------------------------------
# GOTOOLCHAIN=auto: go.mod fija la version del toolchain (1.26.9); un Go local mas viejo la descarga.
if ! (cd "$root/demo/target-app" 2>/dev/null && GOTOOLCHAIN=auto go build -o "$work/target-app" .); then
  echo "FALLA build-target-app (demo/target-app no compila o no existe)"
  exit 1
fi
echo "OK build-target-app"

openssl rand -hex 16 > "$work/token"
chmod 644 "$work/token"

podman rm -f "$pg_name" >/dev/null 2>&1 || true
if ! podman run -d --name "$pg_name" -e POSTGRES_PASSWORD="$pg_pass" \
    -p "127.0.0.1:$pg_port:5432" "$pg_img" >/dev/null; then
  echo "FALLA postgres-podman (no arranco $pg_img)"
  exit 1
fi
pg_ready=0
for _ in $(seq 1 120); do
  if podman exec "$pg_name" pg_isready -U postgres >/dev/null 2>&1; then pg_ready=1; break; fi
  sleep 1
done
if [ "$pg_ready" -ne 1 ]; then
  echo "FALLA postgres-podman (no quedo listo)"
  exit 1
fi
echo "OK postgres-podman"

if [ "$mode" = baseline ]; then
  # CA-2: el script real, dentro de la imagen de go-reset (alpine, busybox), contra la app.
  # label=disable: SELinux (enforcing) impide montar el script y el token en el contenedor (como policies.sh).
  start_app "" || { echo "FALLA app-arranque"; exit 1; }
  run_reset() { # <subcomando>: imprime la salida del script; el rc del contenedor es el de la prueba
    podman run --rm --network host --security-opt label=disable --entrypoint sh \
      -e TARGET_APP_BASELINE_URL="$base_url" \
      -e TARGET_APP_BASELINE_TOKEN_FILE=/etc/aqs/target-app-baseline/token \
      -v "$root/demo/target-app/baseline/baseline.sh:/baseline.sh:ro" \
      -v "$work/token:/etc/aqs/target-app-baseline/token:ro" \
      "$reset_img" /baseline.sh "$1"
  }
  check "baseline-sh-clean-rc0" run_reset clean
  verify_out=$(run_reset verify 2>/dev/null)
  check "baseline-sh-verify-entero" bash -c "[[ '$verify_out' =~ ^[0-9]+\$ ]]"
  version_out=$(run_reset version 2>/dev/null)
  check "baseline-sh-version-ref-v1" bash -c "[ '$version_out' = ref-v1 ]"
  if [ "$fail" -eq 0 ]; then echo "rc=0"; else echo "rc=1"; fi
  exit "$fail"
fi

# --- CA-1 ------------------------------------------------------------------------------
# Fase A: sin defecto. La invariante se sostiene: vender de mas responde 409.
start_app "" || { echo "FALLA app-arranque"; exit 1; }

check "openapi-valido" bash -c "curl -fsS '$app_url/openapi.json' | jq -e '.openapi and (.paths | has(\"/products\") and has(\"/orders\") and has(\"/health\"))' >/dev/null"

check "baseline-sin-token-401" bash -c "[ \"\$(curl -s -o /dev/null -w '%{http_code}' '$base_url/__aqs/baseline/verify')\" = 401 ]"

bl POST /__aqs/baseline/clean >/dev/null
check "baseline-clean-verify-0" bash -c "[ \"\$(curl -fsS -H 'Authorization: Bearer $(cat "$work/token")' '$base_url/__aqs/baseline/verify' | tr -d '[:space:]')\" = 0 ]"

check "invariante-stock-sin-defecto" bash -c "[ \"\$(curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -d '{\"product_id\":1,\"qty\":11}' '$app_url/orders')\" = 409 ]"

check "invariante-stock-intacto" bash -c "curl -fsS '$app_url/products' | jq -e '[.[] | select(.id==1)][0].stock == 10' >/dev/null"

check "venta-valida-201" bash -c "[ \"\$(curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -d '{\"product_id\":1,\"qty\":1}' '$app_url/orders')\" = 201 ]"

check "baseline-sucio-verify-mayor-que-0" bash -c "v=\$(curl -fsS -H 'Authorization: Bearer $(cat "$work/token")' '$base_url/__aqs/baseline/verify' | tr -d '[:space:]'); [ \"\$v\" -gt 0 ]"

check "baseline-version-ref-v1" bash -c "[ \"\$(curl -fsS -H 'Authorization: Bearer $(cat "$work/token")' '$base_url/__aqs/baseline/version' | tr -d '[:space:]')\" = ref-v1 ]"

# Fase B: con TARGET_APP_BUG=oversell el stock puede quedar negativo (defecto sembrado).
start_app oversell || { echo "FALLA app-arranque-oversell"; exit 1; }
bl POST /__aqs/baseline/clean >/dev/null
check "defecto-oversell-sembrado" bash -c "[ \"\$(curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -d '{\"product_id\":1,\"qty\":11}' '$app_url/orders')\" = 201 ] && curl -fsS '$app_url/products' | jq -e '[.[] | select(.id==1)][0].stock < 0' >/dev/null"

stop_app
if [ "$fail" -eq 0 ]; then echo "rc=0"; else echo "rc=1"; fi
exit "$fail"
