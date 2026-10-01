#!/usr/bin/env bash
# Lista como arreglo JSON los directorios services/*/ y agents/*/ que tienen Dockerfile.
# Se ejecuta desde la raiz del repo. Sin servicios imprime [].
set -euo pipefail

items=()
for d in services/*/ agents/*/; do
  d=${d%/}
  if [ -f "$d/Dockerfile" ]; then
    items+=("\"$d\"")
  fi
done

out=""
for i in "${items[@]}"; do
  out="${out:+$out,}$i"
done
echo "[$out]"
