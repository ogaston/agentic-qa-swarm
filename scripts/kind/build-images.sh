#!/usr/bin/env bash
# Construye con podman las imágenes de la plataforma (U7-T02): services/* y agents/* con Dockerfile.
# Contexto = directorio del servicio (igual que ci.yml), tag <prefijo>/<nombre>:0.0.0 (sin latest).
# AQS_IMAGES="ui-api go-identity" construye solo un subconjunto. No publica nada. Una imagen que
# falla corta el script con su nombre.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
# shellcheck disable=SC1091  # lib.sh se carga por ruta dinámica (relativa a root)
. "$root/scripts/kind/lib.sh"

fail() { echo "build-images: $*" >&2; exit 1; }

command -v podman >/dev/null 2>&1 || fail "falta 'podman' en el PATH"
command -v jq >/dev/null 2>&1 || fail "falta 'jq' en el PATH"

images=$(platform_images "$root") || fail "no se pudo obtener la lista de imágenes"
cd "$root"
while read -r name dir; do
  ref="$IMAGE_PREFIX/$name:$IMAGE_TAG"
  echo "build-images: construyendo $ref desde $dir"
  if ! podman build -t "$ref" "$dir" < /dev/null; then
    fail "falló la construcción de $name ($ref)"
  fi
done <<<"$images"
echo "build-images: OK"
