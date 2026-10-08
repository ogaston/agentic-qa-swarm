import pytest

from agent_reporter.demo_redaction import seeded
from agent_reporter.redact import redact_secrets
from support import ROOT

S = seeded()
CASES = {
    "aws": ("clave " + S["aws"] + " fin", S["aws"]),
    "github": ("t " + S["github"], S["github"]),
    "jwt": ("tok " + S["jwt"], S["jwt"]),
    "authorization": (S["authorization"], "zq81xkd92hs7TTqpW"),
    "bearer": ("x Bea" + "rer abcDEF123456 y", "abcDEF123456"),
    "private_key": ("a\n" + S["private_key"] + "\nb", "MIIEowIBAAKCAQEA"),
    "url": ("conn " + S["url"], "hunter2pass"),
    "password": (S["password"], "S3cr3tValue99"),
    "api_key_json": (S["api_key"], "k9x8w7v6u5t4"),
    "secret_colon": ("client_" + "secret: zzTop99xx", "zzTop99xx"),
}


@pytest.mark.parametrize("name", list(CASES))
def test_redact_each_type_leaves_no_value(name):
    text, value = CASES[name]
    out = redact_secrets(text.encode()).decode()
    assert value not in out
    assert "[REDACTED:" in out


def test_redact_json_and_header_keep_structure():
    import json
    out = redact_secrets(json.dumps({"password": "abc12345", "ok": 1}).encode())
    assert json.loads(out)["password"].startswith("[REDACTED:") and json.loads(out)["ok"] == 1
    hdr = redact_secrets(b"GET /x\nAuthorization: Basic dXNlcjpwYXNz\nHost: a\n")
    assert b"dXNlcjpwYXNz" not in hdr and b"Host: a" in hdr


@pytest.mark.parametrize("name", list(CASES))
def test_redact_idempotent(name):
    once = redact_secrets(CASES[name][0].encode())
    assert redact_secrets(once) == once


def test_redact_unterminated_private_key_is_fully_removed():
    out = redact_secrets(("x\n-----BEGIN PRIV" + "ATE KEY-----\nAAAA1234\nBBBB").encode())
    assert b"AAAA1234" not in out and b"BBBB" not in out


def test_redact_clean_dataset_logs_unchanged():
    files = list((ROOT / "agents/dataset/artifacts").rglob("logs.txt"))
    assert len(files) >= 10
    for f in files:
        assert redact_secrets(f.read_bytes()) == f.read_bytes(), f


def test_redact_binary_input_no_exception():
    blob = bytes(range(256)) * 4 + b"\xff\xfe" + S["aws"].encode()
    out = redact_secrets(blob)
    assert S["aws"].encode() not in out
    assert redact_secrets(bytes(range(256))) == bytes(range(256))


def test_redact_secret_split_by_truncation_never_survives(tmp_path):
    from agent_reporter.reporter import Limits
    from support import bug_finding, resp, run
    for pad in range(0, 40, 7):
        logs = "x" * (500 + pad) + S["aws"] + "y" * 500
        rep, *_ = run(tmp_path / str(pad), {"flow-1": ("failed", logs)}, resp("bug", [bug_finding()]),
                      limits=Limits(max_object_bytes=520))
        from agent_reporter.reporter import prepare
        from agent_reporter.fakes.evidence import DirEvidenceReader
        p = prepare("run-t", [f"s3://aqs-evidence/runs/run-t/flow-1/logs.txt"], DirEvidenceReader(tmp_path / str(pad)), Limits(max_object_bytes=520))
        assert "AKIA" not in p.prompt and S["aws"][4:12] not in p.prompt


def test_redact_secret_in_result_json_is_redacted(tmp_path):
    from agent_reporter.fakes.evidence import DirEvidenceReader
    from agent_reporter.reporter import Limits, prepare
    from support import RUN, mk_evidence
    uris = mk_evidence(tmp_path, {"flow-1": ("failed", "ok")})
    rj = tmp_path / "runs" / RUN / "flow-1" / "result.json"
    rj.write_text('{"flow_id":"flow-1","status":"failed","password":"zzSeeded99"}')
    p = prepare(RUN, uris, DirEvidenceReader(tmp_path), Limits())
    assert "zzSeeded99" not in p.prompt and p.flows["flow-1"] == "failed"
