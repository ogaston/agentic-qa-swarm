"""Validacion fail-closed de la salida del modelo. No repara ni recorta."""
from __future__ import annotations

import json
import re

from .errors import Limits, PlanRejected
from .schemas_loader import validator

FLOW_ID_RE = re.compile(r"[a-z0-9]([a-z0-9-]{0,40}[a-z0-9])?")


def _bad_const(_):
    raise ValueError("constante no JSON")


def parse_and_validate(text: str, surface: dict, workflow: str, limits: Limits) -> dict:
    try:
        plan = json.loads(text, parse_constant=_bad_const)
    except (ValueError, RecursionError):
        raise PlanRejected("json_invalido") from None
    if not isinstance(plan, dict):
        raise PlanRejected("no_es_objeto")
    errs = sorted(validator("flow-plan.schema.json").iter_errors(plan), key=lambda e: list(map(str, e.absolute_path)))
    if errs:
        e = errs[0]
        where = "/".join(str(p) for p in e.absolute_path) or "raiz"
        raise PlanRejected(f"esquema:{where}:{e.validator}")
    if plan["run_id"] != surface["run_id"]:
        raise PlanRejected("run_id_distinto")
    if plan["workflow"] != workflow:
        raise PlanRejected("workflow_distinto")
    flows = plan["flows"]
    if len(flows) > limits.max_flows:
        raise PlanRejected("demasiados_flujos")
    observed = {(e["method"], e["path"]) for e in surface["endpoints"]}
    seen = set()
    for i, fl in enumerate(flows):
        fid = fl["flow_id"]
        if not FLOW_ID_RE.fullmatch(fid):
            raise PlanRejected(f"flow_id_invalido:{i}")
        if fid in seen:
            raise PlanRejected(f"flow_id_duplicado:{i}")
        seen.add(fid)
        if len(fl["steps"]) > limits.max_steps:
            raise PlanRejected(f"demasiados_pasos:{i}")
        for j, st in enumerate(fl["steps"]):
            if (st["method"], st["path"]) not in observed:
                raise PlanRejected(f"endpoint_no_observado:{i}/{j}")
    return plan
