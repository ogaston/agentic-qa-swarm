"""Genera el fixture del reporter para kind (U8-T05): prompt exacto de la evidencia sembrada → respuesta.

Uso (desde la raíz del repo, con el venv del reporter):
    PYTHONPATH=agents/agent-reporter/src agents/agent-reporter/.venv/bin/python \
        scripts/kind/gen_agent_fixtures.py > deploy/flux/kind/agents/reporter/fixtures.json

La evidencia es la del dataset bug-ecom-01 (agents/dataset/artifacts/bug-ecom-01/evidence), sembrada en
MinIO por scripts/test/u8-agents-kind.sh bajo s3://evidence/runs/<RUN>/... El prompt depende de esos bytes
exactos (con la redacción y el recorte que aplica prepare), así que este generador usa la misma función.
"""
from __future__ import annotations

import json
import sys
from pathlib import Path
from urllib.parse import urlsplit

from agent_reporter.errors import EvidenceUnavailable
from agent_reporter.reporter import Limits, prepare

RUN = "run-bug-ecom-01"
BUCKET = "evidence"
ROOT = Path(__file__).resolve().parents[2]
EVIDENCE = ROOT / "agents" / "dataset" / "artifacts" / "bug-ecom-01" / "evidence"
URIS = [
    f"s3://{BUCKET}/runs/{RUN}/flow-1/logs.txt",
    f"s3://{BUCKET}/runs/{RUN}/flow-1/result.json",
    f"s3://{BUCKET}/runs/{RUN}/flow-2/logs.txt",
    f"s3://{BUCKET}/runs/{RUN}/flow-2/result.json",
]
RESPONSE = {
    "verdict": "bug",
    "summary": "el stock quedo negativo tras una orden",
    "findings": [{
        "finding_id": "flow-1",
        "root_cause": "invariante violado en la orden",
        "invariant": "el stock nunca es negativo tras una orden",
        "method": "POST",
        "path": "/orders",
        "evidence_uris": [f"s3://{BUCKET}/runs/{RUN}/flow-1/result.json"],
    }],
}


class DatasetReader:
    """Lee s3://evidence/runs/<RUN>/<rel> desde el dataset; cualquier otra cosa falla."""

    def get(self, uri: str) -> bytes:
        p = urlsplit(uri)
        prefix = f"/runs/{RUN}/"
        if p.scheme != "s3" or p.netloc != BUCKET or not p.path.startswith(prefix):
            raise EvidenceUnavailable("objeto inexistente")
        f = EVIDENCE / p.path[len(prefix):]
        if not f.is_file():
            raise EvidenceUnavailable("objeto inexistente")
        return f.read_bytes()


def main() -> int:
    prep = prepare(RUN, URIS, DatasetReader(), Limits.from_env({}))
    doc = [{"run_id": RUN, "prompt": prep.prompt, "response": RESPONSE}]
    json.dump(doc, sys.stdout, ensure_ascii=True, indent=2)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
