#!/usr/bin/env bash
# Recorrido por NATS JetStream con los servicios reales, sin clúster (U8-T02, CA-2 y CA-3).
#   - nats-server en podman con la imagen fijada de deploy/flux/base/nats/statefulset.yaml;
#   - el stream AQS_EVENTS lo crea deploy/flux/base/nats/nats-init.sh, ejecutado en la nats-box
#     fijada de deploy/flux/base/nats/job-init.yaml;
#   - go-identity, go-intake, ui-api y go-run-controller compilados fuera del repo, con EVENTS_TRANSPORT=nats;
#   - el gobierno (GOVERNANCE_URL) lo simula un stub HTTP local que siempre permite.
# Uso (desde la raíz del worktree):
#   bash scripts/test/u8-nats-local.sh              recorrido completo: webhook -> inbox -> confirmación -> controlador, y sin duplicados
#   bash scripts/test/u8-nats-local.sh --redelivery se para ui-api, se publica, se arranca: la notificación aparece una vez
# Todo en 127.0.0.1 y con puertos libres elegidos al azar. El estado (binarios, logs, usuario demo) va en
# un mktemp -d 700 que se borra al salir, también si falla. El contenedor de nats-server se borra al salir.
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
mode=full
case "${1:-}" in
  "") ;;
  --redelivery) mode=redelivery ;;
  *) echo "uso: u8-nats-local.sh [--redelivery]" >&2; exit 2 ;;
esac

NATS_IMAGE='nats:2.10.24-alpine@sha256:d13ec5ce79a02e1be937820dd36db611e25bd0c08cd9947fa9a5d52a56bf91fc'
NATS_BOX='docker.io/natsio/nats-box:0.14.5@sha256:0784ab710aefaf6ef037ed797ee7dcde613c6ad208c4dbff1945fc7c1b5b5375'
SECRET='u8-webhook-secret'
GOV_TOKEN='u8-governance-token'
STREAM_NS='7a1c520e-9b3d-4f6a-8e21-c45d900b6e17' # espacio de nombres de run.confirmed (ui-api/inbox/publish.go)

state=$(mktemp -d "${TMPDIR:-/tmp}/aqs-u8-XXXXXX") || exit 1
chmod 700 "$state"
cname="aqs-u8-nats-$$"
declare -A PID=()
rc=0

cleanup() {
  for n in "${!PID[@]}"; do kill "${PID[$n]}" 2>/dev/null; done
  podman rm -f "$cname" >/dev/null 2>&1
  rm -rf "$state"
}
trap cleanup EXIT

fail() { echo "u8-nats-local: $*" >&2; exit 1; }
ok() { echo "OK $1"; }
falla() { echo "FALLA $1: $2"; rc=1; }

