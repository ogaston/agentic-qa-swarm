#!/usr/bin/env bash
# Solo lectura: ¿el clúster kind está en la revisión que tiene GitHub? (entrega del módulo 8).
# Imprime la rama seguida, el commit en GitHub, la revisión aplicada por Flux y el estado Ready.
# Sale 0 si aqs-kind está Ready en el mismo commit que GitHub; 1 si no.
set -uo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
# shellcheck disable=SC1091
. "$root/scripts/kind/lib.sh"
require_kind_context

branch=$(kubectl -n flux-system get gitrepository agentic-qa-swarm -o jsonpath='{.spec.ref.branch}' 2>/dev/null)
[ -n "$branch" ] || { echo "FALLA Flux no está instalado (scripts/kind/flux-up.sh)"; exit 1; }
want=$(git ls-remote https://github.com/ogaston/agentic-qa-swarm "refs/heads/$branch" | cut -f1)
src=$(kubectl -n flux-system get gitrepository agentic-qa-swarm -o jsonpath='{.status.artifact.revision}')
rev=$(kubectl -n flux-system get kustomization aqs-kind -o jsonpath='{.status.lastAppliedRevision}')
ready=$(kubectl -n flux-system get kustomization aqs-kind -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}')
msg=$(kubectl -n flux-system get kustomization aqs-kind -o jsonpath='{.status.conditions[?(@.type=="Ready")].message}')
echo "rama:            $branch"
echo "GitHub:          $want"
echo "fuente (Flux):   $src"
echo "aplicado (Flux): $rev  Ready=$ready"
echo "mensaje:         $msg"
if [ "$ready" = True ] && [ "${rev##*:}" = "$want" ]; then echo "OK clúster en la revisión de Git"; exit 0; fi
echo "FALLA clúster fuera de la revisión de Git (Flux reconcilia cada minuto)"; exit 1
