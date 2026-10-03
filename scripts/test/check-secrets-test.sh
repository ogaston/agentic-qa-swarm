#!/usr/bin/env bash
# Pruebas negativas de la guardia check-secrets.sh (U5-T14, F-01, F-04..F-07). Cada caso se anade a una copia en
# mktemp -d y se compara el rc con el esperado: 1 = la guardia debe rechazar, 0 = debe aceptar.
# Sale 1 si CUALQUIER caso da un resultado inesperado.
set -uo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
t=$(mktemp -d)
trap 'rm -rf "$t"' EXIT
{ git ls-files; git ls-files --others --exclude-standard; } | sort -u | while IFS= read -r f; do [ -e "$f" ] && printf '%s\n' "$f"; done | tar -c -T - | tar -x -C "$t"
chmod -R a+rX "$t"
base="$t/deploy/flux/base"

rc_all=0
# case_ <nombre> <rc-esperado> <archivo bajo base/> <contenido> [texto a anadir a base/kustomization.yaml]
case_() {
  mkdir -p "$(dirname "$base/$3")"
  printf '%s' "$4" > "$base/$3"
  cp "$base/kustomization.yaml" "$t/kz.bak"
  [ -z "${5:-}" ] || printf '%s\n' "$5" >> "$base/kustomization.yaml"
  (cd "$t" && bash scripts/ci/check-secrets.sh > /dev/null 2>&1)
  rc=$?
  if [ "$rc" -eq "$2" ]; then r=ok; else r=INESPERADO; rc_all=1; fi
  echo "$r  $1: rc=$rc (esperado $2)"
  rm -rf "$base/zz" "$base"/caso*
  rm -f "$base/secret"
  cp "$t/kz.bak" "$base/kustomization.yaml"
}
V='ENC[AES256_GCM,data:YWJj,iv:ZGVm,tag:Z2hp,type:str]'
M='ENC[AES256_GCM,data:bWFj,iv:ZGVm,tag:Z2hp,type:str]'
H=$'kind: Secret\nmetadata: {name: f}\n'
p='stringData: {password: hunter2}'

# Ronda 1
case_ "comentario" 1 caso.yaml $'kind: Secret # c\nmetadata: {name: f}\n'"$p"$'\n'
case_ "comillas dobles" 1 caso.yaml $'kind: "Secret"\nmetadata: {name: f}\n'"$p"$'\n'
case_ "comillas simples" 1 caso.yaml $'kind: \'Secret\'\nmetadata: {name: f}\n'"$p"$'\n'
case_ "flow una linea" 1 caso.yaml '{apiVersion: v1, kind: Secret, metadata: {name: f}, stringData: {password: hunter2}}'$'\n'
case_ "json" 1 caso.json '{"apiVersion":"v1","kind":"Secret","metadata":{"name":"f"},"stringData":{"password":"hunter2"}}'$'\n'
case_ "List" 1 caso.yaml $'apiVersion: v1\nkind: List\nitems:\n  - kind: Secret\n    metadata: {name: f}\n    '"$p"$'\n'
case_ "yml" 1 caso.yml "$H$p"$'\n'
case_ "yaml invalido con Secret" 1 caso.yaml $'kind: Secret\n  : [\n'
case_ "ENC[ falso con sops vacio" 1 caso.yaml "$H"$'stringData: {password: "ENC[hunter2"}\nsops: {}\n'
case_ "ENC[ falso con mac" 1 caso.yaml "$H"$'stringData: {password: "ENC[hunter2"}\nsops: {mac: x}\n'
# Ronda 2
case_ "nombre con pipes y salto final" 1 caso.yaml $'kind: Secret\nmetadata: {name: "x|1|0\\n"}\n'"$p"$'\n'
case_ "nombre en bloque con pipes + doc trampa" 1 caso.yaml $'kind: Secret\nmetadata:\n  name: |\n    x|1|0\n'"$p"$'\n---\nkind: Secret\nmetadata: {name: ok}\nstringData: {a: "'"$V"$'"}\nsops: {mac: "'"$M"$'"}\n'
case_ "data en claro tapado por stringData" 1 caso.yaml "$H"$'data: {password: aHVudGVyMg==}\nstringData: {password: "'"$V"$'"}\nsops: {mac: "'"$M"$'"}\n'
case_ "valor multilinea con ENC en la 1a linea" 1 caso.yaml "$H"$'stringData: {password: "'"${V}"$'\\nhunter2"}\nsops: {mac: "'"$M"$'"}\n'
case_ "ENC[AES256_GCM,data:hunter2 con mac" 1 caso.yaml "$H"$'stringData: {password: "ENC[AES256_GCM,data:hunter2"}\nsops: {mac: "'"$M"$'"}\n'
case_ "mac con formato invalido" 1 caso.yaml "$H"$'stringData: {password: "'"$V"$'"}\nsops: {mac: x}\n'
case_ "binaryData en claro" 1 caso.yaml "$H"$'binaryData: {k: aHVudGVyMg==}\nsops: {mac: "'"$M"$'"}\n'
case_ "data en claro sin sops" 1 caso.yaml "$H"$'data: {password: aHVudGVyMg==}\n'
case_ ".txt referenciado desde resources" 1 zz/leak.txt "$H$p"$'\n' '  - zz/leak.txt'
case_ ".txt no referenciado (barrido de archivos)" 1 zz/leak.txt "$H$p"$'\n'
case_ "a.YAML" 1 caso.YAML "$H$p"$'\n'
case_ "archivo sin extension" 1 secret "$H$p"$'\n'
case_ "a.yaml.tpl con plantilla" 1 caso.yaml.tpl $'kind: Secret\nmetadata: {name: "{{ .n }}"}\n{{ .x }}\n'
case_ "a.jsonc" 1 caso.jsonc $'// c\n{"kind":"Secret","stringData":{"p":"hunter2"}}\n'
case_ "secretGenerator con literals (via build)" 1 zz/vacio.txt 'x' $'secretGenerator:\n  - name: g\n    namespace: aqs-system\n    literals:\n      - password=hunter2'
# Aceptados
case_ "cifrado valido (formato completo)" 0 caso.yaml "$H"$'data: {a: "'"$V"$'"}\nstringData: {password: "'"$V"$'"}\nbinaryData: {b: "'"$V"$'"}\nsops: {mac: "'"$M"$'"}\n'
case_ "solo documentos vacios" 0 caso.yaml $'---\n---\n'
case_ "script con AWS_SECRET_ACCESS_KEY" 0 caso.sh $'#!/bin/sh\n: "${AWS_SECRET_ACCESS_KEY:?}"\n'
exit "$rc_all"
