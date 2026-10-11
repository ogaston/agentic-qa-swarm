#!/usr/bin/env bash
# CA-3 de U8-T05: agentes disponibles y aislados en el clúster kind `aqs`.
# - Deployments agent-planner y agent-reporter con availableReplicas = 1.
# - OK planner-plan-valido: POST /v1/plan con la superficie de la app de referencia (fixture del overlay kind)
#   desde un pod con la etiqueta de go-run-controller → FlowPlan válido contra flow-plan.schema.json.
# - OK reporter-lee-minio: evidencia de bug-ecom-01 sembrada en MinIO (usuario aqs-evidence) → POST /v1/report
#   desde go-run-controller → reporte válido contra report.schema.json, guardado en MinIO y leído de vuelta.
# - OK otro-pod-bloqueado: un pod sin esa etiqueta no alcanza a los dos agentes (timeout, no DNS).
# Validación de contratos en el host, con el venv del planner (jsonschema) y contracts/plans/*.schema.json.
# Imprime «OK|FALLA <comprobacion>» y sale 0 solo si todas pasan. Limpia sus pods siempre (trap).
# Solo actúa sobre el clúster kind `aqs` (guarda require_kind_context, sale 3 en otro contexto).
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
# shellcheck disable=SC1091  # lib.sh se carga por ruta dinámica (relativa a root)
. "$root/scripts/kind/lib.sh"

require_kind_context # sale 3 si el contexto no es kind-aqs

NS=aqs-system
MC_POD=u8-t05-mc
CTRL_POD=u8-t05-controller
OTRO_POD=u8-t05-otro
RUN=run-bug-ecom-01 # la misma que gen_agent_fixtures.py (el prompt del fixture depende de estos bytes)
EV_DIR="$root/agents/dataset/artifacts/bug-ecom-01/evidence"
FIXTURE="$root/deploy/flux/kind/agents/planner/target-app.json"
PYH="${U8_AGENTS_PY:-$root/agents/agent-planner/.venv/bin/python}"
SCHEMAS="$root/contracts/plans"
fail=0
work=$(mktemp -d "${TMPDIR:-/tmp}/u8t05-agents.XXXXXX")
chmod 700 "$work"

cleanup() {
  pod_down "$NS" "$MC_POD" "$CTRL_POD" "$OTRO_POD"
  rm -rf "$work"
}
trap cleanup EXIT

check() { # <comprobacion> <comando...>
  local name=$1 out
  shift
  if out=$("$@" 2>&1); then
    echo "OK $name"
  else
    echo "FALLA $name"
    printf '%s\n' "$out" | sed 's/^/    /' >&2
    fail=1
  fi
}

# Python de los pods: POST de stdin a la URL de argv[1]; imprime el cuerpo de la respuesta (2xx) o falla.
PY_POST='import sys, urllib.request as u
r = u.Request(sys.argv[1], data=sys.stdin.buffer.read(), headers={"Content-Type": "application/json"}, method="POST")
sys.stdout.write(u.urlopen(r, timeout=60).read().decode())'

# Python de los pods: GET de argv[1] con timeout corto; sale distinto de 0 si no conecta.
PY_GET='import sys, urllib.request as u
u.urlopen(sys.argv[1], timeout=5)
print("conecta")'

# Validación en el host: argv = esquema, respuesta, [fichero de referencia].
PY_VALIDA='import json, sys
from jsonschema import Draft202012Validator
schema = json.load(open(sys.argv[1], encoding="utf-8"))
doc = json.load(open(sys.argv[2], encoding="utf-8"))
errs = sorted(Draft202012Validator(schema).iter_errors(doc), key=lambda e: list(e.path))
if errs:
    print("esquema:", "; ".join(e.message for e in errs[:3]))
    sys.exit(1)
if len(sys.argv) > 3:
    ref = json.load(open(sys.argv[3], encoding="utf-8"))
    for k in ("run_id", "workflow"):
        if k in ref and doc.get(k) != ref[k]:
            print(f"{k} no coincide")
            sys.exit(1)'

deploy_ok() { # <deployment>: availableReplicas = 1
  local d=$1 n
  n=$(kubectl get deploy "$d" -n "$NS" -o jsonpath='{.status.availableReplicas}' 2>/dev/null)
  [ "${n:-0}" = 1 ] || { echo "availableReplicas=${n:-0}"; return 1; }
}

