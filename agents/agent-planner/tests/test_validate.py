import copy
import json

import pytest

from agent_planner.errors import Limits, PlanRejected
from agent_planner.validate import parse_and_validate
from helpers import SURFACE, good_plan

L = Limits(max_flows=3, max_steps=2)


def run(p, text=None, limits=L):
    return parse_and_validate(json.dumps(p) if text is None else text, SURFACE, "wf-1", limits)


def second_flow(fid="flow-2"):
    f = copy.deepcopy(good_plan()["flows"][0])
    f["flow_id"] = fid
    return f


def test_validate_accepts_good_plan():
    assert run(good_plan()) == good_plan()


@pytest.mark.parametrize("text", ["no json", "[]", "NaN", '{"run_id": NaN}', ""])
def test_validate_broken_json_or_not_object(text):
    with pytest.raises(PlanRejected):
        run(None, text)


def test_validate_schema_violations():
    for mut in (
        lambda p: p.pop("flows"),
        lambda p: p.update(extra=1),
        lambda p: p["flows"][0].update(extra=1),
        lambda p: p["flows"][0]["steps"][0].pop("expect_status"),
        lambda p: p["flows"][0]["steps"][0].update(extra=1),
        lambda p: p.update(flows=[]),
    ):
        p = good_plan(); mut(p)
        with pytest.raises(PlanRejected) as e:
            run(p)
        assert e.value.reason.startswith("esquema:")


def test_validate_unobserved_endpoint_and_method_mismatch():
    p = good_plan(); p["flows"][0]["steps"][0]["path"] = "/otra"
    with pytest.raises(PlanRejected):
        run(p)
    p = good_plan(); p["flows"][0]["steps"][0]["method"] = "DELETE"  # /orders existe solo como POST
    with pytest.raises(PlanRejected):
        run(p)


def test_validate_runid_and_workflow_mismatch():
    for k, v in (("run_id", "x"), ("workflow", "y")):
        p = good_plan(); p[k] = v
        with pytest.raises(PlanRejected):
            run(p)


@pytest.mark.parametrize("fid", ["Flow", "-a", "a-", "a_b", "a" * 43, "", "a b"])
def test_validate_flow_id_invalid(fid):
    p = good_plan(); p["flows"][0]["flow_id"] = fid
    with pytest.raises(PlanRejected):
        run(p)


def test_validate_flow_id_valid_edges():
    for fid in ("a", "ab", "a" * 42, "a-b"):
        p = good_plan(); p["flows"][0]["flow_id"] = fid
        run(p)


def test_validate_duplicate_flow_id():
    p = good_plan(); p["flows"].append(second_flow("flow-1"))
    with pytest.raises(PlanRejected):
        run(p)


def test_validate_too_many_flows_not_trimmed():
    p = good_plan(); p["flows"] += [second_flow(f"f-{i}") for i in range(3)]
    with pytest.raises(PlanRejected) as e:
        run(p)
    assert e.value.reason == "demasiados_flujos"
    p["flows"] = p["flows"][:3]
    assert len(run(p)["flows"]) == 3


def test_validate_too_many_steps():
    p = good_plan(); p["flows"][0]["steps"] *= 3
    with pytest.raises(PlanRejected) as e:
        run(p)
    assert e.value.reason.startswith("demasiados_pasos")
