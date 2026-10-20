#!/usr/bin/env bash
# kind-e2e.sh (U8-T08) — demo del ciclo completo de QA en kind, de punta a punta.
#
# Ciclo real sobre la plataforma desplegada (contexto kind-aqs), leyendo SIEMPRE el sistema
# (no el log de este script):
#   POST /webhooks/github firmado (HMAC)            -> go-intake  202 con la notificacion
#   GET  /notifications (ui-api)                    -> la notificacion queda en el inbox
#   POST /notifications/{id}/confirm (ui-api)       -> 201 con Receipt {run_id}
#   GET  /runs/{run_id} (go-run-controller)         -> confirmed->warm_ready->deploying->
#                                                      inferring->rehearsing->running->
#                                                      resetting->reporting->done
#   GET  /deploys/{run_id} (go-warm-manager)        -> estado del deploy sobre el warm
#   POST /surface        (go-warm-manager)          -> superficie externa (incluye run_id)
#   Jobs en aqs-test con aqs.io/run-id=<run>         -> runners (fase run) y ensayo (fase rehearse)
#   MinIO s3://evidence/runs/<run>/...              -> logs.txt, result.json, report-*.json
#   GET  /warm (ui-api)                             -> {state, reset_verified}
#
# Escenarios:
#   --bug off        ciclo nominal; veredicto sin hallazgos (todos los flujos pasan).
#   --bug oversell   la app demo se parchea con TARGET_APP_BUG=oversell DESPUES del ensayo
#                    (deployment warm-app de aqs-test; se revierte SIEMPRE con trap). BLOQUEADO
#                    POR CONTRATO (U8-T08 ronda 2, candidata): el defecto solo se activa con un
#                    POST /orders con cuerpo JSON (qty), que flow-plan no expresa; y el flujo que
#                    debe fallar hace fallar el ensayo, que corre el mismo plan antes del defecto.
#                    Este escenario imprime FALLA en ensayo-job y siguientes; no fabrica el veredicto.
#   --sin-confirmar  webhook valido, NINGUNA confirmacion: la notificacion sigue `pending` en el
#                    inbox, no hay recibo y no nace ningun Job de corrida.
#
# Fixtures fake (U8-T05) y el run_id REAL: el FakeLLM de los agentes falla cerrado salvo para el
# prompt canonico exacto. Las fixtures commiteadas llevan run-id de siembra, asi que este script
# RE-GENERA la del planner (build_prompt(surface, workflow) del propio repo) con la superficie real
# del warm-manager y la del reporter (prepare() del propio repo) con la evidencia real de MinIO.
# La del planner necesita la superficie (fase infer, despues del deploy); la del reporter necesita
# la evidencia ya escrita (justo tras los runners, antes de la fase report). En esa ventana el
# controlador se pausa (scale 0) y se reanuda (scale 1): su diario es un PVC, el estado sobrevive.
#
# Puertos en loopback 18615-18619; sin solape con el humo (18600-18606), el dashboard
# (18600/18601/18603 + vite 18610) ni scripts/test/u8-target-app-kind.sh (18611-18613), para que la
# demo pueda correr el e2e con el dashboard arriba. El lock compartido
# ${XDG_RUNTIME_DIR:-/tmp}/aqs-kind.lock serializa TODO lo que toca el cluster: flock(fd 9) al
# arrancar, se suelta al salir; si otro test lo tiene, se espera hasta 600 s y se sale con 2.
#
# Higiene (CA-4): al terminar no queda NADA de este script: port-forwards muertos, pods efimeros
# (mc/probe) borrados y esperados, bug del warm revertido y fixtures restauradas desde el arbol Git.
# La evidencia de la corrida en MinIO NO se borra (es la evidencia).
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
# shellcheck disable=SC1091  # lib.sh se carga por ruta dinamica (relativa a root)
# shellcheck source=/dev/null
. "$root/scripts/kind/lib.sh"

require_kind_context # sale 3 si el contexto actual no es kind-aqs, ANTES de tocar nada

# --- lock del cluster --------------------------------------------------------
# Serializa lo que toca el cluster entre codificadores. AVISO de entorno: el bringup de kind,
# cuando se corre dentro de flock (la practica documentada), hereda el fd del lock en los
# contenedores del nodo (aardvark-dns/conmon/rootlessport), que no lo sueltan hasta que el cluster
# muere: el lock queda tomado por procesos que ya no son tests. Por eso la toma es NO bloqueante:
# si el lock esta tomado y NINGUN otro test de codificador corre, se avisa y se continua; si otro
# test corre, se sale con 2 (no se corren dos pruebas a la vez). Ver bitacora U8-T08 (candidata).
LOCKFILE="${XDG_RUNTIME_DIR:-/tmp}/aqs-kind.lock"
touch "$LOCKFILE" 2>/dev/null || true
exec 9>"$LOCKFILE" || { echo "no se pudo abrir $LOCKFILE" >&2; exit 2; }
if flock -n 9; then
  :
