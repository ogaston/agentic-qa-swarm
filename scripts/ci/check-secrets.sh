#!/usr/bin/env bash
# Guardia de secrets (U5-T14): bajo deploy/ todo `kind: Secret` debe tener data/stringData
# cifrados por SOPS (cada valor ENC[...] y bloque sops: presente). Imprime OK|FALLA check-secrets.
set -uo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
YQ=mikefarah/yq:4.44.3

bad=0
while IFS= read -r f; do
  # Por cada documento Secret: lista de problemas (una linea por problema).
  out=$(docker run --rm -i --security-opt label=disable "$YQ" -N '
    select(.kind == "Secret") |
    ((select(.sops == null) | .metadata.name + ": falta el bloque sops"),
     (((.data // {}) + (.stringData // {})) | to_entries | .[]
       | select((.value | tostring | test("^ENC\\[")) | not)
       | .key + ": valor en claro"))' < "$f" 2>&1) || { echo "no se pudo analizar $f" >&2; bad=1; continue; }
  if [ -n "$out" ]; then
    echo "$f: $out" >&2
    bad=1
  fi
done < <(grep -rlE '^kind:[[:space:]]*Secret[[:space:]]*$' deploy --include='*.yaml' --include='*.yml' 2>/dev/null)

if [ "$bad" -eq 0 ]; then
  echo "OK check-secrets"
else
  echo "FALLA check-secrets"
fi
exit "$bad"
