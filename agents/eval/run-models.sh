#!/usr/bin/env bash
# Corre los 12 casos del dataset contra cada modelo y compara (entrega del módulo 8, «prueba de cambio de modelo»).
# Entre una corrida y otra solo cambia LLM_MODEL: ni código ni prompts.
#
#   export LLM_BASE_URL=https://<gateway compatible con OpenAI>/v1   # http solo hacia loopback
#   export LLM_API_KEY_FILE=$HOME/.local/state/aqs-kind/<clave>      # la clave nunca se imprime
#   agents/eval/run-models.sh <modelo-1> <modelo-2> [...]
#
# Topes: por defecto los del clúster kind (120 s, 16000/8000 tokens); se pueden sobrescribir con PLANNER_* y
# REPORTER_* (mismos nombres que en los Deployments). Deja out/<modelo>/report.json y out/comparacion.md.
# Sale 0 si todos los modelos cumplen los umbrales del PRD; 1 si alguno no; 2 si falta configuración.
set -uo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
cd "$here" || exit 2
[ $# -ge 1 ] || { echo "uso: $0 <modelo> [<modelo>...]" >&2; exit 2; }
: "${LLM_BASE_URL:?falta LLM_BASE_URL}" "${LLM_API_KEY_FILE:?falta LLM_API_KEY_FILE}"
py=${PYTHON:-.venv/bin/python}
[ -x "$py" ] || { echo "falta $py: python -m venv .venv && .venv/bin/pip install -r requirements-dev.txt" >&2; exit 2; }

# Mismos topes que los agentes en el clúster (deploy/flux/kind/kustomization.yaml). Los defectos de las bibliotecas
# (25-30 s, 1024-4000 tokens) son para el LLM falso: un modelo de razonamiento agota la salida pensando y devuelve
# content vacío. Cualquier variable ya exportada gana.
export PLANNER_TIMEOUT_S=${PLANNER_TIMEOUT_S:-120} PLANNER_MAX_INPUT_TOKENS=${PLANNER_MAX_INPUT_TOKENS:-16000}
export PLANNER_MAX_OUTPUT_TOKENS=${PLANNER_MAX_OUTPUT_TOKENS:-8000}
export REPORTER_TIMEOUT_S=${REPORTER_TIMEOUT_S:-120} REPORTER_MAX_INPUT_TOKENS=${REPORTER_MAX_INPUT_TOKENS:-16000}
export REPORTER_MAX_OUTPUT_TOKENS=${REPORTER_MAX_OUTPUT_TOKENS:-8000}
echo "topes: planner ${PLANNER_TIMEOUT_S}s/${PLANNER_MAX_OUTPUT_TOKENS} tok, reporter ${REPORTER_TIMEOUT_S}s/${REPORTER_MAX_OUTPUT_TOKENS} tok"

rc=0; reports=()
for m in "$@"; do
  d="out/$(printf '%s' "$m" | tr -c 'A-Za-z0-9._-' '_')"
  echo "== $m"
  LLM_MODEL="$m" "$py" -m agent_eval run --llm http --out "$d"
  r=$?; [ $r -eq 2 ] && exit 2; [ $r -ne 0 ] && rc=1
  reports+=("$d/report.json")
done
"$py" -m agent_eval compare "${reports[@]}" | tee out/comparacion.md
exit $rc