# Seis puertos libres en 127.0.0.1 (nats, identity, intake, ui-api, controlador, gobierno).
read -r p_nats p_id p_intake p_uiapi p_run p_gov < <(python3 -c '
import socket
ss = []
for _ in range(6):
    s = socket.socket(); s.bind(("127.0.0.1", 0)); ss.append(s)
print(" ".join(str(s.getsockname()[1]) for s in ss))
for s in ss:
    s.close()
') || fail "no se pudieron reservar puertos"

# Espera a que un puerto acepte conexiones (máximo 20 s).
esperar_puerto() { # <puerto>
  for _ in $(seq 100); do
    (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null && return 0
    sleep 0.2
  done
  return 1
}

# Espera a que una condición (comando) se cumpla, máximo <segundos>.
esperar() { # <segundos> <comando...>
  local t=$1; shift
  for _ in $(seq $((t * 5))); do
    "$@" && return 0
    sleep 0.2
  done
  return 1
}

arranca() { # <nombre> <comando...>
  local name=$1; shift
  "$@" >"$state/$name.log" 2>&1 &
  PID[$name]=$!
}

# 1. Binarios fuera del repo.
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
mkdir -p "$state/bin"
for pair in go-identity:./cmd/go-identity go-intake:./cmd/go-intake ui-api:./cmd/ui-api go-run-controller:./cmd/go-run-controller; do
  svc=${pair%%:*}; pkg=${pair#*:}
  (cd "$root/services/$svc" && go build -o "$state/bin/$svc" "$pkg") >>"$state/build.log" 2>&1 \
    || fail "no compila $svc (log en $state/build.log)"
done

# 2. nats-server en podman y stream AQS_EVENTS con el init real.
podman run -d --rm --name "$cname" -p "127.0.0.1:$p_nats:4222" "$NATS_IMAGE" -js >"$state/podman-nats.log" 2>&1 \
  || fail "no arranca nats-server en podman (log en $state/podman-nats.log)"
esperar_puerto "$p_nats" || fail "nats-server no acepta conexiones"
# El init entra por stdin (sh -s): un montaje de solo lectura del repo no es legible por el usuario de la imagen.
podman run --rm -i --network host -e NATS_URL="nats://127.0.0.1:$p_nats" --entrypoint /bin/sh "$NATS_BOX" -s \
  <"$root/deploy/flux/base/nats/nats-init.sh" >"$state/nats-init.log" 2>&1 \
  || fail "nats-init no creó el stream (log en $state/nats-init.log)"
grep -q 'AQS_EVENTS' "$state/nats-init.log" || fail "nats-init no informa del stream AQS_EVENTS"

# 3. Usuario demo de go-identity (contraseña aleatoria, hash PHC) y stub de gobierno.
python3 - "$state" <<'PY' || fail "no se pudo generar el usuario demo"
import json, os, secrets, subprocess, sys
state = sys.argv[1]
pw = secrets.token_urlsafe(24)
fd = os.open(os.path.join(state, "demo.pw"), os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
os.write(fd, pw.encode()); os.close(fd)
h = subprocess.run([os.path.join(state, "bin", "go-identity"), "hash-password"], input=pw.encode(),
                   capture_output=True, check=True).stdout.decode().strip()
fd = os.open(os.path.join(state, "users.json"), os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
os.write(fd, json.dumps([{"username": "demo", "password_hash": h, "role": "user"}]).encode()); os.close(fd)
PY
cat >"$state/gov.py" <<'PY'
import http.server, json, sys

class H(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        n = int(self.headers.get("Content-Length") or 0)
        self.rfile.read(n)
        body = json.dumps({"allow": True, "reason": "stub U8-T02", "audit_ref": "u8-stub"}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args):
        pass

http.server.ThreadingHTTPServer(("127.0.0.1", int(sys.argv[1])), H).serve_forever()
PY
arranca gov python3 "$state/gov.py" "$p_gov"
arranca identity env IDENTITY_USERS_FILE="$state/users.json" LISTEN_ADDR="127.0.0.1:$p_id" LOG_LEVEL=warn \
  "$state/bin/go-identity"
esperar_puerto "$p_id" || fail "go-identity no arranca (log en $state/identity.log)"

# 4. Los tres servicios con EVENTS_TRANSPORT=nats. Sin archivos de eventos.
mkdir -p "$state/intake" "$state/uiapi" "$state/run"
nats_url="nats://127.0.0.1:$p_nats"
start_all() {
  arranca intake env GITHUB_WEBHOOK_SECRET="$SECRET" INTAKE_DATA_DIR="$state/intake" \
    EVENTS_TRANSPORT=nats NATS_URL="$nats_url" LISTEN_ADDR="127.0.0.1:$p_intake" LOG_LEVEL=warn "$state/bin/go-intake"
  arranca uiapi env UIAPI_AUTH=identity IDENTITY_URL="http://127.0.0.1:$p_id" UIAPI_DATA_DIR="$state/uiapi" \
    EVENTS_TRANSPORT=nats NATS_URL="$nats_url" LISTEN_ADDR="127.0.0.1:$p_uiapi" LOG_LEVEL=warn "$state/bin/ui-api"
  arranca run env RUN_DATA_DIR="$state/run" RUN_PHASES=fake RUN_ALLOW_FAKE_PHASES=true \
    EVENTS_TRANSPORT=nats NATS_URL="$nats_url" GOVERNANCE_URL="http://127.0.0.1:$p_gov" \
    GOVERNANCE_SERVICE_TOKEN="$GOV_TOKEN" IDENTITY_URL="http://127.0.0.1:$p_id" \
    LISTEN_ADDR="127.0.0.1:$p_run" LOG_LEVEL=warn "$state/bin/go-run-controller"
}
start_all
for p in "$p_intake" "$p_uiapi" "$p_run"; do
  esperar 20 curl -sf -o /dev/null "http://127.0.0.1:$p/readyz" || fail "un servicio no queda listo en /readyz (logs en $state)"
done

tok=$(curl -sf -X POST "http://127.0.0.1:$p_id/auth/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"demo\",\"password\":\"$(cat "$state/demo.pw")\"}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])') \
  || fail "no se pudo iniciar sesión en go-identity"

# webhook <delivery> <sha> <repo>: push firmado. Devuelve el código HTTP y deja la respuesta en $state/last.json.
webhook() {
  local body sig
  body=$(python3 -c 'import json,sys; print(json.dumps({"ref":"refs/heads/main","after":sys.argv[1],"repository":{"full_name":sys.argv[2]}}))' "$2" "$3")
  sig=$(python3 -c 'import hmac,hashlib,sys; print("sha256="+hmac.new(sys.argv[1].encode(), sys.argv[2].encode(), hashlib.sha256).hexdigest())' "$SECRET" "$body")
  curl -s -o "$state/last.json" -w '%{http_code}' -X POST "http://127.0.0.1:$p_intake/webhooks/github" \
    -H 'Content-Type: application/json' -H 'X-GitHub-Event: push' -H "X-GitHub-Delivery: $1" \
    -H "X-Hub-Signature-256: $sig" --data-binary "$body"
}

# cuenta_inbox <sha|id> <valor>: cuántas notificaciones del inbox cumplen el campo.
cuenta_inbox() {
  curl -s -H "Authorization: Bearer $tok" "http://127.0.0.1:$p_uiapi/notifications" | python3 -c '
import json, sys
campo, valor = sys.argv[1], sys.argv[2]
print(sum(1 for n in json.load(sys.stdin) if n.get(campo) == valor))' "$1" "$2"
}

sha=$(python3 -c 'import secrets; print(secrets.token_hex(20))')
# Funciones (no comandos con sustitución) para que esperar() reevalúe la condición en cada intento.
inbox_sha_1() { [ "$(cuenta_inbox sha "$sha")" = 1 ]; }
if [ "$mode" = full ]; then
  # CA-2: webhook firmado -> 202 -> inbox -> confirmación -> controlador.
  code=$(webhook "$(python3 -c 'import uuid; print(uuid.uuid4())')" "$sha" acme/shop)
  if [ "$code" = 202 ]; then ok webhook-firmado-202; else falla webhook-firmado-202 "HTTP $code"; fi
  nid=$(python3 -c 'import json; print(json.load(open("'"$state"'/last.json"))["id"])')

  if esperar 20 inbox_sha_1; then ok inbox-recibe-notificacion; else falla inbox-recibe-notificacion "sin notificación en 20 s"; fi

  code=$(curl -s -o "$state/confirm.json" -w '%{http_code}' -X POST "http://127.0.0.1:$p_uiapi/notifications/$nid/confirm" \
    -H "Authorization: Bearer $tok" -H 'Content-Type: application/json' -d '{"flows":["login"]}')
  run=$(python3 -c 'import json; print(json.load(open("'"$state"'/confirm.json"))["run_id"])' 2>/dev/null)
  # La tarea escribe «confirm-202»; el contrato OpenAPI y la API actual responden 201. Se comprueba el real.
  if [ "$code" = 201 ]; then ok confirm-201; else falla confirm-201 "HTTP $code"; fi

  getrun() { curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $tok" "http://127.0.0.1:$p_run/runs/$1"; }
  run_200() { [ "$(getrun "$run")" = 200 ]; }
  if esperar 20 run_200; then ok controlador-recibe-run-confirmed; else falla controlador-recibe-run-confirmed "GET /runs/$run no es 200"; fi

  # sin-duplicados: el mismo event_id dos veces. Se inyecta con nats pub (dos mensajes sin Nats-Msg-Id,
  # así que la deduplicación la hace el consumidor por event_id, no el stream).
  dup_nid="n-dup-$(python3 -c 'import secrets; print(secrets.token_hex(4))')"
  dup_sha=$(python3 -c 'import secrets; print(secrets.token_hex(20))')
  dup_notify=$(python3 - "$dup_nid" "$dup_sha" <<'PY'
import json, sys, uuid
from datetime import datetime, timezone
print(json.dumps({"event_id": str(uuid.uuid4()), "type": "notify.created", "version": 1,
  "occurred_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
  "trace_id": "0af7651916cd43dd8448eb211c80319c",
  "data": {"notification_id": sys.argv[1], "github_event": "commit", "repo": "acme/shop", "sha": sys.argv[2],
           "artifact": {"kind": "build-from-repo", "ref": "main"}}}, separators=(",", ":")))
PY
)
  # run.confirmed con el event_id que produce ui-api para este run (UUIDv5 de run_id) y se reenvía tal cual.
  dup_confirm=$(python3 - "$run" "$nid" "$STREAM_NS" <<'PY'
import json, sys, uuid
from datetime import datetime, timezone
run, nid, ns = sys.argv[1], sys.argv[2], sys.argv[3]
event_id = str(uuid.uuid5(uuid.UUID(ns), run))
print(json.dumps({"event_id": event_id, "type": "run.confirmed", "version": 1,
  "occurred_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
  "trace_id": "0af7651916cd43dd8448eb211c80319c",
  "data": {"run_id": run, "notification_id": nid, "confirmed_by": "demo", "flows": ["login"]}}, separators=(",", ":")))
PY
)
  nats_pub() { # <subject> <mensaje>: dos publicaciones idénticas
    podman run --rm --network host -e NATS_URL="$nats_url" -e SUBJ="$1" -e MSG="$2" --entrypoint /bin/sh "$NATS_BOX" \
      -c 'nats --server "$NATS_URL" pub "$SUBJ" "$MSG" >/dev/null 2>&1 && nats --server "$NATS_URL" pub "$SUBJ" "$MSG" >/dev/null 2>&1'
  }
  nats_pub aqs.events.notify.created "$dup_notify" || falla sin-duplicados "no se pudo publicar notify.created duplicado"
  nats_pub aqs.events.run.confirmed "$dup_confirm" || falla sin-duplicados "no se pudo publicar run.confirmed duplicado"
  sleep 2
  dup_count=$(cuenta_inbox id "$dup_nid")
  if [ "$dup_count" = 1 ] && [ "$(getrun "$run")" = 200 ]; then
    ok sin-duplicados
  else
    falla sin-duplicados "notificaciones con el id duplicado=$dup_count (quiero 1) o GET /runs/$run distinto de 200"
  fi
else
  # CA-3: para ui-api, publica, la arranca: la notificación aparece una vez.
  kill "${PID[uiapi]}" 2>/dev/null
  esperar 10 bash -c "! kill -0 ${PID[uiapi]} 2>/dev/null" || fail "ui-api no se detuvo"
  code=$(webhook "$(python3 -c 'import uuid; print(uuid.uuid4())')" "$sha" acme/shop)
  [ "$code" = 202 ] || fail "webhook con ui-api parada: HTTP $code"
  arranca uiapi env UIAPI_AUTH=identity IDENTITY_URL="http://127.0.0.1:$p_id" UIAPI_DATA_DIR="$state/uiapi" \
    EVENTS_TRANSPORT=nats NATS_URL="$nats_url" LISTEN_ADDR="127.0.0.1:$p_uiapi" LOG_LEVEL=warn "$state/bin/ui-api"
  esperar 20 curl -sf -o /dev/null "http://127.0.0.1:$p_uiapi/readyz" || fail "ui-api no queda lista tras reiniciarla"
  if esperar 20 inbox_sha_1; then
    sleep 2
    if [ "$(cuenta_inbox sha "$sha")" = 1 ]; then ok reentrega-tras-reinicio; else falla reentrega-tras-reinicio "la notificación aparece más de una vez"; fi
  else
    falla reentrega-tras-reinicio "la notificación no aparece tras reiniciar ui-api"
  fi
fi

exit "$rc"
