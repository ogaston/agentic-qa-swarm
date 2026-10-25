#!/usr/bin/env bash
# U11-T02 CA-5: pruebas SIN clúster de los scripts de kind contra el repo objetivo
# (ogaston/demo-target-app-qa-swarm). kubectl, curl y git se simulan con ejecutables en un PATH
# temporal; cada invocación de kubectl se registra para demostrar que no se tocó ningún clúster.
#
# Casos:
#   (a) TARGET_SHA mal formado: kind-e2e.sh sale != 0 ANTES de tocar el clúster y de GHCR.
#   (b) sin red (git ls-remote falla, rc 128): sale != 0 con mensaje «sin red o error de git».
#   (b2) repo objetivo no encontrado: mensaje «no encontrado», distinto de (b).
#   (b3) rama sin commits (ls-remote sin resultados): mensaje «no existe en».
#   (c) SHA sin imagen (GHCR 404): «no hay imagen», sale != 0 antes de firmar.
#   (e) token anónimo de GHCR fallido: «no pude comprobar la imagen», NO «no hay imagen».
#   (f) GHCR 429 en el manifiesto: «no pude comprobar la imagen», NO «no hay imagen».
#   (d) cuerpo firmado con TARGET_SHA DISTINTO del HEAD remoto simulado: after = TARGET_SHA,
#       full_name = ogaston/demo-target-app-qa-swarm, ref = refs/heads/main, HMAC válido, sin ls-remote.
#   (d2) TARGET_BRANCH=develop sin TARGET_SHA: ls-remote de refs/heads/develop, ref firmado = refs/heads/develop.
#   (m) mutación: una copia de kind-e2e.sh/lib.sh que IGNORA TARGET_SHA debe fallar (d).
#   (g) github-webhook-check.sh sin argumento toma el HEAD del repo objetivo por URL, no `origin`.
# Uso: bash scripts/test/u11-scripts-test.sh   → imprime OK/FALLA por caso y termina con OK o FALLA.
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
REAL_GIT=$(command -v git) || { echo "FALLA: falta git"; exit 1; }
REPO=ogaston/demo-target-app-qa-swarm
SHA_HEAD=b97a719199d5ccbc2ff24f9cfc408ec7ee31114d    # lo que devuelve el ls-remote simulado
SHA_FORZADO=2f8c4e1a9b7d3c5e6f0a1b2c3d4e5f6a7b8c9d0e  # TARGET_SHA distinto del HEAD remoto
SHA_MALO=abc123
SECRETO=s3cret

T=$(mktemp -d "${TMPDIR:-/tmp}/u11-scripts-test.XXXXXX") || exit 1
trap 'rm -rf "$T"' EXIT
M="$T/bin"; LOG="$T/log"; mkdir -p "$M" "$LOG" "$T/xdg" "$T/state/aqs-kind"
printf "usuario\nclave-simulada\n" >"$T/state/aqs-kind/demo.txt"
fail=0

# --- simuladores ------------------------------------------------------------
cat >"$M/kubectl" <<EOF
#!/usr/bin/env bash
echo "\$*" >>"$LOG/kubectl.log"
case "\$*" in
  "config current-context") echo kind-aqs; exit 0 ;;
  *"get secret"*) printf '%s' "$(printf '%s' "$SECRETO" | base64)"; exit 0 ;;
  *"port-forward"*) exec sleep 60 ;;
esac
exit 1
EOF

# git: solo intercepta ls-remote; registra GIT_TERMINAL_PROMPT y argumentos.
cat >"$M/git" <<EOF
#!/usr/bin/env bash
case "\$*" in
  *ls-remote*)
    echo "GIT_TERMINAL_PROMPT=\${GIT_TERMINAL_PROMPT:-unset} \$*" >>"$LOG/git.log"
    if [ "\${MOCK_GIT_FALLA:-0}" = 1 ]; then echo "fatal: unable to access 'https://github.com/x/': Could not resolve host: github.com" >&2; exit 128; fi
    if [ "\${MOCK_GIT_NOREPO:-0}" = 1 ]; then echo "remote: Repository not found." >&2; echo "fatal: repository 'https://github.com/x/' not found" >&2; exit 128; fi
    if [ "\${MOCK_GIT_RAMA_VACIA:-0}" = 1 ]; then exit 0; fi
    rama=\${*##*refs/heads/}
    printf '%s\trefs/heads/%s\n' "$SHA_HEAD" "\$rama"; exit 0 ;;
esac
exec "$REAL_GIT" "\$@"
EOF

# curl: GHCR (token y manifiesto), health/login de port-forwards y webhook de go-intake.
cat >"$M/curl" <<EOF
#!/usr/bin/env bash
url=""; out=""; data=""; hdrs=""; wfmt=0
args=("\$@")
for ((i=0; i<\${#args[@]}; i++)); do
  a=\${args[\$i]}
  case "\$a" in
    -o) out=\${args[\$((i+1))]} ;;
    -w) wfmt=1 ;;
    --data) data=\${args[\$((i+1))]} ;;
    -H) hdrs+="\${args[\$((i+1))]}"\$'\n' ;;
    http://*|https://*) url=\$a ;;
  esac
