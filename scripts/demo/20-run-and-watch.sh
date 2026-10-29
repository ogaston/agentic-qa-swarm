#!/usr/bin/env bash
# 20-run-and-watch.sh — conduce la corrida de la demo en vivo (docs/demo-real.md, Pasos 1-3) con un
# commit del repo objetivo ogaston/demo-target-app-qa-swarm y muestra los resultados:
#   webhook real (github-webhook-check.sh) -> notificación pending -> confirmación (humana en el
#   dashboard, o automática con --confirm-auto) -> fases de la corrida hasta done/failed ->
#   veredicto del reporte, imagen desplegada y reset_verified del warm.
#
# Uso:  bash scripts/demo/20-run-and-watch.sh <SHA40> [--confirm-auto]
#   <SHA40>         commit del repo objetivo (40 hex minúsculas) con imagen publicada en GHCR.
#   --confirm-auto  confirma la notificación contra ui-api (POST /notifications/{id}/confirm, como
#                   kind-e2e.sh) en lugar de pausar para la confirmación humana en el dashboard.
#   ${CONFIRM_TIMEOUT_S:-900}   espera máxima a la confirmación humana (solo sin --confirm-auto).
#   ${DEMO_RUN_TIMEOUT_S:-1800} espera máxima a que la corrida llegue a done/failed.
#
# Códigos de salida: 0 OK (corrida done sin fail_reason, imagen del SHA y reset verificado);
# 1 la corrida falló o el resumen no cuadra; 2 uso incorrecto o imagen ausente/no comprobable;
# 3 el contexto de kubectl no es kind-aqs.
#
# No crea pods auxiliares: el reporte se lee por ui-api (GET /reports/{run_id}), no de MinIO.
# Port-forwards propios en 127.0.0.1:18625-18627 (ui-api, go-identity, go-run-controller), cerrados
# siempre (trap). Respeta el lock compartido ${XDG_RUNTIME_DIR:-/tmp}/aqs-kind.lock como kind-e2e.sh
# (toma no bloqueante: si otro script de prueba corre, sale con 2).
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
# shellcheck disable=SC1091  # lib.sh se carga por ruta dinámica (relativa a root)
. "$root/scripts/kind/lib.sh"

NS_SYS=aqs-system
NS_TEST=aqs-test
P_UI=18625
P_IDENTITY=18626
P_RUN=18627
DASHBOARD_URL=http://127.0.0.1:18610   # vite preview de scripts/kind/dashboard-up.sh
CONFIRM_TIMEOUT_S=${CONFIRM_TIMEOUT_S:-900}
DEMO_RUN_TIMEOUT_S=${DEMO_RUN_TIMEOUT_S:-1800}

usage() {
  echo "uso: bash scripts/demo/20-run-and-watch.sh <SHA40> [--confirm-auto]" >&2
  exit 2
}

# --- argumentos ---------------------------------------------------------------
SHA=""
CONFIRM_AUTO=0
while [ $# -gt 0 ]; do
  case "$1" in
    --confirm-auto) CONFIRM_AUTO=1 ;;
    -h|--help) usage ;;
    *)
      if [ -z "$SHA" ] && [[ "$1" =~ ^[0-9a-f]{40}$ ]]; then SHA=$1; else usage; fi ;;
  esac
  shift
done
if [ -z "$SHA" ]; then
  echo "FALLA demo-corrida: falta el SHA del commit (40 hex minúsculas) del repo objetivo $TARGET_REPO_FULL" >&2
  usage
fi

command -v jq >/dev/null || { echo "falta jq" >&2; exit 2; }

# Guarda de contexto: sale 3 si no es kind-aqs (antes de cualquier otra operación).
require_kind_context