sembrar() { # siembra la evidencia del dataset en s3://evidence/runs/<RUN>/ con el usuario de agentes
  local f rel
  while IFS= read -r f; do
    rel=${f#"$EV_DIR"/}
    mc_as aqs "$MC_POD" pipe "m/evidence/runs/$RUN/$rel" <"$f" >/dev/null || return 1
  done < <(find "$EV_DIR" -type f | sort)
}

uris_json() { # lista de URIs de la evidencia sembrada, en el orden del dataset
  local f rel
  while IFS= read -r f; do
    rel=${f#"$EV_DIR"/}
    printf 's3://evidence/runs/%s/%s\n' "$RUN" "$rel"
  done < <(find "$EV_DIR" -type f | sort) | jq -R . | jq -s .
}

planner_ok() {
  jq '{surface, workflow}' "$FIXTURE" >"$work/plan-req.json" || return 1
  kubectl exec -i -n "$NS" "$CTRL_POD" -- python -c "$PY_POST" \
    http://agent-planner.aqs-system.svc:8080/v1/plan <"$work/plan-req.json" >"$work/plan.json" || return 1
  "$PYH" -c "$PY_VALIDA" "$SCHEMAS/flow-plan.schema.json" "$work/plan.json" "$work/plan-req.json"
}

reporter_ok() {
  jq -n --arg run "$RUN" --argjson uris "$(uris_json)" '{run_id: $run, evidence_uris: $uris}' >"$work/rep-req.json" || return 1
  kubectl exec -i -n "$NS" "$CTRL_POD" -- python -c "$PY_POST" \
    http://agent-reporter.aqs-system.svc:8080/v1/report <"$work/rep-req.json" >"$work/report.json" || return 1
  "$PYH" -c "$PY_VALIDA" "$SCHEMAS/report.schema.json" "$work/report.json" || return 1
  local uri key
  uri=$(jq -r '.report_uri' "$work/report.json")
  key=${uri#s3://evidence/}
  case "$uri" in s3://evidence/runs/"$RUN"/report-*.json) ;; *) echo "report_uri inesperado"; return 1 ;; esac
  mc_as aqs "$MC_POD" cat "m/evidence/$key" >"$work/stored.json" || { echo "no se pudo leer $uri"; return 1; }
  # Lo guardado es el reporte sin report_uri (lo que añade el servicio al responder).
  jq -e --slurpfile r "$work/report.json" '. == ($r[0] | del(.report_uri))' "$work/stored.json" >/dev/null \
    || { echo "lo leído de MinIO no coincide con la respuesta"; return 1; }
  jq -e '.verdict == "bug"' "$work/stored.json" >/dev/null || { echo "veredicto inesperado"; return 1; }
}

otro_bloqueado() {
  local out
  if out=$(kubectl exec -i -n "$NS" "$OTRO_POD" -- python -c "$PY_GET" \
      http://agent-planner.aqs-system.svc:8080/healthz 2>&1); then
    echo "conecta sin la etiqueta: NetworkPolicy no aplica"; return 1
  fi
  grep -qi 'timed out' <<<"$out" || { echo "fallo sin timeout (¿DNS?):"; echo "$out"; return 1; }
  for u in http://agent-reporter.aqs-system.svc:8080/healthz; do
    if out=$(kubectl exec -i -n "$NS" "$OTRO_POD" -- python -c "$PY_GET" "$u" 2>&1); then
      echo "conecta a reporter sin la etiqueta"; return 1
    fi
    grep -qi 'timed out' <<<"$out" || { echo "reporter: fallo sin timeout"; return 1; }
  done
}

# 1. Disponibilidad de los Deployments.
check deploy-agent-planner-disponible deploy_ok agent-planner
check deploy-agent-reporter-disponible deploy_ok agent-reporter

# 2. Pods de prueba: el cliente (con la etiqueta de go-run-controller) y un intruso sin ella.
PLANNER_IMG=$(kubectl get deploy agent-planner -n "$NS" -o jsonpath='{.spec.template.spec.containers[0].image}') || PLANNER_IMG=""
if [ -z "$PLANNER_IMG" ]; then echo "FALLA imagen-de-agentes"; exit 1; fi
pod_sleep_up "$NS" "$CTRL_POD" "$PLANNER_IMG" go-run-controller || { echo "FALLA pod-controlador (no arranca)"; exit 1; }
pod_sleep_up "$NS" "$OTRO_POD" "$PLANNER_IMG" || { echo "FALLA pod-intruso (no arranca)"; exit 1; }
mc_pod_up "$NS" "$MC_POD" || { echo "FALLA pod-mc (no arranca)"; exit 1; }

# 3. Evidencia en MinIO: sembrar antes de pedir el reporte.
check sembrar-evidencia sembrar

check planner-plan-valido planner_ok
check reporter-lee-minio reporter_ok
check otro-pod-bloqueado otro_bloqueado

if [ "$fail" -eq 0 ]; then echo "rc=0"; else echo "rc=1"; fi
exit "$fail"
