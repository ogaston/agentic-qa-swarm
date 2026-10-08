import json

import pytest
from jsonschema import Draft202012Validator

from agent_planner.errors import NoSurface
from agent_planner.planner import plan
from helpers import DS, LIMITS, ROOT, dataset_ids, llm_for, load, to_flow_plan

FLOW_SCHEMA = Draft202012Validator(load(ROOT / "contracts" / "plans" / "flow-plan.schema.json"))
WORKFLOW = "wf-dataset"


@pytest.mark.parametrize("aid", dataset_ids())
def test_dataset_artifact_plan(aid):
    d = DS / "artifacts" / aid
    surface, exp = load(d / "surface.json"), load(d / "expected.json")["planner"]
    if exp["outcome"] == "error":
        llm = llm_for(surface, WORKFLOW, "{}")
        with pytest.raises(NoSurface):
            plan(surface, WORKFLOW, llm, LIMITS)
        assert llm.calls == 0
        return
    expected = to_flow_plan(surface, WORKFLOW, load(d / "planner.response.json"))
    llm = llm_for(surface, WORKFLOW, json.dumps(expected))
    got = plan(surface, WORKFLOW, llm, LIMITS)
    assert not list(FLOW_SCHEMA.iter_errors(got))
    assert json.dumps(got, sort_keys=True) == json.dumps(expected, sort_keys=True)
    for inv in exp["must_cover_invariants"]:
        assert any(inv in f["invariant"] for f in got["flows"]), inv
    # determinismo: segunda corrida identica byte a byte
    again = plan(surface, WORKFLOW, llm, LIMITS)
    assert json.dumps(again, sort_keys=True) == json.dumps(got, sort_keys=True)


def test_dataset_schemas_packaged_copy_matches_contracts():
    from agent_planner import schemas_loader
    for n in ("flow-plan.schema.json", "surface-artifact.schema.json"):
        a = (schemas_loader._DIR / n).read_bytes()
        assert a == (ROOT / "contracts" / "plans" / n).read_bytes(), n
