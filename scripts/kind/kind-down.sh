#!/usr/bin/env bash
# Destruye el clúster kind efímero `aqs` (U7-T01). Idempotente: si no existe, sale 0.
# Solo borra `aqs`; no toca otros clústeres ni contenedores de podman.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
# shellcheck disable=SC1091  # lib.sh se carga por ruta dinámica (relativa a root)
. "$root/scripts/kind/lib.sh"

if kind_cluster_exists; then
  kind delete cluster --name "$KIND_CLUSTER_NAME"
  echo "kind-down: clúster $KIND_CLUSTER_NAME eliminado"
else
  echo "kind-down: el clúster $KIND_CLUSTER_NAME no existe; nada que hacer"
fi
