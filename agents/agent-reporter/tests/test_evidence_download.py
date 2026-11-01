"""Runner artifact access is limited to valid run/flow/file names."""
from agent_reporter.reporter import Limits
from agent_reporter.server import Service, handle_get_evidence, handle_get_report_evidence


class Reader:
    def __init__(self):
        self.uris = []

    def get(self, uri):
        self.uris.append(uri)
        return b'{"status":"passed"}'


def test_artifact_request_uses_configured_bucket_and_rejects_paths():
    reader = Reader()
    svc = Service(reader, None, None, None, Limits(), evidence_bucket="test-bucket")
    run = "run-0123456789abcdef"
    code, data = handle_get_evidence(svc, run, "flow-1", "result.json")
    assert (code, data) == (200, b'{"status":"passed"}')
    assert reader.uris == [f"s3://test-bucket/runs/{run}/flow-1/result.json"]
    assert handle_get_evidence(svc, run, "../other", "result.json")[0] == 400
    assert handle_get_evidence(svc, run, "flow-1", "secret.txt")[0] == 400
    assert len(reader.uris) == 1


def test_report_evidence_is_restricted_to_bucket_and_run():
    reader = Reader()
    svc = Service(reader, None, None, None, Limits(), evidence_bucket="test-bucket")
    run = "run-0123456789abcdef"
    valid = f"s3://test-bucket/runs/{run}/flow-1/trace.har"
    assert handle_get_report_evidence(svc, run, valid)[0] == 200
    for uri in (f"s3://other/runs/{run}/file", f"s3://test-bucket/runs/run-aaaaaaaaaaaaaaaa/file",
                f"s3://test-bucket/runs/{run}/../file", valid + "?x=1"):
        assert handle_get_report_evidence(svc, run, uri)[0] == 400
    assert reader.uris == [valid]
