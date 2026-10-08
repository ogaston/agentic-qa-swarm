import json
import threading
import urllib.error
import urllib.request

import pytest

from agent_reporter.fakes.evidence import DirEvidenceReader
from agent_reporter.fakes.fake_llm import FakeLLM
from agent_reporter.fakes.publisher import RecordingPublisher
from agent_reporter.fakes.store import MemoryReportStore
from agent_reporter.llm import LLMUnavailable
from agent_reporter.reporter import Limits, prepare
from agent_reporter.server import Service, build_service, make_server
from support import RUN, bug_finding, mk_evidence, resp

MARK = "MARCADOR-UNICO-ZX81"
FLOWS = {"flow-1": ("failed", "ERROR " + MARK), "flow-2": ("passed", "ok")}


@pytest.fixture
def srv(tmp_path):
    uris = mk_evidence(tmp_path, FLOWS)
    llm = FakeLLM()
    svc = Service(DirEvidenceReader(tmp_path), llm, MemoryReportStore(), RecordingPublisher(), Limits())
    s = make_server(svc)
    threading.Thread(target=s.serve_forever, daemon=True).start()
    yield s, svc, llm, uris
    s.shutdown(); s.server_close()


def call(s, method, path, body=None, ctype="application/json", raw=None):
    data = raw if raw is not None else (json.dumps(body).encode() if body is not None else None)
    req = urllib.request.Request(f"http://127.0.0.1:{s.server_address[1]}{path}", data=data, method=method)
    if data is not None and ctype:
        req.add_header("Content-Type", ctype)
    try:
        with urllib.request.urlopen(req) as r:
            return r.status, r.read().decode()
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()


def test_server_200_422_502_504(srv):
    s, svc, llm, uris = srv
    llm.register(prepare(RUN, uris, svc.reader, svc.limits).prompt, resp("bug", [bug_finding()]))
    code, body = call(s, "POST", "/v1/report", {"run_id": RUN, "evidence_uris": uris})
    assert code == 200 and json.loads(body)["verdict"] == "bug"
    assert call(s, "POST", "/v1/report", {"run_id": RUN, "evidence_uris": []})[0] == 422
    assert json.loads(call(s, "POST", "/v1/report", {"run_id": RUN, "evidence_uris": []})[1])["error"] == "no_evidence"
    assert json.loads(call(s, "POST", "/v1/report", {"run_id": RUN})[1])["error"] == "invalid_request"
    assert json.loads(call(s, "POST", "/v1/report", raw=b"{no")[1])["error"] == "invalid_request"
    # prompt sin guion -> el fake falla cerrado -> 502
    code, body = call(s, "POST", "/v1/report", {"run_id": RUN, "evidence_uris": uris[:1]})
    assert code == 502 and json.loads(body)["error"] == "llm_unavailable"
    llm.register(prepare(RUN, uris[:2], svc.reader, svc.limits).prompt, error=LLMUnavailable())
    assert call(s, "POST", "/v1/report", {"run_id": RUN, "evidence_uris": uris[:2]})[0] == 502
    llm.register(prepare(RUN, uris[:3], svc.reader, svc.limits).prompt, resp("bug"), latency_s=999)
    code, body = call(s, "POST", "/v1/report", {"run_id": RUN, "evidence_uris": uris[:3]})
    assert code == 504 and json.loads(body)["error"] == "budget_exceeded"
    missing = ["s3://aqs-evidence/runs/run-t/flow-9/logs.txt"]
    assert json.loads(call(s, "POST", "/v1/report", {"run_id": RUN, "evidence_uris": missing})[1])["error"] == "evidence_unavailable"


def test_server_report_rejected_422(srv):
    s, svc, llm, uris = srv
    llm.register(prepare(RUN, uris, svc.reader, svc.limits).prompt, resp("sin-hallazgos"))
    code, body = call(s, "POST", "/v1/report", {"run_id": RUN, "evidence_uris": uris})
    assert code == 422 and json.loads(body)["error"] == "report_rejected"


def test_server_413_415_and_health(srv):
    s, *_ = srv
    assert call(s, "POST", "/v1/report", raw=b"x" * (70 * 1024))[0] == 413
    assert call(s, "POST", "/v1/report", raw=b"{}", ctype="text/plain")[0] == 415
    assert call(s, "GET", "/healthz")[0] == 200 and call(s, "GET", "/readyz")[0] == 200
    assert call(s, "GET", "/nada")[0] == 404


def test_server_no_evidence_content_in_responses_logs_or_metrics(srv, caplog):
    s, svc, llm, uris = srv
    caplog.set_level("DEBUG")
    llm.register(prepare(RUN, uris, svc.reader, svc.limits).prompt, resp("sin-hallazgos"))
    code, body = call(s, "POST", "/v1/report", {"run_id": RUN, "evidence_uris": uris})
    llm.register(prepare(RUN, uris[:2], svc.reader, svc.limits).prompt, "no json " + MARK)
    c2, b2 = call(s, "POST", "/v1/report", {"run_id": RUN, "evidence_uris": uris[:2]})
    metrics = call(s, "GET", "/metrics")[1]
    assert MARK not in body and MARK not in b2 and MARK not in metrics and MARK not in caplog.text
    assert "aqs_reporter_requests_total{" in metrics and "aqs_reporter_redactions_total" in metrics


def test_server_fake_provider_guards():
    ok = {"LLM_PROVIDER": "fake", "REPORTER_ALLOW_FAKE": "1"}
    assert build_service(ok)
    for bad in ({"LLM_PROVIDER": "fake"}, {**ok, "REPORTER_ENV": "prod"}, {"LLM_PROVIDER": "openai", "REPORTER_ALLOW_FAKE": "1"}, {}):
        with pytest.raises(SystemExit):
            build_service(bad)


def test_server_event_published_after_store_via_http(srv):
    s, svc, llm, uris = srv
    llm.register(prepare(RUN, uris, svc.reader, svc.limits).prompt, resp("bug", [bug_finding()]))
    call(s, "POST", "/v1/report", {"run_id": RUN, "evidence_uris": uris})
    assert len(svc.publisher.events) == 1 and svc.publisher.events[0]["type"] == "report.ready"


def test_server_malformed_uri_is_422_no_evidence_not_500(srv):
    s, svc, llm, uris = srv
    code, body = call(s, "POST", "/v1/report", {"run_id": RUN, "evidence_uris": ["s3://[/x/y/result.json"]})
    assert code == 422 and json.loads(body)["error"] == "no_evidence"


def test_reader_invalid_uri_maps_to_no_evidence_even_if_validator_accepts(tmp_path, monkeypatch):
    """Defensa en profundidad: aunque el validador deje pasar una URI, el lector que no la interpreta da 422."""
    from agent_reporter import reporter
    from agent_reporter.errors import InvalidEvidenceUri
    from agent_reporter.fakes.evidence import _rel
    from agent_reporter.server import handle_report
    with pytest.raises(InvalidEvidenceUri):
        _rel("s3://[/x")
    monkeypatch.setattr(reporter, "validate_evidence_uris", lambda d: [])
    svc = Service(DirEvidenceReader(tmp_path), FakeLLM(), MemoryReportStore(), RecordingPublisher(), Limits())
    code, body = handle_report(svc, json.dumps({"run_id": RUN, "evidence_uris": ["s3://[/x/y/result.json"]}).encode())
    assert (code, body["error"]) == (422, "no_evidence")
