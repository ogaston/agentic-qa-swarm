#!/usr/bin/env bash
# 00-setup.sh — «Paso 0 asistido» de la demo en vivo (docs/demo-real.md) con el repo objetivo real
# ogaston/demo-target-app-qa-swarm. Deja el entorno listo y comprueba las precondiciones:
#   1. clúster kind `aqs` (contexto kind-aqs) existe
#   2. despliegue de la plataforma: pods clave Running en aqs-system y aqs-test
#      (no redespliega por sí solo; con --deploy corre scripts/kind/deploy.sh si falta)
#   3. Secrets generados (incluido go-intake-webhook)
#   4. cloudflared instalado y túnel activo hacia go-intake (port-forward loopback 18622);
#      si no corre, lo arranca en segundo plano (nohup, log y pid en /tmp) e imprime la URL pública
#   5. red a ghcr.io: pull anónimo de la imagen del HEAD de main del repo objetivo alcanzable
#   6. humo: bash scripts/kind/kind-smoke.sh --preflight con 0 líneas FALLA (sin confirmar corridas)
# Cierra con el resumen de lo que el humano hace a mano (webhook en GitHub, clon del repo objetivo)
# y la URL del dashboard.
#
# Uso:  bash scripts/demo/00-setup.sh [--deploy]
# Códigos de salida: 0 todo OK · 1 alguna comprobación FALLA · 2 lock ocupado por otro test ·
#   3 contexto de kubectl equivocado · 4 el clúster kind-aqs no existe.
#
# Idempotente: no recrea lo que ya está (túnel, port-forward, despliegue, secretos). Nunca imprime
# valores de Secrets. Las esperas sobre el clúster respetan el lock compartido
# ${XDG_RUNTIME_DIR:-/tmp}/aqs-kind.lock con la misma pauta de kind-e2e.sh: toma no bloqueante; si lo
# tiene otro script de prueba vivo se sale con 2; si lo tienen residuos del bringup, se avisa y sigue.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
# shellcheck disable=SC1091  # lib.sh se carga por ruta dinámica (relativa a root)
. "$root/scripts/kind/lib.sh"

DEPLOY=0
case "${1:-}" in
  --deploy) DEPLOY=1 ;;
  "") ;;
  *) echo "uso: bash scripts/demo/00-setup.sh [--deploy]" >&2; exit 2 ;;
esac

NS_SYS=aqs-system
NS_TEST=aqs-test
P_INTAKE=18622   # loopback de go-intake para el túnel (docs/operaciones/webhook-github-cloudflared.md)
TUNNEL_NAME=aqs-demo
CF_LOG=/tmp/aqs-demo-cloudflared.log
CF_PID=/tmp/aqs-demo-cloudflared.pid
PF_LOG=/tmp/aqs-demo-pf-intake.log
PF_PID=/tmp/aqs-demo-pf-intake.pid

fail=0
work=$(mktemp -d "${XDG_RUNTIME_DIR:-/tmp}/aqs-demo-setup.XXXXXX")
chmod 700 "$work"
trap 'rm -rf "$work"' EXIT

# check <nombre> <función...>: imprime OK o FALLA y el detalle indentado (pauta de kind-smoke.sh).
check() {
  local name=$1
  shift
  if "$@" >"$work/last.out" 2>&1; then
    echo "OK $name"
  else
    echo "FALLA $name"
    sed 's/^/    /' "$work/last.out"
    fail=1
  fi
}

# --- 1. Clúster ---------------------------------------------------------------
if ! kind_cluster_exists; then
  echo "FALLA cluster-kind-aqs"
  echo "    el clúster '$KIND_CLUSTER_NAME' no existe; créalo con: bash scripts/kind/kind-up.sh"
  exit 4
fi
echo "OK cluster-kind-aqs"

require_kind_context # sale 3 si el contexto actual no es kind-aqs (antes de tocar el clúster)
echo "OK contexto-$KIND_CONTEXT"

