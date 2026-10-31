#!/usr/bin/env bash
# Reconstruye, carga y reinicia las imágenes de la plataforma en el clúster kind ya existente
# (U12-T03). Sin esto, un clúster levantado antes de un cambio sigue corriendo las imágenes viejas:
# el tag fijo :0.0.0 no cambia y los pods no se reinician aunque la imagen del nodo se recargue.
#
# Orden: guarda de contexto -> build-images.sh -> load-images.sh -> rollout restart/status de los
# deployments que usan imágenes de la plataforma -> lectura de vuelta del startTime de sus pods.
# AQS_IMAGES="ui-api go-identity" limita build, load y reinicio a ese subconjunto.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
# shellcheck disable=SC1091  # lib.sh se carga por ruta dinámica (relativa a root)
. "$root/scripts/kind/lib.sh"

fail() { echo "refresh-images: $*" >&2; exit 1; }

# Guarda: sale 3 si el contexto no es kind-aqs. Va antes de cualquier podman, kind o kubectl.
require_kind_context

command -v kubectl >/dev/null 2>&1 || fail "falta 'kubectl' en el PATH"

# build-images.sh y load-images.sh leen AQS_IMAGES del entorno; se propaga tal cual si está definida.
if [ -n "${AQS_IMAGES:-}" ]; then
  export AQS_IMAGES
fi

bash "$root/scripts/kind/build-images.sh"
bash "$root/scripts/kind/load-images.sh"

# Hora del reinicio en UTC (RFC 3339). Todo pod en ejecución de los deployments reiniciados debe
# tener un startTime posterior: si no, quedó una réplica con la imagen vieja.
restart_time=$(date -u +%Y-%m-%dT%H:%M:%SZ)
restart_epoch=$(date -u -d "$restart_time" +%s)

# Deployments con alguna imagen de la plataforma (prefijo IMAGE_PREFIX). Con AQS_IMAGES, solo los
# que usan esas imágenes. Emite «namespace<TAB>nombre<TAB>selector» por deployment. No toca
# deployments con otras imágenes (p. ej. aqs-test/warm-app, que usa la imagen del repo objetivo).
platform_deployments() {
  kubectl --context "$KIND_CONTEXT" get deployments --all-namespaces -o json | jq -r \
    --arg prefix "$IMAGE_PREFIX/" --arg subset "${AQS_IMAGES:-}" '
      def de_la_plataforma:
        . as $img
        | ($img | startswith($prefix))
          and (if $subset == "" then true
               else (($img | ltrimstr($prefix)) | split(":")[0]) as $name
                    | ($subset | split(" ") | index($name)) != null
               end);
      .items[]
      | select(.spec.template.spec.containers | any(.image | de_la_plataforma))
      | [ .metadata.namespace,
          .metadata.name,
          ([.spec.selector.matchLabels // {} | to_entries[] | "\(.key)=\(.value)"] | join(",")) ]
      | @tsv'
}

deployments=$(platform_deployments) || fail "no se pudo listar los deployments"

count=0
restarted=()
while IFS=$'\t' read -r ns name selector; do
  [ -n "$ns" ] || continue
  kubectl --context "$KIND_CONTEXT" -n "$ns" rollout restart "deployment/$name" >/dev/null
  kubectl --context "$KIND_CONTEXT" -n "$ns" rollout status "deployment/$name" --timeout=180s >/dev/null
  count=$((count + 1))
  restarted+=("$ns|$name|$selector")
done <<<"$deployments"

# Clúster recién creado y aún sin desplegar: no hay nada que reiniciar.
if [ "$count" -eq 0 ]; then
  echo "refresh-images: OK (0 deployments reiniciados)"
  exit 0
fi

# Lectura de vuelta. Se excluyen los pods en terminación: `rollout status` da por bueno el rollout
# en cuanto la réplica nueva está Ready y la vieja puede seguir en Running unos segundos.
bad=0
for entry in "${restarted[@]}"; do
  IFS='|' read -r ns name selector <<<"$entry"
  if [ -n "$selector" ]; then
    pods=$(kubectl --context "$KIND_CONTEXT" -n "$ns" get pods -l "$selector" -o json) ||
      fail "no se pudieron leer los pods de $ns/$name"
  else
    pods=$(kubectl --context "$KIND_CONTEXT" -n "$ns" get pods -o json) ||
      fail "no se pudieron leer los pods de $ns/$name"
  fi
  while IFS=$'\t' read -r pod start; do
    [ -n "$pod" ] || continue
    [ -n "$start" ] || continue
    pod_epoch=$(date -u -d "$start" +%s 2>/dev/null) ||
      fail "startTime ilegible del pod $ns/$pod: '$start'"
    if [ "$pod_epoch" -lt "$restart_epoch" ]; then
      echo "refresh-images: el pod $ns/$pod arrancó antes del reinicio ($start < $restart_time)" >&2
      bad=1
    fi
  done < <(jq -r '
    .items[]
    | select(.metadata.deletionTimestamp == null)
    | select(.status.phase == "Running")
    | [ .metadata.name, (.status.startTime // "") ] | @tsv' <<<"$pods")
done
[ "$bad" -eq 0 ] || exit 1

echo "refresh-images: OK ($count deployments reiniciados)"
