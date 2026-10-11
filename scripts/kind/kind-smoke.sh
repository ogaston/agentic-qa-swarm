#!/usr/bin/env bash
# Prueba de humo de U7-T04 sobre la plataforma desplegada en el clúster kind `aqs` (contexto kind-aqs).
# Abre port-forward en loopback (127.0.0.1:18600-18606) a los 7 servicios del control plane, comprueba
# salud, login, inbox, estado del warm, RBAC, NetworkPolicy (con pod de prueba efímero en aqs-test) y
# que los logs no contienen secretos. Imprime «OK|FALLA <comprobación>» y sale 0 solo si ninguna falla.
# Al final imprime «PENDIENTE <paso> (<motivo>)» para lo que el ciclo de corrida aún no hace en kind.
# No imprime valores de Secrets ni tokens. Limpia port-forwards y el pod de prueba siempre (trap).
# shellcheck disable=SC2317  # las funciones de comprobación se invocan indirectamente (check)
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
# shellcheck disable=SC1091  # lib.sh se carga por ruta dinámica (relativa a root)
. "$root/scripts/kind/lib.sh"

require_kind_context # sale 3 si el contexto no es kind-aqs (antes de crear nada)

NS_SYS=aqs-system
NS_TEST=aqs-test
PROBE=aqs-smoke-probe
PROBE_IMAGE=busybox:1.37.0
STATE_DIR="${XDG_RUNTIME_DIR:-/tmp}/aqs-kind"
P_IDENTITY=18600
P_UI=18601
P_INTAKE=18602
P_RUN=18603
P_WARM=18604
P_RESET=18605
P_GOV=18606
SERVICES=(go-identity ui-api go-intake go-run-controller go-warm-manager go-reset go-governance)
declare -A PORT=([go-identity]=$P_IDENTITY [ui-api]=$P_UI [go-intake]=$P_INTAKE [go-run-controller]=$P_RUN \
  [go-warm-manager]=$P_WARM [go-reset]=$P_RESET [go-governance]=$P_GOV)

fail=0
TOKEN=""
PF_PIDS=()
PROBE_OK=0
work=""