# --- 1b. Nodo arrancado (el contenedor queda Exited tras reiniciar la máquina) -
ensure_control_plane() {
  local engine="" c="${KIND_CLUSTER_NAME}-control-plane" i
  if kubectl --context "$KIND_CONTEXT" --request-timeout=5s get --raw=/healthz >/dev/null 2>&1; then
    return 0
  fi
  echo "la API del clúster no responde; se intenta arrancar el contenedor del nodo" >&2
  for e in podman docker; do
    if command -v "$e" >/dev/null 2>&1; then engine=$e; break; fi
  done
  if [ -z "$engine" ]; then
    echo "ni podman ni docker disponibles para arrancar el nodo" >&2
    return 1
  fi
  if ! "$engine" inspect "$c" >/dev/null 2>&1; then
    echo "el contenedor '$c' no existe; recrea el clúster con: bash scripts/kind/kind-up.sh" >&2
    return 1
  fi
  "$engine" start "$c" >&2 || return 1
  for i in $(seq 1 12); do
    if kubectl --context "$KIND_CONTEXT" --request-timeout=5s get --raw=/healthz >/dev/null 2>&1; then
      return 0
    fi
    sleep 5
  done
  echo "la API sigue sin responder 60 s tras arrancar '$c'" >&2
  return 1
}
check nodo-arrancado ensure_control_plane
[ "$fail" = 0 ] || { echo "setup: FALLA (el clúster no está operativo; el resto de comprobaciones se salta)"; exit 1; }

# --- lock del clúster (misma pauta que kind-e2e.sh) ----------------------------
LOCKFILE="${XDG_RUNTIME_DIR:-/tmp}/aqs-kind.lock"
touch "$LOCKFILE" 2>/dev/null || true
exec 9>"$LOCKFILE" || { echo "no se pudo abrir $LOCKFILE" >&2; exit 2; }
if flock -n 9; then
  :
