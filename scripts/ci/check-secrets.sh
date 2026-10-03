#!/usr/bin/env bash
# Guardia de secrets (U5-T14). Falla si algun Secret de Kubernetes bajo deploy/ no esta cifrado por SOPS.
# Doble barrido, ambos con la misma expresion de yq:
#  (a) la salida real de `kustomize build` de dev, prod y clusters/{dev,prod} (todo lo que Flux aplicaria,
#      incluidos los Secrets de secretGenerator y de rutas con cualquier extension);
#  (b) TODO archivo bajo deploy/ (cualquier extension; un Secret en claro commiteado ya esta publicado):
#      se analiza como YAML/JSON; si no se puede analizar, o no contiene ningun documento con kind, y su
#      contenido menciona un objeto Secret (kind ... Secret, sin distinguir mayusculas), FALLA: no se puede demostrar que es seguro.
# Decision: la toma yq. yq emite un objeto JSON por violacion (-o=json -I=0, una linea, escapado); bash NO
# parsea campos del manifiesto: solo comprueba si hubo salida. data, stringData y binaryData se evaluan por
# separado. Cifrado = cada valor coincide ENTERO con el formato SOPS (una sola linea) y sops.mac tambien.
# Imprime OK|FALLA check-secrets (los detalles, con archivo u overlay, van a stderr).
set -uo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
YQ=mikefarah/yq:4.44.3
KUSTOMIZE=registry.k8s.io/kustomize/kustomize:v5.4.3

RE='^ENC\\[AES256_GCM,data:[A-Za-z0-9+/=]+,iv:[A-Za-z0-9+/=]+,tag:[A-Za-z0-9+/=]+,type:[a-z]+\\]$'
bads() { # <campo>: cuenta de valores que no son ENC[...] completos (un valor no escalar cuenta como malo)
  echo "([(.$1 // {}) | to_entries | .[] | .value] | map(select((tag != \"!!str\") or ((test(\"$RE\")) | not))) | length)"
}
# Un objeto por violacion. Documentos no-mapa se ignoran; los items de un kind: List se expanden.
VIOL='select(tag == "!!map") | [., (select(.kind == "List") | .items[])] | .[] | select(.kind == "Secret") | . as $s |
 [{"secret": ($s.metadata.name | tostring), "campo": "sops.mac", "n": ([$s.sops.mac] | map(select((tag != "!!str") or ((test("'"$RE"'")) | not))) | length)},
  {"secret": ($s.metadata.name | tostring), "campo": "data", "n": '"$(bads data)"'},
  {"secret": ($s.metadata.name | tostring), "campo": "stringData", "n": '"$(bads stringData)"'},
  {"secret": ($s.metadata.name | tostring), "campo": "binaryData", "n": '"$(bads binaryData)"'}]
 | map(select(.n > 0)) | .[]'
# "Menciona Secret": un objeto Secret necesita `kind` seguido (con comillas, espacios o llaves) de Secret.
# No se usa un 'secret' suelto porque los .sh legitimos de deploy/ nombran AWS_SECRET_ACCESS_KEY (decision fail-closed documentada).
MENCION='kind[^[:alnum:]]*secret([^[:alnum:]]|$)'
KINDS='select(tag == "!!map") | select(.kind != null) | .kind'

bad=0
yq_stream() { # <etiqueta>: lee YAML por stdin
  local out
  if ! out=$(docker run --rm -i --security-opt label=disable "$YQ" -N -p yaml -o=json -I=0 "$VIOL" 2>&1); then
    echo "$1: no se pudo analizar: $out" >&2
    bad=1
  elif [ -n "$out" ]; then
    echo "$1: Secret sin cifrar por SOPS: $out" >&2
    bad=1
  fi
}

# (a) salida real de kustomize build
for o in deploy/flux/dev deploy/flux/prod deploy/flux/clusters/dev deploy/flux/clusters/prod; do
  if ! built=$(docker run --rm --security-opt label=disable -v "$root":/w:ro -w /w "$KUSTOMIZE" build "$o" 2>&1); then
    echo "build $o: kustomize build fallo: $built" >&2
    bad=1
  else
    yq_stream "build $o" <<< "$built"
  fi
done

# (b) todos los archivos bajo deploy/, de cualquier extension
inner='for f; do
  if out=$(yq -N -p yaml -o=json -I=0 "$VIOL" "$f" 2>&1); then
    [ -z "$out" ] || printf "%s: Secret sin cifrar por SOPS: %s\n" "$f" "$out"
    k=$(yq -N -p yaml "$KINDS" "$f" 2>/dev/null | wc -l)
    if [ "$k" -eq 0 ] && grep -Eqi "$MENCION" "$f"; then echo "$f: sin documentos con kind pero menciona Secret"; fi
  elif grep -Eqi "$MENCION" "$f"; then
    printf "%s: no analizable y menciona Secret: %s\n" "$f" "$out"
  fi
done'
if ! out=$(docker run --rm --security-opt label=disable -v "$root/deploy":/w/deploy:ro -w /w -e VIOL="$VIOL" -e KINDS="$KINDS" -e MENCION="$MENCION" --entrypoint sh "$YQ" \
  -c 'find deploy -type f | grep -q . || echo "deploy/ vacio"; find deploy -type f -exec sh -c "$0" {} +' "$inner" 2>&1); then
  echo "barrido de archivos fallo: $out" >&2
  bad=1
elif [ -n "$out" ]; then
  echo "$out" >&2
  bad=1
fi

if [ "$bad" -eq 0 ]; then
  echo "OK check-secrets"
else
  echo "FALLA check-secrets"
fi
exit "$bad"
