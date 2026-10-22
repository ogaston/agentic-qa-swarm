#!/usr/bin/env bash
# Comprueba la entrega REAL de GitHub (U8-T09, CA-3): un push del humano llega por el túnel cloudflared a
# go-intake, se valida (HMAC) y aparece en el inbox de ui-api con el SHA empujado.
#
# Uso:  bash scripts/kind/github-webhook-check.sh [<sha-40>]
#   Sin <sha>: toma el HEAD remoto de la rama ${WEBHOOK_BRANCH:-main} (git ls-remote origin).
#   ${WEBHOOK_TIMEOUT_S:-600}: espera máxima a que la notificación aparezca (sondeo cada 5 s).
#
# Abre port-forwards propios en 127.0.0.1:18625 (ui-api) y 127.0.0.1:18626 (go-identity) y los cierra
# siempre (trap). Imprime `OK webhook-github-recibido` o `FALLA webhook-github-recibido: <motivo>`.
# Sale 0 solo si la notificación con ese SHA aparece en el inbox; 1 si no; 2 con un SHA mal formado; 3 con
# el contexto de kubectl equivocado.
#
# No imprime secretos: ni el token del inbox, ni la clave del HMAC, ni la contraseña demo. El paso del
# humano (el secreto del webhook en GitHub y el túnel) está en docs/operaciones/webhook-github-cloudflared.md.
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
# shellcheck disable=SC1091  # lib.sh se carga por ruta dinámica (relativa a root)
. "$root/scripts/kind/lib.sh"

NS_SYS=aqs-system
P_UI=18625
P_IDENTITY=18626
TIMEOUT_S="${WEBHOOK_TIMEOUT_S:-600}"
BRANCH="${WEBHOOK_BRANCH:-main}"

work=$(mktemp -d "${XDG_RUNTIME_DIR:-/tmp}/aqs-webhook.XXXXXX")
chmod 700 "$work"
pids=()
cleanup() {
  local p
  for p in "${pids[@]:-}"; do [ -n "$p" ] && kill "$p" 2>/dev/null; done
  rm -rf "$work"
}
trap cleanup EXIT

fail_msg() { echo "FALLA webhook-github-recibido: $*"; exit 1; }

# Guarda de contexto: sale 3 si no es kind-aqs (antes de cualquier otra operación).
require_kind_context

# 1. SHA esperado: argumento o HEAD remoto de la rama.
sha="${1:-}"
if [ -z "$sha" ]; then
  sha=$(git ls-remote origin "refs/heads/$BRANCH" 2>/dev/null | awk 'NR==1{print $1}')
fi
if ! [[ "$sha" =~ ^[0-9a-f]{40}$ ]]; then
  echo "FALLA webhook-github-recibido: SHA esperado ausente o mal formado (pásalo como argumento de 40 hex)"
  exit 2
fi
echo "sha esperado: $sha (rama $BRANCH)"

# 2. Port-forwards propios (ui-api para el inbox, go-identity para el login del usuario demo).
kubectl port-forward --address 127.0.0.1 -n "$NS_SYS" svc/ui-api "$P_UI:8080" >"$work/pf-ui.log" 2>&1 &
pids+=("$!")
kubectl port-forward --address 127.0.0.1 -n "$NS_SYS" svc/go-identity "$P_IDENTITY:8080" >"$work/pf-id.log" 2>&1 &
pids+=("$!")
for i in $(seq 1 60); do
  if curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$P_UI/healthz" && \
     curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$P_IDENTITY/healthz"; then
    break
  fi
  sleep 1
  [ "$i" -lt 60 ] || fail_msg "los port-forwards de ui-api y go-identity no responden"
done

# 3. Login del usuario demo (la contraseña no sale del archivo de estado).
pw=$(sed -n 2p "$AQS_KIND_STATE_DIR/demo.txt" 2>/dev/null) || pw=""
[ -n "$pw" ] || fail_msg "no hay contraseña demo en $AQS_KIND_STATE_DIR/demo.txt (ejecuta scripts/kind/secrets.sh)"
jq -n --arg u demo --arg p "$pw" '{username: $u, password: $p}' >"$work/login.json" || fail_msg "no se pudo preparar el login"
code=$(curl -s -o "$work/login.resp" -w '%{http_code}' --max-time 10 -H 'Content-Type: application/json' \
  --data @"$work/login.json" "http://127.0.0.1:$P_IDENTITY/auth/login") || code=000
[ "$code" = 200 ] || fail_msg "POST /auth/login respondió $code"
token=$(jq -r '.token // empty' "$work/login.resp")
[ -n "$token" ] || fail_msg "POST /auth/login sin token"

# 4. Sondeo del inbox: una notificación con ese SHA (la entrega de GitHub validada y transportada por NATS).
deadline=$(( $(date +%s) + TIMEOUT_S ))
while :; do
  code=$(curl -s -o "$work/inbox.json" -w '%{http_code}' --max-time 10 \
    -H "Authorization: Bearer $token" "http://127.0.0.1:$P_UI/notifications") || code=000
  if [ "$code" = 200 ]; then
    hit=$(jq -c --arg s "$sha" '[.[] | select(.sha == $s)] | .[0] // empty' "$work/inbox.json" 2>/dev/null)
    if [ -n "$hit" ]; then
      id=$(jq -r '.id' <<<"$hit")
      ev=$(jq -r '.github_event' <<<"$hit")
      repo=$(jq -r '.repo' <<<"$hit")
      state=$(jq -r '.state' <<<"$hit")
      echo "OK webhook-github-recibido"
      echo "  notificación=$id evento=$ev repo=$repo estado=$state sha=$sha"
      exit 0
    fi
  fi
  [ "$(date +%s)" -lt "$deadline" ] || break
  sleep 5
done
fail_msg "no llegó ninguna notificación con el SHA $sha en ${TIMEOUT_S}s (revisa las entregas en GitHub y el túnel; ver docs/operaciones/webhook-github-cloudflared.md)"