else
  otros=""
  for pid in $(pgrep -f 'scripts/(kind|test)/[a-z0-9_.-]+\.sh' 2>/dev/null); do
    [ "$pid" = "$$" ] && continue
    # solo cuentan invocaciones reales (`bash <script> ...`), no el envoltorio `bash -c "..."`
    case $( { tr '\0' '\n' </proc/"$pid"/cmdline; } 2>/dev/null | sed -n 2p) in
      */scripts/kind/*.sh|*/scripts/test/*.sh|scripts/kind/*.sh|scripts/test/*.sh) otros="$otros $pid" ;;
    esac
  done
  if [ -n "$otros" ]; then
    echo "otro test de codificador corre ($otros) y el lock aqs-kind no se pudo tomar; se sale (rc=2)" >&2
    exit 2
  fi
  echo "kind-e2e: AVISO: $LOCKFILE esta tomado por residuos del bringup (fd heredado a los contenedores de kind); se continua sin el" >&2
fi

STATE_DIR="${XDG_RUNTIME_DIR:-/tmp}/aqs-kind"
NS_SYS=aqs-system
NS_TEST=aqs-test
WARM_DEPLOY=warm-app   # deployment del entorno warm (deploy/flux/base/warm.yaml)
P_INTAKE=18615
P_IDENTITY=18616
P_UI=18617
P_RUN=18618
P_WARM=18619
P_RESET=18620   # solo para el reset inicial del warm (warm_prepare); go-reset no se expone fuera
P_GOV=18621     # solo para dar de alta la politica workflows (gov_policy_seed)
DEMO_USER=demo
PLAZO_SIN_CONFIRMAR=${PLAZO_SIN_CONFIRMAR:-30}

# --- argumentos -------------------------------------------------------------
BUG=off
SIN_CONFIRMAR=0
KEEP=0
while [ $# -gt 0 ]; do
  case "$1" in
    --bug)
      [ $# -ge 2 ] || { echo "--bug requiere valor" >&2; exit 2; }
      BUG=$2; shift ;;
    --bug=*) BUG=${1#*=} ;;
    --sin-confirmar) SIN_CONFIRMAR=1 ;;
    --keep) KEEP=1 ;;
    *) echo "opcion desconocida: $1" >&2; exit 2 ;;
  esac
  shift
done
case "$BUG" in
  off|oversell) ;;
  *) echo "--bug solo vale off u oversell (recibi: $BUG)" >&2; exit 2 ;;
esac
if [ "$SIN_CONFIRMAR" = 1 ] && [ "$BUG" != off ]; then
  echo "--sin-confirmar excluye --bug oversell" >&2; exit 2
fi

STATE_DIR="${XDG_RUNTIME_DIR:-/tmp}/aqs-kind"
mkdir -p "$STATE_DIR" 2>/dev/null || true
work=$(mktemp -d "${STATE_DIR}/aqs-e2e.XXXXXX") || exit 1
chmod 700 "$work"

declare -a PF_PIDS=()
PF_RUN_PID=""
MC_POD=""
PROBE_POD=""
BUG_PATCHED=0
CTRL_PAUSED=0
BUG_PREV=""
BUG_PREV_SET=0
FIXTURES_DIRTY=""
fail=0
CHECKS_OK=0
RUN_ID=""
NOTIF_ID=""
AFTER=""
TOKEN=""
WARM_TOKEN=""
MC_POD_N=aqs-e2e-mc

# --- helpers de reporte ------------------------------------------------------
# check <nombre> <funcion...>: imprime OK <nombre> o FALLA <nombre> (+detalle a stderr).
# Las funciones corren en ESTE shell (no en un subshell), asi que pueden fijar RUN_ID/TOKEN.
check() {
  local name=$1
  shift
  if "$@" >"$work/last.out" 2>&1; then
    echo "OK $name"
    CHECKS_OK=$((CHECKS_OK + 1))
  else
    echo "FALLA $name"
    sed 's/^/    /' "$work/last.out" >&2
    fail=1
    if [ "$BUG" = off ] && [ "$SIN_CONFIRMAR" = 0 ]; then
      echo "kind-e2e: FALLA en escenario nominal (--bug off); se aborta con rc=1." >&2
      exit 1
    fi
  fi
}

expect_http() { # <codigo> <url> [args curl...]: cuerpo en $work/body
  local want=$1 url=$2 code
  shift 2
  code=$(curl -sS -o "$work/body" -w '%{http_code}' --max-time 15 "$@" "$url") || code=000
  if [ "$code" = "$want" ]; then
    return 0
  fi
  echo "HTTP $code (esperaba $want) $url"
  head -c 400 "$work/body" 2>/dev/null
  echo
  return 1
}

kcx() { kubectl --context "$KIND_CONTEXT" "$@"; }

# --- limpieza (CA-4: no deja NADA colgado; corre con o sin fallo) ------------
cleanup() {
  local rc=$?
  trap '' INT TERM
  if [ "$BUG_PATCHED" = 1 ]; then
    # Reversion FIEL: la base (deploy/flux/base/warm.yaml) NO define TARGET_APP_BUG. Si no existia
    # antes, se BORRA la entrada de la lista (`$patch: delete`); si existia, se restaura su valor.
    # Un `"value":null` dejaria una entrada vacia que ya no coincide con el manifiesto declarado.
    if [ "$BUG_PREV_SET" = 1 ]; then
      kcx -n "$NS_TEST" patch deployment "$WARM_DEPLOY" --type=strategic \
        -p "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"warm-app\",\"env\":[{\"name\":\"TARGET_APP_BUG\",\"value\":\"${BUG_PREV}\"}]}]}}}}" >/dev/null 2>&1 || true
    else
      kcx -n "$NS_TEST" patch deployment "$WARM_DEPLOY" --type=strategic \
        -p '{"spec":{"template":{"spec":{"containers":[{"name":"warm-app","env":[{"name":"TARGET_APP_BUG","$patch":"delete"}]}]}}}}' >/dev/null 2>&1 || true
    fi
    kcx -n "$NS_TEST" rollout status deployment/"$WARM_DEPLOY" --timeout=120s >/dev/null 2>&1 || true
  fi
  # el controlador no puede quedar pausado (0 replicas) si el escenario se corta en la ventana del planner
  if [ "$CTRL_PAUSED" = 1 ]; then
    kcx -n "$NS_SYS" scale deploy/go-run-controller --replicas=1 >/dev/null 2>&1 || true
    CTRL_PAUSED=0
  fi
  [ -n "$FIXTURES_DIRTY" ] && restore_fixtures
  [ -n "$MC_POD" ] && pod_down "$NS_SYS" "$MC_POD"
  [ -n "$PROBE_POD" ] && kcx delete pod "$PROBE_POD" -n "$NS_TEST" --ignore-not-found --wait=true --timeout=60s >/dev/null 2>&1 || true
  local p
  for p in "${PF_PIDS[@]:-}"; do
    [ -n "$p" ] && kill "$p" 2>/dev/null || true
  done
  for p in "${PF_PIDS[@]:-}"; do
    [ -n "$p" ] && wait "$p" 2>/dev/null || true
  done
  if [ "$KEEP" = 1 ]; then
    echo "kind-e2e: --keep: work dir conservado en $work" >&2
  else
    rm -rf "$work"
  fi
  echo "kind-e2e: checks OK=${CHECKS_OK} rc=${rc} fail=${fail}"
  if [ "$fail" -ne 0 ] || [ "$rc" -ne 0 ]; then
    exit 1
  fi
  exit 0
}
trap cleanup EXIT
trap 'exit 130' INT TERM

restore_fixtures() {
  local cm
  for cm in $FIXTURES_DIRTY; do
    case "$cm" in
      agents-fixtures-planner)
        kcx -n "$NS_SYS" create configmap agents-fixtures-planner \
          --from-file=target-app.json="$root/deploy/flux/kind/agents/planner/target-app.json" \
          --dry-run=client -o yaml 2>/dev/null | kcx apply -f - >/dev/null 2>&1 || true
        kcx -n "$NS_SYS" rollout restart deploy/agent-planner >/dev/null 2>&1 || true
        kcx -n "$NS_SYS" rollout status deploy/agent-planner --timeout=180s >/dev/null 2>&1 || true ;;
      agents-fixtures-reporter)
        kcx -n "$NS_SYS" create configmap agents-fixtures-reporter \
          --from-file=fixtures.json="$root/deploy/flux/kind/agents/reporter/fixtures.json" \
          --dry-run=client -o yaml 2>/dev/null | kcx apply -f - >/dev/null 2>&1 || true
        kcx -n "$NS_SYS" rollout restart deploy/agent-reporter >/dev/null 2>&1 || true
        kcx -n "$NS_SYS" rollout status deploy/agent-reporter --timeout=180s >/dev/null 2>&1 || true ;;
    esac
  done
}

# --- port-forwards -----------------------------------------------------------
# OJO: se arranca `kubectl` DIRECTO, no via la funcion kcx. En segundo plano, `kcx ... &` crearia
# un subshell y $! seria el pid del subshell: matarlo dejaria kubectl huerfano escuchando en el
# puerto (rompe la higiene de CA-4 y la siguiente corrida usaria el fantasma creyendo que arranco
# el suyo). Con el comando externo directo, $! es el pid de kubectl y `kill` lo tumba de verdad.
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
start_pf() {
  # Un puerto ya ocupado significa un port-forward huerfano de otra corrida: mejor fallar claro que
  # usar el fantasma y dar un OK falso.
  local p
  for p in "$P_INTAKE" "$P_IDENTITY" "$P_UI" "$P_RUN" "$P_WARM"; do
    if ss -ltn 2>/dev/null | grep -q "127.0.0.1:$p "; then
      echo "127.0.0.1:$p ya esta ocupado (port-forward huerfano); se aborta en vez de usarlo"
      return 1
    fi
  done
  pf go-intake "$P_INTAKE"
  pf go-identity "$P_IDENTITY"
  pf ui-api "$P_UI"
  pf go-run-controller "$P_RUN"
  PF_RUN_PID=${PF_PIDS[-1]}
  pf go-warm-manager "$P_WARM"
  wait_pf "$P_INTAKE" && wait_pf "$P_IDENTITY" && wait_pf "$P_UI" && wait_pf "$P_RUN" && wait_pf "$P_WARM"
}

# El port-forward del controlador muere cuando el Deployment se escala a 0 (ventana del reporter):
# la conexion al pod se corta y kubectl sale. Se rehace tras reanudar, o los checks siguientes
# (GET /runs) fallarian con «connection refused».
pf_run_restart() {
  [ -n "$PF_RUN_PID" ] && { kill "$PF_RUN_PID" 2>/dev/null || true; wait "$PF_RUN_PID" 2>/dev/null || true; }
  kubectl --context "$KIND_CONTEXT" port-forward --address 127.0.0.1 -n "$NS_SYS" "svc/go-run-controller" "$P_RUN:8080" \
    >"$work/pf-run.log" 2>&1 &
  PF_RUN_PID=$!
  PF_PIDS+=("$PF_RUN_PID")
  wait_pf "$P_RUN"
}

# --- auth demo ---------------------------------------------------------------
auth_token() {
  local pw
  pw=$(sed -n 2p "$STATE_DIR/demo.txt" 2>/dev/null) || pw=""
  [ -n "$pw" ] || { echo "no hay contrasena demo en $STATE_DIR/demo.txt (scripts/kind/secrets.sh)"; return 1; }
  jq -n --arg u "$DEMO_USER" --arg p "$pw" '{username: $u, password: $p}' >"$work/login.json" || return 1
  expect_http 200 "http://127.0.0.1:$P_IDENTITY/auth/login" -H 'Content-Type: application/json' --data @"$work/login.json" || return 1
  TOKEN=$(jq -r '.token // empty' "$work/body")
  [ -n "$TOKEN" ] || { echo "login sin token"; return 1; }
  expect_http 200 "http://127.0.0.1:$P_IDENTITY/auth/session" -H "Authorization: Bearer $TOKEN" || return 1
  [ "$(jq -r '.role // empty' "$work/body")" = user ] || { echo "sesion sin role=user"; return 1; }
  WARM_TOKEN=$(kcx -n "$NS_SYS" get secret go-warm-manager-service-token -o jsonpath='{.data.token}' 2>/dev/null | base64 -d)
  [ -n "$WARM_TOKEN" ] || { echo "secret go-warm-manager-service-token vacio"; return 1; }
}

# --- webhook push firmado (202) ----------------------------------------------
sha40() { printf '%s' "e2e-$(date +%s%N)-$$-$RANDOM" | sha1sum | cut -c1-40; }

post_webhook() {
  AFTER=$(sha40)
  local secret sig body code delivery
  secret=$(kcx -n "$NS_SYS" get secret go-intake-webhook -o jsonpath='{.data.secret}' | base64 -d)
  [ -n "$secret" ] || { echo "secret go-intake-webhook vacio"; return 1; }
  delivery=$(cat /proc/sys/kernel/random/uuid 2>/dev/null || printf 'd-%s-%s' "$$" "$(date +%s%N)")
  jq -n --arg a "$AFTER" '{ref:"refs/heads/main", after:$a, repository:{full_name:"ogaston/agentic-qa-swarm"}}' >"$work/hook.json" || return 1
  body=$(cat "$work/hook.json")
  sig="sha256=$(printf '%s' "$body" | openssl dgst -sha256 -hmac "$secret" -hex | awk '{print $2}')"
  code=$(curl -sS -o "$work/hook.out" -w '%{http_code}' --max-time 15 \
    -H 'Content-Type: application/json' -H "X-Hub-Signature-256: $sig" -H "X-GitHub-Event: push" \
    -H "X-GitHub-Delivery: $delivery" \
    --data "$body" "http://127.0.0.1:$P_INTAKE/webhooks/github") || code=000
  [ "$code" = 202 ] || { echo "POST /webhooks/github respondio $code (esperaba 202): $(head -c 200 "$work/hook.out")"; return 1; }
  NOTIF_ID=$(jq -r '.id // empty' "$work/hook.out")
  [ -n "$NOTIF_ID" ] && [ "$NOTIF_ID" != null ] || { echo "el 202 no trae notification.id: $(head -c 200 "$work/hook.out")"; return 1; }
  cp "$work/hook.out" "$work/notification.json"
}

# --- inbox y confirmacion ------------------------------------------------------
inbox_list() {
  expect_http 200 "http://127.0.0.1:$P_UI/notifications" -H "Authorization: Bearer $TOKEN" || return 1
  jq -e 'type == "array"' "$work/body" >/dev/null || { echo "GET /notifications no devuelve un array JSON"; return 1; }
  cp "$work/body" "$work/inbox.json"
}

inbox_check() { # la notificacion del webhook esta en el inbox, en estado pending
  inbox_list || return 1
  jq -e --arg n "$NOTIF_ID" '[.[] | select(.id == $n)] | length == 1' "$work/inbox.json" >/dev/null || {
    echo "la notificacion $NOTIF_ID no esta en el inbox: $(jq -c '[.[].id]' "$work/inbox.json")"
    return 1
  }
  local st
  st=$(jq -r --arg n "$NOTIF_ID" '[.[] | select(.id == $n)][0].state' "$work/inbox.json")
  [ "$st" = pending ] || { echo "la notificacion $NOTIF_ID no esta pending (state=$st)"; return 1; }
}

# --- precondicion: warm verificado (U8-T08, ronda 2) -----------------------------
# Un warm recien desplegado nace con reset_verified=false: go-reset solo lo pone en true al cerrar
# un reset exitoso (services/go-reset/internal/core/service.go:172). El gate de confirmed->warm_ready
# exige reset_verified=true (services/go-run-controller/runctl), asi que sin un reset inicial toda
# corrida muere en `confirmed` (medido en vivo: «hechos exigidos no cumplidos: reset_verified=false»).
# Este paso hace SOLO lo necesario: si GET /warm ya esta ready+verificado no toca nada; si no, pide
# el mismo reset que el ciclo haria al cerrar una corrida (POST /resets a go-reset, token
# go-reset-service-token) y exige reset_verified=true y state=ready al volver a leer /warm.
warm_prepare() {
  expect_http 200 "http://127.0.0.1:$P_UI/warm" -H "Authorization: Bearer $TOKEN" || return 1
  if [ "$(jq -r '.state // empty' "$work/body")" = ready ] && [ "$(jq -r '.reset_verified // empty' "$work/body")" = true ]; then
    echo "warm ya ready y verificado; no hace falta reset inicial"
    return 0
  fi
  local rtok rid code
  rtok=$(kcx -n "$NS_SYS" get secret go-reset-service-token -o jsonpath='{.data.token}' 2>/dev/null | base64 -d)
  [ -n "$rtok" ] || { echo "secret go-reset-service-token vacio"; return 1; }
  pf go-reset "$P_RESET" || return 1
  wait_pf "$P_RESET" || return 1
  rid="run-$(od -An -N8 -tx1 /dev/urandom | tr -d ' \n')"
  code=$(curl -sS -o "$work/reset.json" -w '%{http_code}' --max-time 300 -X POST \
    -H "Authorization: Bearer $rtok" -H 'Content-Type: application/json' \
    -d "{\"run_id\":\"$rid\"}" "http://127.0.0.1:$P_RESET/resets") || code=000
  [ "$code" = 200 ] || { echo "POST /resets (reset inicial) respondio $code: $(head -c 300 "$work/reset.json")"; return 1; }
  jq -e '.reset_verified == true' "$work/reset.json" >/dev/null || {
    echo "reset inicial sin reset_verified=true: $(head -c 300 "$work/reset.json")"; return 1
  }
  expect_http 200 "http://127.0.0.1:$P_UI/warm" -H "Authorization: Bearer $TOKEN" || return 1
  [ "$(jq -r '.state // empty' "$work/body")" = ready ] || { echo "warm tras el reset inicial no esta ready: $(head -c 200 "$work/body")"; return 1; }
  [ "$(jq -r '.reset_verified // empty' "$work/body")" = true ] || { echo "GET /warm tras el reset inicial: reset_verified!=true: $(head -c 200 "$work/body")"; return 1; }
}

# --- precondicion: politica `workflows` de go-governance (U8-T08, ronda 2) ----------
# En kind, go-governance arranca con policies.jsonl VACIO (0 bytes, medido): ninguna corrida pasa la
# fase run («workflow "happy-path" no está en la política workflows»). El workflow de la corrida es
# U3_DEFAULT_WORKFLOW del controlador. Se da de alta como lo haria el admin: login de `admin` con OTP
# (secreto local de scripts/kind/secrets.sh, TOTP RFC 6238 SHA-1 a 6 digitos) y PUT /policies/workflows
# con el workflow anadido a los que ya hubiera. Idempotente: si ya esta, no escribe. Las credenciales
# no se imprimen ni salen en la linea de comandos del curl (van por fichero).
totp_now() { # <secreto base32>: codigo TOTP actual (stdlib de python, sin dependencias)
  python3 -I -c '
import base64, hashlib, hmac, struct, sys, time
s = sys.argv[1].strip().upper().rstrip("=")
s += "=" * (-len(s) % 8)
h = hmac.new(base64.b32decode(s), struct.pack(">Q", int(time.time()) // 30), hashlib.sha1).digest()
o = h[-1] & 0x0F
print("%06d" % ((struct.unpack(">I", h[o:o + 4])[0] & 0x7FFFFFFF) % 1000000))' "$1"
}

gov_policy_seed() {
  local wf pw secret tok code existing merged try
  wf=$(workflow_default)
  pw=$(sed -n 2p "$STATE_DIR/admin.txt" 2>/dev/null) || pw=""
  secret=$(sed -n 1p "$STATE_DIR/admin-mfa-secret.txt" 2>/dev/null) || secret=""
  [ -n "$pw" ] && [ -n "$secret" ] || { echo "faltan credenciales de admin en $STATE_DIR (scripts/kind/secrets.sh)"; return 1; }
  pf go-governance "$P_GOV" || return 1
  wait_pf "$P_GOV" || return 1
  # login admin: un codigo TOTP solo vale una vez por ventana (lastOTP en go-identity); un reintento
  # espera la siguiente ventana de 30 s si el primero cayo por reutilizacion.
  tok=""
  for try in 1 2; do
    jq -n --arg p "$pw" --arg o "$(totp_now "$secret")" '{username: "admin", password: $p, otp: $o}' >"$work/admin-login.json" || return 1
    code=$(curl -sS -o "$work/admin-login.out" -w '%{http_code}' --max-time 15 \
      -H 'Content-Type: application/json' --data @"$work/admin-login.json" "http://127.0.0.1:$P_IDENTITY/auth/login") || code=000
    if [ "$code" = 200 ]; then tok=$(jq -r '.token // empty' "$work/admin-login.out"); break; fi
    sleep 31
  done
  rm -f "$work/admin-login.json"
  [ -n "$tok" ] || { echo "login de admin fallo (ultimo $code): $(head -c 200 "$work/admin-login.out")"; return 1; }
  code=$(curl -sS -o "$work/pol.json" -w '%{http_code}' --max-time 15 -H "Authorization: Bearer $tok" \
    "http://127.0.0.1:$P_GOV/policies/workflows") || code=000
  case "$code" in
    200) existing=$(jq -c '.value.workflows // []' "$work/pol.json") ;;
    404) existing='[]' ;;
    *) echo "GET /policies/workflows -> $code: $(head -c 200 "$work/pol.json")"; return 1 ;;
  esac
  if jq -e --arg w "$wf" 'any(.[]; .name == $w)' <<<"$existing" >/dev/null; then
    echo "politica workflows ya contiene $wf"
    return 0
  fi
  merged=$(jq -c --arg w "$wf" '. + [{name: $w, complexity: "low", max_runs_per_day: 1000, timeout_seconds: 3600, requires_approval: false}]' <<<"$existing") || return 1
  jq -n --argjson ws "$merged" '{value: {workflows: $ws}}' >"$work/policy-workflows.json" || return 1
  code=$(curl -sS -o "$work/pol-put.json" -w '%{http_code}' --max-time 15 -X PUT -H "Authorization: Bearer $tok" \
    -H 'Content-Type: application/json' --data @"$work/policy-workflows.json" "http://127.0.0.1:$P_GOV/policies/workflows") || code=000
  [ "$code" = 200 ] || { echo "PUT /policies/workflows -> $code: $(head -c 200 "$work/pol-put.json")"; return 1; }
  echo "politica workflows: alta de $wf (version $(jq -r '.version // "?"' "$work/pol-put.json"))"
}

confirm_notification() {
  jq -n '{flows: ["happy-path"]}' >"$work/confirm.json" || return 1
  expect_http 201 "http://127.0.0.1:$P_UI/notifications/$NOTIF_ID/confirm" \
    -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' --data @"$work/confirm.json" || return 1
  RUN_ID=$(jq -r '.run_id // empty' "$work/body")
  [ -n "$RUN_ID" ] && [ "$RUN_ID" != null ] || { echo "el 201 no trae run_id: $(head -c 200 "$work/body")"; return 1; }
}

# --- GET /runs/{id} en el controlador -----------------------------------------
run_get() {
  local code
  code=$(curl -sS -o "$work/run.json" -w '%{http_code}' --max-time 10 \
    -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:$P_RUN/runs/$RUN_ID") || code=000
  case "$code" in
    200) return 0 ;;
    404) echo "run $RUN_ID aun no existe (404)"; return 1 ;;
    *) echo "GET /runs/$RUN_ID -> $code: $(head -c 200 "$work/run.json")"; return 1 ;;
  esac
}

run_state() { jq -r '.state // empty' "$work/run.json" 2>/dev/null; }

run_is_failed() {
  run_get >/dev/null 2>&1 || return 1
  [ "$(run_state)" = failed ]
}

run_diag() {
  local code
  echo "-- run_id=$RUN_ID state=$(run_state)"
  kcx -n "$NS_SYS" logs deploy/go-run-controller --tail=300 2>/dev/null | grep -F "$RUN_ID" | tail -4
  code=$(curl -sS -o "$work/wm.out" -w '%{http_code}' --max-time 10 -H "Authorization: Bearer $WARM_TOKEN" \
    "http://127.0.0.1:$P_WARM/deploys/$RUN_ID") || code=000
  echo "-- go-warm-manager GET /deploys/$RUN_ID -> $code: $(head -c 200 "$work/wm.out")"
}

# wait_state <segundos> <estado...>: espera a que GET /runs/{id} quede en alguno; corta en failed.
wait_state() {
  local secs=$1 i=0 st
  shift
  while [ "$i" -lt "$secs" ]; do
    if run_get; then
      st=$(run_state)
      case " $* " in *" $st "*) return 0 ;; esac
      if [ "$st" = failed ]; then
        echo "la corrida quedo en failed (esperaba alguno de: $*)"
        run_diag
        return 1
      fi
    fi
    i=$((i + 2)); sleep 2
  done
  echo "no se alcanzo [$*] en ${secs}s (ultimo estado: ${st:-desconocido})"
  run_diag
  return 1
}

# --- deploy sobre el warm (GET /deploys/{run_id} en go-warm-manager) -----------
# Es la señal autoritativa de la fase deploy: done -> OK; failed -> FALLA con la razon;
# 400 -> FALLA (el warm-manager rechazo el run_id, p.ej. por longitud); 404 -> aun no arranca.
deploy_done() {
  local secs=${1:-300} i=0 code st
  while [ "$i" -lt "$secs" ]; do
    code=$(curl -sS -o "$work/deploy.json" -w '%{http_code}' --max-time 10 -H "Authorization: Bearer $WARM_TOKEN" \
      "http://127.0.0.1:$P_WARM/deploys/$RUN_ID") || code=000
    case "$code" in
      200)
        st=$(jq -r '.state // empty' "$work/deploy.json")
        [ "$st" = done ] && return 0
        if [ "$st" = failed ]; then
          echo "el deploy fallo: $(head -c 300 "$work/deploy.json")"
          return 1
        fi
        ;;
      400)
        echo "go-warm-manager GET /deploys/$RUN_ID -> 400: $(head -c 200 "$work/deploy.json")"
        return 1
        ;;
    esac
    i=$((i + 3)); sleep 3
  done
  echo "el deploy no llego a done en ${secs}s (ultimo: $code $(head -c 200 "$work/deploy.json"))"
  return 1
}

# --- superficie (POST /surface en go-warm-manager; incluye el run_id real) -----
warm_surface() { # reintenta hasta que el deploy termina y /surface responde 200
  local secs=${1:-180} i=0 code
  while [ "$i" -lt "$secs" ]; do
    if run_is_failed; then
      echo "la corrida ya esta en failed; no se produjo superficie"
      run_diag
      return 1
    fi
    code=$(curl -sS -o "$work/surface.json" -w '%{http_code}' --max-time 10 -H "Authorization: Bearer $WARM_TOKEN" \
      -H 'Content-Type: application/json' -d "{\"run_id\":\"$RUN_ID\"}" "http://127.0.0.1:$P_WARM/surface") || code=000
    if [ "$code" = 200 ]; then
      jq -e '.run_id and .base_url and (.endpoints | type == "array")' "$work/surface.json" >/dev/null || {
        echo "superficie invalida: $(head -c 200 "$work/surface.json")"; return 1
      }
      return 0
    fi
    i=$((i + 3)); sleep 3
  done
  echo "POST /surface no respondio 200 en ${secs}s (ultimo: $code $(head -c 120 "$work/surface.json"))"
  return 1
}

# --- fixture del planner para el run_id REAL -----------------------------------
py_bin() { # <pkg-src> [extra]: elige interprete con el paquete del repo importable
  local cand
  for cand in "$STATE_DIR/venv-$1/bin/python" "$root/agents/$2/.venv/bin/python" "$(command -v python3 || true)"; do
    [ -n "$cand" ] && [ -x "$cand" ] && { printf '%s' "$cand"; return 0; }
  done
  return 1
}

workflow_default() {
  local wf
  wf=$(kcx -n "$NS_SYS" get deploy go-run-controller -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="U3_DEFAULT_WORKFLOW")].value}' 2>/dev/null)
  [ -n "$wf" ] || wf="happy-path"
  printf '%s' "$wf"
}

regen_planner_fixture() {
  local py wf
  py=$(py_bin planner agent-planner) || { echo "no hay interprete para build_prompt del planner"; return 1; }
  wf=$(workflow_default)
  jq -e '(.endpoints | type == "array") and (.run_id != null)' "$work/surface.json" >/dev/null || {
    echo "superficie ausente o invalida: $(head -c 200 "$work/surface.json")"; return 1
  }
  PYTHONPATH="$root/agents/agent-planner/src" "$py" - "$RUN_ID" "$wf" "$work/surface.json" "$work/planner-fixture.json" "$BUG" <<'PY'
import json, sys
from pathlib import Path
from agent_planner.prompt import build_prompt

run, wf, surf_path, out, bug = sys.argv[1:6]
surface = json.loads(Path(surf_path).read_text())

has = {(e["method"], e["path"]) for e in surface.get("endpoints", [])}
def pick(*pairs):
    return [{"method": m, "path": p} for m, p in pairs if (m, p) in has]
def step(m, p, expect):
    return {"method": m, "path": p, "expect_status": expect}

if bug == "off":
    # Flujos que PASAN: solo lecturas GET 200 (el runner no envia cuerpos, asi que un POST
    # acabaria en 400; se usan GET para no depender de eso).
    flows = [
        {"flow_id": "flow-1", "name": "la app responde el health", "invariant": "GET /health responde 200",
         "steps": [step("GET", "/health", 200)]},
        {"flow_id": "flow-2", "name": "los productos se listan", "invariant": "GET /products responde 200",
         "steps": [step("GET", "/products", 200)]},
        {"flow_id": "flow-3", "name": "los pedidos se listan", "invariant": "GET /orders responde 200",
         "steps": [step("GET", "/orders", 200)]},
    ]
else:
    # --bug oversell: >=1 flujo que FALLA. Se pide 201 a un POST sin cuerpo (la app responde 400),
    # de modo que el flujo no pasa y el reporter puede declarar `bug` con flujo fallido.
    fails = pick(("POST", "/orders")) or [{"method": "POST", "path": "/orders"}]
    flows = [
        {"flow_id": "flow-1", "name": "el pedido se crea", "invariant": "el stock nunca es negativo tras una orden",
         "steps": [step(fails[0]["method"], fails[0]["path"], 201)]},
        {"flow_id": "flow-2", "name": "el health responde", "invariant": "GET /health responde 200",
         "steps": [step("GET", "/health", 200)]},
    ]

plan = {"run_id": run, "workflow": wf, "flows": flows}
# Forma que espera agent_planner.fakes.surfaces.register_surfaces: UN objeto por archivo (no una
# lista). Con una lista, doc["surface"] lanza TypeError, config.py lo convierte en ConfigError y el
# planner entra en CrashLoopBackOff (medido en vivo en U8-T08 ronda 2).
doc = {"surface": surface, "workflow": wf, "plan": plan}
Path(out).write_text(json.dumps(doc, ensure_ascii=True, indent=2) + "\n")
PY
  [ -s "$work/planner-fixture.json" ] || { echo "no se genero la fixture del planner"; return 1; }
}

# wait_no_terminating <etiqueta> [segundos]: espera a que no quede ningun pod de esa etiqueta en
# Terminating. `rollout status` da por bueno el rollout en cuanto la replica nueva esta Ready, pero la
# vieja sigue en el Service un rato: medido en vivo (U8-T08 ronda 2), el /v1/plan de la corrida caia
# en la replica vieja con la fixture anterior y respondia llm_unavailable.
wait_no_terminating() {
  local sel=$1 secs=${2:-120} i=0 left
  while [ "$i" -lt "$secs" ]; do
    left=$(kcx -n "$NS_SYS" get pods -l "$sel" -o jsonpath='{range .items[?(@.metadata.deletionTimestamp)]}{.metadata.name}{" "}{end}' 2>/dev/null)
    [ -z "$left" ] && return 0
    i=$((i + 1)); sleep 1
  done
  echo "pods con $sel aun en Terminating tras ${secs}s: $left"
  return 1
}

apply_planner_fixture() {
  kcx -n "$NS_SYS" create configmap agents-fixtures-planner \
    --from-file=target-app.json="$work/planner-fixture.json" \
    --dry-run=client -o yaml | kcx apply -f - >/dev/null || return 1
  FIXTURES_DIRTY="$FIXTURES_DIRTY agents-fixtures-planner"
  kcx -n "$NS_SYS" rollout restart deploy/agent-planner >/dev/null || return 1
  kcx -n "$NS_SYS" rollout status deploy/agent-planner --timeout=180s >/dev/null || return 1
  wait_no_terminating app.kubernetes.io/name=agent-planner 120 || return 1
}

# Envoltorio para `check`: las funciones corren en ESTE shell (con sus globales), no en un `bash -c`.
planner_fixture_apply() { regen_planner_fixture && apply_planner_fixture; }

# --- Jobs del controlador (runner: aqs.io/phase=run; ensayo: aqs.io/phase=rehearse) ----
jobs_phase_done() { # <phase>: hay >=1 Job de ese phase del run_id y todos los que hay terminaron ok
  local phase=$1 n done
  n=$(kcx -n "$NS_TEST" get jobs -l "aqs.io/run-id=$RUN_ID,aqs.io/phase=$phase" -o name 2>/dev/null | wc -l)
  [ "$n" -ge 1 ] || { echo "no hay Jobs de fase $phase para $RUN_ID"; return 1; }
  done=$(kcx -n "$NS_TEST" get jobs -l "aqs.io/run-id=$RUN_ID,aqs.io/phase=$phase" \
    -o jsonpath='{range .items[*]}{.status.succeeded}{"\n"}{end}' 2>/dev/null | grep -c '^[1-9]')
  [ "$done" -ge 1 ] || { echo "ninguno de los $n Jobs de fase $phase completo"; return 1; }
}

# Envoltorio para `check`: `jobs_phase_done rehearse` con el argumento fijo del ensayo.
ensayo_job_ok() { jobs_phase_done rehearse; }

# --- MinIO (mc dentro de un pod; usuario de agentes aqs-evidence) --------------
minio_up() { [ -n "$MC_POD" ] && return 0; MC_POD=$MC_POD_N; mc_pod_up "$NS_SYS" "$MC_POD_N"; }
# `mc ls --recursive <prefijo>/` imprime la ruta RELATIVA al prefijo (medido: «flow-1/logs.txt»); se
# normaliza a la clave completa «runs/<run>/flow-1/logs.txt» que usan download_evidence y los checks.
minio_ls() {
  local k
  mc_as aqs "$MC_POD" ls --recursive "m/evidence/runs/$RUN_ID/" 2>/dev/null | awk '{print $NF}' | grep -v '/$' | grep . |
    while IFS= read -r k; do
      case "$k" in
        runs/*) printf '%s\n' "$k" ;;
        *) printf 'runs/%s/%s\n' "$RUN_ID" "$k" ;;
      esac
    done
}

evidence_ok() {
  minio_up || return 1
  minio_ls | grep -E 'logs\.txt|result\.json' | head -1 | grep -q . || {
    echo "no hay evidencia logs.txt/result.json bajo runs/$RUN_ID/"; return 1
  }
}

download_evidence() { # baja la evidencia flow-*/ de MinIO a $work/evidence
  minio_up || return 1
  mkdir -p "$work/evidence" || return 1
  local key rel
  for key in $(minio_ls); do
    case "$key" in
      runs/"$RUN_ID"/flow-*/logs.txt|runs/"$RUN_ID"/flow-*/result.json) ;;
      *) continue ;;
    esac
    rel=${key#runs/$RUN_ID/}
    mkdir -p "$work/evidence/$(dirname "$rel")" || return 1
    mc_as aqs "$MC_POD" cat "m/evidence/$key" >"$work/evidence/$rel" || return 1
  done
  ls "$work/evidence"/flow-*/logs.txt >/dev/null 2>&1 || { echo "no baje evidencia flow-*"; return 1; }
}

