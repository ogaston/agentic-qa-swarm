#!/usr/bin/env bash
# Crea (o reutiliza) el clúster kind efímero `aqs` sobre podman (U7-T01). Idempotente: si `aqs`
# ya existe no lo recrea y sale 0. Deja el contexto de kubectl en kind-aqs. No toca otros clústeres.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
# shellcheck disable=SC1091  # lib.sh se carga por ruta dinámica (relativa a root)
. "$root/scripts/kind/lib.sh"

fail() { echo "kind-up: $*" >&2; exit 1; }

# 1. Requisitos. Si falta algo, sale con un mensaje accionable sin crear nada.
for bin in kind podman kubectl; do
  command -v "$bin" >/dev/null 2>&1 || fail "falta '$bin' en el PATH (instálalo; ver deploy/kind/README.md)"
done
podman info >/dev/null 2>&1 || fail "podman no responde (¿servicio o socket disponible? ver deploy/kind/README.md)"

[ "$(stat -fc %T /sys/fs/cgroup)" = cgroup2fs ] || fail "se requiere cgroup v2 (ver deploy/kind/README.md)"
if [ "$(id -u)" -ne 0 ]; then
  # Podman sin root: el cgroup del usuario debe delegar cpu, memory y pids.
  cg_path=$(grep '^0::' /proc/self/cgroup | cut -d: -f3)
  controllers=$(cat "/sys/fs/cgroup${cg_path}/cgroup.controllers" 2>/dev/null || true)
  for c in cpu memory pids; do
    case " $controllers " in
      *" $c "*) ;;
      *) fail "podman sin root necesita delegación de cgroup v2 para '$c' (ver deploy/kind/README.md)" ;;
    esac
  done
fi

# 2. Idempotencia: si el clúster ya existe, solo se exporta su kubeconfig (deja kind-aqs como actual).
if kind_cluster_exists; then
  echo "kind-up: el clúster $KIND_CLUSTER_NAME ya existe; no se recrea"
  kind export kubeconfig --name "$KIND_CLUSTER_NAME" >/dev/null 2>&1 || fail "no se pudo exportar el kubeconfig de $KIND_CLUSTER_NAME"
else
  kind create cluster --name "$KIND_CLUSTER_NAME" --config "$root/deploy/kind/cluster.yaml" || fail "kind create cluster falló"
fi

# 3. Guarda y espera de disponibilidad del nodo.
require_kind_context
kubectl wait --for=condition=Ready node --all --timeout=180s
echo "kind-up: OK (contexto $KIND_CONTEXT)"