# --- lock del clúster (misma política no bloqueante que kind-e2e.sh) -----------
LOCKFILE="${XDG_RUNTIME_DIR:-/tmp}/aqs-kind.lock"
touch "$LOCKFILE" 2>/dev/null || true
exec 9>"$LOCKFILE" || { echo "no se pudo abrir $LOCKFILE" >&2; exit 2; }
if ! flock -n 9; then
  otros=""
  for pid in $(pgrep -f 'scripts/(kind|test|demo)/[a-z0-9_.-]+\.sh' 2>/dev/null); do
    # The interactive launcher is an ancestor, not a competing test.
    ancestor=$$
    while [ "$ancestor" != 0 ] && [ -r "/proc/$ancestor/status" ]; do
      [ "$pid" = "$ancestor" ] && break
      ancestor=$(awk '/^PPid:/ {print $2}' "/proc/$ancestor/status")
      ancestor=${ancestor:-0}
    done
    [ "$pid" = "$ancestor" ] && continue
    case $( { tr '\0' '\n' </proc/"$pid"/cmdline; } 2>/dev/null | sed -n 2p) in
      */scripts/kind/*.sh|*/scripts/test/*.sh|*/scripts/demo/*.sh|scripts/kind/*.sh|scripts/test/*.sh|scripts/demo/*.sh)
        otros="$otros $pid" ;;
    esac
  done
  if [ -n "$otros" ]; then
    echo "otro script de prueba corre ($otros) y el lock aqs-kind no se pudo tomar; se sale (rc=2)" >&2
    exit 2
  fi
  echo "20-run-and-watch: AVISO: $LOCKFILE tomado por residuos del bringup; se continúa sin él" >&2
fi

# --- precondición del guion: la imagen del commit ya existe en GHCR -----------
# docs/demo-real.md: NO confirmar antes de que la CI del repo objetivo publique la imagen.
if ! require_target_image "$SHA"; then
  echo "FALLA demo-corrida: sin imagen $TARGET_IMAGE_REPO:$SHA no se empuja el webhook ni se confirma (rc=2)" >&2
  exit 2
fi
echo "imagen verificada: $TARGET_IMAGE_REPO:$SHA"

# --- efímeros y limpieza -------------------------------------------------------
work=$(mktemp -d "${XDG_RUNTIME_DIR:-/tmp}/aqs-demo-run.XXXXXX")
chmod 700 "$work"
PF_PIDS=()
cleanup() {
  local p
  for p in "${PF_PIDS[@]:-}"; do [ -n "$p" ] && kill "$p" 2>/dev/null || true; done
  for p in "${PF_PIDS[@]:-}"; do [ -n "$p" ] && wait "$p" 2>/dev/null || true; done
  rm -rf "$work"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

# --- webhook real de GitHub -> notificación pending ----------------------------
echo "== paso 1: webhook de GitHub (github-webhook-check.sh) =="
if ! bash scripts/kind/github-webhook-check.sh "$SHA" >"$work/webhook.out" 2>&1; then
  sed 's/^/    /' "$work/webhook.out" >&2
  echo "FALLA webhook" >&2
  exit 1
fi
grep -E '^(OK|  notificación=)' "$work/webhook.out"
NOTIF_ID=$(sed -n 's/^  notificación=\([^ ]*\).*/\1/p' "$work/webhook.out" | head -n1)
if [ -z "$NOTIF_ID" ]; then
  echo "FALLA webhook: github-webhook-check.sh OK pero sin id de notificación en su salida" >&2
  exit 1
fi
echo "notificación: $NOTIF_ID (pending)"

# --- port-forwards propios (el webhook-check ya cerró los suyos) ---------------
pf() { # <svc> <puerto>
  kubectl --context "$KIND_CONTEXT" port-forward --address 127.0.0.1 -n "$NS_SYS" "svc/$1" "$2:8080" \
    >"$work/pf-$1.log" 2>&1 &
  PF_PIDS+=("$!")
}
wait_pf() { # <puerto>
  local i
  for i in $(seq 1 60); do
    curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$1/healthz" && return 0
    sleep 1
  done
  echo "port-forward $1 no responde /healthz" >&2
  tail -3 "$work"/pf-*.log >&2 2>/dev/null || true
  return 1
}
for p in "$P_UI" "$P_IDENTITY" "$P_RUN"; do
  if ss -ltn 2>/dev/null | grep -q "127.0.0.1:$p "; then
    echo "FALLA demo-corrida: 127.0.0.1:$p ya está ocupado (port-forward huérfano); se aborta" >&2
    exit 1
  fi
done
pf ui-api "$P_UI"
pf go-identity "$P_IDENTITY"
pf go-run-controller "$P_RUN"
if ! wait_pf "$P_UI" || ! wait_pf "$P_IDENTITY" || ! wait_pf "$P_RUN"; then
  echo "FALLA port-forwards" >&2
  exit 1
