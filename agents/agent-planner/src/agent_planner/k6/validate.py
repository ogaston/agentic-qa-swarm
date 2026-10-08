import json
import os
import re
import subprocess
from pathlib import Path

from jsonschema import Draft202012Validator

from .errors import FlowValidationError, K6InspectError
from .render import (FLOW_PREFIX, PLACEHOLDER, TEMPLATE_SHA256, extract_flow, load_template,
                     template_sha256)
from .store import ID_RE

SCHEMA_PATH = Path(__file__).resolve().parents[5] / "contracts" / "plans" / "flow-plan.schema.json"
K6_IMAGE = "docker.io/grafana/k6:0.55.0"
FORBIDDEN = [re.compile(p) for p in (
    r"\beval\s*\(", r"\bFunction\s*\(", r"\brequire\s*\(", r"\bopen\s*\(", r"\bimport\s*\(",
    r"\bXMLHttpRequest\b", r"\bfetch\s*\(", r"__ENV\s*\[", r"__ENV\.(?!BASE_URL\b)",
)]


def validate_plan(plan: dict) -> None:
    """Capa 1: esquema + identificadores seguros y unicos."""
    schema = json.loads(SCHEMA_PATH.read_text(encoding="utf-8"))
    errs = sorted(Draft202012Validator(schema).iter_errors(plan), key=lambda e: list(e.path))
    if errs:
        raise FlowValidationError("-", "plan", errs[0].message)
    if not ID_RE.fullmatch(plan["run_id"]):
        raise FlowValidationError("-", "plan", "run_id fuera del patron")
    seen = set()
    for f in plan["flows"]:
        if not ID_RE.fullmatch(f["flow_id"]) or f["flow_id"] in seen:
            raise FlowValidationError(f["flow_id"], "plan", "flow_id invalido o duplicado")
        seen.add(f["flow_id"])


def validate_script(script: str, plan: dict, flow: dict) -> None:
    """Capa 2: plantilla v1 exacta (hash), round-trip, sin cadenas prohibidas fuera del literal."""
    fid = flow["flow_id"]

    def bad(msg):
        raise FlowValidationError(fid, "script", msg)

    if template_sha256() != TEMPLATE_SHA256:
        bad("la plantilla en disco no coincide con el hash fijado")
    lines = script.split("\n")
    idx = [i for i, ln in enumerate(lines) if ln.startswith(FLOW_PREFIX)]
    if len(idx) != 1:
        bad("literal FLOW ausente o repetido")
    skeleton = "\n".join(lines[:idx[0]] + [FLOW_PREFIX + PLACEHOLDER + ";"] + lines[idx[0] + 1:])
    if skeleton != load_template():
        bad("el script no es la plantilla v1 exacta")
    outside = "\n".join(lines[:idx[0]] + lines[idx[0] + 1:])
    for pat in FORBIDDEN:
        if pat.search(outside):
            bad(f"cadena prohibida fuera del literal: {pat.pattern}")
    try:
        if extract_flow(script) != flow:
            bad("round-trip: el literal no coincide con el flujo del plan")
    except ValueError as e:
        bad(str(e))


def inspect_with_k6(path) -> None:
    """Capa 3: k6 inspect real (K6_BIN o podman con la imagen fijada). Lanza K6InspectError si rc != 0."""
    p = Path(path).resolve()
    k6 = os.environ.get("K6_BIN")
    if k6:
        cmd = [k6, "inspect", str(p)]
    else:
        cmd = ["podman", "run", "--rm", "--user", "0:0", "--network", "none", "-v", f"{p.parent}:/w:z", "-w", "/w",
               K6_IMAGE, "inspect", f"/w/{p.name}"]
    r = subprocess.run(cmd, capture_output=True, text=True, timeout=120)
    if r.returncode != 0:
        raise K6InspectError(f"rc={r.returncode}: {r.stderr[-500:]}")
