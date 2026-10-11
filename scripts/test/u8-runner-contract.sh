#!/usr/bin/env bash
# Contrato de la imagen aqs-runner con los Jobs de go-run-controller (U8-T03, CA-2).
# La interfaz NO se copia a mano: se renderizan los Jobs con `go run ./cmd/go-run-controller
# render-rehearsal-job` y `render-runner-job`, se toman de ellos la imagen, los args y el entorno,
# y se ejecuta esa imagen con podman contra un servidor HTTP local (uno sano y uno que responde 500).
# Imprime OK|FALLA por comprobación y sale 0 solo si todas pasan. No toca ningún clúster.
# Requiere go, podman, python3 (con PyYAML) y jq.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
work=$(mktemp -d)
pids=()
cleanup() {
  local p
  for p in "${pids[@]:-}"; do
    [ -n "$p" ] && kill "$p" 2>/dev/null || true
  done
  rm -rf "$work"
}
trap cleanup EXIT

fail=0
ok() { echo "OK $1"; }
bad() {
  echo "FALLA $1"
  fail=1
}

# Imágenes tal como las fija el plano de control (fuente de verdad: control-plane.yaml).
read -r IMG_REHEARSAL IMG_RUNNER < <(python3 -I - "$root/deploy/flux/base/control-plane.yaml" <<'PY'
import sys, yaml
want = {"REHEARSAL_IMAGE", "RUNNER_IMAGE"}
found = {}
def walk(n):
    if isinstance(n, dict):
        if n.get("name") in want and "value" in n:
            found[n["name"]] = n["value"]
        for v in n.values():
            walk(v)
    elif isinstance(n, list):
        for v in n:
            walk(v)
for doc in yaml.safe_load_all(open(sys.argv[1])):
    walk(doc)
print(found.get("REHEARSAL_IMAGE", ""), found.get("RUNNER_IMAGE", ""))
PY
)
if [ -z "${IMG_REHEARSAL:-}" ] || [ -z "${IMG_RUNNER:-}" ]; then
  echo "FALLA no se encontraron REHEARSAL_IMAGE/RUNNER_IMAGE en control-plane.yaml" >&2
  exit 1
fi

if ! podman image exists "$IMG_REHEARSAL"; then
  AQS_IMAGES=aqs-runner bash "$root/scripts/kind/build-images.sh" >&2
fi

pick_port() {
  python3 -I -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])'
}

# Servidor de prueba: /health responde <status>; el resto, 404.
cat >"$work/server.py" <<'PY'
import http.server, sys
port, status = int(sys.argv[1]), int(sys.argv[2])
class H(http.server.BaseHTTPRequestHandler):
    def _r(self):
        code = status if self.path == "/health" else 404
        self.send_response(code)
        self.send_header("Content-Length", "0")
        self.end_headers()
    do_GET = _r
    do_POST = _r
    def log_message(self, *a):
        pass
http.server.ThreadingHTTPServer(("127.0.0.1", port), H).serve_forever()
PY

start_server() { # <puerto> <status>
  python3 -I "$work/server.py" "$1" "$2" >/dev/null 2>&1 &
  pids+=("$!")
  local i
  for i in $(seq 1 50); do
    if (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; then return 0; fi
    sleep 0.1
  done
  echo "el servidor de prueba no arrancó en el puerto $1" >&2
  exit 1
}

render() { # <image-var-name>=<valor> <target> <subcomando> — imprime el Job como YAML
  local img_kv=$1 target=$2 sub=$3
  # GOTOOLCHAIN=auto: el go.mod pide 1.26.9; un Go local más viejo lo descarga en vez de fallar.
  (cd "$root/services/go-run-controller" && env "$img_kv" REHEARSAL_TARGET_URL="$target" \
    GOTOOLCHAIN="${GOTOOLCHAIN:-auto}" go run ./cmd/go-run-controller "$sub" --run r-1 --flow f-1)
}

job_spec() { # <yaml del Job> — imprime {image, args, env} en JSON
  python3 -I - "$1" <<'PY'
import sys, json, yaml
j = yaml.safe_load(open(sys.argv[1]))
c = j["spec"]["template"]["spec"]["containers"][0]
print(json.dumps({"image": c["image"], "args": c.get("args", []),
                  "env": {e["name"]: e["value"] for e in c.get("env", [])}}))
PY
}

run_job() { # <spec JSON> — ejecuta la imagen con args y env del Job; devuelve su código de salida
  local spec=$1 img rc=0
  local -a args=() envs=() eargs=()
  img=$(jq -r '.image' <<<"$spec")
  mapfile -t args < <(jq -r '.args[]' <<<"$spec")
  mapfile -t envs < <(jq -r '.env | to_entries[] | "\(.key)=\(.value)"' <<<"$spec")
  local e
  for e in "${envs[@]}"; do eargs+=(--env "$e"); done
  podman run --rm --pull=never --network host --user 65532:65532 --read-only \
    --cap-drop=ALL --security-opt=no-new-privileges "${eargs[@]}" "$img" "${args[@]}" \
    >"$work/run.out" 2>"$work/run.err" || rc=$?
  return "$rc"
}

GOOD_PORT=$(pick_port)
BAD_PORT=$(pick_port)
start_server "$GOOD_PORT" 200
start_server "$BAD_PORT" 500
GOOD="http://127.0.0.1:$GOOD_PORT"
BAD="http://127.0.0.1:$BAD_PORT"

# 1) Job de ensayo: args y env del Job renderizado, contra el servidor sano.
render "REHEARSAL_IMAGE=$IMG_REHEARSAL" "$GOOD" render-rehearsal-job >"$work/rehearse.yaml"
rspec=$(job_spec "$work/rehearse.yaml")
want='["rehearse","--run","r-1","--flow","f-1","--target","'"$GOOD"'"]'
if [ "$(jq -c '.args' <<<"$rspec")" = "$want" ] && run_job "$rspec"; then
  ok rehearse-args
else
  bad rehearse-args
  cat "$work/run.out" "$work/run.err" >&2 || true
fi

# 2) Job runner: args y env del Job renderizado, contra el servidor sano.
render "RUNNER_IMAGE=$IMG_RUNNER" "$GOOD" render-runner-job >"$work/runner.yaml"
nspec=$(job_spec "$work/runner.yaml")
want='["http-steps","--flow","f-1","--target","'"$GOOD"'"]'
if [ "$(jq -c '.args' <<<"$nspec")" = "$want" ] && run_job "$nspec"; then
  ok http-steps-args
else
  bad http-steps-args
  cat "$work/run.out" "$work/run.err" >&2 || true
fi

# 3) Paso que no cumple su expect_status: el Job de ensayo contra el servidor que responde 500 debe salir distinto de 0.
render "REHEARSAL_IMAGE=$IMG_REHEARSAL" "$BAD" render-rehearsal-job >"$work/rehearse-bad.yaml"
bspec=$(job_spec "$work/rehearse-bad.yaml")
if run_job "$bspec"; then
  bad paso-fallido-rc
else
  ok paso-fallido-rc
fi

exit "$fail"