cleanup() {
  local p
  trap '' INT TERM # F-03: una señal durante la limpieza no debe dejar $work sin borrar
  for p in "${PF_PIDS[@]:-}"; do
    [ -n "$p" ] && kill "$p" 2>/dev/null
  done
  for p in "${PF_PIDS[@]:-}"; do
    [ -n "$p" ] && wait "$p" 2>/dev/null
  done
  kubectl delete pod "$PROBE" -n "$NS_TEST" --ignore-not-found --wait=true --timeout=60s >/dev/null 2>&1
  [ -n "$work" ] && rm -rf "$work"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

work=$(mktemp -d "${XDG_RUNTIME_DIR:-/tmp}/aqs-smoke.XXXXXX") || exit 1
chmod 700 "$work"

# check <nombre> <función...>: ejecuta la función en este shell; imprime OK o FALLA y su detalle.
check() {
  local name=$1
  shift
  FAIL_SUFFIX=""
  if "$@" >"$work/last.out" 2>&1; then
    echo "OK $name"
  else
    echo "FALLA $name$FAIL_SUFFIX"
    sed 's/^/    /' "$work/last.out"
    fail=1
  fi
}

# expect_http <código> <url> [argumentos de curl...]: deja el cuerpo en $work/body.
expect_http() {
  local want=$1 url=$2 code
  shift 2
  code=$(curl -s -o "$work/body" -w '%{http_code}' --max-time 10 "$@" "$url") || code=000
  if [ "$code" = "$want" ]; then
    return 0
  fi
  echo "GET $url -> $code (se esperaba $want)"
  return 1
}

# Port-forwards en loopback, cada uno con su PID.
for svc in "${SERVICES[@]}"; do
  kubectl port-forward --address 127.0.0.1 -n "$NS_SYS" "svc/$svc" "${PORT[$svc]}:8080" \
    >"$work/pf-$svc.log" 2>&1 &
  PF_PIDS+=("$!")
done

wait_port() { # <puerto>: espera hasta 60 s a que responda /healthz
  for _ in $(seq 1 60); do
    curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$1/healthz" && return 0
    sleep 1
  done
  return 1
}
for svc in "${SERVICES[@]}"; do
  wait_port "${PORT[$svc]}" || echo "port-forward de $svc no responde en ${PORT[$svc]} (ver $work/pf-$svc.log sin valores)" >&2
done

# 1. Salud y preparación de los 7 servicios.
for svc in "${SERVICES[@]}"; do
  check "healthz-$svc" expect_http 200 "http://127.0.0.1:${PORT[$svc]}/healthz"
done
for svc in "${SERVICES[@]}"; do
  check "readyz-$svc" expect_http 200 "http://127.0.0.1:${PORT[$svc]}/readyz"
done

# 2. Login del usuario demo (contraseña de $STATE_DIR/demo.txt, línea 2) y sesión con rol user.
login_demo() {
  local pw body="$work/login.json" resp="$work/login.resp" sess="$work/session.json" code role
  pw=$(sed -n 2p "$STATE_DIR/demo.txt" 2>/dev/null) || pw=""
  [ -n "$pw" ] || { echo "no hay contraseña demo en $STATE_DIR/demo.txt (ver scripts/kind/secrets.sh)"; return 1; }
  jq -n --arg u demo --arg p "$pw" '{username: $u, password: $p}' >"$body" || return 1
  code=$(curl -s -o "$resp" -w '%{http_code}' --max-time 10 -H 'Content-Type: application/json' \
    --data @"$body" "http://127.0.0.1:$P_IDENTITY/auth/login") || code=000
  [ "$code" = 200 ] || { echo "POST /auth/login respondió $code"; return 1; }
  TOKEN=$(jq -r '.token // empty' "$resp")
  [ -n "$TOKEN" ] || { echo "POST /auth/login sin token"; return 1; }
  code=$(curl -s -o "$sess" -w '%{http_code}' --max-time 10 -H "Authorization: Bearer $TOKEN" \
    "http://127.0.0.1:$P_IDENTITY/auth/session") || code=000
  [ "$code" = 200 ] || { echo "GET /auth/session respondió $code"; return 1; }
  role=$(jq -r '.role // empty' "$sess")
  [ "$role" = user ] || { echo "role='$role' (se esperaba user)"; return 1; }
}
check login-demo login_demo

# 3. Inbox a través de ui-api (valida el token contra go-identity).
inbox_con_token() {
  expect_http 200 "http://127.0.0.1:$P_UI/notifications" -H "Authorization: Bearer $TOKEN" || return 1
  jq -e 'type == "array"' "$work/body" >/dev/null || { echo "GET /notifications no devuelve un array JSON"; return 1; }
}
check inbox-con-token inbox_con_token
check inbox-sin-token-401 expect_http 401 "http://127.0.0.1:$P_UI/notifications"

# 4. Estado del warm: un state del enum de WarmState (contracts/plans/warm-state.schema.json).
warm_estado() {
  expect_http 200 "http://127.0.0.1:$P_UI/warm" -H "Authorization: Bearer $TOKEN" || return 1
  local st enum
  st=$(jq -r '.state // empty' "$work/body")
  enum=$(jq -r '.properties.state.enum[]' "$root/contracts/plans/warm-state.schema.json") || return 1
  grep -qxF "$st" <<<"$enum" || { echo "state='$st' no pertenece al enum WarmState"; return 1; }
}
check warm-estado warm_estado

# 5. RBAC: los permisos de go-warm-manager y go-reset coinciden con docs/seguridad/rbac-matriz.csv
#    (la matriz que valida scripts/ci/rbac-matrix.sh). Permisos en aqs-test; ninguno en aqs-system/default.
rbac_test_ns_only() {
  local csv="$root/docs/seguridad/rbac-matriz.csv" subj ns verb res exp got n=0 bad=0
  [ -f "$csv" ] || { echo "falta $csv"; return 1; }
  while IFS=, read -r subj ns verb res exp; do
    [ "$subj" = sujeto ] && continue
    case "$subj" in go-warm-manager|go-reset) ;; *) continue ;; esac
    n=$((n + 1))
    got=$(kubectl auth can-i --as="system:serviceaccount:$NS_SYS:$subj" "$verb" "$res" -n "$ns" 2>/dev/null) || true
    want=no
    [ "$exp" = yes ] && want=yes
    if [ "$got" != "$want" ]; then
      echo "difiere $subj $ns $verb $res: esperado=$want obtenido=${got:-vacio}"
      bad=$((bad + 1))
    fi
  done <"$csv"
  [ "$n" -gt 0 ] || { echo "ninguna fila de go-warm-manager/go-reset en $csv"; return 1; }
  echo "filas comparadas: $n"
  [ "$bad" -eq 0 ]
}
check rbac-test-ns-only rbac_test_ns_only

# 6. NetworkPolicy con un pod efímero en aqs-test (imagen fijada, SecurityContext restringido).
start_probe() {
  kubectl delete pod "$PROBE" -n "$NS_TEST" --ignore-not-found --wait=true --timeout=60s >/dev/null 2>&1
  cat <<YAML | kubectl apply -f - >/dev/null || return 1
apiVersion: v1
kind: Pod
metadata:
  name: $PROBE
  namespace: $NS_TEST
  labels:
    aqs.io/smoke: "true"
spec:
  restartPolicy: Never
  automountServiceAccountToken: false
  securityContext:
    runAsNonRoot: true
    runAsUser: 65534
    seccompProfile: {type: RuntimeDefault}
  containers:
    - name: probe
      image: $PROBE_IMAGE
      imagePullPolicy: IfNotPresent
      command: ["sleep", "900"]
      securityContext:
        allowPrivilegeEscalation: false
        capabilities: {drop: [ALL]}
      resources:
        requests: {cpu: 10m, memory: 16Mi}
        limits: {cpu: 100m, memory: 64Mi}
YAML
  kubectl wait --for=condition=Ready "pod/$PROBE" -n "$NS_TEST" --timeout=180s >/dev/null || return 1
  kubectl exec -n "$NS_TEST" "$PROBE" -- sh -c 'command -v nc' >/dev/null 2>&1 || return 1
}
if start_probe; then PROBE_OK=1; else PROBE_OK=0; fi