regen_reporter_fixture() { # prepare() del propio repo sobre los bytes REALES de la evidencia
  local py
  py=$(py_bin reporter agent-reporter) || { echo "no hay interprete para prepare() del reporter"; return 1; }
  PYTHONPATH="$root/agents/agent-reporter/src" "$py" - "$RUN_ID" "$work/evidence" "$work/reporter-fixture.json" "$BUG" <<'PY'
import json, sys
from pathlib import Path
from urllib.parse import urlsplit
from agent_reporter.reporter import Limits, prepare

run, ev_dir, out, bug = sys.argv[1:5]
ev = Path(ev_dir)
BUCKET = "evidence"

class LocalReader:
    def get(self, uri: str) -> bytes:
        p = urlsplit(uri)
        prefix = f"/runs/{run}/"
        if p.scheme != "s3" or p.netloc != BUCKET or not p.path.startswith(prefix):
            raise RuntimeError("objeto fuera de la corrida")
        return (ev / p.path[len(prefix):]).read_bytes()

uris, n = [], 1
while True:
    f = ev / f"flow-{n}" / "logs.txt"
    r = ev / f"flow-{n}" / "result.json"
    if not f.is_file() or not r.is_file():
        break
    uris += [f"s3://{BUCKET}/runs/{run}/flow-{n}/logs.txt", f"s3://{BUCKET}/runs/{run}/flow-{n}/result.json"]
    n += 1
if not uris:
    raise SystemExit("sin evidencia flow-* para computar el prompt")
prep = prepare(run, uris, LocalReader(), Limits.from_env({}))
if bug == "off":
    resp = {"verdict": "sin-hallazgos", "summary": "la corrida e2e no encontro defectos de logica de negocio", "findings": []}
else:
    resp = {"verdict": "bug", "summary": "el stock quedo inconsistente tras el pedido de la corrida e2e",
            "findings": [{"finding_id": "flow-1",
                          "root_cause": "la app acepta el pedido sin respetar el stock disponible",
                          "invariant": "el stock nunca es negativo tras una orden",
                          "method": "POST", "path": "/orders",
                          "evidence_uris": [f"s3://{BUCKET}/runs/{run}/flow-1/result.json"]}]}
Path(out).write_text(json.dumps([{"run_id": run, "prompt": prep.prompt, "response": resp}], ensure_ascii=True, indent=2) + "\n")
PY
  [ -s "$work/reporter-fixture.json" ] || { echo "no se genero la fixture del reporter"; return 1; }
}

