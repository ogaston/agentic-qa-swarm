#!/usr/bin/env bash
# Regresiones del Paso 0 sin clúster: todos los accesos externos se simulan en PATH.
# Se corta antes del túnel y del humo; ningún caso lee secretos ni modifica infraestructura.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/demo-setup-test.XXXXXX")
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/repo/scripts/demo" "$work/repo/scripts/kind" "$work/bin" "$work/runtime"
cp "$root/scripts/kind/demo-preflight.sh" "$work/repo/scripts/kind/"
cp "$root/scripts/kind/lib.sh" "$work/repo/scripts/kind/"
cat >"$work/repo/scripts/kind/deploy.sh" <<'MOCK'
#!/usr/bin/env bash
echo deploy >>"$MOCK_LOG"
exit 1
MOCK
cat >"$work/bin/kind" <<'MOCK'
#!/usr/bin/env bash
echo aqs
MOCK
cat >"$work/bin/kubectl" <<'MOCK'
#!/usr/bin/env bash
printf 'kubectl %s\n' "$*" >>"$MOCK_LOG"
args=()
while [ "$#" -gt 0 ]; do
  case "$1" in
    --context) shift 2 ;;
    --request-timeout=*) shift ;;
    *) args+=("$1"); shift ;;
  esac
done
set -- "${args[@]}"
case "$*" in
  'config current-context')
    if [ "$MOCK_CASE" = wrong-context ]; then echo production; else echo kind-aqs; fi ;;
  'get --raw=/healthz') echo ok ;;
  'get namespace '*)
    case "$MOCK_CASE" in
      absent) exit 0 ;;
      api-error) echo 'connection refused' >&2; exit 1 ;;
      *) echo namespace/mock ;;
    esac ;;
  'get deploy,statefulset '*) echo statefulset.apps/database ;;
  *'{.spec.replicas}'*)
    if [ "$MOCK_CASE" = replicas-error ]; then exit 1; fi
    echo 1 ;;
  *'{.status.readyReplicas}'*) echo 1 ;;
  *'{.status.availableReplicas}'*) echo 0 ;;
  'get pods '*)
    if [ "$MOCK_CASE" = crashloop ]; then
      echo '{"items":[{"metadata":{"name":"app-crashing"},"status":{"phase":"Running","containerStatuses":[{"state":{"waiting":{"reason":"CrashLoopBackOff","message":"back-off restarting failed container"}}}]}}]}'
    elif [ "$MOCK_CASE" = image-error ]; then
      echo '{"items":[{"metadata":{"name":"warm-app-pending"},"status":{"phase":"Pending","containerStatuses":[{"state":{"waiting":{"reason":"ImagePullBackOff","message":"image example.invalid/app:missing not found"}}}]}}]}'
    else
      echo '{"items":[{"metadata":{"name":"database-0"},"status":{"phase":"Running"}}]}'
    fi ;;
  'get secret '*) exit 1 ;;
  *) echo "unexpected kubectl call: $*" >&2; exit 99 ;;
esac
MOCK
cat >"$work/bin/sleep" <<'MOCK'
#!/usr/bin/env bash
echo sleep >>"$MOCK_LOG"
MOCK
cat >"$work/bin/pgrep" <<'MOCK'
#!/usr/bin/env bash
exit 1
MOCK
chmod +x "$work/bin/"*
fail=0
run_case() {
  local scenario=$1 expected=$2 rc=0
  shift 2
  : >"$work/calls"
  env PATH="$work/bin:$PATH" XDG_RUNTIME_DIR="$work/runtime" MOCK_LOG="$work/calls" MOCK_CASE="$scenario" \
    bash "$work/repo/scripts/kind/demo-preflight.sh" "$@" >"$work/out" 2>&1 || rc=$?
  if [ "$rc" -ne "$expected" ]; then
    echo "FALLA $scenario: rc=$rc, esperado=$expected"
    cat "$work/out"
    fail=1
  fi
}
assert_has() {
  if ! grep -qF "$1" "$work/out"; then
    echo "FALLA falta diagnóstico: $1"
    cat "$work/out"
    fail=1
  fi
}
assert_no_call() {
  if grep -qE "$1" "$work/calls"; then
    echo "FALLA llamada inesperada: $1"
    fail=1
  fi
}

run_case absent 1
assert_has 'el namespace aqs-system no existe'
assert_no_call '^sleep$|^deploy$|port-forward|apply'
echo 'OK namespace ausente: fallo inmediato sin despliegue'

run_case api-error 1
assert_has 'no se pudo consultar el namespace aqs-system (API, red o permisos)'
if grep -q 'no existe' "$work/out"; then echo 'FALLA API confundida con namespace ausente'; fail=1; fi
assert_no_call '^deploy$|port-forward|apply'
echo 'OK error de API: no se confunde con namespace ausente'

run_case healthy 1
assert_has 'OK despliegue-plataforma'
assert_has 'FALLA secrets-generados'
assert_no_call '^sleep$|^deploy$|port-forward|apply'
echo 'OK StatefulSet disponible por readyReplicas'

run_case replicas-error 1
assert_has 'no se pudieron consultar las réplicas'
assert_has 'FALLA despliegue-plataforma'
echo 'OK error al consultar réplicas: falla cerrado'

run_case image-error 1
assert_has 'FALLA despliegue-plataforma'
assert_has 'ImagePullBackOff: image example.invalid/app:missing not found'
assert_no_call '^deploy$|port-forward|apply'
echo 'OK imagen ausente: diagnóstico con causa de espera'

run_case crashloop 1
assert_has 'FALLA despliegue-plataforma'
assert_has 'CrashLoopBackOff: back-off restarting failed container'
echo 'OK CrashLoopBackOff: se rechaza aunque la fase del pod sea Running'

run_case wrong-context 3
assert_has 'contexto production no es kind-aqs'
assert_no_call 'get |^deploy$|^sleep$'
echo 'OK contexto incorrecto: rechazo antes de consultar cargas'

run_case absent 1 --deploy
if ! grep -qx deploy "$work/calls"; then echo 'FALLA no se invocó deploy autorizado'; fail=1; fi
assert_no_call 'sleep'
echo 'OK --deploy sin namespaces: invocación inmediata simulada'

if [ "$fail" -eq 0 ]; then echo 'demo-setup-test: OK'; else echo 'demo-setup-test: FALLA'; fi
exit "$fail"