else
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
      */scripts/kind/*.sh|*/scripts/test/*.sh|*/scripts/demo/*.sh|scripts/kind/*.sh|scripts/test/*.sh|scripts/demo/*.sh) otros="$otros $pid" ;;
    esac
  done
  if [ -n "$otros" ]; then
    echo "otro script de prueba corre ($otros) y el lock aqs-kind no se pudo tomar; se sale (rc=2)" >&2
    exit 2
  fi
  echo "00-setup: AVISO: $LOCKFILE tomado por residuos del bringup (fd heredado a contenedores kind); se continúa sin él" >&2
fi

# --- 2. Despliegue -------------------------------------------------------------
# Toda carga de aqs-system y aqs-test disponible (réplicas disponibles >= deseadas) y ningún pod
# fuera de Running/Succeeded/Completed.
pods_plataforma_ok() {
  local ns obj want got objs namespace field pods bad_pods mal=0 ausente=0
  for ns in "$NS_SYS" "$NS_TEST"; do
    if ! namespace=$(kubectl --context "$KIND_CONTEXT" --request-timeout=5s get namespace "$ns" --ignore-not-found -o name 2>/dev/null); then
      echo "no se pudo consultar el namespace $ns (API, red o permisos); no se puede determinar si existe"
      mal=1
      continue
    fi
    if [ -z "$namespace" ]; then
      echo "el namespace $ns no existe (¿falta bash scripts/kind/deploy.sh?)"
      mal=1
      ausente=1
      continue
    fi
    if ! objs=$(kubectl --context "$KIND_CONTEXT" --request-timeout=5s get deploy,statefulset -n "$ns" -o name 2>/dev/null); then
      echo "$ns: no se pudieron consultar Deployments y StatefulSets (API, red o permisos)"
      mal=1
      continue
    fi
    if [ -z "$objs" ]; then
      echo "$ns: sin Deployments ni StatefulSets (despliegue ausente)"
      mal=1
      ausente=1
      continue
    fi
    while IFS= read -r obj; do
      [ -n "$obj" ] || continue
      field=availableReplicas
      case "$obj" in statefulset.apps/*) field=readyReplicas ;; esac
      if ! want=$(kubectl --context "$KIND_CONTEXT" --request-timeout=5s get "$obj" -n "$ns" -o jsonpath='{.spec.replicas}' 2>/dev/null) ||
         ! got=$(kubectl --context "$KIND_CONTEXT" --request-timeout=5s get "$obj" -n "$ns" -o "jsonpath={.status.$field}" 2>/dev/null); then
        echo "$ns/$obj: no se pudieron consultar las réplicas (API, red o permisos)"
        mal=1
        continue
      fi
      if [ "${got:-0}" -lt "${want:-1}" ]; then
        echo "$ns/$obj: disponibles=${got:-0} deseadas=${want:-1}"
        mal=1
      fi
    done <<<"$objs"
    if ! pods=$(kubectl --context "$KIND_CONTEXT" --request-timeout=5s get pods -n "$ns" -o json 2>/dev/null); then
      echo "$ns: no se pudieron consultar los pods (API, red o permisos)"
      mal=1
      continue
    fi
    # La fase Pending por sí sola oculta ErrImagePull/ImagePullBackOff y la imagen que falta.
    if ! bad_pods=$(jq -r --arg ns "$ns" '
      .items[] | select((.status.phase != "Running" and .status.phase != "Succeeded") or
        any(((.status.initContainerStatuses // []) + (.status.containerStatuses // []))[];
          .state.waiting != null)) |
        "\($ns)/\(.metadata.name): \(.status.phase)" +
        ([((.status.initContainerStatuses // []) + (.status.containerStatuses // []))[] |
          select(.state.waiting != null) |
          "\(.state.waiting.reason): \(.state.waiting.message // "")"] |
          if length > 0 then " — " + join("; ") else "" end)
    ' <<<"$pods"); then
      echo "$ns: no se pudo interpretar el estado de los pods"
      mal=1
    elif [ -n "$bad_pods" ]; then
      echo "$bad_pods"
      mal=1
    fi
  done
  # Sin namespaces o cargas no hay nada que una espera pueda recuperar.
  [ "$ausente" -eq 0 ] || return 2
  [ "$mal" -eq 0 ] || return 1
}

pods_rc=0
pods_plataforma_ok >"$work/pods.out" 2>&1 || pods_rc=$?
if [ "$pods_rc" -eq 1 ]; then
  # Tras arrancar el nodo los pods tardan en asentarse (imágenes, probes): espera antes de declarar FALLA.
  echo "despliegue incompleto; se espera a que los pods se asienten (plazo de 180 s; consultas con timeout de 5 s)"
  pods_deadline=$((SECONDS + 180))
  for _ in $(seq 1 36); do
    [ "$SECONDS" -lt "$pods_deadline" ] || break
    sleep 5
    pods_rc=0
    pods_plataforma_ok >"$work/pods.out" 2>&1 || pods_rc=$?
    [ "$pods_rc" -eq 1 ] || break
  done
fi
if [ "$pods_rc" -ne 0 ] && [ "$DEPLOY" -eq 1 ]; then
  echo "despliegue incompleto; --deploy: se ejecuta scripts/kind/deploy.sh"
  bash "$root/scripts/kind/deploy.sh"
fi
check despliegue-plataforma pods_plataforma_ok
[ "$fail" -eq 0 ] || {
  echo "    sugerencia: bash scripts/kind/deploy.sh (o repite este script con --deploy)"
  echo "setup: FALLA (despliegue incompleto; el resto de comprobaciones se salta)"
  exit 1
}

# --- 3. Secrets ----------------------------------------------------------------
# Mismo conjunto que crea scripts/kind/secrets.sh; solo se comprueba existencia, nunca valores.
secrets_ok() {
  local par falta=0
  for par in \
    "$NS_SYS/minio-root" "$NS_SYS/minio-kms" "$NS_SYS/minio-tls" "$NS_SYS/aqs-evidence-s3" \
    "$NS_SYS/go-governance-service-token" "$NS_SYS/go-reset-service-token" \
    "$NS_SYS/go-warm-manager-service-token" "$NS_SYS/go-identity-users" \
    "$NS_SYS/go-intake-webhook" "$NS_SYS/agents-llm-key" "$NS_SYS/llm-upstream" \
    "$NS_SYS/target-app-baseline-token" \
    "$NS_TEST/warm-db-credentials" "$NS_TEST/target-app-baseline-token"; do
    if ! kubectl get secret "${par#*/}" -n "${par%%/*}" >/dev/null 2>&1; then
      echo "falta el Secret $par"
      falta=1
    fi
  done
  [ "$falta" -eq 0 ] || return 1
}
check secrets-generados secrets_ok
[ "$fail" -eq 0 ] || {
  echo "    sugerencia: bash scripts/kind/secrets.sh"
  echo "setup: FALLA (faltan Secrets; el resto de comprobaciones se salta)"
  exit 1
}

# --- 4. Túnel cloudflared + port-forward de go-intake --------------------------
puerto_escucha() { # <puerto>: 0 si algo escucha en 127.0.0.1:<puerto>
  (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null
}

tunnel_activo() {
  pgrep -f "cloudflared tunnel.*(run $TUNNEL_NAME|--url http://127.0.0.1:$P_INTAKE)" >/dev/null 2>&1
}

URL_PUBLICA=""
url_desde_log() { # imprime la URL pública del túnel desde el log; excluye las URL de los textos de cloudflare
  local u
  u=$(grep -oE 'https://[A-Za-z0-9.-]+\.trycloudflare\.com' "$1" 2>/dev/null | head -n1)
  if [ -z "$u" ]; then
    u=$(grep -oE 'https://[A-Za-z0-9][A-Za-z0-9.-]*' "$1" 2>/dev/null | grep -v 'cloudflare\.com$' | head -n1)
  fi
  printf '%s' "$u"
}

check cloudflared-instalado command -v cloudflared

if [ "$fail" -eq 0 ]; then
  # Port-forward de go-intake en 127.0.0.1:18622 (paso 1 del doc de operaciones).
  pf_intake() {
    nohup kubectl --context "$KIND_CONTEXT" -n "$NS_SYS" port-forward --address 127.0.0.1 \
      "svc/go-intake" "$P_INTAKE:8080" >"$PF_LOG" 2>&1 &
    echo $! >"$PF_PID"
  }
  if ! puerto_escucha "$P_INTAKE"; then
    pf_intake
    echo "port-forward de go-intake arrancado (pid $(cat "$PF_PID"), log $PF_LOG)"
  fi
  intake_local_ok() {
    # Reintenta 60 s; si el port-forward muere (p. ej. el pod reinicia tras arrancar el
    # nodo), lo levanta de nuevo y sigue esperando.
    local i
    for i in $(seq 1 60); do
      curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$P_INTAKE/healthz" && return 0
      if ! puerto_escucha "$P_INTAKE"; then
        pf_intake
      fi
      sleep 1
    done
    echo "go-intake no responde en 127.0.0.1:$P_INTAKE/healthz (ver $PF_LOG)"
    return 1
  }
  check intake-port-forward intake_local_ok

  if [ "$fail" -eq 0 ]; then
    if [ -n "${AQS_DEMO_PUBLIC_URL:-}" ]; then
      URL_PUBLICA=${AQS_DEMO_PUBLIC_URL%/}
      echo "usando URL pública: $URL_PUBLICA"
      if ! tunnel_activo; then
        nohup cloudflared tunnel --url "http://127.0.0.1:$P_INTAKE" run "$TUNNEL_NAME" >"$CF_LOG" 2>&1 &
        echo $! >"$CF_PID"
      fi
    elif ! tunnel_activo; then
      if cloudflared tunnel list 2>/dev/null | grep -q "$TUNNEL_NAME"; then
        # Túnel con nombre (URL estable, recomendado por el doc de operaciones).
        nohup cloudflared tunnel --url "http://127.0.0.1:$P_INTAKE" run "$TUNNEL_NAME" \
          >"$CF_LOG" 2>&1 &
      else
        # Túnel rápido: la URL cambia en cada arranque (rehacer la Payload URL en GitHub).
        echo "aviso: no existe el túnel con nombre '$TUNNEL_NAME'; se usa túnel rápido (URL efímera)" >&2
        nohup cloudflared tunnel --url "http://127.0.0.1:$P_INTAKE" >"$CF_LOG" 2>&1 &
      fi
      echo $! >"$CF_PID"
      echo "túnel cloudflared arrancado (pid $(cat "$CF_PID"), log $CF_LOG)"
      # El túnel tarda unos segundos en publicar la URL.
      for _ in $(seq 1 30); do
        URL_PUBLICA=$(url_desde_log "$CF_LOG")
        [ -n "$URL_PUBLICA" ] && break
        sleep 1
      done
    else
      echo "túnel cloudflared ya estaba activo; no se reinicia"
      URL_PUBLICA=$(url_desde_log "$CF_LOG")
    fi

    # El log de un túnel CON NOMBRE no imprime la URL pública: se deriva del hostname de
    # ~/.cloudflared/config.yml (la que el humano fijó con `cloudflared tunnel route dns`).
    if [ -z "$URL_PUBLICA" ] && [ -f "$HOME/.cloudflared/config.yml" ]; then
      host_cfg=$(awk '/^[[:space:]]*-[[:space:]]*hostname:/ { print $3; exit }' "$HOME/.cloudflared/config.yml")
      [ -n "$host_cfg" ] && URL_PUBLICA="https://$host_cfg"
    fi

    # Comprueba que el túnel responde. Con URL conocida: la URL pública debe entregar el /healthz
    # de go-intake. Con túnel con nombre y hostname no derivable: basta el proceso vivo y el
    # endpoint local sano (la URL la conoce el humano; se le pide en el resumen).
    if [ -z "$URL_PUBLICA" ]; then
      echo "aviso: túnel con nombre activo pero hostname no derivable del log ni de ~/.cloudflared/config.yml;" >&2
      echo "       la URL pública es la del \`cloudflared tunnel route dns $TUNNEL_NAME <host>\` que configuraste" >&2
    fi
    tunnel_responde() {
      if [ -z "$URL_PUBLICA" ]; then
        tunnel_activo || { echo "el proceso cloudflared no está vivo (ver $CF_LOG)"; return 1; }
        curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$P_INTAKE/healthz" || return 1
        return 0
      fi
      local i code
      for i in $(seq 1 30); do
        code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$URL_PUBLICA/healthz") || code=000
        [ "$code" = 200 ] && return 0
        sleep 2
      done
      echo "$URL_PUBLICA/healthz respondió $code (se esperaba 200; comprueba el conector cloudflared y su configuración)"
      return 1
    }
    check tunnel-cloudflared tunnel_responde
    # Un túnel con nombre cuya URL no se pudo derivar no es FALLA, pero el resumen la necesita:
    # la línea de detalle del check (arriba) es el aviso; aquí no se inventa una URL.
  fi
fi

# --- 5. Red a ghcr.io (pull anónimo de la imagen del repo objetivo) ------------
ghcr_alcanzable() {
  local sha rc=0
  sha=$(resolve_target_sha) || return 1
  target_image_exists "$sha" || rc=$?
  case "$rc" in
    0) echo "imagen anónima alcanzable: $TARGET_IMAGE_REPO:$sha" ; return 0 ;;
    1) echo "la imagen $TARGET_IMAGE_REPO:$sha no existe o el paquete no es público (404)" ; return 1 ;;
    *) echo "no pude comprobar la imagen (token anónimo, red o límite de GHCR)" ; return 1 ;;
  esac
}
check red-ghcr ghcr_alcanzable

# Wake the already verified warm environment before the preflight. Never reset it here.
wake_warm_if_idle() (
  local pf pid port token code state i
  pf="$work/warm-pf.log"
  kubectl --context "$KIND_CONTEXT" -n "$NS_SYS" port-forward --address 127.0.0.1 \
    svc/go-warm-manager :8080 >"$pf" 2>&1 &
  pid=$!
  trap 'kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true' EXIT
  port=""
  for i in $(seq 1 30); do
    port=$(sed -n 's/^Forwarding from 127\.0\.0\.1:\([0-9]*\) -> 8080$/\1/p' "$pf" | head -n 1)
    [ -n "$port" ] && break
    kill -0 "$pid" 2>/dev/null || { echo "falló el port-forward de warm-manager"; return 1; }
    sleep 1
  done
  [ -n "$port" ] || { echo "timeout iniciando port-forward de warm-manager"; return 1; }
  token=$(kubectl --context "$KIND_CONTEXT" -n "$NS_SYS" get secret go-warm-manager-service-token -o jsonpath='{.data.token}' | base64 -d) || return 1
  [ -n "$token" ] || { echo "token de warm-manager no disponible"; return 1; }
  printf 'Authorization: Bearer %s\n' "$token" >"$work/warm-auth"
  unset token
  code=$(curl -sS --max-time 10 -o "$work/warm-state" -w '%{http_code}' -H @"$work/warm-auth" "http://127.0.0.1:$port/warm") || return 1
  [ "$code" = 200 ] || { echo "GET /warm respondió $code"; return 1; }
  state=$(jq -r '.state // empty' "$work/warm-state") || return 1
  if [ "$state" = idle-escalado ]; then
    code=$(curl -sS --max-time 150 -o "$work/warm-state" -w '%{http_code}' -X POST -H @"$work/warm-auth" "http://127.0.0.1:$port/warm/ensure") || return 1
    [ "$code" = 200 ] || { echo "POST /warm/ensure respondió $code"; return 1; }
    jq -e '.state == "ready" and .reset_verified == true' "$work/warm-state" >/dev/null || return 1
  fi
)
if [ "$fail" -eq 0 ]; then
  check warm-despertar wake_warm_if_idle
fi

# --- 6. Humo --------------------------------------------------------------------
smoke_out="$work/smoke.out"
smoke_rc=0
bash "$root/scripts/kind/kind-smoke.sh" --preflight >"$smoke_out" 2>&1 || smoke_rc=$?
smoke_fallas=$(grep -c '^FALLA' "$smoke_out" || true)
if [ "$smoke_fallas" -eq 0 ] && [ "$smoke_rc" -eq 0 ]; then
  echo "OK kind-smoke preflight (0 líneas FALLA; ciclo de corrida pendiente de confirmación humana)"
else
  echo "FALLA kind-smoke ($smoke_fallas líneas FALLA, rc=$smoke_rc; esperado 0 — ver Estado conocido de docs/demo-real.md)"
  sed 's/^/    /' "$smoke_out"
  fail=1
fi

# --- 7. Resumen ------------------------------------------------------------------
echo
echo "================ resumen del Paso 0 ================"
if [ -n "$URL_PUBLICA" ]; then
  echo "URL pública del webhook: $URL_PUBLICA/webhooks/github"
else
  echo "URL pública del webhook: NO DISPONIBLE (revisa el túnel: log $CF_LOG)"
fi
if [ "$fail" -eq 0 ]; then
  echo "setup: OK (entorno listo para la demo en vivo)"
  exit 0
fi
echo "setup: FALLA (ver las líneas FALLA)"
exit 1