# alcanza <ip> <puerto>: 0 si el pod de prueba abre TCP en 5 s.
alcanza() {
  kubectl exec -n "$NS_TEST" "$PROBE" -- nc -z -w 5 "$1" "$2" >/dev/null 2>&1
}
ip_de() { kubectl get svc "$1" -n "$2" -o jsonpath='{.spec.clusterIP}'; }

netpol_control_positivo() {
  [ "$PROBE_OK" -eq 1 ] || { echo "pod de prueba no disponible"; return 1; }
  local ip
  ip=$(ip_de warm-app "$NS_TEST") || return 1
  [ -n "$ip" ] || { echo "warm-app sin ClusterIP"; return 1; }
  alcanza "$ip" 80 || { echo "el pod de prueba no alcanza warm-app ($ip:80) dentro de aqs-test"; return 1; }
}

# F-01: testigo interno (ClusterIP de kubernetes.default:443, abierto sin políticas y sin depender de
# Internet) y testigo externo 1.1.1.1:443. Un testigo alcanzable hace FALLA y se nombra en la línea FALLA.
netpol_sin_egress_test() {
  [ "$PROBE_OK" -eq 1 ] || { echo "pod de prueba no disponible"; return 1; }
  local kip hit=0
  kip=$(kubectl get svc kubernetes -n default -o jsonpath='{.spec.clusterIP}') || return 1
  [ -n "$kip" ] || { echo "kubernetes.default sin ClusterIP"; return 1; }
  if alcanza "$kip" 443; then
    echo "kubernetes.default ($kip:443) alcanzable desde aqs-test"
    FAIL_SUFFIX=" (testigo interno alcanzable: kubernetes.default $kip:443)"
    hit=1
  fi
  if alcanza 1.1.1.1 443; then
    echo "1.1.1.1:443 alcanzable desde aqs-test"
    FAIL_SUFFIX="${FAIL_SUFFIX:- (testigo externo alcanzable: 1.1.1.1:443)}"
    hit=1
  fi
  [ "$hit" -eq 0 ]
}
check netpol-control-positivo netpol_control_positivo
check netpol-sin-egress-test netpol_sin_egress_test

# 7. Logs de los 7 Deployments sin hashes de contraseña, ni $argon2id$, ni tokens de servicio ni la contraseña demo.
secrets_no_en_logs() {
  local pats="$work/patterns" logs="$work/logs.txt" s v svc
  : >"$pats"
  for s in go-governance go-reset go-warm-manager; do
    v=$(kubectl get secret "$s-service-token" -n "$NS_SYS" -o jsonpath='{.data.token}' | base64 -d) || return 1
    [ -n "$v" ] && printf '%s\n' "$v" >>"$pats"
  done
  v=$(sed -n 2p "$STATE_DIR/demo.txt" 2>/dev/null) || v=""
  [ -n "$v" ] && printf '%s\n' "$v" >>"$pats"
  # shellcheck disable=SC2016  # patrón literal: el $ no debe expandirse
  printf '%s\n' 'password_hash' '$argon2id$' >>"$pats"
  : >"$logs"
  for svc in "${SERVICES[@]}"; do
    kubectl logs "deploy/$svc" -n "$NS_SYS" --all-containers >>"$logs" 2>&1 || { echo "kubectl logs deploy/$svc falló"; return 1; }
  done
  # F-04: cero líneas en total no demuestra ausencia de secretos; sería un falso OK.
  [ -s "$logs" ] || { echo "los logs de los 7 Deployments están vacíos (0 líneas leídas)"; return 1; }
  if grep -q -F -f "$pats" "$logs"; then
    echo "coincidencias de secretos en logs: $(grep -c -F -f "$pats" "$logs")"
    return 1
  fi
}
check secrets-no-en-logs secrets_no_en_logs

# Ciclo de corrida: no se fuerza. Lo que no funciona en kind queda declarado, sin FALLA.
echo "PENDIENTE webhook->notificacion->confirm->run.confirmed->controlador (P1: los eventos viajan por archivos en el disco de cada pod; no hay transporte entre pods)"
echo "PENDIENTE planner/reporter (P2: los agentes no tienen Deployment; falta LiteLLM y egress limitado)"
echo "PENDIENTE reset verificado (C-104: go-reset-baseline en kind es un relleno kind-stub que no verifica nada)"
echo "PENDIENTE MinIO en agentes (P3: adaptadores S3 de los agentes; usuario de aqs-evidence-s3 sin aprovisionar, C-107)"

if [ "$fail" -eq 0 ]; then
  echo "smoke: OK (sin FALLA; pendientes declarados arriba)"
  exit 0
fi
echo "smoke: FALLA (ver las líneas FALLA)"
exit 1
