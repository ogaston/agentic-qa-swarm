#!/usr/bin/env bash
# Arranque local de U6 (U6-T06), todo en 127.0.0.1:
#   go-identity :18500 (usuario demo, rol user), ui-api :18501 (identity real),
#   stub warm-manager :18502, stub run-controller :18503 (scripts/test/u6_stubs.py),
#   vite preview del build del dashboard :18504 (el origen del navegador; proxy /api).
# Estado (binarios, usuarios, contraseña, tokens, eventos, pids y logs) en un mktemp -d propio con
# permisos 700; los secretos van en ficheros 600 y nunca se imprimen. u6-down.sh lo para y lo borra.
# U6_PORT_BASE (defecto 18500) desplaza los cinco puertos; solo para pruebas en máquinas con puertos ocupados.
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
pointer="${TMPDIR:-/tmp}/aqs-u6-local.dir"
base="${U6_PORT_BASE:-18500}"
p_identity=$base p_uiapi=$((base + 1)) p_warm=$((base + 2)) p_run=$((base + 3)) p_web=$((base + 4))

fail() { echo "u6-up: $*" >&2; [ -n "${state:-}" ] && [ -d "$state" ] && bash "$root/scripts/test/u6-down.sh"; exit 1; }

# Un arranque previo sin parar se cierra antes (como u3-up).
[ -f "$pointer" ] && bash "$root/scripts/test/u6-down.sh"

for p in "$p_identity" "$p_uiapi" "$p_warm" "$p_run" "$p_web"; do
  if (exec 3<>"/dev/tcp/127.0.0.1/$p") 2>/dev/null; then
    echo "u6-up: el puerto 127.0.0.1:$p ya está ocupado; no se arranca" >&2
    exit 1
  fi
done

state=$(mktemp -d "${TMPDIR:-/tmp}/aqs-u6-XXXXXX") || exit 1
chmod 700 "$state"
printf '%s\n' "$state" >"$pointer"
chmod 600 "$pointer"
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"

# 1. Binarios de go-identity y ui-api, fuera del repo.
(cd "$root/services/go-identity" && go build -o "$state/go-identity" ./cmd/go-identity) >"$state/build-identity.log" 2>&1 \
  || fail "no compila go-identity (log en $state)"
(cd "$root/services/ui-api" && go build -o "$state/ui-api" ./cmd/ui-api) >"$state/build-uiapi.log" 2>&1 \
  || fail "no compila ui-api (log en $state)"

# 2. Build del dashboard (dist es ignorado por git).
(cd "$root/web/dashboard" && npm run -s build) >"$state/build-web.log" 2>&1 \
  || fail "no compila el dashboard (log en $state)"

# 3. Usuario demo con contraseña aleatoria; el hash PHC va al archivo de usuarios.
python3 - "$state" <<'PY' || fail "no se pudo generar el usuario demo"
import os, secrets, subprocess, sys, json
state = sys.argv[1]
pw = secrets.token_urlsafe(24)
path = os.path.join(state, "demo.pw")
fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
os.write(fd, pw.encode()); os.close(fd)
h = subprocess.run([os.path.join(state, "go-identity"), "hash-password"], input=pw.encode(),
                   capture_output=True, check=True).stdout.decode().strip()
users = [{"username": "demo", "password_hash": h, "role": "user"}]
path = os.path.join(state, "users.json")
fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
os.write(fd, json.dumps(users).encode()); os.close(fd)
PY

# 4. Token de servicio del warm-manager (el de ui-api y el del stub).
python3 -c 'import secrets,sys; sys.stdout.write(secrets.token_urlsafe(32))' >"$state/warm-token" \
  || fail "no se pudo generar el token del warm"
chmod 600 "$state/warm-token"

# 5. Eventos notify.created sembrados (dos pendientes válidos contra su esquema).
python3 - "$state" <<'PY' || fail "no se pudo sembrar el outbox de eventos"
import json, os, sys, uuid
from datetime import datetime, timezone
state = sys.argv[1]
now = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
lines = []
for i, sha in enumerate(["1" * 40, "2" * 40], start=1):
    lines.append(json.dumps({
        "event_id": str(uuid.uuid4()), "type": "notify.created", "version": 1,
        "occurred_at": now, "trace_id": "0af7651916cd43dd8448eb211c80319c",
        "data": {"notification_id": f"n-demo-{i}", "github_event": "commit",
                 "repo": "acme/shop", "sha": sha,
                 "artifact": {"kind": "build-from-repo", "ref": "main"}},
    }, separators=(",", ":")))
with open(os.path.join(state, "events.jsonl"), "w", encoding="utf-8") as fh:
    fh.write("\n".join(lines) + "\n")
PY
mkdir -p "$state/data" && chmod 700 "$state/data"
: >"$state/outbox.jsonl"

# Arranque de un proceso en segundo plano con su pid en <nombre>.pid.
start() { # <nombre> <comando...>
  local name=$1; shift
  "$@" >"$state/$name.log" 2>&1 &
  echo $! >"$state/$name.pid"
}

start go-identity env IDENTITY_USERS_FILE="$state/users.json" LISTEN_ADDR="127.0.0.1:$p_identity" LOG_LEVEL=warn \
  "$state/go-identity"
# ui-api: con UIAPI_AUTH=identity valida cada petición contra go-identity real.
start ui-api env UIAPI_AUTH=identity IDENTITY_URL="http://127.0.0.1:$p_identity" \
  WARM_URL="http://127.0.0.1:$p_warm" UIAPI_WARM_TOKEN_FILE="$state/warm-token" \
  UIAPI_EVENTS_FILE="$state/events.jsonl" UIAPI_OUTBOX_FILE="$state/outbox.jsonl" \
  UIAPI_DATA_DIR="$state/data" LISTEN_ADDR="127.0.0.1:$p_uiapi" LOG_LEVEL=warn \
  "$state/ui-api"
start warm python3 "$root/scripts/test/u6_stubs.py" warm --port "$p_warm" --token-file "$state/warm-token"
start run python3 "$root/scripts/test/u6_stubs.py" run --port "$p_run" --identity-url "http://127.0.0.1:$p_identity"
(cd "$root/web/dashboard" && start web env AQS_IDENTITY_URL="http://127.0.0.1:$p_identity" \
  AQS_RUN_CONTROLLER_URL="http://127.0.0.1:$p_run" AQS_UI_API_URL="http://127.0.0.1:$p_uiapi" \
  ./node_modules/.bin/vite preview --host 127.0.0.1 --port "$p_web" --strictPort)

# Espera a que todo responda. Los procesos arrancados por "env" tienen el pid de env, que hace exec: correcto.
for p in "$p_identity" "$p_uiapi" "$p_warm" "$p_run"; do
  up=0
  for _ in $(seq 100); do
    if curl -sf -o /dev/null "http://127.0.0.1:$p/healthz"; then up=1; break; fi
    sleep 0.2
  done
  [ "$up" -eq 1 ] || fail "el servicio en 127.0.0.1:$p no responde (logs en $state)"
done
up=0
for _ in $(seq 100); do
  if curl -sf -o /dev/null "http://127.0.0.1:$p_web/"; then up=1; break; fi
  sleep 0.2
done
[ "$up" -eq 1 ] || fail "vite preview en 127.0.0.1:$p_web no responde (logs en $state)"
exit 0
