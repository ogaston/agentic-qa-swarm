#!/usr/bin/env bash
# Imprime los lenguajes reconocidos de <dir> (uno por linea: go, python, node).
# Marcadores: go.mod | pyproject.toml o requirements.txt | package.json.
# Sale 1 si no hay ninguno (un servicio Go bajo workspace sin go.mod propio cuenta como no reconocido).
set -euo pipefail

dir=${1:?uso: detect-lang.sh <dir>}
found=0
if [ -f "$dir/go.mod" ]; then echo go; found=1; fi
if [ -f "$dir/pyproject.toml" ] || [ -f "$dir/requirements.txt" ]; then echo python; found=1; fi
if [ -f "$dir/package.json" ]; then echo node; found=1; fi
if [ "$found" -eq 0 ]; then
  echo "::error::$dir no tiene marcador reconocido (go.mod, pyproject.toml, requirements.txt, package.json)" >&2
  exit 1
fi
