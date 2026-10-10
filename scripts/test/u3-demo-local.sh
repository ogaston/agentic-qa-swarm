#!/usr/bin/env bash
# Recorrido local de U3 (U3-T07): u3-up -> contrato U2 contra el planner real -> POST /v1/report con
# evidencia sembrada del dataset -> reporte validado contra report.schema.json -> u3-down.
# Imprime "OK|FALLA <comprobacion>" y sale distinto de 0 si alguna falla.
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
scripts="$root/scripts/test"
state="${U3_LOCAL_STATE:-${TMPDIR:-/tmp}/aqs-u3-local}"
planner_url=http://127.0.0.1:18400
reporter_url=http://127.0.0.1:18401
fail=0

check() { # <comprobacion> <comando...>
  local name=$1
  shift
  if out=$("$@" 2>&1); then
    echo "OK $name"
  else
    echo "FALLA $name"
    printf '%s\n' "$out" | sed 's/^/    /' >&2
    fail=1
  fi
}

gocontract() { # <regex de prueba> : contrato U2 (go-run-controller) contra U3_URL
  (cd "$root/services/go-run-controller" && U3_URL="$planner_url" GOTOOLCHAIN="${GOTOOLCHAIN:-auto}" \
    go test -count=1 -tags contract -run "$1" ./adapters)
}

reporte_post() { # POST /v1/report con la petición sembrada; deja la respuesta en $state/report.json
  local code
  code=$(curl -s -o "$state/report.json" -w '%{http_code}' -H 'Content-Type: application/json' \
    --data @"$state/report-request.json" "$reporter_url/v1/report") || return 1
  [ "$code" = 200 ] || { echo "POST /v1/report respondio $code"; return 1; }
}

reporte_valido() {
  reporte_post || return 1
  "$root/agents/agent-reporter/.venv/bin/python" -c '
import json, sys
from jsonschema import Draft202012Validator
schema, rep = json.load(open(sys.argv[1])), json.load(open(sys.argv[2]))
errs = [e.message for e in Draft202012Validator(schema).iter_errors(rep)]
if errs:
    print("\n".join(errs)); sys.exit(1)
' "$root/contracts/plans/report.schema.json" "$state/report.json"
}

reporte_con_evidencia() {
  "$root/agents/agent-reporter/.venv/bin/python" -c '
import json, sys
rep = json.load(open(sys.argv[1])); req = json.load(open(sys.argv[2]))
uris = set(req["evidence_uris"])
assert rep["verdict"] == "bug", "veredicto distinto de bug"
assert rep["findings"], "sin hallazgos"
for f in rep["findings"]:
    assert f["evidence_uris"] and set(f["evidence_uris"]) <= uris, "enlace a evidencia fuera de la peticion"
' "$state/report.json" "$state/report-request.json"
}

planner_detenido() { ! curl -s --max-time 2 -o /dev/null "$planner_url/healthz"; }
reporter_detenido() { ! curl -s --max-time 2 -o /dev/null "$reporter_url/healthz"; }
puertos_libres() { planner_detenido && reporter_detenido; }

if ! bash "$scripts/u3-up.sh" >&2; then
  echo "FALLA u3-up"
  exit 1
fi
echo "OK u3-up"

check contrato-u2-plan-valido gocontract '^TestContractU3ValidFlowPlan$'
check contrato-u2-plan-invalido gocontract '^TestContractU3InvalidResponseFailsPhase$'
check contrato-u2-u3-detenido gocontract '^TestContractU3StoppedStubFailsPhase$'
check reporte-valido reporte_valido
check reporte-con-evidencia reporte_con_evidencia

bash "$scripts/u3-down.sh" >&2
check u3-detenido-limpio puertos_libres
exit "$fail"
