#!/usr/bin/env bash
# Matriz RBAC sin cluster (U4-T05). Construye el overlay, calcula los permisos efectivos
# de los Role/RoleBinding renderizados y los compara con docs/seguridad/rbac-matriz.csv
# (sujeto,namespace,verbo,recurso,esperado). Falla si:
#   - una fila del CSV difiere del permiso efectivo (yes/no);
#   - existe un permiso efectivo que ninguna fila "yes" cubre (el CSV esta incompleto o un Role crecio);
#   - hay ClusterRole/ClusterRoleBinding o algun "*" en un Role.
# Uso: rbac-matrix.sh [overlay ...]            (por defecto dev prod; imprime OK|FALLA rbac-matrix <overlay>)
#      rbac-matrix.sh --manifest <build.yaml> <etiqueta>   (usa un build ya hecho; solo codigo de salida)
# Requiere docker (kustomize y yq con imagenes fijadas) y jq. Nunca toca un cluster.
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
KUSTOMIZE=registry.k8s.io/kustomize/kustomize:v5.4.3
YQ=mikefarah/yq:4.44.3
csv=docs/seguridad/rbac-matriz.csv

work=$(mktemp -d)
chmod 755 "$work"
trap 'rm -rf "$work"' EXIT

# comprobar <build.yaml> <etiqueta>: 0 si coincide
comprobar() {
  local manifest=$1 label=$2 rc=0
  docker run --rm -i --security-opt label=disable "$YQ" -N -o=json -I=0 '.' < "$manifest" | jq -s '.' > "$work/docs.json" || return 1
  [ "$(jq 'length' "$work/docs.json")" -gt 0 ] || { echo "rbac-matrix $label: build vacio" >&2; return 1; }

  # Prohibidos: ClusterRole(Binding) y wildcards.
  jq -r '.[] | select(.kind=="ClusterRole" or .kind=="ClusterRoleBinding") | "\(.kind)/\(.metadata.name)"' "$work/docs.json" > "$work/prohibidos"
  jq -r '.[] | select(.kind=="Role") | . as $r | (.rules // [])[]? | [.apiGroups, .resources, .verbs][] | select(type=="array") | select(index("*")) | "Role/\($r.metadata.name): wildcard"' "$work/docs.json" >> "$work/prohibidos"
  if [ -s "$work/prohibidos" ]; then
    sed "s/^/rbac-matrix $label: prohibido: /" "$work/prohibidos" >&2
    rc=1
  fi

  # Permisos efectivos: sujeto,namespace,verbo,recurso (el apiGroup no se distingue).
  jq -r '
    def lst: if type=="array" then . else [] end;
    ([.[] | select(.kind=="Role") | {key: "\(.metadata.namespace)/\(.metadata.name)", value: (.rules | lst)}] | from_entries) as $roles
    | .[] | select(.kind=="RoleBinding") | . as $b
    | ($roles["\($b.metadata.namespace)/\($b.roleRef.name)"] // []) as $rules
    | ($b.subjects | lst)[] | (if .kind=="ServiceAccount" then .name else "\(.kind):\(.name)" end) as $s
    | $rules[] | (.resources | lst)[] as $res | (.verbs | lst)[] as $v
    | "\($s),\($b.metadata.namespace),\($v),\($res)"' "$work/docs.json" | sort -u > "$work/grants"

  awk -F, -v label="$label" '
    FILENAME == ARGV[1] { g[$0] = 1; next }
    FNR == 1 { next }
    NF != 5 { printf "rbac-matrix %s: fila mal formada: %s\n", label, $0 > "/dev/stderr"; bad = 1; next }
    {
      yes[$1 "," $2 "," $3 "," $4] = ($5 == "yes")
      # efectivo: coincidencia exacta o por wildcard en el permiso concedido
      ef = 0
      for (k in g) {
        split(k, p, ",")
        if (p[1] == $1 && p[2] == $2 && (p[3] == $3 || p[3] == "*") && (p[4] == $4 || p[4] == "*")) { ef = 1; break }
      }
      if (($5 == "yes") != (ef == 1)) {
        printf "rbac-matrix %s: difiere %s,%s,%s,%s esperado=%s efectivo=%s\n", label, $1, $2, $3, $4, $5, (ef ? "yes" : "no") > "/dev/stderr"
        bad = 1
      }
    }
    END {
      for (k in g) if (!(k in yes) || !yes[k]) {
        printf "rbac-matrix %s: permiso efectivo sin fila yes en el CSV: %s\n", label, k > "/dev/stderr"
        bad = 1
      }
      exit bad
    }' "$work/grants" "$csv" || rc=1
  return "$rc"
}

if [ "${1:-}" = "--manifest" ]; then
  [ $# -eq 3 ] || { echo "uso: rbac-matrix.sh --manifest <build.yaml> <etiqueta>" >&2; exit 2; }
  comprobar "$2" "$3"
  exit $?
fi

[ -f "$csv" ] || { echo "FALLA rbac-matrix (falta $csv)"; exit 1; }
overlays=("$@")
[ ${#overlays[@]} -gt 0 ] || overlays=(dev prod)
fail=0
for e in "${overlays[@]}"; do
  if docker run --rm --security-opt label=disable -v "$root":/w -w /w "$KUSTOMIZE" build "deploy/flux/$e" > "$work/$e.yaml" \
    && comprobar "$work/$e.yaml" "$e"; then
    echo "OK rbac-matrix $e"
  else
    echo "FALLA rbac-matrix $e"
    fail=1
  fi
done
exit "$fail"
