import json
import shutil
from pathlib import Path

from jsonschema import Draft202012Validator

ROOT = Path(__file__).resolve().parents[3]
DS = ROOT / "agents" / "dataset"
EVAL = Path(__file__).resolve().parents[1]
SCHEMA = json.loads((EVAL / "report.schema.json").read_text())


def report_ok(rep):
    return not list(Draft202012Validator(SCHEMA).iter_errors(rep))


def copy_ds(tmp_path) -> Path:
    dst = tmp_path / "ds"
    shutil.copytree(DS, dst)
    return dst


def edit(path: Path, fn):
    d = json.loads(path.read_text())
    fn(d)
    path.write_text(json.dumps(d))


def by_id(rep, aid):
    return next(r for r in rep["per_artifact"] if r["id"] == aid)


def rule(rec, name):
    return [c for c in rec["checks"] if c["rule"] == name]


def rec(kind, exp_verdict, got_verdict, findings=(), rc=None, checks=()):
    return {
        "id": "x", "kind": kind,
        "expected": {"planner": "plan", "verdict": exp_verdict, "root_cause": rc},
        "planner": {"outcome": "plan", "flows": 1, "error": None},
        "reporter": {"verdict": got_verdict, "findings": list(findings), "error": None},
        "checks": list(checks),
    }


RC = {"invariant": "i", "method": "POST", "path": "/o"}


def fnd(invariant="i", method="POST", path="/o", fid="f"):
    return {"finding_id": fid, "invariant": invariant, "method": method, "path": path}
