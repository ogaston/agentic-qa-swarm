"""Precision contra el subset golden/bug del dataset (la medicion completa es de U3-T05)."""
from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path

from agent_reporter.errors import ReporterError
from agent_reporter.fakes.evidence import DirEvidenceReader
from agent_reporter.fakes.fake_llm import FakeLLM
from agent_reporter.reporter import Limits, correlate_postmortem, prepare

THRESHOLD = 0.8
DEFAULT_DATASET = Path(__file__).resolve().parents[4] / "agents" / "dataset"


def _canned(art: Path, uris: list[str]) -> str:
    resp = json.loads((art / "reporter.response.json").read_text(encoding="utf-8"))
    out = {"verdict": resp["verdict"], "summary": "post-mortem sintetico", "findings": []}
    rc = resp.get("root_cause")
    for f in resp.get("findings", []):
        ev = [u for u in uris if u.endswith(f"/{f['flow_id']}/result.json")]
        out["findings"].append(
            {"finding_id": f["flow_id"], "root_cause": f["summary"], "invariant": rc["invariant"],
             "method": rc["method"], "path": rc["path"], "evidence_uris": ev}
        )
    return json.dumps(out)


def precision(dataset: Path | str | None = None) -> tuple[int, int]:
    ds = Path(dataset or os.environ.get("AQS_DATASET") or DEFAULT_DATASET)
    ok = total = 0
    for a in json.loads((ds / "manifest.json").read_text(encoding="utf-8"))["artifacts"]:
        if a["kind"] not in ("golden", "bug-sembrado"):
            continue
        art = ds / "artifacts" / a["id"]
        exp = json.loads((art / "expected.json").read_text(encoding="utf-8"))["reporter"]
        ev = json.loads((art / "evidence-uris.json").read_text(encoding="utf-8"))
        total += 1
        # los URIs del dataset son s3://aqs-evidence/runs/<run>/<flow>/<f>
        reader = _StripReader(art / "evidence", f"/runs/{ev['run_id']}/")
        limits = Limits()
        llm = FakeLLM()
        prep = prepare(ev["run_id"], ev["uris"], reader, limits)
        llm.register(prep.prompt, _canned(art, ev["uris"]))
        try:
            rep = correlate_postmortem(ev["run_id"], ev["uris"], reader, llm, limits)
        except ReporterError:
            continue
        if rep["verdict"] != exp["verdict"]:
            continue
        if rep["verdict"] == "bug":
            rc = exp["root_cause"]
            got = {(f["invariant"], f["method"], f["path"]) for f in rep["findings"]}
            if (rc["invariant"], rc["method"], rc["path"]) not in got:
                continue
        ok += 1
    return ok, total


class _StripReader(DirEvidenceReader):
    def __init__(self, root, prefix):
        super().__init__(root)
        self.prefix = prefix

    def get(self, uri):
        i = uri.index(self.prefix)
        return super().get("x://h/" + uri[i + len(self.prefix):])


def main(argv=None) -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--dataset")
    args = ap.parse_args(argv)
    ok, total = precision(args.dataset)
    p = ok / total if total else 0.0
    print(json.dumps({"evaluated": total, "correct": ok, "precision": round(p, 4)}))
    return 0 if p >= THRESHOLD else 1


if __name__ == "__main__":
    sys.exit(main())
