#!/usr/bin/env bash
# Pruebas negativas de la guardia check-secrets.sh (U5-T14, F-01/F-04). Cada caso se anade a una copia en mktemp -d.
# Esperado: todos los casos "claro" dan rc=1 y el caso "cifrado valido" rc=0. Sale 1 si algo difiere.
set -uo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
t=$(mktemp -d)
trap 'rm -rf "$t"' EXIT
{ git ls-files; git ls-files --others --exclude-standard; } | sort -u | while IFS= read -r f; do [ -e "$f" ] && printf '%s\n' "$f"; done | tar -c -T - | tar -x -C "$t"

rc_all=0
case_() { # <nombre> <rc-esperado> <archivo-relativo> <contenido>
  printf '%s' "$4" > "$t/deploy/flux/base/caso.${3##*.}"
  (cd "$t" && bash scripts/ci/check-secrets.sh > /dev/null 2>&1)
  rc=$?
  echo "$1: rc=$rc (esperado $2)"
  [ "$rc" -eq "$2" ] || rc_all=1
  rm -f "$t"/deploy/flux/base/caso.*
}
p='stringData: {password: hunter2}'
case_ "comentario" 1 x.yaml $'kind: Secret # c\nmetadata: {name: f}\n'"$p"$'\n'
case_ "comillas dobles" 1 x.yaml $'kind: "Secret"\nmetadata: {name: f}\n'"$p"$'\n'
case_ "comillas simples" 1 x.yaml $'kind: \'Secret\'\nmetadata: {name: f}\n'"$p"$'\n'
case_ "flow una linea" 1 x.yaml '{apiVersion: v1, kind: Secret, metadata: {name: f}, stringData: {password: hunter2}}'$'\n'
case_ "json" 1 x.json '{"apiVersion":"v1","kind":"Secret","metadata":{"name":"f"},"stringData":{"password":"hunter2"}}'$'\n'
case_ "List" 1 x.yaml $'apiVersion: v1\nkind: List\nitems:\n  - kind: Secret\n    metadata: {name: f}\n    '"$p"$'\n'
case_ "yml" 1 x.yml $'kind: Secret\nmetadata: {name: f}\n'"$p"$'\n'
case_ "yaml invalido" 1 x.yaml $'kind: Secret\n  : [\n'
case_ "ENC[ falso con sops vacio" 1 x.yaml $'kind: Secret\nmetadata: {name: f}\nstringData: {password: "ENC[hunter2"}\nsops: {}\n'
case_ "ENC[ falso con mac" 1 x.yaml $'kind: Secret\nmetadata: {name: f}\nstringData: {password: "ENC[hunter2"}\nsops: {mac: x}\n'
case_ "cifrado valido (forma)" 0 x.yaml $'kind: Secret\nmetadata: {name: f}\nstringData: {password: "ENC[AES256_GCM,data:abc,iv:d,tag:e,type:str]"}\nsops: {mac: "ENC[AES256_GCM,data:m]"}\n'
exit "$rc_all"
