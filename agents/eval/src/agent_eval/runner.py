"""Ejecucion de planner y reporter sobre un artefacto. Cada agente corre como biblioteca, con su FakeLLM."""
from __future__ import annotations

import json
from pathlib import Path

from agent_planner.errors import Limits as PlannerLimits
from agent_planner.fakes.fake_llm import FakeLLM as PlannerFake
from agent_planner.planner import plan as real_plan
from agent_planner.prompt import build_prompt
from agent_reporter.fakes.evidence import DirEvidenceReader
from agent_reporter.fakes.fake_llm import FakeLLM as ReporterFake
from agent_reporter.precision import _canned
from agent_reporter.reporter import Limits as ReporterLimits
from agent_reporter.reporter import correlate_postmortem as real_report
from agent_reporter.reporter import prepare

from . import checks as C
from .dataset import Artifact, read_json
from .netguard import NetworkGuard

WORKFLOW = "wf-dataset"


class _Rec:
    """Mixin: registra los prompts realmente enviados al modelo."""

    def __init__(self, *a, **k) -> None:
        super().__init__(*a, **k)
        self.prompts: list[str] = []

    def complete(self, prompt, **kw):
        self.prompts.append(prompt)
        return super().complete(prompt, **kw)


class RecPlannerLLM(_Rec, PlannerFake):
    pass


class RecReporterLLM(_Rec, ReporterFake):
    pass


class StripReader(DirEvidenceReader):
    """Las URIs del dataset son s3://aqs-evidence/runs/<run>/<flujo>/<archivo>; se mapean a evidence/."""

    def __init__(self, root, prefix: str) -> None:
        super().__init__(root)
        self.prefix = prefix

    def get(self, uri):
        i = uri.index(self.prefix)
        return super().get("x://h/" + uri[i + len(self.prefix):])


def canned_plan(surface: dict, workflow: str, resp: dict) -> dict:
    """Adapta planner.response.json del dataset al contrato FlowPlan."""
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


def _err(e: BaseException) -> str:
    return type(e).__name__


def run_planner(art: Artifact, surface: dict, plan_fn, limits: PlannerLimits):
    llm = RecPlannerLLM()
    result = {"outcome": "error", "flows": 0, "error": None}
    plan = None
    try:
        resp = read_json(art.dir / "planner.response.json")
        text = "{}" if "error" in resp else json.dumps(canned_plan(surface, WORKFLOW, resp))
        llm.register(build_prompt(surface, WORKFLOW), text)
        plan = plan_fn(surface, WORKFLOW, llm, limits)
        result.update(outcome="plan", flows=len(plan["flows"]))
    except Exception as e:  # el arnes registra y sigue: un agente roto es un dato, no un cierre
        result["error"] = _err(e)
        plan = None
    return result, plan, llm.prompts


def run_reporter(art: Artifact, surface: dict, report_fn, limits: ReporterLimits):
    llm = RecReporterLLM()
    result = {"verdict": "error", "findings": [], "error": None}
    report, uris = None, []
    try:
        ev_path = art.dir / "evidence-uris.json"
        if ev_path.is_file():
            ev = read_json(ev_path)
            run_id, uris = ev["run_id"], ev["uris"]
        else:
            run_id, uris = surface["run_id"], []
        reader = StripReader(art.dir / "evidence", f"/runs/{run_id}/")
        if uris:
            llm.register(prepare(run_id, uris, reader, limits).prompt, _canned(art.dir, uris))
        report = report_fn(run_id, uris, reader, llm, limits)
        result.update(
            verdict=report["verdict"],
            findings=[
                {k: f[k] for k in ("finding_id", "invariant", "method", "path")} for f in report["findings"]
            ],
        )
    except Exception as e:
        result["error"] = _err(e)
        report = None
    return result, report, uris, llm.prompts


def run_artifact(
    art: Artifact,
    expected: dict,
    guard: NetworkGuard,
    *,
    plan_fn=real_plan,
    report_fn=real_report,
    planner_limits: PlannerLimits | None = None,
    reporter_limits: ReporterLimits | None = None,
) -> dict:
    planner_limits = planner_limits or PlannerLimits()
    reporter_limits = reporter_limits or ReporterLimits()
    surface = read_json(art.dir / "surface.json")
    before = len(guard.attempts)
    p_res, plan, p_prompts = run_planner(art, surface, plan_fn, planner_limits)
    r_res, report, uris, r_prompts = run_reporter(art, surface, report_fn, reporter_limits)
    attempts = len(guard.attempts) - before

    cks = []
    if expected["planner"]["outcome"] == "plan":
        cks.append(C.a1_steps_observed(plan, surface))
        cks.append(C.a2_plan_limits(plan, planner_limits.max_flows, planner_limits.max_steps))
        cks.append(C.a7_planner_prompt(p_prompts[0] if len(p_prompts) == 1 else None, surface, WORKFLOW))
    if art.kind == "no-arranca":
        cks.append(C.a3_fail_closed(p_res, r_res))
    if report is not None:
        cks.append(C.a4_cited_uris(report, uris))
    if p_prompts or report is not None:
        cks.append(C.a5_no_secrets(p_prompts + r_prompts, [report] if report is not None else []))
    cks.append(C.a6_no_network(attempts))
    return {
        "id": art.id,
        "kind": art.kind,
        "expected": {"planner": expected["planner"]["outcome"], **expected["reporter"]},
        "planner": p_res,
        "reporter": r_res,
        "checks": cks,
    }