done
echo "\$url" >>"$LOG/curl.log"
respond() { # respond <cuerpo> <codigo>
  if [ -n "\$out" ]; then printf '%s' "\$1" >"\$out"
  elif [ \$wfmt = 0 ]; then printf '%s' "\$1"; fi
  [ \$wfmt = 1 ] && printf '%s' "\$2"
  exit 0
}
case "\$url" in
  *ghcr.io/token*)
    [ "\${MOCK_TOKEN_FALLA:-0}" = 1 ] && exit 22
    respond '{"token":"tok-simulado"}' 200 ;;
  *ghcr.io/v2/*/manifests/*) respond '' "\${MOCK_GHCR_CODIGO:-200}" ;;
  */healthz*|*/health) respond 'ok' 200 ;;
  */auth/login) respond '{"token":"tok-simulado"}' 200 ;;
  */auth/session) respond '{"role":"user"}' 200 ;;
  */webhooks/github)
    printf '%s' "\$data" >"$LOG/hook-body.json"
    printf '%s' "\$hdrs" >"$LOG/hook-headers.txt"
    respond '{"id":"notif-sim-1"}' 202 ;;
esac
exit 7
EOF
chmod +x "$M/kubectl" "$M/git" "$M/curl"

reset_logs() { : >"$LOG/kubectl.log"; rm -f "$LOG/curl.log" "$LOG/git.log" "$LOG/hook-body.json" "$LOG/hook-headers.txt"; }

# correr_e2e <timeout-s> [env...]: ejecuta kind-e2e (o $E2E_SCRIPT) con los simuladores.
correr_e2e() {
  local t=$1; shift
  env -i PATH="$M:$PATH" HOME="$HOME" XDG_RUNTIME_DIR="$T/xdg" XDG_STATE_HOME="$T/state" \
    AQS_KIND_STATE_DIR="$T/state/aqs-kind" "$@" \
    timeout "$t" bash "${E2E_SCRIPT:-$root/scripts/kind/kind-e2e.sh}" --bug off 2>"$T/err.txt"
}

sin_tocar_cluster() { # solo se permite el chequeo de contexto
  [ "$(grep -cv '^config current-context$' "$LOG/kubectl.log" 2>/dev/null)" = 0 ]
}

# Rechazo previo: rc != 0, mensaje esperado, sin clúster, sin webhook.
rechazo_previo() { # rechazo_previo <id> <descripcion> <patron> [env...]
  local id=$1 desc=$2 patron=$3; shift 3
  reset_logs
  correr_e2e 30 "$@"; local rc=$?
  if [ "$rc" != 0 ] && grep -q "$patron" "$T/err.txt" && sin_tocar_cluster && [ ! -f "$LOG/hook-body.json" ]; then
    echo "OK ($id) $desc: rc=$rc, mensaje «$patron»"
  else
    echo "FALLA ($id) $desc: rc=$rc"; cat "$T/err.txt"; fail=1
  fi
}

rechazo_previo a "TARGET_SHA mal formado" 'no es un SHA de 40 hex' TARGET_SHA="$SHA_MALO"
rechazo_previo b "sin red (git ls-remote rc 128)" 'sin red o error de git' MOCK_GIT_FALLA=1
rechazo_previo b2 "repo objetivo no encontrado" 'no encontrado o inaccesible' MOCK_GIT_NOREPO=1
rechazo_previo b3 "rama sin commits" 'no existe en' MOCK_GIT_RAMA_VACIA=1
rechazo_previo c "SHA sin imagen (GHCR 404)" 'no hay imagen' TARGET_SHA="$SHA_HEAD" MOCK_GHCR_CODIGO=404
rechazo_previo e "token anónimo de GHCR fallido" 'no pude comprobar la imagen' TARGET_SHA="$SHA_HEAD" MOCK_TOKEN_FALLA=1
rechazo_previo f "GHCR 429 en el manifiesto" 'no pude comprobar la imagen' TARGET_SHA="$SHA_HEAD" MOCK_GHCR_CODIGO=429
# Separación explícita: con token fallido o 429 NO debe aparecer «no hay imagen».
for caso in "MOCK_TOKEN_FALLA=1" "MOCK_GHCR_CODIGO=429"; do
  correr_e2e 30 TARGET_SHA="$SHA_HEAD" "$caso" >/dev/null
  if grep -q 'no hay imagen' "$T/err.txt"; then echo "FALLA (f') $caso se confunde con «no hay imagen»"; fail=1
  else echo "OK (f') $caso no se confunde con «no hay imagen»"; fi
done