apply_reporter_fixture() {
  kcx -n "$NS_SYS" create configmap agents-fixtures-reporter \
    --from-file=fixtures.json="$work/reporter-fixture.json" \
    --dry-run=client -o yaml | kcx apply -f - >/dev/null || return 1
  FIXTURES_DIRTY="$FIXTURES_DIRTY agents-fixtures-reporter"
  kcx -n "$NS_SYS" rollout restart deploy/agent-reporter >/dev/null || return 1
  kcx -n "$NS_SYS" rollout status deploy/agent-reporter --timeout=180s >/dev/null || return 1
  wait_no_terminating app.kubernetes.io/name=agent-reporter 120 || return 1
}

pause_controller() {
  kcx -n "$NS_SYS" scale deploy/go-run-controller --replicas=0 >/dev/null || return 1
  CTRL_PAUSED=1
  kcx -n "$NS_SYS" wait --for=delete pod -l app.kubernetes.io/name=go-run-controller --timeout=120s >/dev/null 2>&1 || true
}
resume_controller() {
  kcx -n "$NS_SYS" scale deploy/go-run-controller --replicas=1 >/dev/null || return 1
  CTRL_PAUSED=0
  kcx -n "$NS_SYS" rollout status deploy/go-run-controller --timeout=180s >/dev/null || return 1
}

