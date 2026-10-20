"""Planes de ejemplo: contracts/plans/examples/valid + planes derivados del dataset."""
import json
from pathlib import Path

from .render import k6_problem

ROOT = Path(__file__).resolve().parents[5]


def example_plans(root: Path = ROOT) -> list[tuple[str, dict]]:
    out = []
    for p in sorted((root / "contracts" / "plans" / "examples" / "valid").glob("flow-plan.*.json")):
        plan = json.loads(p.read_text(encoding="utf-8"))
        # Los flujos con body o invariant_probe no son renderizables en k6 (U8-T08b); se prueban en test_flow_plan_probe.py.
        if any(k6_problem(f) for f in plan["flows"]):
            continue
        out.append((p.stem.replace("flow-plan.", "ej-"), plan))
    for d in sorted((root / "agents" / "dataset" / "artifacts").iterdir()):
        resp = json.loads((d / "planner.response.json").read_text(encoding="utf-8"))
        if "flows" not in resp:
            continue
        flows = []
        for f in resp["flows"]:
            steps = [{"method": s["method"], "path": s["path"],
                      "expect_status": 201 if s["method"] == "POST" else 200} for s in f["steps"]]
            flows.append({"flow_id": f["id"], "name": f["id"], "steps": steps,
                          "invariant": f["invariants"][0]})
        out.append((d.name, {"run_id": d.name, "workflow": "dataset", "flows": flows}))
    return out
