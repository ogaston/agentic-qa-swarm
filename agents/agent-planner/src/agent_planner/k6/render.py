"""Render determinista de un flujo del FlowPlan a un script k6 (plantilla fija + literal JSON)."""
import hashlib
import json
from pathlib import Path

TEMPLATE_PATH = Path(__file__).with_name("template.js")
PLACEHOLDER = "__AQS_FLOW__"
FLOW_PREFIX = "const FLOW = "
TEMPLATE_SHA256 = "23fc5a575cb31e109973a8180eada975a5fcc2f34587447fdf837b2f3c31a1c8"


def load_template() -> str:
    return TEMPLATE_PATH.read_bytes().decode("utf-8")


def template_sha256(template: str | None = None) -> str:
    t = load_template() if template is None else template
    return hashlib.sha256(t.encode("utf-8")).hexdigest()


def flow_literal(flow: dict) -> str:
    data = {k: flow[k] for k in ("flow_id", "name", "invariant", "steps")}
    return json.dumps(data, sort_keys=True, ensure_ascii=True)


def render_flow(plan: dict, flow: dict) -> str:
    """Puro: mismo plan/flujo -> mismos bytes. El plan solo entra como literal JSON."""
    template = load_template()
    return template.replace(PLACEHOLDER, flow_literal(flow), 1)


def extract_flow(script: str) -> dict:
    """Inversa de render_flow: lee el literal FLOW."""
    lines = [ln for ln in script.split("\n") if ln.startswith(FLOW_PREFIX)]
    if len(lines) != 1 or not lines[0].endswith(";"):
        raise ValueError("el script no tiene exactamente un literal FLOW")
    return json.loads(lines[0][len(FLOW_PREFIX):-1])