# Ventana del planner (U8-T08, ronda 2). El controlador llama a U3 (/v1/plan) en cuanto el warm
# tiene superficie: la fase infer es un POST /surface y la fase rehearse lanza el plan enseguida
# (medido: la llamada llegaba antes de que existiera la fixture del planner, que depende del run_id).
# Solucion: tras el POST /deploys del controlador, se pausa el controlador; el deploy, la superficie
# y la fixture se obtienen SIN el controlador (warm-manager es asincrono); luego se reanuda. Reanudar
# en `deploying` es seguro: WarmClient.Deploy no reenvia POST /deploys si el deploy ya existe.
planner_pause_wait() {
  local i=0 code=000
  while [ "$i" -lt 120 ]; do
    code=$(curl -sS -o /dev/null -w '%{http_code}' --max-time 10 -H "Authorization: Bearer $WARM_TOKEN" \
      "http://127.0.0.1:$P_WARM/deploys/$RUN_ID") || code=000
    [ "$code" = 200 ] && break
    i=$((i + 2)); sleep 2
  done
  [ "$code" = 200 ] || { echo "el controlador no emitio el deploy de $RUN_ID en 120s (ultimo $code)"; return 1; }
  pause_controller || { echo "no se pudo pausar el controlador"; return 1; }
}

