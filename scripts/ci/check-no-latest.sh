#!/usr/bin/env bash
# Falla si algun `image:` bajo <dir> usa :latest o no lleva tag ni digest.
set -euo pipefail

dir=${1:?uso: check-no-latest.sh <dir>}
if [ ! -d "$dir" ]; then
  echo "::warning:: $dir no existe, nada que verificar" >&2
  exit 0
fi

bad=0
while IFS= read -r -d '' f; do
  while IFS= read -r line; do
    ref=${line#*image:}
    ref=${ref%%#*}
    ref=$(echo "$ref" | tr -d " \"'")
    [ -z "$ref" ] && continue
    last=${ref##*/}
    case "$ref" in
      *@sha256:*) continue ;;
    esac
    case "$last" in
      *:latest) echo "$f: imagen con tag mutable: $ref" >&2; bad=1 ;;
      *:?*) ;;
      *) echo "$f: imagen sin tag ni digest: $ref" >&2; bad=1 ;;
    esac
  done < <(grep -E '^[[:space:]-]*image:[[:space:]]*[^[:space:]]' "$f" || true)
done < <(find "$dir" -type f \( -name '*.yaml' -o -name '*.yml' \) -print0)

exit "$bad"
