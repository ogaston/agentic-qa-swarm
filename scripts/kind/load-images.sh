#!/usr/bin/env bash
# Carga en el clúster aqs las imágenes de la plataforma (U7-T02): `podman save` a un archivo en
# un mktemp -d (borrado con trap) y `kind load image-archive --name aqs`. Exige el contexto kind-aqs
# antes de tocar nada. AQS_IMAGES="ui-api go-identity" carga solo un subconjunto.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
# shellcheck disable=SC1091  # lib.sh se carga por ruta dinámica (relativa a root)
. "$root/scripts/kind/lib.sh"

fail() { echo "load-images: $*" >&2; exit 1; }

# Guarda: sale 3 si el contexto no es kind-aqs. Va antes de cualquier otra operación.
require_kind_context

command -v podman >/dev/null 2>&1 || fail "falta 'podman' en el PATH"
command -v kind >/dev/null 2>&1 || fail "falta 'kind' en el PATH"

images=$(all_images "$root") || fail "no se pudo obtener la lista de imágenes"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

while read -r name dir; do
  ref="$IMAGE_PREFIX/$name:$IMAGE_TAG"
  podman image exists "$ref" || fail "falta $ref en podman; ejecuta scripts/kind/build-images.sh"
  archive="$tmp/$name.tar"
  podman save --format docker-archive -o "$archive" "$ref" < /dev/null \
    || fail "podman save falló para $ref"
  echo "load-images: cargando $ref en $KIND_CLUSTER_NAME"
  kind load image-archive "$archive" --name "$KIND_CLUSTER_NAME" \
    || fail "kind load image-archive falló para $ref"
  rm -f "$archive"
done <<<"$images"

# Imagen de terceros fijada (U8-T09, LiteLLM). Debe coincidir con deploy/flux/kind/litellm.yaml.
# Se descarga si no está en podman y se carga como las de la plataforma.
third_party="ghcr.io/berriai/litellm:v1.106.0"
if ! podman image exists "$third_party"; then
  podman pull "$third_party" >/dev/null || fail "no se pudo descargar $third_party"
fi
archive="$tmp/litellm.tar"
podman save --format docker-archive -o "$archive" "$third_party" < /dev/null \
  || fail "podman save falló para $third_party"
echo "load-images: cargando $third_party en $KIND_CLUSTER_NAME"
kind load image-archive "$archive" --name "$KIND_CLUSTER_NAME" \
  || fail "kind load image-archive falló para $third_party"
rm -f "$archive"
echo "load-images: OK"
