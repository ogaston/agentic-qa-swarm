"""U8-T08b: los campos opcionales `body` (paso) e `invariant_probe` (flujo) son emitibles y validados;
los planes sin ellos siguen válidos y renderizan igual en k6."""
import copy
import json

import pytest

from agent_planner.errors import PlanRejected
from agent_planner.k6.render import extract_flow, render_flow
from agent_planner.prompt import INSTRUCTIONS, build_prompt
from agent_planner.schemas_loader import validator
from agent_planner.validate import parse_and_validate
from helpers import LIMITS, ROOT, SURFACE, good_plan, load

VALID_DIR = ROOT / "contracts" / "plans" / "examples" / "valid"
INVALID_DIR = ROOT / "contracts" / "plans" / "examples" / "invalid"


def run(p):
    return parse_and_validate(json.dumps(p), SURFACE, "wf-1", LIMITS)


def plan_con_sondeo():
    p = copy.deepcopy(good_plan())
    p["flows"][0]["steps"][0]["body"] = {"product_id": 1, "qty": 11}
    p["flows"][0]["invariant_probe"] = True
    return p


def test_plan_con_body_e_invariant_probe_se_acepta():
    assert run(plan_con_sondeo()) == plan_con_sondeo()


def test_plan_sin_campos_nuevos_sigue_valido():
    assert run(good_plan()) == good_plan()
    for f in (VALID_DIR).glob("flow-plan*.json"):
        assert list(validator("flow-plan.schema.json").iter_errors(load(f))) == [], f.name


@pytest.mark.parametrize("body", [[1, 2], "x", 3, None])
def test_body_que_no_es_objeto_se_rechaza(body):
    p = plan_con_sondeo()
    p["flows"][0]["steps"][0]["body"] = body
    with pytest.raises(PlanRejected) as e:
        run(p)
    assert e.value.reason.startswith("esquema:flows/0/steps/0")


def test_invariant_probe_no_booleano_se_rechaza():
    p = plan_con_sondeo()
    p["flows"][0]["invariant_probe"] = "si"
    with pytest.raises(PlanRejected) as e:
        run(p)
    assert e.value.reason.startswith("esquema:flows/0")


def test_campo_extra_en_paso_sigue_rechazado():
    p = plan_con_sondeo()
    p["flows"][0]["steps"][0]["headers"] = {"X-Debug": "1"}
    with pytest.raises(PlanRejected):
        run(p)


def test_ejemplos_invalidos_nuevos_fallan_el_esquema():
    for name in ("flow-plan.body-no-objeto.json", "flow-plan.invariant-probe-no-booleano.json",
                 "flow-plan.campo-extra-en-paso.json"):
        assert list(validator("flow-plan.schema.json").iter_errors(load(INVALID_DIR / name))), name


def test_plan_viejo_renderiza_en_k6_sin_cambio():
    plan = load(VALID_DIR / "flow-plan.un-flujo.json")
    flow = plan["flows"][0]
    script = render_flow(plan, flow)
    assert extract_flow(script) == {k: flow[k] for k in ("flow_id", "name", "invariant", "steps")}


def test_prompt_documenta_body_e_invariant_probe():
    assert "body" in INSTRUCTIONS and "invariant_probe" in INSTRUCTIONS
    assert "invariant_probe" in build_prompt(SURFACE, "wf-1")


def test_render_k6_falla_cerrado_con_body_o_sondeo():
    from agent_planner.k6.errors import FlowValidationError
    from agent_planner.k6.render import render_flow as rf
    p = plan_con_sondeo()
    with pytest.raises(FlowValidationError, match="body"):
        rf(p, p["flows"][0])
    q = copy.deepcopy(good_plan())
    q["flows"][0]["invariant_probe"] = True
    with pytest.raises(FlowValidationError, match="invariant_probe"):
        rf(q, q["flows"][0])


def test_prompt_pide_sondeos_de_invariantes_con_cuerpos_validos():
    # El LLM real solo planeaba caminos felices: el prompt debe exigir sondeos y mantener cuerpos conformes.
    assert "DEBES incluir al menos un flujo con invariant_probe true" in INSTRUCTIONS
    assert "stock" in INSTRUCTIONS and "rechazo" in INSTRUCTIONS
    assert "nunca el formato" in INSTRUCTIONS
