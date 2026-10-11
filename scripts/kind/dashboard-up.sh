#!/usr/bin/env bash
# Sirve el build de web/dashboard con `vite preview` en 127.0.0.1:18610 (U7-T05) contra la plataforma
# desplegada en el clúster kind `aqs` (contexto kind-aqs). Port-forward en loopback a go-identity
# (18600), ui-api (18601) y go-run-controller (18603); el proxy /api de vite apunta a esos puertos.
# No cambia vite.config.ts: solo exporta las variables AQS_* que ya exige. Estado (pids y logs) en
# ${XDG_RUNTIME_DIR:-/tmp}/aqs-dashboard; dashboard-down.sh lo para y lo borra. Nunca imprime secretos.
# Sale 0 si todo responde; 1 si algo falla (y limpia lo que arrancó).
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
# shellcheck disable=SC1091  # lib.sh se carga por ruta dinámica (relativa a root)
. "$root/scripts/kind/lib.sh"

require_kind_context # sale 3 si el contexto no es kind-aqs (antes de arrancar nada)

NS_SYS=aqs-system
STATE_DIR="${XDG_RUNTIME_DIR:-/tmp}/aqs-dashboard"
P_IDENTITY=18600
P_UI=18601
P_RUN=18603
P_WEB=18610

fail() {
  echo "dashboard-up: $*" >&2
  bash "$root/scripts/kind/dashboard-down.sh" >/dev/null 2>&1
  exit 1
}

# Un arranque previo sin parar se cierra antes (idempotente).
bash "$root/scripts/kind/dashboard-down.sh" >/dev/null 2>&1
mkdir -p "$STATE_DIR" && chmod 700 "$STATE_DIR" || exit 1

for p in "$P_IDENTITY" "$P_UI" "$P_RUN" "$P_WEB"; do
  if (exec 3<>"/dev/tcp/127.0.0.1/$p") 2>/dev/null; then
    fail "el puerto 127.0.0.1:$p ya está ocupado; no se arranca"
  fi
done

# 1. Build fresco del dashboard (dist es ignorado por git).
(cd "$root/web/dashboard" && npm run -s build) >"$STATE_DIR/build.log" 2>&1 \
  || fail "no compila el dashboard (log en $STATE_DIR/build.log)"

# 2. Port-forwards en loopback, uno por servicio, con su pid.
pf() { # <servicio> <puerto local>
  kubectl port-forward --address 127.0.0.1 -n "$NS_SYS" "svc/$1" "$2:8080" \
    >"$STATE_DIR/pf-$1.log" 2>&1 &
  echo $! >"$STATE_DIR/pf-$1.pid"
}
pf go-identity "$P_IDENTITY"
pf ui-api "$P_UI"
pf go-run-controller "$P_RUN"

wait_url() { # <url>: hasta 60 s a que responda 200
  local i
  for i in $(seq 1 60); do
    curl -s -o /dev/null -w '%{http_code}' --max-time 2 "$1" | grep -qx 200 && return 0
    sleep 1
  done
  return 1
}
wait_url "http://127.0.0.1:$P_IDENTITY/healthz" || fail "port-forward a go-identity no responde (log en $STATE_DIR/pf-go-identity.log)"
wait_url "http://127.0.0.1:$P_UI/healthz" || fail "port-forward a ui-api no responde (log en $STATE_DIR/pf-ui-api.log)"
wait_url "http://127.0.0.1:$P_RUN/healthz" || fail "port-forward a go-run-controller no responde (log en $STATE_DIR/pf-go-run-controller.log)"

# 3. vite preview del build con el proxy /api apuntando a los port-forwards. `exec` hace que $! sea el pid de vite.
(
  cd "$root/web/dashboard" || exit 1
  export AQS_IDENTITY_URL="http://127.0.0.1:$P_IDENTITY"
  export AQS_RUN_CONTROLLER_URL="http://127.0.0.1:$P_RUN"
  export AQS_UI_API_URL="http://127.0.0.1:$P_UI"
  exec ./node_modules/.bin/vite preview --host 127.0.0.1 --port "$P_WEB" --strictPort
) >"$STATE_DIR/web.log" 2>&1 &
echo $! >"$STATE_DIR/web.pid"

wait_url "http://127.0.0.1:$P_WEB/" || fail "vite preview no responde en 127.0.0.1:$P_WEB (log en $STATE_DIR/web.log)"

echo "dashboard-up: OK (http://127.0.0.1:$P_WEB)"
