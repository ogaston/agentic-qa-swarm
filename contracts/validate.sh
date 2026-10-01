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

run() { "${AJV[@]}" validate --spec=draft2020 -c ajv-formats -s "$1" -d "$2" >/dev/null 2>&1; }

for f in examples/valid/*.json; do
  s=$(schema_for "$f") || true
  [ -n "${s:-}" ] || { echo "SIN ESQUEMA $f"; fail=1; continue; }
  run "$s" "$f" && echo "ok   valido   $f" || { echo "FALLA valido deberia pasar: $f ($s)"; fail=1; }
done
for f in examples/invalid/*.json; do
  s=$(schema_for "$f") || true
  [ -n "${s:-}" ] || { echo "SIN ESQUEMA $f"; fail=1; continue; }
  if run "$s" "$f"; then echo "FALLA invalido fue aceptado: $f ($s)"; fail=1; else echo "ok   invalido  $f"; fi
done
exit $fail
