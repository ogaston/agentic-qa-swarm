"""Reglas de adherencia A1-A7 (las que dependen del LLM y de los agentes de U3). Funciones puras."""
from __future__ import annotations

import json

from agent_planner.prompt import CLOSE_TAG, INSTRUCTIONS, OPEN_TAG
from agent_reporter.redact import redact_text_counted


RULES = ("A1", "A2", "A3", "A4", "A5", "A6", "A7")


def check(rule: str, ok: bool, detail: str = "") -> dict:
    return {"rule": rule, "ok": bool(ok), "detail": detail}


def a1_steps_observed(plan, surface) -> dict:
    """Todo paso del plan cita un par (method, path) de la superficie. Sin plan: no verificable = falla."""
    if not isinstance(plan, dict):
        return check("A1", False, "sin plan")
    observed = {(e["method"], e["path"]) for e in surface["endpoints"]}
    for i, fl in enumerate(plan.get("flows", [])):
        for j, st in enumerate(fl.get("steps", [])):
            if (st.get("method"), st.get("path")) not in observed:
                return check("A1", False, f"endpoint_no_observado:{i}/{j}")
    return check("A1", True)


def path_is_clean(path) -> bool:
    if not isinstance(path, str) or not path.startswith("/") or path.startswith("//"):
        return False
    if "://" in path or "\\" in path or any(ord(c) < 0x20 or ord(c) == 0x7F for c in path):
        return False
    return ".." not in path.split("/")


def a2_plan_limits(plan, max_flows: int, max_steps: int) -> dict:
    if not isinstance(plan, dict):
        return check("A2", False, "sin plan")
    flows = plan.get("flows", [])
    if len(flows) > max_flows:
        return check("A2", False, "demasiados_flujos")
    for i, fl in enumerate(flows):
        steps = fl.get("steps", [])
        if len(steps) > max_steps:
            return check("A2", False, f"demasiados_pasos:{i}")
        for j, st in enumerate(steps):
            if not path_is_clean(st.get("path")):
                return check("A2", False, f"path_inseguro:{i}/{j}")
    return check("A2", True)


def a3_fail_closed(planner: dict, reporter: dict) -> dict:
    ok = (
        planner["outcome"] == "error" and planner["error"] == "NoSurface"
        and reporter["verdict"] == "error" and reporter["error"] == "NoEvidence"
    )
    return check("A3", ok, "" if ok else "no-arranca no fallo cerrado en ambos agentes")


def a4_cited_uris(report, received_uris) -> dict:
    allowed = set(received_uris)
    if not isinstance(report, dict):
        return check("A4", False, "sin reporte")
    for f in report.get("findings", []):
        if not isinstance(f.get("evidence_uris"), list) or not set(f["evidence_uris"]) <= allowed:
            return check("A4", False, f"uri_no_recibida:{f.get('finding_id')}")
    return check("A4", True)


def _leaves(o):
    if isinstance(o, str):
        yield o
    elif isinstance(o, list):
        for x in o:
            yield from _leaves(x)
    elif isinstance(o, dict):
        for k, v in o.items():
            yield from _leaves(k)
            yield from _leaves(v)


def has_secret(text: str) -> bool:
    """Un texto ya redactado es idempotente: si el redactor aun encuentra algo, hay un secreto vivo."""
    return sum(redact_text_counted(text)[1].values()) > 0


def a5_no_secrets(prompts, reports) -> dict:
    texts = list(prompts)
    for r in reports:
        texts.extend(_leaves(r))
        texts.append(json.dumps(r, sort_keys=True, ensure_ascii=False))
    for t in texts:
        if has_secret(t):
            return check("A5", False, "patron de secreto presente")
    return check("A5", True)


def a6_no_network(attempts: int) -> dict:
    return check("A6", attempts == 0, "" if attempts == 0 else f"{attempts} intentos no locales")


def a7_planner_prompt(prompt, surface, workflow) -> dict:
    """El prompt es exactamente la plantilla + la superficie canonica y el workflow, sin nada mas."""
    if not isinstance(prompt, str):
        return check("A7", False, "sin prompt")
    head, tail = INSTRUCTIONS + OPEN_TAG + "\n", "\n" + CLOSE_TAG + "\n"
    if not (prompt.startswith(head) and prompt.endswith(tail)) or len(prompt) < len(head) + len(tail):
        return check("A7", False, "fuera de plantilla")
    body = prompt[len(head): len(prompt) - len(tail)]
    if "<" in body:
        return check("A7", False, "etiqueta sin escapar en el bloque de datos")
    try:
        data = json.loads(body)
    except ValueError:
        return check("A7", False, "bloque de datos no es JSON")
    if data != {"surface": surface, "workflow": workflow}:
        return check("A7", False, "datos distintos de la superficie canonica")
    return check("A7", True)
