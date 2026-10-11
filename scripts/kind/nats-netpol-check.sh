#!/usr/bin/env bash
# Aislamiento de NATS (U8-T01, CA-4). Requiere el clúster kind-aqs con NATS y nats-init desplegados.
# Lanza en aqs-system dos pods de un solo uso con la misma imagen de cliente:
#   - nats-netpol-ok: con la etiqueta aqs.io/nats-client=true (cliente permitido) → debe leer el stream.
#   - nats-netpol-ko: sin la etiqueta (otro pod del namespace) → no debe alcanzar NATS.
# Imprime «OK|FALLA <comprobacion>» por cada caso y sale 0 solo si ambos son correctos. Borra sus pods
# siempre (trap), también si falla. No toca otros pods ni otros namespaces.
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
# shellcheck disable=SC1091  # lib.sh se carga por ruta dinámica (relativa a root)
. "$root/scripts/kind/lib.sh"

NS=aqs-system
URL=nats://nats.aqs-system.svc:4222
IMG=docker.io/natsio/nats-box:0.14.5
OK_POD=nats-netpol-ok
KO_POD=nats-netpol-ko
# cwd en /tmp: con cwd «/» el CLI nats v0.1.5 falla como uid 1000 (ver deploy/flux/base/nats/job-init.yaml).
CMD=(sh -c "cd /tmp && exec nats --server $URL --timeout 5s stream info AQS_EVENTS --json")

fail=0
cleanup() { kubectl delete pod "$OK_POD" "$KO_POD" -n "$NS" --ignore-not-found --wait=false >/dev/null 2>&1; }
trap cleanup EXIT

require_kind_context # sale 3 si el contexto no es kind-aqs

# run_pod <nombre> [etiqueta]: crea el pod, espera a que termine y deja su código de salida en $rc.
run_pod() {
  local name=$1 label=${2:-}
  local args=(-n "$NS" run "$name" --image="$IMG" --restart=Never)
  [ -n "$label" ] && args+=(--labels="$label")
  kubectl delete pod "$name" -n "$NS" --ignore-not-found --wait=true >/dev/null 2>&1
  kubectl "${args[@]}" -- "${CMD[@]}" >/dev/null 2>&1 || { rc=crear; return; }
  local phase=""
  for _ in $(seq 1 60); do
    phase=$(kubectl get pod "$name" -n "$NS" -o jsonpath='{.status.phase}' 2>/dev/null)
    case "$phase" in Succeeded|Failed) break ;; esac
    sleep 2
  done
  rc=$(kubectl get pod "$name" -n "$NS" -o jsonpath='{.status.containerStatuses[0].state.terminated.exitCode}' 2>/dev/null)
  [ -n "$rc" ] || rc=sin-terminar
  log=$(kubectl logs "$name" -n "$NS" 2>/dev/null | tail -n 3)
  printf '%s\n' "$log" | sed 's/^/  /' >&2
  kubectl delete pod "$name" -n "$NS" --ignore-not-found --wait=false >/dev/null 2>&1
}

run_pod "$OK_POD" "aqs.io/nats-client=true"
if [ "$rc" = 0 ]; then
  echo "OK nats-cliente-permitido"
else
  echo "FALLA nats-cliente-permitido (rc=$rc)"
  fail=1
fi

run_pod "$KO_POD"
# Solo cuenta como bloqueado un timeout de conexión (código 1..255 y mensaje de timeout). Un fallo de
# DNS («no such host») no demuestra la NetworkPolicy, así que no cuenta.
if [[ "$rc" =~ ^[0-9]+$ ]] && [ "$rc" -ge 1 ] && printf '%s' "$log" | grep -qiE 'timeout|timed out|deadline exceeded'; then
  echo "OK nats-otro-pod-bloqueado"
else
  echo "FALLA nats-otro-pod-bloqueado (rc=$rc; sin timeout de conexión: alcanzó NATS o falló por otra causa)"
  fail=1
fi

exit "$fail"