# (d) TARGET_SHA distinto del HEAD remoto: el webhook lleva TARGET_SHA, no el HEAD.
reset_logs
correr_e2e 90 TARGET_SHA="$SHA_FORZADO"
if [ -s "$LOG/hook-body.json" ]; then
  body=$(cat "$LOG/hook-body.json")
  full=$(jq -r '.repository.full_name' <<<"$body"); after=$(jq -r '.after' <<<"$body"); ref=$(jq -r '.ref' <<<"$body")
  sig=$(sed -n 's/^.*X-Hub-Signature-256: \(sha256=[0-9a-f]*\).*$/\1/p' "$LOG/hook-headers.txt" | tr -d '\r')
  esperada="sha256=$(printf '%s' "$body" | openssl dgst -sha256 -hmac "$SECRETO" -hex | awk '{print $2}')"
  if [ "$full" = "$REPO" ] && [ "$after" = "$SHA_FORZADO" ] && [ "$ref" = refs/heads/main ] \
     && [ "$sig" = "$esperada" ] && ! grep -q ls-remote "$LOG/git.log" 2>/dev/null; then
    echo "OK (d) webhook con TARGET_SHA distinto del HEAD: after=$after (no $SHA_HEAD), full_name=$full, ref=$ref, firma válida, sin ls-remote"
  else
    echo "FALLA (d) webhook: full_name=$full after=$after ref=$ref sig_ok=$([ "$sig" = "$esperada" ] && echo si || echo no)"; fail=1
  fi
else
  echo "FALLA (d) no se capturó ningún webhook"; tail -5 "$T/err.txt"; fail=1
fi

# (d2) TARGET_BRANCH=develop sin TARGET_SHA: ls-remote de esa rama, ref firmado y GIT_TERMINAL_PROMPT=0.
reset_logs
correr_e2e 90 TARGET_BRANCH=develop
if [ -s "$LOG/hook-body.json" ]; then
  body=$(cat "$LOG/hook-body.json"); ref=$(jq -r '.ref' <<<"$body"); after=$(jq -r '.after' <<<"$body")
  if [ "$ref" = refs/heads/develop ] && [ "$after" = "$SHA_HEAD" ] && grep -q 'GIT_TERMINAL_PROMPT=0 .*refs/heads/develop' "$LOG/git.log"; then
    echo "OK (d2) TARGET_BRANCH=develop: ref=$ref, SHA del HEAD de develop, GIT_TERMINAL_PROMPT=0"
  else
    echo "FALLA (d2) ref=$ref after=$after"; cat "$LOG/git.log" 2>/dev/null; fail=1
  fi
else
  echo "FALLA (d2) no se capturó ningún webhook"; tail -5 "$T/err.txt"; fail=1
fi

# (m) mutación: copia de kind-e2e.sh + lib.sh que IGNORA TARGET_SHA; (d) debe detectarla.
mut="$T/mut"; mkdir -p "$mut"; cp -r "$root/scripts" "$mut/"
sed -i 's/if \[ -n "\${TARGET_SHA:-}" \]; then/if false; then/' "$mut/scripts/kind/lib.sh"
if ! grep -q 'if false; then' "$mut/scripts/kind/lib.sh"; then echo "FALLA (m) no pude aplicar la mutación"; fail=1; fi
reset_logs
E2E_SCRIPT="$mut/scripts/kind/kind-e2e.sh" correr_e2e 90 TARGET_SHA="$SHA_FORZADO"
if [ -s "$LOG/hook-body.json" ] && [ "$(jq -r '.after' "$LOG/hook-body.json")" = "$SHA_FORZADO" ]; then
  echo "FALLA (m) la mutación que ignora TARGET_SHA NO fue detectada"; fail=1
else
  echo "OK (m) con la mutación que ignora TARGET_SHA, (d) falla (after distinto o sin webhook)"
fi

# (g) github-webhook-check.sh sin argumento: HEAD por URL del repo objetivo, nunca `origin`.
reset_logs
env -i PATH="$M:$PATH" HOME="$HOME" XDG_RUNTIME_DIR="$T/xdg" XDG_STATE_HOME="$T/state" \
  AQS_KIND_STATE_DIR="$T/state/aqs-kind" WEBHOOK_TIMEOUT_S=1 \
  timeout 60 bash "$root/scripts/kind/github-webhook-check.sh" >"$T/wc.out" 2>&1
if grep -q "sha esperado: $SHA_HEAD" "$T/wc.out" && grep -q "GIT_TERMINAL_PROMPT=0 .*https://github.com/$REPO refs/heads/main" "$LOG/git.log" \
   && ! grep -q ' origin ' "$LOG/git.log"; then
  echo "OK (g) github-webhook-check toma el HEAD de https://github.com/$REPO (no origin)"
else
  echo "FALLA (g) github-webhook-check"; cat "$T/wc.out"; cat "$LOG/git.log" 2>/dev/null; fail=1
fi

if [ "$fail" = 0 ]; then echo OK; else echo FALLA; exit 1; fi
