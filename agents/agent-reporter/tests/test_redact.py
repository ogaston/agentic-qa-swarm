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


class _MemReader:
    def __init__(self, data):
        self.data = data

    def get(self, uri):
        return self.data


def _prompt_for(text, limit):
    from agent_reporter.reporter import Limits, prepare
    return prepare("r", ["s3://b/runs/r/flow-1/logs.txt"], _MemReader(text.encode()), Limits(max_object_bytes=limit)).prompt


def test_redact_secret_split_by_truncation_never_survives():
    """Barre el desplazamiento hasta cruzar la frontera cabeza y la de cola. Una sola pasada: ANTES de recortar."""
    sec, lim, crossed = S["aws"], 520, 0
    for k in range(0, 1300):  # el secreto cruza el final de la cabeza
        p = _prompt_for("x" * k + sec + "y" * 2000, lim)
        assert "AKIA" not in p and "ABCDEF" not in p, k
    for k in range(0, 700):  # el secreto cruza el inicio de la cola
        p = _prompt_for("x" * 2000 + sec + "y" * k, lim)
        assert "AKIA" not in p and "ABCDEF" not in p, k
        crossed += "recortado" in p
    assert crossed > 0


@pytest.mark.parametrize("shape", ["a", "token", "password", "eyJ", "://", "a:", "Authorization", "AKIA", "ghp_", "secret=", "Bearer ", "a://b:"])
def test_redact_performance_linear_on_64kib(shape):
    import time
    blob = (shape * (65536 // len(shape) + 1))[:65536].encode()
    t = time.perf_counter()
    redact_secrets(blob)
    assert time.perf_counter() - t < 1.5, shape


def test_redact_secret_in_result_json_is_redacted(tmp_path):
    from agent_reporter.fakes.evidence import DirEvidenceReader
    from agent_reporter.reporter import Limits, prepare
    from support import RUN, mk_evidence
    uris = mk_evidence(tmp_path, {"flow-1": ("failed", "ok")})
    rj = tmp_path / "runs" / RUN / "flow-1" / "result.json"
    rj.write_text('{"flow_id":"flow-1","status":"failed","password":"zzSeeded99"}')
    p = prepare(RUN, uris, DirEvidenceReader(tmp_path), Limits())
    assert "zzSeeded99" not in p.prompt and p.flows["flow-1"] == "failed"


FORMS = {
    "authorization": ['{"Authorization": "Basic dXNlcjpwYXNzd29yZA=="}', "Authorization: Basic dXNlcjpwYXNzd29yZA==", "'authorization'='Digest zzTailQ9'"],
    "bearer": ["Bea" + "rer abcDEF123456", "x-h: bea" + "rer Zz9.yy8-tail_01", "{\"h\":\"Bea" + "rer abcDEF123456\"}"],
    "password": ['{"password":"pa\\"ss-tail-secret"}', "password='pa ss tail'", "password='it\\'s-tail'", 'password="unterminated-secret-tail', "password='unterminated-tail", "passwd: tailsecret1", "db_password = tailsecret1"],
    "url": ["postgres" + "://user:p@ss@host/db", "https" + "://u:pw@h/x", "redis" + "://:pw@h:6379"],
    "token": ['{"access_token": "tailsecret1"}', "TOKEN=tailsecret1", "api-key: tailsecret1"],
    "private_key": ["-----BEGIN EC PRIV" + "ATE KEY-----\nTAILKEY\n-----END EC PRIV" + "ATE KEY-----", "-----BEGIN PRIV" + "ATE KEY-----\nTAILKEY"],
    "jwt": ["ey" + "Jhb.eyJz.sig_-1", "t=ey" + "JhbGciOi.eyJzdWIi.abc-_"],
    "aws": [S["aws"], "id=" + S["aws"] + ","],
    "github": [S["github"], "gh" + "s_" + "A" * 25, "gh" + "o_" + "b" * 40],
}
TAILS = ("tail", "TAIL", "dXNlcjpwYXNz", "Zz9.yy8", "abcDEF", "zzTail", "MIIE", "sig_", "ABCDEF", "A1b2", "AAAAAAAAAAAAAAAAAAAAAAAAA", "bbbbbbbbbbbbbbbbbbbb", "pw@", "p@ss", "ss-", "ss@", ":pw", "u:pw", "Jhb", "eyJz", "A1b2C3")


@pytest.mark.parametrize("kind", list(FORMS))
def test_redact_forms_leave_no_tail(kind):
    for text in FORMS[kind]:
        out = redact_secrets(text.encode()).decode()
        assert "[REDACTED:" in out, text
        low = out.lower()
        for t in TAILS:
            if t.lower() in text.lower():
                assert t.lower() not in low.replace("[redacted:", ""), (text, out)
        assert redact_secrets(out.encode()).decode() == out, text
