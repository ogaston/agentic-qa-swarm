#!/usr/bin/env bash
# Politicas y validacion de manifiestos (U5-T15). Corre igual en local y en CI.
# Construye los overlays dev y prod y ejecuta: kubeconform -strict, conftest verify,
# conftest test --all-namespaces, conftest test --combine (regla de conjunto
# default-deny) y promtool test rules. Ejecuta TODAS las comprobaciones aunque una
# falle y sale distinto de 0 al final. Imprime "OK|FALLA <comprobacion> [overlay]".
# Requiere docker; imagenes fijadas, como en las tareas de U5.
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1

KUSTOMIZE=registry.k8s.io/kustomize/kustomize:v5.4.3
KUBECONFORM=ghcr.io/yannh/kubeconform:v0.6.7
CONFTEST=openpolicyagent/conftest:v0.56.0
PROMETHEUS=prom/prometheus:v2.55.1
YQ=mikefarah/yq:4.44.3
CRDS='https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json'

work=$(mktemp -d)
chmod 755 "$work"
trap 'rm -rf "$work"' EXIT

dk() { docker run --rm --security-opt label=disable "$@"; }
conftest() { dk -i -v "$root":/project -w /project "$CONFTEST" "$@"; }

fail=0
report() { # <rc> <comprobacion> [overlay]
  local rc=$1
  shift
  if [ "$rc" -eq 0 ]; then
    echo "OK $*"
  else
    echo "FALLA $*"
    fail=1
  fi
}

overlays=(dev prod)
# Para validar tambien deploy/flux/clusters (U5-T13), basta con crearlo: se anade solo si existe.
if [ -d deploy/flux/clusters ]; then
  echo "AVISO deploy/flux/clusters existe pero no se valida en U5-T15" >&2
fi

for e in "${overlays[@]}"; do
  if ! dk -v "$root":/w -w /w "$KUSTOMIZE" build "deploy/flux/$e" > "$work/$e.yaml"; then
    echo "FALLA kustomize-build $e"
    fail=1
    : > "$work/$e.broken"
  fi
done

for e in "${overlays[@]}"; do
  if [ -e "$work/$e.broken" ]; then
    for c in kubeconform conftest-test conftest-combine; do echo "FALLA $c $e"; done
    continue
  fi
  dk -i "$KUBECONFORM" -strict -summary -schema-location default -schema-location "$CRDS" - < "$work/$e.yaml" >&2
  report $? kubeconform "$e"
  conftest test --no-color --policy policy --all-namespaces - < "$work/$e.yaml" >&2
  report $? conftest-test "$e"
  conftest test --no-color --policy policy --combine - < "$work/$e.yaml" >&2
  report $? conftest-combine "$e"
done

conftest verify --no-color --policy policy >&2
report $? conftest-verify

# promtool: reglas extraidas del build de prod (PrometheusRule -> .spec) + rules_test.yaml
mkdir "$work/p"
chmod 755 "$work/p"
rc=1
if [ ! -e "$work/prod.broken" ] \
  && dk -i "$YQ" -N 'select(.kind == "PrometheusRule" and .metadata.name == "aqs-backup-rules") | .spec' < "$work/prod.yaml" > "$work/p/rules.yaml" \
  && [ -s "$work/p/rules.yaml" ] \
  && cp deploy/flux/base/backup/rules_test.yaml "$work/p/rules_test.yaml" \
  && chmod 644 "$work/p"/*.yaml; then
  dk -v "$work/p":/r -w /r --entrypoint promtool "$PROMETHEUS" test rules rules_test.yaml >&2
  rc=$?
fi
report "$rc" promtool-rules

exit "$fail"