resume_after_planner() { resume_controller && pf_run_restart; }

# Ventana del reporter: con la evidencia ya escrita, pausa el controlador, baja la evidencia,
# genera y aplica la fixture del reporter y SIEMPRE reanuda (aunque algo falle, el controlador
# no puede quedar en 0 replicas).
reporter_window() {
  pause_controller || { echo "no se pudo pausar el controlador"; return 1; }
  local rc=0
  if ! download_evidence || ! regen_reporter_fixture || ! apply_reporter_fixture; then
    rc=1
  fi
  resume_controller || rc=1
  pf_run_restart || rc=1
  return "$rc"
}

# --- bug del warm (oversell): deployment warm-app de aqs-test -------------------
patch_warm_bug() {
  kcx -n "$NS_TEST" get deploy "$WARM_DEPLOY" >/dev/null 2>&1 || { echo "no hay deployment $WARM_DEPLOY en $NS_TEST"; return 1; }
  # Estado previo (para revertir fiel): valor de TARGET_APP_BUG y si la entrada existia.
  local names
  BUG_PREV_SET=0
  BUG_PREV=""
  names=$(kcx -n "$NS_TEST" get deploy "$WARM_DEPLOY" -o jsonpath='{.spec.template.spec.containers[?(@.name=="warm-app")].env[*].name}' 2>/dev/null)
  if printf '%s' "$names" | tr ' ' '\n' | grep -qx 'TARGET_APP_BUG'; then
    BUG_PREV_SET=1
    BUG_PREV=$(kcx -n "$NS_TEST" get deploy "$WARM_DEPLOY" -o jsonpath='{.spec.template.spec.containers[?(@.name=="warm-app")].env[?(@.name=="TARGET_APP_BUG")].value}' 2>/dev/null)
  fi
  kcx -n "$NS_TEST" patch deployment "$WARM_DEPLOY" --type=strategic \
    -p '{"spec":{"template":{"spec":{"containers":[{"name":"warm-app","env":[{"name":"TARGET_APP_BUG","value":"oversell"}]}]}}}}' >/dev/null || return 1
  BUG_PATCHED=1
  kcx -n "$NS_TEST" rollout status deployment/"$WARM_DEPLOY" --timeout=180s >/dev/null || return 1
}

