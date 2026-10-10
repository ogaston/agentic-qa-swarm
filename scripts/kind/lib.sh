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

# Imágenes de la plataforma (U7-T02): prefijo y tag que esperan los manifiestos de deploy/flux/base.
# shellcheck disable=SC2034  # variables usadas por build-images.sh y load-images.sh al hacer source
IMAGE_PREFIX=ghcr.io/ogaston/agentic-qa-swarm
# shellcheck disable=SC2034  # ídem
IMAGE_TAG=0.0.0

# Imprime una línea «<nombre> <dir>» por imagen, tomando la lista de scripts/ci/list-services.sh
# (no se escribe a mano). Exige 9 entradas. Si AQS_IMAGES está definida (nombres separados por
# espacio), filtra a ese subconjunto y rechaza nombres desconocidos. Sale 1 con mensaje si algo falla.
# Uso: images=$(platform_images "$root") || exit 1
platform_images() {
  local root=$1 list dir name n out="" count=0
  local -a dirs=()
  list=$(cd "$root" && bash scripts/ci/list-services.sh) || {
    echo "no se pudo ejecutar scripts/ci/list-services.sh" >&2
    return 1
  }
  while IFS= read -r dir; do
    dirs+=("$dir")
  done < <(jq -r '.[]' <<<"$list")
  count=${#dirs[@]}
  if [ "$count" -ne 9 ]; then
    echo "la lista de imágenes da $count entradas; se esperaban 9" >&2
    return 1
  fi
  for dir in "${dirs[@]}"; do
    name=${dir##*/}
    if [ -n "${AQS_IMAGES:-}" ]; then
      case " $AQS_IMAGES " in *" $name "*) ;; *) continue ;; esac
    fi
    out+="$name $dir"$'\n'
  done
  for n in ${AQS_IMAGES:-}; do
    case "$out" in *"$n "*) ;; *)
      echo "AQS_IMAGES: '$n' no es una imagen de la plataforma" >&2
      return 1 ;;
    esac
  done
  printf '%s' "$out"
}
