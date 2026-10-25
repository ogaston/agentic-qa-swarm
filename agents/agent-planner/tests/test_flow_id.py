"""U11-T04: flow_id normalizado por el planner antes de validar; cadena completa sin clúster."""
import json

from agent_planner.planner import plan
from helpers import LIMITS, SURFACE, good_plan, llm_for


def test_cadena_flow_id_malo_llega_normalizado():
    bad = good_plan()
    bad["flows"][0]["flow_id"] = "Oversell_Probe"
    out = plan(SURFACE, "wf-1", llm_for(SURFACE, "wf-1", json.dumps(bad)), LIMITS)
    assert out["flows"][0]["flow_id"] == "oversell-probe"


import copy
import re

from hypothesis import given
from hypothesis import strategies as st

from agent_planner.flow_id import FLOW_ID_PATTERN, normalize_flow_id, normalize_plan_flow_ids
from helpers import load, ROOT

FLOW_RE = re.compile(FLOW_ID_PATTERN)


def test_normalize_ejemplo_del_spec():
    assert normalize_flow_id("Oversell_Invariant Probe!") == "oversell-invariant-probe"


def test_normalize_largo_acotado_estable_y_distinto():
    a = normalize_flow_id("x" * 60)
    assert len(a) <= 35 and FLOW_RE.fullmatch(a)
    assert a == normalize_flow_id("x" * 60)  # estable entre llamadas
    b = normalize_flow_id("x" * 59 + "y")  # mismo comienzo, otro id
    assert a != b


def test_normalize_vacio_o_solo_simbolos_es_flujo():
    for raw in ("", "!!!", "___", "   ", "ñ日本"):
        assert normalize_flow_id(raw) == "flujo", repr(raw)


def test_normalize_colision_dentro_del_plan():
    plan = {"flows": [{"flow_id": "Flujo"}, {"flow_id": "flujo!"}, {"flow_id": "flujo"}]}
    ids = [f["flow_id"] for f in normalize_plan_flow_ids(plan)["flows"]]
    assert ids == ["flujo", "flujo-2", "flujo-3"]


def test_normalize_colision_respeta_el_limite_de_35():
    base = "a" * 35
    plan = {"flows": [{"flow_id": base}, {"flow_id": base}]}
    ids = [f["flow_id"] for f in normalize_plan_flow_ids(plan)["flows"]]
    assert len(set(ids)) == 2 and all(len(i) <= 35 and FLOW_RE.fullmatch(i) for i in ids)


def test_normalize_plan_ignora_lo_que_no_es_cadena():
    plan = {"flows": [{"flow_id": 7}, {"name": "sin id"}, "no-objeto"]}
    assert normalize_plan_flow_ids(copy.deepcopy(plan)) == plan


@given(st.text())
def test_flow_id_pbt_normalizacion_cumple_patron_e_idempotente(raw):
    out = normalize_flow_id(raw)
    assert FLOW_RE.fullmatch(out) and len(out) <= 35
    assert normalize_flow_id(out) == out


@given(st.lists(st.text(), min_size=1, max_size=8))
def test_flow_id_pbt_plan_unico_y_en_patron(raws):
    flows = normalize_plan_flow_ids({"flows": [{"flow_id": r} for r in raws]})["flows"]
    ids = [f["flow_id"] for f in flows]
    assert len(set(ids)) == len(ids)
    assert all(FLOW_RE.fullmatch(i) for i in ids)


def test_fixtures_planner_kind_pasan_el_validador():
    from agent_planner.validate import validate_plan_dict
    from agent_planner.errors import Limits

    fx = load(ROOT / "deploy" / "flux" / "kind" / "agents" / "planner" / "target-app.json")
    out = validate_plan_dict(copy.deepcopy(fx["plan"]), fx["surface"], fx["workflow"], Limits(max_flows=5, max_steps=10))
    assert out["flows"]