bug_active() {
  local envs
  envs=$(kcx -n "$NS_TEST" get deploy "$WARM_DEPLOY" -o jsonpath='{.spec.template.spec.containers[?(@.name=="warm-app")].env}' 2>/dev/null)
  printf '%s' "$envs" | grep -q 'TARGET_APP_BUG' || { echo "warm-app no tiene la env del bug"; return 1; }
  printf '%s' "$envs" | grep -q 'oversell' || { echo "TARGET_APP_BUG no vale oversell"; return 1; }
}

# Envoltorio para `check`: parchea el bug y comprueba que quedo activo (mismo shell, mismo trap).
bug_apply() { patch_warm_bug && bug_active; }

# --- warm / reset / reporte ----------------------------------------------------
# reset-verificado: la corrida debe haber pasado por `resetting` (esta en reporting/done) y el warm
# debe estar `ready` con `reset_verified=true` REAL. Sin atarlo al estado de la corrida, un warm ya
# limpio de una corrida previa daria un OK trivial.
reset_verified() {
  # el reset ocurre en `resetting`: se espera a que la corrida pase a reporting/done antes de leer el warm
  wait_state 240 reporting done || return 1
  local rs
  rs=$(run_state)
  case "$rs" in
    reporting|done) ;;
    *) echo "la corrida esta en ${rs:-?} (se esperaba reporting/done: garantiza que paso por resetting)"; return 1 ;;
  esac
  expect_http 200 "http://127.0.0.1:$P_UI/warm" -H "Authorization: Bearer $TOKEN" || return 1
  local st rv
  st=$(jq -r '.state // empty' "$work/body")
  rv=$(jq -r '.reset_verified // empty' "$work/body")
  [ "$st" = ready ] || { echo "warm state=${st:-vacio} (esperaba ready)"; return 1; }
  [ "$rv" = true ] || { echo "warm reset_verified=${rv:-vacio} (esperaba true)"; return 1; }
}

