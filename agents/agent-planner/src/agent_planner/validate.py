"""Validacion fail-closed de la salida del modelo. No repara ni recorta."""
from __future__ import annotations

import json

from jsonschema import Draft202012Validator
from jsonschema.exceptions import SchemaError, UnknownType

from .errors import Limits, PlanRejected
from .flow_id import FLOW_ID_RE
from .schemas_loader import validator


def _bad_const(_):
    raise ValueError("constante no JSON")


BODY_METHODS = {"POST", "PUT", "PATCH"}


def _checkable(schema) -> bool:
    """Un esquema con $ref (local o externo) no se resuelve aqui (sin red): no se comprueba el cuerpo."""
    if isinstance(schema, dict):
        return "$ref" not in schema and all(_checkable(v) for v in schema.values())
    if isinstance(schema, list):
        return all(_checkable(v) for v in schema)
    return True


def _check_body(st: dict, request_body, i: int, j: int) -> None:
    """Con request_body en la superficie, un paso POST/PUT/PATCH debe llevar body conforme."""
    if request_body is None or st["method"] not in BODY_METHODS:
        return
    if "body" not in st:
        raise PlanRejected(f"body_requerido:{i}/{j}")
    if not _checkable(request_body):
        return  # degradacion documentada: el esquema no se puede comprobar sin resolver referencias
    try:
        Draft202012Validator.check_schema(request_body)
        err = next(Draft202012Validator(request_body).iter_errors(st["body"]), None)
    except (SchemaError, UnknownType):
        return  # esquema que el validador no soporta: no se comprueba, nunca es error de ejecucion
    if err is not None:
        where = "/".join(str(p) for p in err.absolute_path) or "raiz"
        raise PlanRejected(f"body_invalido:{i}/{j}:{where}:{err.validator}")


def load_plan_text(text: str) -> dict:
    """Texto de la respuesta del modelo -> objeto JSON (sin validar el esquema)."""
    try:
        plan = json.loads(text, parse_constant=_bad_const)
    except (ValueError, RecursionError):
        raise PlanRejected("json_invalido") from None
    if not isinstance(plan, dict):
        raise PlanRejected("no_es_objeto")
    return plan


def parse_and_validate(text: str, surface: dict, workflow: str, limits: Limits) -> dict:
    return validate_plan_dict(load_plan_text(text), surface, workflow, limits)


def validate_plan_dict(plan: dict, surface: dict, workflow: str, limits: Limits) -> dict:
    """Validacion fail-closed de un plan ya parseado (no repara: la normalizacion de flow_id va antes, en planner)."""
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
    surface_bodies = {(e["method"], e["path"]): e["request_body"] for e in surface["endpoints"] if "request_body" in e}
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
            _check_body(st, surface_bodies.get((st["method"], st["path"])), i, j)
    return plan
