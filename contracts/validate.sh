#!/usr/bin/env bash
# Valida los ejemplos de eventos contra sus esquemas (draft 2020-12).
# Los validos deben pasar; los invalidos deben fallar. Sale != 0 si alguno se comporta al reves.
set -u
cd "$(dirname "$0")/events"
AJV=(npx --yes -p ajv-cli@5.0.0 -p ajv-formats@3.0.1 ajv)
fail=0

schema_for() { # <ejemplo.json> -> esquema cuyo nombre es el prefijo mas largo del archivo
  local base best="" s n
  base=$(basename "$1" .json)
  for s in *.schema.json; do
    n=${s%.schema.json}
    case "$base." in "$n."*) [ ${#n} -gt ${#best} ] && best=$n ;; esac
  done
  [ -n "$best" ] && echo "$best.schema.json"
}

# run: 0 = ajv acepta; 1 = ajv rechaza por el esquema (su salida dice "invalid");
# 2 = fallo de herramienta/JSON roto (ni valido ni invalido: siempre es error).
run() {
  local out rc
  out=$("${AJV[@]}" validate --spec=draft2020 -c ajv-formats -s "$1" -d "$2" 2>&1); rc=$?
  [ $rc -eq 0 ] && return 0
  printf '%s\n' "$out" | grep -q -E "^$2 invalid\$" && return 1
  return 2
}

for f in examples/valid/*.json; do
  s=$(schema_for "$f") || true
  [ -n "${s:-}" ] || { echo "SIN ESQUEMA $f"; fail=1; continue; }
  t=$(jq -r .type "$f" 2>/dev/null) || t=""
  [ "$t" = "$(basename "$f" .json)" ] || { echo "FALLA type '$t' no coincide con el nombre del archivo: $f"; fail=1; continue; }
  run "$s" "$f"; r=$?
  [ $r -eq 0 ] && echo "ok   valido   $f" || { echo "FALLA valido deberia pasar: $f ($s, rc=$r)"; fail=1; }
done
for f in examples/invalid/*.json; do
  s=$(schema_for "$f") || true
  [ -n "${s:-}" ] || { echo "SIN ESQUEMA $f"; fail=1; continue; }
  run "$s" "$f"; r=$?
  case $r in
    0) echo "FALLA invalido fue aceptado: $f ($s)"; fail=1 ;;
    1) echo "ok   invalido  $f" ;;
    *) echo "FALLA invalido no rechazado por el esquema (JSON roto o herramienta): $f"; fail=1 ;;
  esac
done
exit $fail
