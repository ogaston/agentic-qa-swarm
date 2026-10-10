#!/usr/bin/env bash
# Funciones comunes de scripts/kind/ (U7-T01). Se carga con `source`; no ejecuta nada al cargarse.
# Nombre del clúster efímero y contexto de kubectl que le corresponde.
KIND_CLUSTER_NAME=aqs
KIND_CONTEXT=kind-aqs
export KIND_EXPERIMENTAL_PROVIDER=podman

# Guarda: todo script que hable con el clúster la llama antes de cualquier kubectl.
# Sale con 3 si el contexto actual de kubectl no es exactamente kind-aqs.
require_kind_context() {
  local ctx
  ctx=$(kubectl config current-context 2>/dev/null) || ctx=""
  if [ "$ctx" != "$KIND_CONTEXT" ]; then
    echo "contexto ${ctx:-(ninguno)} no es ${KIND_CONTEXT}" >&2
    exit 3
  fi
}

# Indica si el clúster efímero `aqs` existe (no toca otros clústeres).
kind_cluster_exists() {
  kind get clusters 2>/dev/null | grep -qx "$KIND_CLUSTER_NAME"
}