fi
echo "OK port-forwards (ui-api:$P_UI go-identity:$P_IDENTITY go-run-controller:$P_RUN)"

# --- login del usuario demo (la contraseña no sale del archivo de estado) ------
pw=$(sed -n 2p "$AQS_KIND_STATE_DIR/demo.txt" 2>/dev/null) || pw=""
if [ -z "$pw" ]; then
  echo "FALLA login: no hay contraseña demo en $AQS_KIND_STATE_DIR/demo.txt (ejecuta scripts/kind/secrets.sh)" >&2
  exit 1
fi
jq -n --arg u demo --arg p "$pw" '{username: $u, password: $p}' >"$work/login.json" || exit 1
code=$(curl -sS -o "$work/login.resp" -w '%{http_code}' --max-time 10 -H 'Content-Type: application/json' \
  --data @"$work/login.json" "http://127.0.0.1:$P_IDENTITY/auth/login") || code=000
rm -f "$work/login.json"
if [ "$code" != 200 ]; then
  echo "FALLA login: POST /auth/login respondió $code" >&2
  exit 1
fi
TOKEN=$(jq -r '.token // empty' "$work/login.resp")
[ -n "$TOKEN" ] || { echo "FALLA login: respuesta sin token" >&2; exit 1; }
echo "OK login-demo"

# --- paso 2: confirmación ------------------------------------------------------
RUN_ID=""
if [ "$CONFIRM_AUTO" = 1 ]; then
  echo "== paso 2: confirmación automática (POST /notifications/$NOTIF_ID/confirm) =="
  jq -n '{flows: ["happy-path"]}' >"$work/confirm.json" || exit 1
  code=$(curl -sS -o "$work/receipt.json" -w '%{http_code}' --max-time 15 \
    -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
    --data @"$work/confirm.json" "http://127.0.0.1:$P_UI/notifications/$NOTIF_ID/confirm") || code=000
  rm -f "$work/confirm.json"
  if [ "$code" != 201 ]; then
    echo "FALLA confirmación: POST /notifications/$NOTIF_ID/confirm respondió $code: $(head -c 200 "$work/receipt.json")" >&2
    exit 1
  fi
  RUN_ID=$(jq -r '.run_id // empty' "$work/receipt.json")
  [ -n "$RUN_ID" ] && [ "$RUN_ID" != null ] || { echo "FALLA confirmación: el 201 no trae run_id" >&2; exit 1; }
else
  echo "== paso 2: confirmación HUMANA =="
  echo "Abre el dashboard en $DASHBOARD_URL (si no está levantado: bash scripts/kind/dashboard-up.sh),"
  echo "entra como el usuario demo y confirma la notificación $NOTIF_ID (sha $SHA)."
  echo "Espero el recibo hasta ${CONFIRM_TIMEOUT_S}s (CONFIRM_TIMEOUT_S para cambiarlo)..."
  deadline=$(( $(date +%s) + CONFIRM_TIMEOUT_S ))
  while :; do
    code=$(curl -sS -o "$work/confirms.json" -w '%{http_code}' --max-time 10 \
      -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:$P_UI/confirmations") || code=000
    if [ "$code" = 200 ]; then
      RUN_ID=$(jq -r --arg n "$NOTIF_ID" '[.[] | select(.notification_id == $n)][0].run_id // empty' "$work/confirms.json" 2>/dev/null)
      [ -n "$RUN_ID" ] && [ "$RUN_ID" != null ] && break
      RUN_ID=""
    fi
    if [ "$(date +%s)" -ge "$deadline" ]; then
      echo "FALLA confirmación: ningún recibo para $NOTIF_ID en ${CONFIRM_TIMEOUT_S}s (¿confirmaste en el dashboard?)" >&2
      exit 1
    fi
    sleep 5
  done
fi
echo "OK confirmación: run_id=$RUN_ID"

# --- paso 3: vigilancia de la corrida hasta done/failed ------------------------
echo "== paso 3: corrida $RUN_ID (hasta ${DEMO_RUN_TIMEOUT_S}s; DEMO_RUN_TIMEOUT_S para cambiarlo) =="
LAST_STATE=""
FAIL_REASON=""
FINAL_STATE=""
t0=$(date +%s)
deadline=$(( t0 + DEMO_RUN_TIMEOUT_S ))
while :; do
  code=$(curl -sS -o "$work/run.json" -w '%{http_code}' --max-time 10 \
    -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:$P_RUN/runs/$RUN_ID") || code=000
  if [ "$code" = 200 ]; then
    st=$(jq -r '.state // empty' "$work/run.json")
    fr=$(jq -r '.fail_reason // empty' "$work/run.json")
    [ -n "$fr" ] && FAIL_REASON=$fr
    if [ "$st" != "$LAST_STATE" ]; then
      now=$(date +%s)
      if [ -z "$LAST_STATE" ]; then
        echo "fase: $st (t+$((now - t0))s)"
      else
        echo "fase: $LAST_STATE -> $st (t+$((now - t0))s)"
      fi
      [ -n "$fr" ] && echo "      fail_reason: $fr"
      LAST_STATE=$st
    fi
    case "$st" in
      done|failed) FINAL_STATE=$st; break ;;
    esac
  fi
  if [ "$(date +%s)" -ge "$deadline" ]; then
    echo "FALLA ${LAST_STATE:-desconocida}: la corrida no llegó a done/failed en ${DEMO_RUN_TIMEOUT_S}s" >&2
    exit 1
  fi
  sleep 3
done

# --- paso 4: resumen -----------------------------------------------------------
# La imagen desplegada en el warm, el veredicto del reporte (ui-api, no MinIO) y el reset del warm.
# F-01 de U8-T08: `resetting` no es un final feliz; una corrida con fail_reason sale por reset a
# `failed`. Aquí solo vale done sin fail_reason, y se muestra siempre el fail_reason observado.
IMG=$(kubectl --context "$KIND_CONTEXT" -n "$NS_TEST" get deploy warm-app -o jsonpath='{..image}' 2>/dev/null)
IMG_OK=no
case "$IMG" in *":$SHA") IMG_OK=si ;; esac