report_uri_ok() {
  run_get || return 1
  local uri key
  uri=$(jq -r '.report_uri // empty' "$work/run.json")
  [ -n "$uri" ] && [ "$uri" != null ] || { echo "GET /runs no expone report_uri"; return 1; }
  key=${uri#s3://evidence/}
  [ "$key" != "$uri" ] || { echo "report_uri inesperado: $uri"; return 1; }
  minio_up || return 1
  mc_as aqs "$MC_POD" stat "m/evidence/$key" >/dev/null || { echo "el reporte no esta en MinIO ($uri)"; return 1; }
  mc_as aqs "$MC_POD" cat "m/evidence/$key" >"$work/report.json" || return 1
  jq -e '.verdict and (.findings | type == "array")' "$work/report.json" >/dev/null || {
    echo "el reporte no tiene verdict/findings: $(head -c 200 "$work/report.json")"; return 1
  }
}

run_verdict_nonempty() { # GET /runs/{id} del controlador expone el veredicto
  run_get || return 1
  local v
  v=$(jq -r '.verdict // empty' "$work/run.json")
  [ -n "$v" ] && [ "$v" != null ] || { echo "GET /runs no expone verdict"; return 1; }
}

report_present() {
  [ -s "$work/report.json" ] || { echo "no hay reporte bajado de MinIO (reporte-en-minio fallo antes)"; return 1; }
}

verdict_sin_hallazgos() {
  run_verdict_nonempty || return 1
  report_present || return 1
  jq -e '.verdict == "sin-hallazgos"' "$work/report.json" >/dev/null || { echo "verdict=$(jq -r '.verdict' "$work/report.json" 2>/dev/null) (esperaba sin-hallazgos)"; return 1; }
  jq -e '(.findings | length) == 0' "$work/report.json" >/dev/null || { echo "el reporte trae hallazgos en modo off"; return 1; }
}

verdict_bug() {
  run_verdict_nonempty || return 1
  report_present || return 1
  jq -e '.verdict == "bug"' "$work/report.json" >/dev/null || { echo "verdict=$(jq -r '.verdict' "$work/report.json" 2>/dev/null) (esperaba bug)"; return 1; }
  local n
  n=$(jq -r '.findings | length' "$work/report.json" 2>/dev/null)
  [ "${n:-0}" -ge 1 ] 2>/dev/null || { echo "el reporte bug no trae hallazgos"; return 1; }
}

hallazgo_cita_invariante_y_evidencia() {
  report_present || return 1
  jq -e '.findings[0].invariant | test("stock")' "$work/report.json" >/dev/null || {
    echo "el hallazgo no cita la invariante de stock: $(jq -c '.findings[0].invariant' "$work/report.json" 2>/dev/null)"; return 1
  }
  local uri key
  uri=$(jq -r '.findings[0].evidence_uris[0] // empty' "$work/report.json")
  [ -n "$uri" ] && [ "$uri" != null ] || { echo "el hallazgo no cita evidence_uri"; return 1; }
  key=${uri#s3://evidence/}
  minio_up || return 1
  mc_as aqs "$MC_POD" stat "m/evidence/$key" >/dev/null || { echo "evidence_uri inexistente en MinIO: $uri"; return 1; }
}

# --- puerta humana (--sin-confirmar) -------------------------------------------
sin_confirmar_no_crea_corrida() {
  local before after out
  before=$(kcx -n "$NS_TEST" get jobs -l 'aqs.io/run-id' -o name 2>/dev/null | wc -l)
  sleep "$PLAZO_SIN_CONFIRMAR"
  # 1) la notificacion sigue pendiente en el inbox
  inbox_check || return 1
  # 2) ningun recibo la confirma
  expect_http 200 "http://127.0.0.1:$P_UI/confirmations" -H "Authorization: Bearer $TOKEN" || return 1
  if jq -e --arg n "$NOTIF_ID" '[.[] | select(.notification_id == $n)] | length == 0' "$work/body" >/dev/null; then
    :
  else
    echo "hay un recibo (confirmacion) para la notificacion $NOTIF_ID sin haberla confirmado"
    return 1
  fi
  # 3) ningun Job nuevo de corrida con etiqueta aqs.io/run-id
  after=$(kcx -n "$NS_TEST" get jobs -l 'aqs.io/run-id' -o name 2>/dev/null | wc -l)
  [ "$before" = "$after" ] || { echo "aparecieron Jobs de corrida tras el plazo (antes=$before ahora=$after)"; return 1; }
}

# =============================================================================
echo "== kind-e2e (U8-T08) :: escenario bug=$BUG sin-confirmar=$SIN_CONFIRMAR =="

command -v jq >/dev/null || { echo "falta jq"; exit 2; }
command -v openssl >/dev/null || { echo "falta openssl"; exit 2; }

check port-forwards start_pf
check login-demo auth_token

check webhook-202 post_webhook
check inbox inbox_check

if [ "$SIN_CONFIRMAR" = 1 ]; then
  check sin-confirmar-no-crea-corrida sin_confirmar_no_crea_corrida
  if [ "$fail" = 0 ] && [ "$CHECKS_OK" -ge 3 ]; then
    echo "kind-e2e: escenario sin-confirmar OK (checks OK=${CHECKS_OK})"
    exit 0
  fi
  echo "kind-e2e: escenario sin-confirmar FAIL (checks OK=${CHECKS_OK})"
  exit 1
fi

check politica-workflow gov_policy_seed
check warm-verificado warm_prepare
check confirm confirm_notification
if [ "$fail" != 0 ] || [ -z "$RUN_ID" ]; then
  echo "kind-e2e: sin confirmacion no hay ciclo (checks OK=${CHECKS_OK})"
  exit 1
fi

# deploy (fase deploy sobre el warm) -> infer -> rehearse -> run -> reset -> report -> done
check ventana-planner planner_pause_wait
check deploy deploy_done 300
check superficie warm_surface 180
check planner-fixture planner_fixture_apply
check reanuda-controlador resume_after_planner
check plan wait_state 240 running resetting reporting done
check ensayo wait_state 240 running resetting reporting done
check ensayo-job ensayo_job_ok

if [ "$BUG" = oversell ]; then
  check bug-activo bug_apply
fi

check runners wait_state 300 resetting reporting done
check evidencia-en-minio evidence_ok

# ventana del reporter: pausar el controlador, bajar evidencia, generar y aplicar la fixture
check reporter-fixture reporter_window

check reset-verificado reset_verified
check reporte-en-minio report_uri_ok

if [ "$BUG" = oversell ]; then
  check veredicto-bug verdict_bug
  check hallazgo-cita-invariante-y-evidencia hallazgo_cita_invariante_y_evidencia
else
  check veredicto-sin-hallazgos verdict_sin_hallazgos
fi

if [ "$fail" != 0 ]; then
  echo "kind-e2e: FAIL (ver las lineas FALLA; checks OK=${CHECKS_OK})"
  exit 1
fi
if [ "$BUG" = off ] && [ "$CHECKS_OK" -lt 12 ]; then
  echo "kind-e2e: FAIL: ${CHECKS_OK} checks OK < 12 (CA-1)"
  exit 1
fi
echo "kind-e2e: OK (escenario bug=$BUG; checks OK=${CHECKS_OK})"
exit 0
