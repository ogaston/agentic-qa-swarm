#!/usr/bin/env bash
# GitOps en kind (entrega del módulo 8): instala Flux v2.4.0 en el clúster `aqs` y deja que reconcilie
# deploy/flux/kind desde GitHub. Desde ahí, el cambio entra por Git (PR + humano que fusiona), no por kubectl.
#
# Orden: guarda de contexto (kind-aqs, sale 3) → secrets.sh (los Secrets nunca van a Git) →
# componentes de Flux (los mismos, fijados, de deploy/flux/clusters/dev) → espera a los controladores →
# GitRepository + Kustomization de deploy/flux/clusters/kind/sync.yaml → espera a que aqs-kind quede Ready
# en la revisión que tiene GitHub para la rama.
#
# AQS_FLUX_BRANCH=<rama>  prueba una rama de PR antes de fusionar (por defecto main).
# Requiere kubectl y git. El CLI `flux` es opcional (solo para inspeccionar).
# Imprime «OK|FALLA <paso>»; sale 0 solo si todo pasa.
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
# shellcheck disable=SC1091
. "$root/scripts/kind/lib.sh"

branch=${AQS_FLUX_BRANCH:-main}
repo_url=https://github.com/ogaston/agentic-qa-swarm
components=deploy/flux/clusters/dev/flux-system/gotk-components.yaml
sync=deploy/flux/clusters/kind/sync.yaml

ok() { echo "OK $*"; }
fail() { echo "FALLA $*"; exit 1; }

require_kind_context

[[ "$branch" =~ ^[A-Za-z0-9._/-]+$ ]] || fail "AQS_FLUX_BRANCH inválida: $branch"
want=$(git ls-remote "$repo_url" "refs/heads/$branch" | cut -f1)
[ -n "$want" ] || fail "la rama $branch no existe en $repo_url (empújala antes)"
ok "rama $branch en GitHub: $want"

bash scripts/kind/secrets.sh >/dev/null || fail "secrets (scripts/kind/secrets.sh)"
ok "secrets"

kubectl apply --server-side --force-conflicts -f "$components" >/dev/null || fail "componentes de Flux"
for d in source-controller kustomize-controller helm-controller notification-controller; do
  kubectl -n flux-system rollout status "deployment/$d" --timeout=300s >/dev/null || fail "flux-system/$d"
done
ok "controladores de Flux (v2.4.0)"

kubectl apply -f "$sync" >/dev/null || fail "GitRepository/Kustomization"
kubectl -n flux-system patch gitrepository agentic-qa-swarm --type=merge \
  -p "{\"spec\":{\"ref\":{\"branch\":\"$branch\"}}}" >/dev/null || fail "rama del GitRepository"
# Fuerza una vuelta inmediata en vez de esperar al intervalo.
now=$(date +%s)
kubectl -n flux-system annotate --overwrite gitrepository/agentic-qa-swarm "reconcile.fluxcd.io/requestedAt=$now" >/dev/null
kubectl -n flux-system annotate --overwrite kustomization/aqs-kind "reconcile.fluxcd.io/requestedAt=$now" >/dev/null
ok "sync aplicado (rama $branch)"

# Espera a que la Kustomization aplique exactamente la revisión de GitHub.
for _ in $(seq 1 60); do
  rev=$(kubectl -n flux-system get kustomization aqs-kind -o jsonpath='{.status.lastAppliedRevision}' 2>/dev/null)
  ready=$(kubectl -n flux-system get kustomization aqs-kind -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null)
  if [ "$ready" = True ] && [ "${rev##*:}" = "$want" ]; then
    ok "aqs-kind Ready en $rev"
    echo "flux-up: OK. Cambia algo en deploy/flux/kind (o base), fusiona, y mira: scripts/kind/flux-status.sh"
    exit 0
  fi
  sleep 5
done
kubectl -n flux-system get gitrepository,kustomization 2>&1
fail "aqs-kind no llegó a Ready en $want (ver arriba)"
