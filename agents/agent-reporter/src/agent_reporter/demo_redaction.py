"""Demo CA-3: evidencia con un secreto por tipo, sembrados en tiempo de ejecucion (fragmentos concatenados)."""
from __future__ import annotations

import argparse
import json
from pathlib import Path

from agent_reporter.fakes.evidence import DirEvidenceReader
from agent_reporter.fakes.fake_llm import FakeLLM
from agent_reporter.reporter import Limits, correlate_postmortem, prepare

RUN = "run-demo"
BASE = f"s3://aqs-evidence/runs/{RUN}"


def seeded() -> dict[str, str]:
    """Un secreto con forma real por tipo. Ninguno existe como literal en el repo."""
    return {
        "aws": "AK" + "IA" + "ABCDEFGHIJKLMNOP",
        "github": "gh" + "p_" + "A1b2C3d4E5f6G7h8I9j0K1l2M3n4O5p6Q7r8",
        "jwt": "ey" + "Jhb" + "GciOiJIUzI1NiJ9." + "eyJzdWIiOiIxMjM0NTYifQ." + "c2lnbmF0dXJlX2Rl" + "bW8",
        "authorization": "Author" + "ization: Bea" + "rer " + "zq81" + "xkd92hs7TTqpW",
        "private_key": "-----BEGIN RSA PRIV" + "ATE KEY-----\nMIIEowIBAAKCAQEA" + "demoDemoDemo\n-----END RSA PRIV" + "ATE KEY-----",
        "url": "postgres" + "://admin" + ":hunter2" + "pass@db.internal:5432/app",
        "password": "pass" + "word=" + "S3cr3tValue" + "99",
        "api_key": '{"api_' + 'key": "' + "k9x8w7v6u5t4" + '"}',
    }


def build_evidence(root: Path) -> list[str]:
    root.mkdir(parents=True, exist_ok=True)
    s = seeded()
    logs = "INFO inicio\n" + "\n".join(f"WARN dato {k}: {v}" if k not in ("authorization", "private_key", "password", "api_key", "url") else v for k, v in s.items()) + "\nERROR invariante violado\n"
    (root / "runs__run-demo__flow-1__logs.txt").write_text(logs, encoding="utf-8")
    (root / "runs__run-demo__flow-1__result.json").write_text(
        json.dumps({"flow_id": "flow-1", "status": "failed", "steps": 2}), encoding="utf-8"
    )
    return [f"{BASE}/flow-1/logs.txt", f"{BASE}/flow-1/result.json"]


def run(out: Path) -> None:
    out.mkdir(parents=True, exist_ok=True)
    ev = out / "evidencia-sembrada"
    uris = build_evidence(ev)
    reader = DirEvidenceReader(ev)
    limits = Limits()
    prep = prepare(RUN, uris, reader, limits)
    leaked = seeded()["aws"]
    canned = {
        "verdict": "bug",
        "summary": "causa vista en logs " + leaked,
        "findings": [{
            "finding_id": "flow-1", "root_cause": "token filtrado " + seeded()["github"],
            "invariant": "el stock nunca es negativo", "method": "POST", "path": "/orders",
            "evidence_uris": [uris[1]],
        }],
    }
    llm = FakeLLM()
    llm.register(prep.prompt, json.dumps(canned))
    report = correlate_postmortem(RUN, uris, reader, llm, limits)
    (out / "prompt.txt").write_text(prep.prompt, encoding="utf-8")
    (out / "report.json").write_text(json.dumps(report, indent=2), encoding="utf-8")


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", required=True)
    run(Path(ap.parse_args().out))


if __name__ == "__main__":
    main()
