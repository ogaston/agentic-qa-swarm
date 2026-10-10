#!/usr/bin/env bash
# Arranque local de U3 (U3-T07): planner en 127.0.0.1:18400 y reporter en 127.0.0.1:18401, ambos con
# LLM_PROVIDER=fake. Sin clúster, sin red externa. Estado (fixtures, evidencia sembrada del dataset,
# pids y logs) en ${U3_LOCAL_STATE:-${TMPDIR:-/tmp}/aqs-u3-local}; lo borra u3-down.sh.
# Requiere .venv en agents/agent-planner y agents/agent-reporter (pip install -r requirements-dev.txt -e .).
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
state="${U3_LOCAL_STATE:-${TMPDIR:-/tmp}/aqs-u3-local}"
planner_port=18400
reporter_port=18401
run=run-bug-ecom-01
ds="$root/agents/dataset/artifacts/bug-ecom-01"

[ -e "$state" ] && bash "$root/scripts/test/u3-down.sh"
mkdir -p "$state/surfaces" "$state/evidence/runs/$run" || exit 1

# Evidencia sembrada: el artefacto bug-ecom-01 del dataset, bajo el layout que lee DirEvidenceReader.
cp -r "$ds/evidence/." "$state/evidence/runs/$run/" || exit 1

# Superficie del contrato U2 (la misma que arma FlowSourceU3 en las pruebas de contrato) y su plan.
cat >"$state/surfaces/contrato-u2.json" <<'JSON'
{
  "surface": {
    "run_id": "r1",
    "base_url": "http://warm-app.aqs-test.svc:8080",
    "endpoints": [{"method": "POST", "path": "/orders"}],
    "source": "openapi"
  },
  "workflow": "checkout",
  "plan": {
    "run_id": "r1",
    "workflow": "checkout",
    "flows": [{
      "flow_id": "f1",
      "name": "crear orden",
      "steps": [{"method": "POST", "path": "/orders", "expect_status": 201}],
      "invariant": "el total coincide"
    }]
  }
}
JSON

# Respuesta programada del reporter para la evidencia sembrada (mismo contenido que el dataset, en
# forma de salida del modelo) y la petición que usará el recorrido.
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
cat >"$state/report-request.json" <<JSON
{
  "run_id": "$run",
  "evidence_uris": [
    "s3://aqs-evidence/runs/$run/flow-1/logs.txt",
    "s3://aqs-evidence/runs/$run/flow-1/result.json",
    "s3://aqs-evidence/runs/$run/flow-2/logs.txt",
    "s3://aqs-evidence/runs/$run/flow-2/result.json"
  ]
}
JSON

env LLM_PROVIDER=fake PLANNER_ALLOW_FAKE=true PLANNER_HOST=127.0.0.1 PLANNER_PORT="$planner_port" \
  U3_FAKE_SURFACES_DIR="$state/surfaces" \
  "$root/agents/agent-planner/.venv/bin/python" -m agent_planner >"$state/planner.log" 2>&1 &
echo $! >"$state/planner.pid"

env LLM_PROVIDER=fake REPORTER_ALLOW_FAKE=1 REPORTER_HOST=127.0.0.1 PORT="$reporter_port" \
  REPORTER_EVIDENCE_DIR="$state/evidence" REPORTER_FAKE_FIXTURES="$state/reporter-fixtures.json" \
  "$root/agents/agent-reporter/.venv/bin/python" -m agent_reporter.server >"$state/reporter.log" 2>&1 &
echo $! >"$state/reporter.pid"

for port in "$planner_port" "$reporter_port"; do
  up=0
  for _ in $(seq 100); do
    if curl -sf -o /dev/null "http://127.0.0.1:$port/healthz"; then up=1; break; fi
    sleep 0.2
  done
  if [ "$up" -ne 1 ]; then
    echo "u3-up: el servicio en 127.0.0.1:$port no responde (logs en $state)" >&2
    bash "$root/scripts/test/u3-down.sh"
    exit 1
  fi
done
exit 0
