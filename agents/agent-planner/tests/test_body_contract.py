"""U8-T09a CA-3: la superficie describe request_body y el planner lo usa (prompt y validacion del cuerpo)."""
import copy
import json

import pytest

from agent_planner.errors import Limits, PlanRejected
from agent_planner.prompt import INSTRUCTIONS, build_prompt
from agent_planner.validate import parse_and_validate
from helpers import SURFACE, good_plan

L = Limits(max_flows=3, max_steps=2)
ORDER_BODY = {
    "type": "object",
    "required": ["product_id", "qty"],
    "properties": {"product_id": {"type": "integer", "minimum": 1}, "qty": {"type": "integer", "minimum": 1}},
}


def surface_rb(schema=ORDER_BODY):
    s = copy.deepcopy(SURFACE)
    s["endpoints"][0]["request_body"] = schema
    return s


def plan_con_body(body=None, method="POST", path="/orders"):
    p = good_plan()
    step = {"method": method, "path": path, "expect_status": 201}
    if body is not None:
        step["body"] = body
    p["flows"][0]["steps"] = [step]
    return p


def run(p, surface):
    return parse_and_validate(json.dumps(p), surface, "wf-1", L)


def test_prompt_documenta_request_body_y_body_conforme():
    assert "request_body" in INSTRUCTIONS
    assert "debe llevar body conforme" in INSTRUCTIONS
    prompt = build_prompt(surface_rb(), "wf-1")
    assert prompt.startswith(INSTRUCTIONS)


def test_validate_body_rechaza_post_sin_body_cuando_hay_request_body():
    with pytest.raises(PlanRejected):
        run(plan_con_body(), surface_rb())


def test_validate_body_rechaza_qty_como_texto():
    with pytest.raises(PlanRejected):
        run(plan_con_body({"product_id": 1, "qty": "1"}), surface_rb())


def test_validate_body_rechaza_qty_bajo_minimo():
    with pytest.raises(PlanRejected):
        run(plan_con_body({"product_id": 1, "qty": 0}), surface_rb())


def test_validate_body_acepta_cuerpo_conforme():
    p = plan_con_body({"product_id": 1, "qty": 1})
    assert run(p, surface_rb()) == p


def test_validate_body_sin_request_body_sigue_igual():
    p = plan_con_body()  # POST /orders sin body, superficie sin request_body
    assert run(p, SURFACE) == p


def test_validate_body_schema_no_comprobable_no_es_error():
    # $ref no se resuelve (sin red): el cuerpo no se comprueba y el plan pasa.
    surf = surface_rb({"$ref": "https://otro.example/orden.json"})
    p = plan_con_body({"cualquier": "cosa"})
    assert run(p, surf) == p
