#!/usr/bin/env bash
# Contrato de la fase report (U8-T06): arranca el reporter real en loopback (LLM fake con fixture,
# evidencia sembrada del dataset bug-ecom-01), corre ContractReporter de go-run-controller contra
# REPORTER_URL y para el reporter. Imprime "OK|FALLA <comprobacion>" y sale distinto de 0 si algo falla.
# Requiere .venv en agents/agent-reporter (pip install -r requirements-dev.txt -e .). Sin clúster.
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
state="${U8_REPORT_STATE:-${TMPDIR:-/tmp}/aqs-u8-report}"
reporter_port=18402
reporter_url=http://127.0.0.1:$reporter_port
run=run-bug-ecom-01
ds="$root/agents/dataset/artifacts/bug-ecom-01"
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

gocontract() { # <regex de prueba>: contrato de la fase report contra REPORTER_URL
  (cd "$root/services/go-run-controller" && REPORTER_URL="$reporter_url" GOTOOLCHAIN="${GOTOOLCHAIN:-auto}" \
    go test -count=1 -tags contract -run "$1" ./adapters)
}

reporter_detenido() { ! curl -s --max-time 2 -o /dev/null "$reporter_url/healthz"; }

reporter_down() {
  local pid
  if [ -f "$state/reporter.pid" ]; then
    pid=$(cat "$state/reporter.pid")
    kill "$pid" 2>/dev/null
    for _ in $(seq 50); do kill -0 "$pid" 2>/dev/null || break; sleep 0.1; done
    kill -9 "$pid" 2>/dev/null
  fi
  rm -rf "$state"
}

[ -e "$state" ] && reporter_down
mkdir -p "$state/evidence/runs/$run" || exit 1
cp -r "$ds/evidence/." "$state/evidence/runs/$run/" || exit 1

# Respuesta programada del reporter para la evidencia sembrada (salida del modelo, como en u3-up.sh).
cat >"$state/reporter-fixtures.json" <<JSON
[{
  "run_id": "$run",
  "evidence_uris": [
    "s3://aqs-evidence/runs/$run/flow-1/logs.txt",
    "s3://aqs-evidence/runs/$run/flow-1/result.json",
    "s3://aqs-evidence/runs/$run/flow-2/logs.txt",
    "s3://aqs-evidence/runs/$run/flow-2/result.json"
  ],
  "response": {
    "verdict": "bug",
    "summary": "el stock quedo negativo tras una orden",
    "findings": [{
      "finding_id": "flow-1",
      "root_cause": "invariante violado en la orden",
      "invariant": "el stock nunca es negativo tras una orden",
      "method": "POST",
      "path": "/orders",
      "evidence_uris": ["s3://aqs-evidence/runs/$run/flow-1/result.json"]
    }]
  }
}]
JSON

env LLM_PROVIDER=fake REPORTER_ALLOW_FAKE=1 REPORTER_HOST=127.0.0.1 PORT="$reporter_port" \
  REPORTER_EVIDENCE_DIR="$state/evidence" REPORTER_FAKE_FIXTURES="$state/reporter-fixtures.json" \
  "$root/agents/agent-reporter/.venv/bin/python" -m agent_reporter.server >"$state/reporter.log" 2>&1 &
echo $! >"$state/reporter.pid"

up=0
for _ in $(seq 100); do
  if curl -sf -o /dev/null "$reporter_url/healthz"; then up=1; break; fi
  sleep 0.2
done
if [ "$up" -ne 1 ]; then
  echo "FALLA reporter-arranque (logs en $state)"
  reporter_down
  exit 1
fi
echo "OK reporter-arranque"

check contrato-reporter-valido gocontract '^TestContractReporterValido$'
check contrato-reporter-caido gocontract '^TestContractReporterCaido$'

reporter_down
check reporter-detenido-limpio reporter_detenido
exit "$fail"
