"""Regresiones fijas halladas por test_pbt_redact_* (U3-T06). Los secretos se arman en tiempo de ejecucion."""
import json

from agent_reporter.redact import redact_secrets

AUTH = "Author" + "ization"
BEAR = "Bear" + "er"
JWT = "ey" + "Jhb" + "GciOiJIUzI1NiJ9." + "eyJzdWIiOiIxMjM0NTYifQ." + "c2lnbmF0dXJl" + "9"


def _out(s):
    return redact_secrets(s.encode()).decode()


def test_json_inside_json_string_authorization_is_redacted():
    # ejemplo reducido: {"msg": " {\"Authorization\": \"Basic 00000000\"} "} dejaba 00000000 en claro
    inner = json.dumps({AUTH: "Ba" + "sic 00000000"})
    out = _out(json.dumps({"msg": " " + inner + " ", "n": 1}))
    assert "00000000" not in out
    assert json.loads(out)["n"] == 1  # el JSON externo sigue siendo valido
    assert _out(out) == out


def test_json_inside_json_string_assignment_is_redacted():
    inner = json.dumps({"pass" + "word": "S3cr3tValue9"})
    out = _out(json.dumps({"msg": inner}))
    assert "S3cr3tValue9" not in out and json.loads(out)


def test_bearer_after_json_escaped_newline_is_redacted():
    # ejemplo reducido: {"msg": "\nBearer 00000000 "}: la `n` del escape \n pegaba la palabra y \b no cortaba
    out = _out(json.dumps({"msg": "\n" + BEAR + " 00000000 "}))
    assert "00000000" not in out


def test_jwt_after_json_escaped_newline_is_redacted():
    out = _out(json.dumps({"msg": "l1\nl2\t" + JWT}))
    assert JWT.split(".")[2] not in out
