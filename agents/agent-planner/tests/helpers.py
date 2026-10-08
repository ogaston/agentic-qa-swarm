"""Utilidades compartidas de las pruebas del planner."""
import copy
import json
from pathlib import Path

from agent_planner.errors import Limits
from agent_planner.fakes.fake_llm import FakeLLM
from agent_planner.prompt import build_prompt

ROOT = Path(__file__).resolve().parents[3]
DS = ROOT / "agents" / "dataset"

LIMITS = Limits(timeout_s=25.0, max_input_tokens=8000, max_output_tokens=4000)


def load(p):
    return json.loads(Path(p).read_text(encoding="utf-8"))


def dataset_ids():
    return [a["id"] for a in load(DS / "manifest.json")["artifacts"]]


def to_flow_plan(surface, workflow, resp):
    """Adapta planner.response.json del dataset (T01) al contrato FlowPlan."""
    return {
        "run_id": surface["run_id"],
        "workflow": workflow,
        "flows": [
            {
                "flow_id": f["id"],
                "name": f["id"],
                "steps": [dict(s, expect_status=200) for s in f["steps"]],
                "invariant": "; ".join(f["invariants"]),
            }
            for f in resp["flows"]
        ],
    }


SURFACE = {
    "run_id": "run-1",
    "base_url": "http://x.warm.svc.cluster.local:8080",
    "endpoints": [{"method": "POST", "path": "/orders"}, {"method": "GET", "path": "/orders/{id}"}],
    "source": "openapi",
}


def good_plan(surface=SURFACE, workflow="wf-1"):
    return {
        "run_id": surface["run_id"],
        "workflow": workflow,
        "flows": [
            {
                "flow_id": "flow-1",
                "name": "crear orden",
                "steps": [{"method": "POST", "path": "/orders", "expect_status": 201}],
                "invariant": "el stock no es negativo",
            }
        ],
    }


def llm_for(surface, workflow, text, **kw):
    f = FakeLLM()
    f.register(build_prompt(surface, workflow), text, **kw)
    return f


def mutated(**changes):
    s = copy.deepcopy(SURFACE)
    s.update(changes)
    return s
