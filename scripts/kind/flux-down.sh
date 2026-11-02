#!/usr/bin/env bash
# Retira Flux de kind SIN borrar la plataforma (entrega del módulo 8). Primero apaga prune en aqs-kind: si se
# borrara la Kustomization con prune activo, Flux eliminaría todo lo que aplicó (plataforma incluida).
# Después de esto el clúster vuelve al modo manual (scripts/kind/deploy.sh).
set -uo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
# shellcheck disable=SC1091
. "$root/scripts/kind/lib.sh"
require_kind_context

if kubectl -n flux-system get kustomization aqs-kind >/dev/null 2>&1; then
  kubectl -n flux-system patch kustomization aqs-kind --type=merge -p '{"spec":{"prune":false,"suspend":true}}' >/dev/null || {
    echo "FALLA no se pudo apagar prune; no se borra nada"; exit 1; }
  kubectl -n flux-system delete kustomization aqs-kind --wait=true >/dev/null && echo "OK Kustomization aqs-kind retirada (sin podar)"
fi
kubectl -n flux-system delete gitrepository agentic-qa-swarm --ignore-not-found >/dev/null && echo "OK GitRepository retirado"
kubectl delete -f "$root/deploy/flux/clusters/dev/flux-system/gotk-components.yaml" --ignore-not-found >/dev/null && echo "OK componentes de Flux retirados"
