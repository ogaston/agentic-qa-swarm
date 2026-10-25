#!/usr/bin/env bash
# U11-T02 CA-5: pruebas SIN clúster de los scripts de kind contra el repo objetivo
# (ogaston/demo-target-app-qa-swarm). kubectl, curl y git se simulan con ejecutables en un PATH
# temporal; cada invocación de kubectl se registra para demostrar que no se tocó ningún clúster.
#
# Casos:
#   (a) TARGET_SHA mal formado: kind-e2e.sh sale != 0 ANTES de tocar el clúster.
#   (b) sin red (git ls-remote falla) y sin TARGET_SHA: kind-e2e.sh sale != 0 con mensaje claro.
#   (c) SHA sin imagen en GHCR: kind-e2e.sh sale != 0 antes de confirmar.
#   (d) cuerpo del webhook firmado: repository.full_name = ogaston/demo-target-app-qa-swarm,
#       after = el SHA elegido, firma HMAC válida con el secreto simulado.
# Uso: bash scripts/test/u11-scripts-test.sh   → imprime OK o FALLA <caso> y termina con OK / FALLA.
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
REAL_GIT=$(command -v git) || { echo "FALLA: falta git"; exit 1; }
REPO=ogaston/demo-target-app-qa-swarm
SHA_OK=b97a719199d5ccbc2ff24f9cfc408ec7ee31114d
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
  *"port-forward"*) exec sleep 600 ;;
esac
exit 1
EOF

cat >"$M/git" <<EOF
#!/usr/bin/env bash
case "\$*" in
  *ls-remote*)
    if [ "\${MOCK_GIT_FALLA:-0}" = 1 ]; then exit 128; fi
    printf '%s\trefs/heads/main\n' "$SHA_OK"; exit 0 ;;
esac
exec "$REAL_GIT" "\$@"
EOF

# curl: GHCR (token y manifest), health/login de port-forwards y webhook de go-intake (captura
# cuerpo y cabeceras). Respuesta: cuerpo a -o y código a stdout si hay -w.
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
  *ghcr.io/token*) respond '{"token":"tok-simulado"}' 200 ;;
  *ghcr.io/v2/*/manifests/*)
    if [ "\${MOCK_GHCR_SIN_IMAGEN:-0}" = 1 ]; then respond '' 404; fi
    respond '' 200 ;;
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

correr_e2e() { # correr_e2e <timeout-s> [env...]: ejecuta kind-e2e con los simuladores
  local t=$1; shift
  env -i PATH="$M:$PATH" HOME="$HOME" XDG_RUNTIME_DIR="$T/xdg" XDG_STATE_HOME="$T/state" \
    AQS_KIND_STATE_DIR="$T/state/aqs-kind" "$@" \
    timeout "$t" bash "$root/scripts/kind/kind-e2e.sh" --bug off 2>"$T/err.txt"
}

sin_tocar_cluster() { # solo se permite el chequeo de contexto
  [ "$(grep -cv '^config current-context$' "$LOG/kubectl.log" 2>/dev/null)" = 0 ]
}

# (a) TARGET_SHA mal formado.
: >"$LOG/kubectl.log"; rm -f "$LOG/curl.log"
correr_e2e 30 TARGET_SHA="$SHA_MALO"; rc=$?
if [ "$rc" != 0 ] && grep -q 'no es un SHA de 40 hex' "$T/err.txt" && sin_tocar_cluster && [ ! -s "$LOG/curl.log" ]; then
  echo "OK (a) TARGET_SHA mal formado: rc=$rc, sin llamadas al clúster ni a GHCR"
else
  echo "FALLA (a) TARGET_SHA mal formado: rc=$rc"; cat "$T/err.txt"; fail=1
fi

# (b) sin red: git ls-remote falla y no hay TARGET_SHA.
: >"$LOG/kubectl.log"; rm -f "$LOG/curl.log"
correr_e2e 30 MOCK_GIT_FALLA=1; rc=$?
if [ "$rc" != 0 ] && grep -q 'no se pudo leer el HEAD' "$T/err.txt" && sin_tocar_cluster; then
  echo "OK (b) sin red: rc=$rc, mensaje claro, sin tocar el clúster"
else
  echo "FALLA (b) sin red: rc=$rc"; cat "$T/err.txt"; fail=1
fi

# (c) SHA sin imagen en GHCR.
: >"$LOG/kubectl.log"; rm -f "$LOG/curl.log"
correr_e2e 30 TARGET_SHA="$SHA_OK" MOCK_GHCR_SIN_IMAGEN=1; rc=$?
if [ "$rc" != 0 ] && grep -q 'no hay imagen' "$T/err.txt" && sin_tocar_cluster && [ ! -f "$LOG/hook-body.json" ]; then
  echo "OK (c) SHA sin imagen: rc=$rc, no se firma ni envía webhook"
else
  echo "FALLA (c) SHA sin imagen: rc=$rc"; cat "$T/err.txt"; fail=1
fi

# (d) cuerpo firmado con el SHA elegido y el repo objetivo.
: >"$LOG/kubectl.log"; rm -f "$LOG/curl.log" "$LOG/hook-body.json" "$LOG/hook-headers.txt"
correr_e2e 90 TARGET_SHA="$SHA_OK"
if [ -s "$LOG/hook-body.json" ]; then
  body=$(cat "$LOG/hook-body.json")
  full=$(jq -r '.repository.full_name' <<<"$body"); after=$(jq -r '.after' <<<"$body")
  sig=$(sed -n 's/^.*X-Hub-Signature-256: \(sha256=[0-9a-f]*\).*$/\1/p' "$LOG/hook-headers.txt" | tr -d '\r')
  esperada="sha256=$(printf '%s' "$body" | openssl dgst -sha256 -hmac "$SECRETO" -hex | awk '{print $2}')"
  if [ "$full" = "$REPO" ] && [ "$after" = "$SHA_OK" ] && [ "$sig" = "$esperada" ]; then
    echo "OK (d) webhook firmado: full_name=$full after=$after firma válida"
  else
    echo "FALLA (d) webhook: full_name=$full after=$after sig_ok=$([ "$sig" = "$esperada" ] && echo si || echo no)"; fail=1
  fi
else
  echo "FALLA (d) no se capturó ningún webhook"; tail -5 "$T/err.txt"; fail=1
fi

if [ "$fail" = 0 ]; then echo OK; else echo FALLA; exit 1; fi
