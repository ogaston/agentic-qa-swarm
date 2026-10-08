import json
from pathlib import Path

from jsonschema import Draft202012Validator

from agent_reporter.fakes.evidence import DirEvidenceReader
from agent_reporter.fakes.fake_llm import FakeLLM
from agent_reporter.fakes.publisher import RecordingPublisher
from agent_reporter.fakes.store import MemoryReportStore
from agent_reporter.reporter import Limits, correlate_postmortem, prepare

ROOT = Path(__file__).resolve().parents[3]
REPORT_SCHEMA = json.loads((ROOT / "contracts/plans/report.schema.json").read_text())
EVENT_SCHEMA = json.loads((ROOT / "contracts/events/report.ready.schema.json").read_text())
RUN = "run-t"
BASE = f"s3://aqs-evidence/runs/{RUN}"


def report_validator():
    return Draft202012Validator(REPORT_SCHEMA)


def mk_evidence(root: Path, flows: dict) -> list[str]:
    """flows: nombre -> (status|None, logs). status None => sin result.json (logs_unavailable)."""
    uris = []
    for name, (status, logs) in flows.items():
        d = root / "runs" / RUN / name
        d.mkdir(parents=True, exist_ok=True)
        (d / "logs.txt").write_text(logs, encoding="utf-8")
        uris.append(f"{BASE}/{name}/logs.txt")
        if status is not None:
            (d / "result.json").write_text(json.dumps({"flow_id": name, "status": status}), encoding="utf-8")
            uris.append(f"{BASE}/{name}/result.json")
    return uris


def bug_finding(fid="flow-1", uri=None):
    return {"finding_id": fid, "root_cause": "causa", "invariant": "inv", "method": "POST",
            "path": "/orders", "evidence_uris": [uri or f"{BASE}/flow-1/result.json"]}


def resp(verdict, findings=(), summary="resumen"):
    return json.dumps({"verdict": verdict, "summary": summary, "findings": list(findings)})


def run(tmp_path, flows, response, limits=None, uris=None, with_ports=False, llm_kwargs=None, store=None):
    all_uris = mk_evidence(tmp_path, flows)
    uris = all_uris if uris is None else uris
    reader = DirEvidenceReader(tmp_path)
    limits = limits or Limits()
    llm = FakeLLM()
    prep = prepare(RUN, uris, reader, limits)
    llm.register(prep.prompt, response, **(llm_kwargs or {}))
    store = store if store is not None else (MemoryReportStore() if with_ports else None)
    pub = RecordingPublisher() if with_ports else None
    rep = correlate_postmortem(RUN, uris, reader, llm, limits, store=store, publisher=pub)
    return rep, store, pub