VERDICT=""
HALLAZGO=""
code=$(curl -sS -o "$work/report.json" -w '%{http_code}' --max-time 15 \
  -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:$P_UI/reports/$RUN_ID") || code=000
if [ "$code" = 200 ]; then
  VERDICT=$(jq -r '.verdict // empty' "$work/report.json")
  HALLAZGO=$(jq -r '[.findings[]?.invariant // empty] | join(" | ")' "$work/report.json" 2>/dev/null)
fi

WARM_STATE=""
RESET_VERIFIED=""
code=$(curl -sS -o "$work/warm.json" -w '%{http_code}' --max-time 10 \
  -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:$P_UI/warm") || code=000
if [ "$code" = 200 ]; then
  WARM_STATE=$(jq -r '.state // empty' "$work/warm.json")
  RESET_VERIFIED=$(jq -r '.reset_verified // empty' "$work/warm.json")
fi

echo "== resumen demo =="
printf '%-22s %s\n' "run_id"               "$RUN_ID"
printf '%-22s %s\n' "sha"                  "$SHA"
printf '%-22s %s\n' "estado corrida"       "${FINAL_STATE:-?}${FAIL_REASON:+ (fail_reason: $FAIL_REASON)}"
printf '%-22s %s\n' "imagen warm-app"      "${IMG:-?} (esperaba …:$SHA -> $IMG_OK)"
printf '%-22s %s\n' "veredicto"            "${VERDICT:-sin reporte (HTTP $code)}"
[ -n "$HALLAZGO" ] && printf '%-22s %s\n' "hallazgo/invariante" "$HALLAZGO"
printf '%-22s %s\n' "warm state"           "${WARM_STATE:-?}"
printf '%-22s %s\n' "reset_verified"       "${RESET_VERIFIED:-?}"

if [ "$FINAL_STATE" = done ] && [ -z "$FAIL_REASON" ] && [ "$IMG_OK" = si ] && [ "$RESET_VERIFIED" = true ]; then
  echo "OK demo-corrida-completa"
  exit 0
fi
if [ "$FINAL_STATE" = failed ]; then
  echo "FALLA ${LAST_STATE:-failed}${FAIL_REASON:+: $FAIL_REASON}"
  exit 1
fi
# done pero el resumen no cuadra (imagen distinta, reset no verificado o fail_reason presente)
echo "FALLA ${LAST_STATE:-resumen}: done sin cuadro completo (imagen_ok=$IMG_OK reset_verified=${RESET_VERIFIED:-?} fail_reason=${FAIL_REASON:-ninguno})"
exit 1
