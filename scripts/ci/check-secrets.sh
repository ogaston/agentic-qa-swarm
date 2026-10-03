#!/usr/bin/env bash
# Guardia de secrets (U5-T14): analiza ESTRUCTURALMENTE con yq todos los *.yaml, *.yml y *.json bajo deploy/.
# Todo documento con .kind == "Secret" (tambien items de un kind: List) debe tener cada valor de
# data/stringData como ENC[AES256_GCM,data:...] y un bloque sops con mac no vacio.
# Un archivo que no se pueda analizar hace fallar la guardia. Imprime OK|FALLA check-secrets.
set -uo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
YQ=mikefarah/yq:4.44.3

# yq emite una linea por documento/item: archivo|kind|nombre|<n. de sops.mac validos>|<n. de valores sin ENC[AES256_GCM,data:>.
# El filtrado por kind == Secret se hace en bash (select seguido de concatenacion se comporta mal en yq 4.44).
expr='[., (select(.kind == "List") | .items[])] | .[] |
  filename + "|" + (.kind // "-") + "|" + (.metadata.name // "?") + "|"
  + (([.sops.mac] | map(select(. != null and . != "")) | length) | tostring) + "|"
  + ((((.data // {}) + (.stringData // {})) | to_entries | map(select((.value | tostring | test("^ENC\\[AES256_GCM,data:")) | not)) | length) | tostring)'

# Una sola invocacion de yq con todos los archivos (JSON es YAML valido). Un archivo ilegible o con
# sintaxis invalida hace fallar yq y, por tanto, la guardia.
mapfile -t files < <(find deploy -type f \( -name '*.yaml' -o -name '*.yml' -o -name '*.json' \) | sort)
bad=0
if [ "${#files[@]}" -eq 0 ]; then
  echo "no hay archivos bajo deploy/" >&2
  bad=1
elif ! out=$(docker run --rm --security-opt label=disable -v "$root/deploy":/w/deploy:ro -w /w "$YQ" -N "$expr" "${files[@]}" 2>&1); then
  echo "no se pudo analizar: $out" >&2
  bad=1
else
  while IFS='|' read -r f kind n mac nbad; do
    [ "$kind" = Secret ] || continue
    if [ "$mac" != 1 ]; then echo "$f: $n: falta sops.mac" >&2; bad=1; fi
    if [ "$nbad" != 0 ]; then echo "$f: $n: $nbad valor(es) en claro o sin cifrar por SOPS" >&2; bad=1; fi
  done <<< "$out"
fi

if [ "$bad" -eq 0 ]; then
  echo "OK check-secrets"
else
  echo "FALLA check-secrets"
fi

exit "$bad"
