#!/usr/bin/env bash
# Despliega deploy/flux/kind en el clúster kind `aqs` y espera a que todo esté disponible (U7-T03).
# Orden: guarda de contexto (kind-aqs, sale 3) → secrets.sh → kubectl apply -k deploy/flux/kind →
# kubectl rollout status de cada Deployment y StatefulSet (300 s) → espera al Job minio-init.
# Imprime «OK <objeto>» o «FALLA <objeto>» por cada comprobación; sale 0 solo si todas pasan, 1 si no.
# Nunca imprime valores de Secrets (secrets.sh tampoco).
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
# shellcheck disable=SC1091  # lib.sh se carga por ruta dinámica (relativa a root)
. "$root/scripts/kind/lib.sh"

fail=0
report() { # <rc> <objeto>
  if [ "$1" -eq 0 ]; then
    echo "OK $2"
  else
    echo "FALLA $2"
    fail=1
  fi
}

# available_ok <ns> <tipo.apps/nombre>: réplicas disponibles >= deseadas, leídas del objeto ahora mismo.
available_ok() {
  local ns=$1 obj=$2 kind name want got restarts
  kind=${obj%%.*}
  name=${obj##*/}
  case "$kind" in
    deployment)
      want=$(kubectl get deployment "$name" -n "$ns" -o jsonpath='{.spec.replicas}' 2>/dev/null)
      got=$(kubectl get deployment "$name" -n "$ns" -o jsonpath='{.status.availableReplicas}' 2>/dev/null) ;;
    statefulset)
      want=$(kubectl get statefulset "$name" -n "$ns" -o jsonpath='{.spec.replicas}' 2>/dev/null)
      got=$(kubectl get statefulset "$name" -n "$ns" -o jsonpath='{.status.readyReplicas}' 2>/dev/null) ;;
    *) return 1 ;;
  esac
  # Reinicios de los pods del objeto (prefijo <nombre>-): un bucle de reinicios no está «disponible»,
  # aunque en un instante concreto tenga un pod Ready.
  restarts=$(kubectl get pods -n "$ns" -o json 2>/dev/null | jq --arg p "$name-" \
    '[.items[] | select(.metadata.name | startswith($p)) | (.status.containerStatuses // [])[] | .restartCount] | add // 0')
  [ "${restarts:-0}" -eq 0 ] || return 1
  [ "${got:-0}" -ge "${want:-1}" ] && [ "${got:-0}" -gt 0 ]
}

require_kind_context # sale 3 si el contexto no es kind-aqs

bash "$root/scripts/kind/secrets.sh"
report $? "secrets"
[ "$fail" -eq 0 ] || { echo "FALLA deploy: no se aplica el overlay sin Secrets"; exit 1; }

kubectl apply -k "$root/deploy/flux/kind" >/dev/null
report $? "apply-k deploy/flux/kind"
[ "$fail" -eq 0 ] || exit 1

# Deployments y StatefulSets de los namespaces que usa el overlay (aqs-system y aqs-test).
for ns in aqs-system aqs-test; do
  while IFS= read -r obj; do
    [ -n "$obj" ] || continue
    rc=0
    kubectl rollout status "$obj" -n "$ns" --timeout=300s >/dev/null 2>&1 || rc=1
    # rollout status puede dar OK en el instante en que un pod en bucle de reinicios pasa a Ready:
    # se exige además que las réplicas disponibles cubran las deseadas en ese mismo momento.
    [ "$rc" -eq 0 ] && { available_ok "$ns" "$obj" || rc=1; }
    report "$rc" "$ns/$obj"
  done < <(kubectl get deploy,statefulset -n "$ns" -o name 2>/dev/null)
done

kubectl wait --for=condition=complete job/minio-init -n aqs-system --timeout=300s >/dev/null 2>&1
report $? "aqs-system/job/minio-init"

if [ "$fail" -eq 0 ]; then
  echo "deploy: OK (overlay deploy/flux/kind en $KIND_CONTEXT)"
  exit 0
fi
echo "deploy: FALLA (ver las líneas FALLA)"
exit 1
